package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/auth"
	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/recording"
	"github.com/smazurov/videonode/internal/streaming"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
	"github.com/smazurov/videonode/internal/updater"
	"github.com/smazurov/videonode/ui"
)

// Server represents the new Huma v2 API server.
type Server struct {
	api            huma.API
	mux            *http.ServeMux
	httpServer     *http.Server
	streamService  streams.StreamService
	sourceService  SourceService
	options        *Options
	deviceDetector devices.DeviceDetector
	eventBus       *events.Bus
	controlServer  *pipelinectl.Manager
	logger         logging.Logger
}

// rtspPortOrDefault returns the configured RTSP publish port (e.g.
// ":8554" or "10.0.0.1:8654"), falling back to the well-known default
// when Options.StreamingRTSPPort wasn't set.
func (s *Server) rtspPortOrDefault() string {
	if s.options != nil && s.options.StreamingRTSPPort != "" {
		return s.options.StreamingRTSPPort
	}
	return ":8554"
}

// srtPortOrDefault mirrors rtspPortOrDefault for the SRT publish port.
func (s *Server) srtPortOrDefault() string {
	if s.options != nil && s.options.StreamingSRTPort != "" {
		return s.options.StreamingSRTPort
	}
	return ":6001"
}

// basicAuthMiddleware creates middleware for HTTP basic authentication.
func (s *Server) basicAuthMiddleware(authenticator auth.Authenticator) func(huma.Context, func(huma.Context)) {
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
				decodedQuery, decodeErr := base64.StdEncoding.DecodeString(queryAuth)
				if decodeErr != nil {
					ctx.SetHeader("WWW-Authenticate", `Basic realm="VideoNode API"`)
					huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "Invalid credentials format", decodeErr)
					return
				}
				credentials = string(decodedQuery)
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

		// Validate credentials using authenticator
		result := authenticator.Authenticate(parts[0], parts[1])
		if result.Error != nil {
			ctx.SetHeader("WWW-Authenticate", `Basic realm="VideoNode API"`)
			huma.WriteErr(s.api, ctx, http.StatusInternalServerError, "Authentication error")
			return
		}
		if !result.Valid {
			ctx.SetHeader("WWW-Authenticate", `Basic realm="VideoNode API"`)
			huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "Invalid credentials")
			return
		}

		// Continue to next handler
		next(ctx)
	}
}

// Options represents the main application options (imported from main package).
type Options struct {
	Authenticator     auth.Authenticator
	StreamService     streams.StreamService
	SourceService     SourceService   // Optional: enables /api/sources CRUD when set
	EventBus          *events.Bus     // Event bus for in-process events
	PrometheusHandler http.Handler    // Optional Prometheus metrics handler
	UpdateService     updater.Service // Optional self-update service
	LEDController     interface {     // Optional LED controller
		Set(ledType string, enabled bool, pattern string) error
		Available() []string
		Patterns() []string
	}
	WebRTCManager       *streaming.WebRTCManager      // WebRTC signaling manager
	StreamProvider      streaming.StreamProvider      // Stream access for snapshots/recording
	RawSnapshotProvider recording.RawSnapshotProvider // Raw vision pipe snapshot provider
	RecordingDir        string                        // Directory for snapshot images
	ControlServer       *pipelinectl.Manager          // Optional control plane for native sidecars
	ProcessesProvider   ProcessesProvider             // Optional: enables GET /api/processes when set
	// StreamingRTSPPort is the daemon's RTSP listen address as configured
	// at startup (":8554" by default). Used in API responses (rtsp_url
	// field) so clients dial the actual published port, not a hardcoded
	// 8554.
	StreamingRTSPPort string
	// StreamingSRTPort mirrors StreamingRTSPPort for the SRT publish port
	// surfaced as srt_url in API responses.
	StreamingSRTPort string
}

// NewServer creates a new API server with Huma v2 using Go 1.22+ native routing.
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
		sourceService: opts.SourceService,
		options:       opts,
		eventBus:      opts.EventBus,
		controlServer: opts.ControlServer,
		logger:        logging.GetLogger("api"),
	}

	// Apply CORS middleware first (before auth)
	api.UseMiddleware(NewCORSMiddleware(corsConfig))

	// Apply HTTP logging middleware after CORS but before auth
	api.UseMiddleware(HTTPLoggingMiddleware)

	// Apply auth middleware globally if authenticator is provided
	if opts.Authenticator != nil {
		api.UseMiddleware(server.basicAuthMiddleware(opts.Authenticator))
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

// GetMux returns the underlying HTTP ServeMux for additional setup.
func (s *Server) GetMux() *http.ServeMux {
	return s.mux
}

// GetAPI returns the Huma API instance.
func (s *Server) GetAPI() huma.API {
	return s.api
}

// CompositeBroadcaster broadcasts device events to multiple EventBroadcasters.
type CompositeBroadcaster struct {
	broadcasters []devices.EventBroadcaster
}

// BroadcastDeviceDiscovery implements devices.EventBroadcaster interface.
func (cb *CompositeBroadcaster) BroadcastDeviceDiscovery(action string, device devices.DeviceInfo, timestamp string) {
	for _, broadcaster := range cb.broadcasters {
		broadcaster.BroadcastDeviceDiscovery(action, device, timestamp)
	}
}

// BroadcastDeviceDiscovery implements the EventBroadcaster interface for device monitoring
// This is for the Server to broadcast to SSE clients.
func (s *Server) BroadcastDeviceDiscovery(action string, device devices.DeviceInfo, timestamp string) {
	if s.eventBus == nil {
		return
	}

	// Convert devices.DeviceInfo to models.DeviceInfo and publish
	apiDevice := models.DeviceInfo{
		DevicePath: device.DevicePath,
		DeviceName: device.DeviceName,
		DeviceID:   device.DeviceID,
		Caps:       device.Caps,
		Ready:      device.Ready,
		Type:       models.DeviceType(device.Type),
	}

	s.eventBus.Publish(events.DeviceDiscoveryEvent{
		DeviceInfo: apiDevice,
		Action:     action,
		Timestamp:  timestamp,
	})
}

// Start starts the API server on the specified address and begins device monitoring.
func (s *Server) Start(addr string) error {
	s.logger.Info("Starting VideoNode API server", "addr", addr)
	s.logger.Info("OpenAPI documentation available", "url", "http://"+addr+"/docs")

	// Start device monitoring with composite broadcaster
	// This broadcasts device events to both SSE clients (via Server) and stream management (via StreamService)
	s.deviceDetector = devices.NewDetector()
	s.deviceDetector.SetEventBus(s.eventBus)
	compositeBroadcaster := &CompositeBroadcaster{
		broadcasters: []devices.EventBroadcaster{s, s.streamService},
	}
	if err := s.deviceDetector.StartMonitoring(context.Background(), compositeBroadcaster); err != nil {
		s.logger.Warn("Failed to start device monitoring", "error", err)
	}

	// Create HTTP server with proper shutdown support
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}

	return s.httpServer.ListenAndServe()
}

// Stop gracefully shuts down the server.
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

// registerRoutes sets up all API endpoints.
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
	}, func(_ context.Context, _ *struct{}) (*models.HealthResponse, error) {
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

	// Audio endpoints
	s.registerAudioRoutes()

	// Source endpoints (no-op when SourceService is nil)
	s.registerSourceRoutes()

	// Stream endpoints
	s.registerStreamRoutes()

	// Pipeline master switch endpoints
	s.registerPipelineRoutes()

	// Pipeline processes endpoint (no-op when provider is nil — daemon
	// without the new pipeline foundation wired).
	RegisterProcessesRoutes(s.api, s.options.ProcessesProvider)

	// Options endpoints
	s.registerOptionsRoutes()

	// LED endpoints (if LED controller is available)
	s.registerLEDRoutes()

	// Update endpoints (if update service is available)
	s.registerUpdateRoutes()

	// WebRTC signaling endpoints (if WebRTC manager is available)
	if s.options.WebRTCManager != nil {
		streaming.RegisterWebRTCAPI(s.api, s.options.WebRTCManager)
	}

	// Recording endpoints (snapshots, future: video recording)
	if s.options.StreamProvider != nil && s.options.RecordingDir != "" {
		recording.RegisterAPI(s.api, s.mux, s.options.StreamProvider, s.options.RawSnapshotProvider, s.options.RecordingDir)
	}

	// SSE endpoints
	s.registerSSERoutes()

	// Log streaming endpoint
	s.registerLogRoutes()

	// Metrics JSON endpoint (with auth)
	s.registerMetricsRoutes()
}

// withAuth returns security requirement for basic auth.
func withAuth() []map[string][]string {
	return []map[string][]string{
		{"basicAuth": {}},
	}
}
