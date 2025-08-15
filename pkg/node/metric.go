package node

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	patchSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "node_patch_success_total",
		Help: "Total successful node patch operations",
	})
	patchFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "node_patch_failure_total",
		Help: "Total failed node patch operations",
	})
)

func init() {
	prometheus.MustRegister(patchSuccess, patchFailure)
}

func getMetricHandler() http.Handler {
	return promhttp.Handler()
}
