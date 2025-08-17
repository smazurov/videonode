package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowOrigin  string
	AllowMethods []string
	AllowHeaders []string
	MaxAge       int
}

// DefaultCORSConfig returns permissive CORS config for internal tools
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigin:  "*",
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders: []string{"Content-Type", "Authorization", "X-Requested-With", "Accept", "Origin"},
		MaxAge:       86400,
	}
}

// NewCORSMiddleware creates CORS middleware with the given configuration
func NewCORSMiddleware(config CORSConfig) func(huma.Context, func(huma.Context)) {
	// Pre-compute header values
	allowMethods := strings.Join(config.AllowMethods, ", ")
	allowHeaders := strings.Join(config.AllowHeaders, ", ")
	maxAge := strconv.Itoa(config.MaxAge)

	return func(ctx huma.Context, next func(huma.Context)) {
		// Set CORS headers
		ctx.SetHeader("Access-Control-Allow-Origin", config.AllowOrigin)
		ctx.SetHeader("Access-Control-Allow-Methods", allowMethods)
		ctx.SetHeader("Access-Control-Allow-Headers", allowHeaders)
		ctx.SetHeader("Access-Control-Max-Age", maxAge)

		// Handle preflight OPTIONS requests
		if ctx.Method() == http.MethodOptions {
			// For OPTIONS, we just return 204 No Content with headers
			ctx.SetStatus(http.StatusNoContent)
			return
		}

		// Continue to next middleware/handler
		next(ctx)
	}
}

// AddCORSHandler adds a CORS preflight handler to the mux for OPTIONS requests
// This is needed because Huma middleware doesn't intercept OPTIONS before routing
func AddCORSHandler(mux *http.ServeMux, config CORSConfig) {
	// Pre-compute header values
	allowMethods := strings.Join(config.AllowMethods, ", ")
	allowHeaders := strings.Join(config.AllowHeaders, ", ")
	maxAge := strconv.Itoa(config.MaxAge)

	mux.HandleFunc("OPTIONS /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", config.AllowOrigin)
		w.Header().Set("Access-Control-Allow-Methods", allowMethods)
		w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
		w.Header().Set("Access-Control-Max-Age", maxAge)
		w.WriteHeader(http.StatusNoContent)
	})
}
