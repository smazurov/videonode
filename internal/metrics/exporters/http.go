// Package exporters provides HTTP and SSE exporters for metrics.
package exporters

import (
	"encoding/json"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/smazurov/videonode/internal/metrics"
)

// HTTPHandler returns the Prometheus metrics HTTP handler.
// This collects all promauto-registered metrics automatically.
func HTTPHandler() http.Handler {
	return promhttp.Handler()
}

// JSONHandler returns an HTTP handler that serves metrics as JSON.
func JSONHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		data, err := metrics.GetAllMetricsAsJSON()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}
