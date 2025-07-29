package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/smazurov/videonode/internal/sse"
)

func SetupRoutes(r chi.Router, sseManager *sse.Manager) {
	// API routes that should have a timeout
	r.Route("/api", func(apiRouter chi.Router) {
		// Apply timeout specifically to these /api routes
		apiRouter.Use(middleware.Timeout(30 * time.Second))

		apiRouter.Route("/devices", func(r chi.Router) {
			r.Get("/", listDevicesHandler)
			r.Get("/{devicePath}/capabilities", deviceCapabilitiesHandler)
			r.Get("/{devicePath}/resolutions", deviceResolutionsHandler)
			r.Get("/{devicePath}/framerates", deviceFrameratesHandler)
		})

		apiRouter.Route("/encoders", func(r chi.Router) {
			r.Get("/", listEncodersHandler)
		})

		apiRouter.Route("/capture", func(r chi.Router) {
			r.Post("/screenshot", captureScreenshotHandler(sseManager))
		})

		// General streaming routes
		apiRouter.Route("/streams", func(r chi.Router) {
			r.Get("/", listStreamsHandler)
			r.Get("/new-stream-form", newStreamFormHandler)
			r.Get("/close-form", closeStreamFormHandler)
			r.Post("/create", createStreamHandler(sseManager))
			r.Post("/stop", stopStreamFromParamsHandler(sseManager))
		})


		apiRouter.Get("/health", healthCheckHandler)

		// Metrics API routes
		apiRouter.Route("/metrics", func(r chi.Router) {
			r.Get("/", metricsHandler)
		})
	})

	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./favicon.ico")
	})


	// Prometheus metrics endpoint
	r.Get("/metrics/prometheus", prometheusMetricsHandler)

}
