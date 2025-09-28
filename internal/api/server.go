package api

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/version"
	"github.com/smazurov/videonode/ui"
)

// Server represents the new Huma v2 API server
type Server struct {
	api            huma.API
	mux            *http.ServeMux
	httpServer     *http.Server
	streamService  streams.StreamService
	options        *Options
	deviceDetector devices.DeviceDetector
	obsSSEAdapter  *OBSSSEAdapter
	logger         *slog.Logger
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
	PrometheusHandler     http.Handler // Optional Prometheus metrics handler
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
	// Empty servers list will make OpenAPI use relative paths, working with any host
	config.Servers = []*huma.Server{}

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
		logger:        logging.GetLogger("api"),
	}

	// Apply CORS middleware first (before auth)
	api.UseMiddleware(NewCORSMiddleware(corsConfig))

	// Apply HTTP logging middleware after CORS but before auth
	api.UseMiddleware(HTTPLoggingMiddleware)

	// Apply basic auth middleware globally if credentials are provided
	if opts.AuthUsername != "" && opts.AuthPassword != "" {
		api.UseMiddleware(server.basicAuthMiddleware(opts.AuthUsername, opts.AuthPassword))
	}

	// Register Prometheus metrics endpoint before other routes (no auth required)
	// This needs to be done before registerRoutes to avoid conflicts with CORS
	if opts.PrometheusHandler != nil {
		mux.Handle("GET /metrics", opts.PrometheusHandler)
	}

	// Register routes
	server.registerRoutes()

	// Serve frontend assets (in production mode or if dist exists)
	if frontendHandler, err := ui.Handler(); err == nil {
		// Serve frontend at root, but only for non-API paths
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// If path starts with /api, let it fall through to API handlers
			if strings.HasPrefix(r.URL.Path, "/api") {
				http.NotFound(w, r)
				return
			}
			frontendHandler.ServeHTTP(w, r)
		})
	}

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
// BroadcastDeviceDiscovery implements the EventBroadcaster interface for device monitoring
func (s *Server) BroadcastDeviceDiscovery(action string, device devices.DeviceInfo, timestamp string) {
	// Convert devices.DeviceInfo to models.DeviceInfo
	apiDevice := models.DeviceInfo{
		DevicePath: device.DevicePath,
		DeviceName: device.DeviceName,
		DeviceId:   device.DeviceId,
		Caps:       device.Caps,
	}
	// Use the global broadcast function from events.go
	BroadcastDeviceDiscovery(action, apiDevice, timestamp)
}

func (s *Server) Start(addr string) error {
	s.logger.Info("Starting VideoNode API server", "addr", addr)
	s.logger.Info("OpenAPI documentation available", "url", "http://"+addr+"/docs")

	// Start device monitoring
	s.deviceDetector = devices.NewDetector()
	if err := s.deviceDetector.StartMonitoring(context.Background(), s); err != nil {
		s.logger.Warn("Failed to start device monitoring", "error", err)
	}

	// Create HTTP server with proper shutdown support
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}

	return s.httpServer.ListenAndServe()
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	s.logger.Info("Stopping API server")

	// Stop device monitoring
	if s.deviceDetector != nil {
		s.deviceDetector.StopMonitoring()
	}

	// Force immediate shutdown - don't wait for connections
	if s.httpServer != nil {
		return s.httpServer.Close()
	}

	return nil
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

	// Version endpoint - no auth required
	huma.Register(s.api, huma.Operation{
		OperationID: "get-version",
		Method:      http.MethodGet,
		Path:        "/api/version",
		Summary:     "Version",
		Description: "Get application version information",
		Tags:        []string{"system"},
		Security:    []map[string][]string{}, // Empty security = no auth required
	}, func(ctx context.Context, input *struct{}) (*models.VersionResponse, error) {
		versionInfo := version.Get()
		return &models.VersionResponse{
			Body: models.VersionData{
				Version:   versionInfo.Version,
				GitCommit: versionInfo.GitCommit,
				BuildDate: versionInfo.BuildDate,
				BuildID:   versionInfo.BuildID,
				GoVersion: versionInfo.GoVersion,
				Compiler:  versionInfo.Compiler,
				Platform:  versionInfo.Platform,
			},
		}, nil
	})

	// Device endpoints
	s.registerDeviceRoutes()

	// Encoder endpoints
	s.registerEncoderRoutes()

	// Audio endpoints
	s.registerAudioRoutes()

	// Stream endpoints
	s.registerStreamRoutes()

	// Options endpoints
	s.registerOptionsRoutes()

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
