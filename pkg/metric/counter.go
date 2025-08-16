package metric

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type IncrementalCounter interface {
	Inc()
}

type Counter struct {
	Name string
	Help string

	counter IncrementalCounter
}

func (c *Counter) Inc() {
	c.counter.Inc()
}
func NewCounter(name, help string) IncrementalCounter {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: name,
		Help: help,
	})

	prometheus.MustRegister(counter)

	return &Counter{
		Name:    name,
		Help:    help,
		counter: counter,
	}
}

// GetHandler returns an HTTP handler for serving Prometheus metrics.
func GetHandler() http.Handler {
	return promhttp.Handler()
}
