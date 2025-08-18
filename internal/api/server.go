package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/monitoring"
	"github.com/smazurov/videonode/internal/streams"
)

// Server represents the new Huma v2 API server
type Server struct {
	api           huma.API
	mux           *http.ServeMux
	streamService streams.StreamService
	options       *Options
	udevMonitor   *monitoring.UdevMonitor
	obsSSEAdapter *OBSSSEAdapter
}

// basicAuthMiddleware creates middleware for HTTP basic authentication
func (s *Server) basicAuthMiddleware(username, password string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		// Skip auth for operations without security requirements
		op := ctx.Operation()
		if op != nil && len(op.Security) == 0 {
			next(ctx)
			return
		}

		// Try Authorization header first
		authHeader := ctx.Header("Authorization")
		var credentials string
		var parts []string

		if authHeader != "" {
			// Parse "Basic <credentials>" format
			const prefix = "Basic "
			if !strings.HasPrefix(authHeader, prefix) {
				ctx.SetHeader("WWW-Authenticate", `Basic realm="VideoNode API"`)
				huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "Invalid authentication type")
				return
			}

			// Decode base64 credentials
			encoded := authHeader[len(prefix):]
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				ctx.SetHeader("WWW-Authenticate", `Basic realm="VideoNode API"`)
				huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "Invalid credentials format", err)
				return
			}

			credentials = string(decoded)
		} else {
			// For SSE endpoints, try query parameters as fallback
			queryAuth := ctx.Query("auth")
			if queryAuth != "" {
				decoded, err := base64.StdEncoding.DecodeString(queryAuth)
				if err != nil {
					ctx.SetHeader("WWW-Authenticate", `Basic realm="VideoNode API"`)
					huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "Invalid credentials format", err)
					return
				}
				credentials = string(decoded)
			}
		}

		if credentials == "" {
			ctx.SetHeader("WWW-Authenticate", `Basic realm="VideoNode API"`)
			huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "Authentication required")
			return
		}

		// Split username:password
		parts = strings.SplitN(credentials, ":", 2)
		if len(parts) != 2 {
			ctx.SetHeader("WWW-Authenticate", `Basic realm="VideoNode API"`)
			huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "Invalid credentials format")
			return
		}

		// Validate credentials
		if parts[0] != username || parts[1] != password {
			ctx.SetHeader("WWW-Authenticate", `Basic realm="VideoNode API"`)
			huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "Invalid credentials")
			return
		}

		// Continue to next handler
		next(ctx)
	}
}

// Options represents the main application options (imported from main package)
type Options struct {
	AuthUsername          string
	AuthPassword          string
	CaptureDefaultDelayMs int
	StreamService         streams.StreamService
}

// NewServer creates a new API server with Huma v2 using Go 1.22+ native routing
func NewServer(opts *Options) *Server {
	mux := http.NewServeMux()

	// Configure CORS
	corsConfig := DefaultCORSConfig()

	// Add CORS preflight handler for all OPTIONS requests
	AddCORSHandler(mux, corsConfig)

	// Create Huma API with Go standard library adapter
	config := huma.DefaultConfig("VideoNode API", "1.0.0")
	config.Info.Description = "Video capture and streaming API for V4L2 devices"
	config.Servers = []*huma.Server{
		{URL: "http://localhost:8090", Description: "Development server"},
	}

	// Configure basic auth security scheme
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"basicAuth": {
			Type:   "http",
			Scheme: "basic",
		},
	}

	api := humago.New(mux, config)

	server := &Server{
		api:           api,
		mux:           mux,
		streamService: opts.StreamService,
		options:       opts,
	}

	// Apply CORS middleware first (before auth)
	api.UseMiddleware(NewCORSMiddleware(corsConfig))

	// Apply basic auth middleware globally if credentials are provided
	if opts.AuthUsername != "" && opts.AuthPassword != "" {
		api.UseMiddleware(server.basicAuthMiddleware(opts.AuthUsername, opts.AuthPassword))
	}

	// Register routes
	server.registerRoutes()

	return server
}

// GetMux returns the underlying HTTP ServeMux for additional setup
func (s *Server) GetMux() *http.ServeMux {
	return s.mux
}

// GetAPI returns the Huma API instance
func (s *Server) GetAPI() huma.API {
	return s.api
}

// Start starts the HTTP server on the specified address
// BroadcastDeviceDiscovery implements the EventBroadcaster interface for udev monitoring
func (s *Server) BroadcastDeviceDiscovery(action string, device models.DeviceInfo, timestamp string) {
	// Use the global broadcast function from events.go
	BroadcastDeviceDiscovery(action, device, timestamp)
}

func (s *Server) Start(addr string) error {
	fmt.Printf("Starting VideoNode API server on %s\n", addr)
	fmt.Printf("OpenAPI documentation available at: http://%s/docs\n", addr)

	// Start udev monitoring
	s.udevMonitor = monitoring.NewUdevMonitor(s)
	if err := s.udevMonitor.Start(); err != nil {
		fmt.Printf("Warning: Failed to start udev monitoring: %v\n", err)
	}

	return http.ListenAndServe(addr, s.mux)
}

// registerRoutes sets up all API endpoints
func (s *Server) registerRoutes() {
	// Health check endpoint - no auth required
	huma.Register(s.api, huma.Operation{
		OperationID: "health-check",
		Method:      http.MethodGet,
		Path:        "/api/health",
		Summary:     "Health",
		Description: "Check API health status",
		Tags:        []string{"health"},
		Security:    []map[string][]string{}, // Empty security = no auth required
	}, func(ctx context.Context, input *struct{}) (*models.HealthResponse, error) {
		return &models.HealthResponse{
			Body: models.HealthData{
				Status:  "ok",
				Message: "API is healthy",
			},
		}, nil
	})

	// Device endpoints
	s.registerDeviceRoutes()

	// Encoder endpoints
	s.registerEncoderRoutes()

	// Stream endpoints
	s.registerStreamRoutes()

	// SSE endpoints
	s.registerSSERoutes()

	// Metrics SSE endpoint
	s.registerMetricsRoutes()
}

// withAuth returns security requirement for basic auth
func withAuth() []map[string][]string {
	return []map[string][]string{
		{"basicAuth": {}},
	}
}
