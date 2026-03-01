package metric

import (
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type IncrementalCounter interface {
	Increment(val ...string)
}

type Counter struct {
	Name string
	Help string

	vec *prometheus.CounterVec
}

func (c *Counter) Increment(val ...string) {
	c.vec.WithLabelValues(val...).Inc()
}

func NewCounter(name, help string, labels ...string) IncrementalCounter {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: name,
		Help: help,
	}, labels)

	if err := prometheus.Register(counter); err != nil {
		var are prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			counter = are.ExistingCollector.(*prometheus.CounterVec)
		} else {
			panic("failed to register counter " + name + ": " + err.Error())
		}
	}

	return &Counter{
		Name: name,
		Help: help,
		vec:  counter,
	}
}

// GetHandler returns an HTTP handler for serving Prometheus metrics.
func GetHandler() http.Handler {
	return promhttp.Handler()
}
