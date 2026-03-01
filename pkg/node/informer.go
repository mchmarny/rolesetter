package node

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/mchmarny/rolesetter/pkg/log"
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

// Informer is responsible for managing the node role setter controller.
type Informer struct {
	logger    *zap.Logger
	label     string
	replace   bool
	port      int
	namespace string
	clientset kubernetes.Interface
	server    server.Server
}

// Option is a functional option for configuring Informer.
type Option func(*Informer)

func WithReplace(replace bool) Option {
	return func(i *Informer) {
		i.replace = replace
	}
}

// WithLogger sets the logger for the Informer.
func WithLogger(logger *zap.Logger) Option {
	return func(i *Informer) {
		i.logger = logger
	}
}

// WithLabel sets the label for the Informer.
func WithLabel(label string) Option {
	return func(i *Informer) {
		i.label = label
	}
}

// WithPort sets the port for the Informer.
func WithPort(port int) Option {
	return func(i *Informer) {
		i.port = port
	}
}

// WithClientset sets the Kubernetes clientset for the Informer.
func WithClientset(cs kubernetes.Interface) Option {
	return func(i *Informer) {
		i.clientset = cs
	}
}

// WithNamespace sets the namespace for leader election.
// When set, leader election is enabled using a Lease in this namespace.
func WithNamespace(ns string) Option {
	return func(i *Informer) {
		i.namespace = ns
	}
}

// NewInformer creates a new Informer instance using functional options.
func NewInformer(opts ...Option) (*Informer, error) {
	i := &Informer{
		logger: log.GetLogger(),
		port:   servicePortDefault,
	}

	for _, opt := range opts {
		opt(i)
	}

	// set these AFTER options are applied to allow testing to override
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
		)
	}

	if err := i.validate(); err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}
	return i, nil
}

// validate checks if the Informer has valid configuration.
func (i *Informer) validate() error {
	if i.logger == nil {
		return fmt.Errorf("logger must not be nil")
	}
	if i.label == "" {
		return fmt.Errorf("roleLabel must be specified")
	}
	if i.port <= 0 {
		return fmt.Errorf("serverPort must be a positive integer")
	}
	if i.clientset == nil {
		return fmt.Errorf("kubernetes clientset must not be nil")
	}
	if i.server == nil {
		return fmt.Errorf("server must not be nil")
	}
	return nil
}

// Inform runs the node role setter controller with context, logger, and config.
func (i *Informer) Inform(ctx context.Context) error {
	if err := i.validate(); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	i.logger.Info("starting node role setter",
		zap.String("label", i.label),
		zap.Int("port", i.port),
		zap.String("namespace", i.namespace),
	)

	// Start metrics server (always runs, regardless of leadership)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		i.server.Serve(ctx, map[string]http.Handler{
			"/metrics": metric.GetHandler(),
		})
	}()

	// Run informer with or without leader election
	if i.namespace != "" {
		if err := i.runWithLeaderElection(ctx); err != nil {
			return err
		}
	} else {
		if err := i.runInformer(ctx); err != nil {
			return err
		}
	}

	wg.Wait()
	return nil
}

func (i *Informer) runWithLeaderElection(ctx context.Context) error {
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

	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   leaseDuration,
		RenewDeadline:   renewDeadline,
		RetryPeriod:     retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				if runErr := i.runInformer(ctx); runErr != nil {
					i.logger.Error("informer failed", zap.Error(runErr))
				}
			},
			OnStoppedLeading: func() {
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
	return nil
}

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

	factory := informers.NewSharedInformerFactory(i.clientset, resyncInterval)
	eventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			handler.EnsureRole(ctx, obj)
		},
		UpdateFunc: func(_, newObj interface{}) {
			handler.EnsureRole(ctx, newObj)
		},
	}

	inf := factory.Core().V1().Nodes().Informer()
	if _, err := inf.AddEventHandler(eventHandler); err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		return fmt.Errorf("cache sync failed")
	}
	<-ctx.Done()
	return nil
}
