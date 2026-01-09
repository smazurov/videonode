// Package exporters provides HTTP and SSE exporters for metrics.
package exporters

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HTTPHandler returns the Prometheus metrics HTTP handler.
// This collects all promauto-registered metrics automatically.
func HTTPHandler() http.Handler {
	return promhttp.Handler()
}
