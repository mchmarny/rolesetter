package node

import (
	context "context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	rolePrefix     = "node-role.kubernetes.io/"
	resSyncSeconds = 30
)

// NewInformer creates a new Informer instance with the provided logger, label, and port.
func NewInformer(logger *zap.Logger, label string, port int) (*Informer, error) {
	i := &Informer{
		logger: logger,
		label:  label,
		port:   port,
	}
	if err := i.validate(); err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}
	return i, nil
}

// Informer is responsible for managing the node role setter controller.
type Informer struct {
	logger *zap.Logger
	label  string
	port   int
}

// Validate checks if the Informer has valid configuration.
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

	return nil
}

// Inform runs the node role setter controller with context, logger, and config.
func (i *Informer) Inform(ctx context.Context) error {
	if err := i.validate(); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	i.logger.Info("starting node role setter", zap.String("roleLabel", i.label), zap.Int("port", i.port))

	cs, err := i.newClient()
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	factory := informers.NewSharedInformerFactory(cs, resSyncSeconds*time.Second)
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			(&cacheResourceHandler{cs: cs, logger: i.logger, roleLabel: i.label}).ensureRole(obj)
		},
		UpdateFunc: func(_, newObj interface{}) {
			(&cacheResourceHandler{cs: cs, logger: i.logger, roleLabel: i.label}).ensureRole(newObj)
		},
		DeleteFunc: func(_ interface{}) {
			// nothing to do here
		},
	}

	inf := factory.Core().V1().Nodes().Informer()
	if _, err := inf.AddEventHandler(handler); err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	// Start the informer and metrics server
	go func() {
		i.serve(map[string]http.Handler{
			"/metrics": getMetricHandler(),
		})
	}()

	// OS signal handling and context cancellation should be managed by the parent (main)

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		return fmt.Errorf("cache sync failed")
	}
	<-ctx.Done()

	return nil
}

// newClient creates a Kubernetes clientset for interacting with the cluster.
func (i *Informer) newClient() (*kubernetes.Clientset, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}
	return cs, nil
}
