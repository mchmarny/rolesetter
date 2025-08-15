package node

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	patchSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "node_role_patch_success_total",
		Help: "Total successful node patch operations",
	})

	patchFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "node_role_patch_failure_total",
		Help: "Total failed node patch operations",
	})
)

// init registers the metrics with Prometheus.
func init() {
	prometheus.MustRegister(patchSuccess, patchFailure)
}

// getMetricHandler returns an HTTP handler for serving Prometheus metrics.
func getMetricHandler() http.Handler {
	return promhttp.Handler()
}

// incSuccessMetric increments the success metric for node role patch operations.
func incSuccessMetric() {
	patchSuccess.Inc()
}

// incFailureMetric increments the failure metric for node role patch operations.
func incFailureMetric() {
	patchFailure.Inc()
}
