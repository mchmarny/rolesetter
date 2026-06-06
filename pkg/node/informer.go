// Package node owns the controller lifecycle: clientset construction,
// metrics-server bring-up, leader election (when a namespace is provided),
// and the shared informer that drives node-role reconciliation.
package node

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mchmarny/rolesetter/pkg/logger"
	"github.com/mchmarny/rolesetter/pkg/metric"
	"github.com/mchmarny/rolesetter/pkg/role"
	"github.com/mchmarny/rolesetter/pkg/server"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

const (
	resyncInterval     = 5 * time.Minute
	servicePortDefault = 8080
	leaseName          = "node-role-controller"
	leaseDuration      = 15 * time.Second
	renewDeadline      = 10 * time.Second
	retryPeriod        = 2 * time.Second
)

// Informer manages the controller lifecycle.
type Informer struct {
	logger    *zap.Logger
	label     string
	replace   bool
	port      int
	namespace string
	clientset kubernetes.Interface
	server    server.Server

	cacheSynced atomic.Bool
	leading     atomic.Bool
}

// Option is a functional option for configuring Informer.
type Option func(*Informer)

// WithReplace controls whether existing node-role.kubernetes.io/* labels
// other than the desired one are removed during reconciliation.
func WithReplace(replace bool) Option {
	return func(i *Informer) {
		i.replace = replace
	}
}

// WithLogger sets the zap logger used by the controller.
func WithLogger(logger *zap.Logger) Option {
	return func(i *Informer) {
		i.logger = logger
	}
}

// WithLabel sets the source label whose value becomes the node role.
func WithLabel(label string) Option {
	return func(i *Informer) {
		i.label = label
	}
}

// WithPort sets the listen port for the metrics/health HTTP server.
func WithPort(port int) Option {
	return func(i *Informer) {
		i.port = port
	}
}

// WithClientset injects a Kubernetes clientset. When omitted, an
// in-cluster clientset is constructed at NewInformer time.
func WithClientset(cs kubernetes.Interface) Option {
	return func(i *Informer) {
		i.clientset = cs
	}
}

// WithNamespace enables leader election via a coordination.k8s.io/Lease
// in the supplied namespace. When empty, the controller runs without
// leader election (single-replica deployments).
func WithNamespace(ns string) Option {
	return func(i *Informer) {
		i.namespace = ns
	}
}

// WithServer injects a custom Server implementation. Primarily intended
// for tests; when omitted, NewInformer constructs the default server.
func WithServer(s server.Server) Option {
	return func(i *Informer) {
		i.server = s
	}
}

// NewInformer constructs and validates an Informer.
func NewInformer(opts ...Option) (*Informer, error) {
	i := &Informer{
		logger: logger.GetLogger(),
		port:   servicePortDefault,
	}

	for _, opt := range opts {
		opt(i)
	}

	if i.clientset == nil {
		cs, err := newClient()
		if err != nil {
			return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
		}
		i.clientset = cs
	}

	if i.server == nil {
		i.server = server.NewServer(
			server.WithLogger(i.logger),
			server.WithPort(i.port),
			server.WithReady(i.ready),
		)
	}

	if err := i.validate(); err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}
	return i, nil
}

// validate checks that the Informer is fully configured.
func (i *Informer) validate() error {
	if i.logger == nil {
		return errors.New("logger must not be nil")
	}
	if i.label == "" {
		return errors.New("roleLabel must be specified")
	}
	if i.port <= 0 {
		return errors.New("serverPort must be a positive integer")
	}
	if i.clientset == nil {
		return errors.New("kubernetes clientset must not be nil")
	}
	if i.server == nil {
		return errors.New("server must not be nil")
	}
	return nil
}

// ready is the readiness predicate consulted by /readyz. The replica is
// ready when (a) the informer cache has synced at least once, and (b) it
// is the active leader (always true when leader election is disabled).
func (i *Informer) ready() bool {
	return i.cacheSynced.Load() && (i.namespace == "" || i.leading.Load())
}

// Inform starts the metrics server and node informer. It returns the first
// fatal error from either, or nil on graceful shutdown via ctx cancel.
func (i *Informer) Inform(ctx context.Context) error {
	if err := i.validate(); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	i.logger.Info("starting node role setter",
		zap.String("label", i.label),
		zap.Int("port", i.port),
		zap.String("namespace", i.namespace),
		zap.Bool("replace", i.replace),
	)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg     sync.WaitGroup
		srvErr error
		runErr error
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := i.server.Serve(runCtx, map[string]http.Handler{
			"/metrics": metric.GetHandler(),
		}); err != nil {
			srvErr = err
			i.logger.Error("metrics server exited with error", zap.Error(err))
			cancel()
		}
	}()

	if i.namespace != "" {
		runErr = i.runWithLeaderElection(runCtx, cancel)
	} else {
		i.leading.Store(true)
		runErr = i.runInformer(runCtx)
	}

	cancel()
	wg.Wait()

	if runErr != nil {
		return runErr
	}
	return srvErr
}

// runWithLeaderElection drives the informer under leader election. The
// supplied cancel is invoked on any informer failure to release the lease
// (ReleaseOnCancel) and signal the metrics goroutine to shut down.
func (i *Informer) runWithLeaderElection(ctx context.Context, cancel context.CancelFunc) error {
	id, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	i.logger.Info("starting leader election",
		zap.String("identity", id),
		zap.String("namespace", i.namespace),
	)

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: i.namespace,
		},
		Client: i.clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	var informerErr error
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   leaseDuration,
		RenewDeadline:   renewDeadline,
		RetryPeriod:     retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(cbCtx context.Context) {
				i.leading.Store(true)
				if runErr := i.runInformer(cbCtx); runErr != nil {
					i.logger.Error("informer failed; releasing leadership", zap.Error(runErr))
					informerErr = runErr
					cancel()
				}
			},
			OnStoppedLeading: func() {
				i.leading.Store(false)
				i.cacheSynced.Store(false)
				i.logger.Info("lost leadership")
			},
			OnNewLeader: func(identity string) {
				if identity == id {
					return
				}
				i.logger.Info("new leader elected", zap.String("leader", identity))
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create leader elector: %w", err)
	}

	le.Run(ctx)
	return informerErr
}

// runInformer wires the node informer and blocks until ctx is canceled or
// initial cache sync fails.
func (i *Informer) runInformer(ctx context.Context) error {
	handler, err := role.NewCacheResourceHandler(
		i.clientset.CoreV1().Nodes().Patch,
		i.logger,
		i.label,
		i.replace,
	)
	if err != nil {
		return fmt.Errorf("failed to create role handler: %w", err)
	}

	// Restrict the watch to nodes carrying the source label so we don't
	// fan out events for nodes we cannot act on. This trims watch traffic
	// on large clusters at no correctness cost — the reconciler is still
	// a no-op for nodes lacking the label.
	tweak := func(opts *metav1.ListOptions) {
		opts.LabelSelector = i.label
	}
	factory := informers.NewSharedInformerFactoryWithOptions(
		i.clientset,
		resyncInterval,
		informers.WithTweakListOptions(tweak),
	)

	inf := factory.Core().V1().Nodes().Informer()
	if _, err := inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			handler.EnsureRole(ctx, obj)
		},
		UpdateFunc: func(_, newObj interface{}) {
			handler.EnsureRole(ctx, newObj)
		},
	}); err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		return errors.New("cache sync failed")
	}
	i.cacheSynced.Store(true)
	i.logger.Info("informer cache synced")

	<-ctx.Done()
	return nil
}
