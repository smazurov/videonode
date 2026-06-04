package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/streaming"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
	"github.com/smazurov/videonode/internal/streams/services"
	"github.com/smazurov/videonode/internal/streams/store"
)

// CreateOpenAPICmd creates the openapi command that dumps the OpenAPI spec to stdout.
func CreateOpenAPICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "openapi",
		Short: "Dump OpenAPI spec to stdout",
		RunE: func(c *cobra.Command, _ []string) error {
			opts := &api.Options{
				EventBus:          events.New(),
				StreamProvider:    noopStreamProvider{},
				WebRTCManager:     &streaming.WebRTCManager{},
				ProcessesProvider: noopProcessesProvider{},
			}
			// Wire StreamService/SourceService/ComposerService so all CRUD
			// routes surface in the generated spec. The in-memory store
			// never writes to disk during openapi gen; pipeline is nil
			// because route registration walks the table without serving
			// traffic.
			memStore := store.NewInMemory()
			if es, ok := memStore.(streams.EntityStore); ok {
				opts.SourceService = services.NewSourceService(services.SourceServiceOptions{Store: es})
				opts.ComposerService = services.NewComposerService(services.ComposerServiceOptions{Store: es})
				opts.StreamService = services.NewStreamService(services.StreamServiceOptions{
					Store:          es,
					PipelineSwitch: memStore,
				})
				opts.SensorService = services.NewSensorService(services.SensorServiceOptions{Store: es})
			}
			server := api.NewServer(opts)

			enc := json.NewEncoder(c.OutOrStdout())
			enc.SetIndent("", "  ")
			if err := enc.Encode(server.GetAPI().OpenAPI()); err != nil {
				return fmt.Errorf("encode OpenAPI spec: %w", err)
			}
			return nil
		},
	}
}

type noopStreamProvider struct{}

// GetStream implements streaming.StreamProvider.
func (noopStreamProvider) GetStream(string) *streaming.Stream { return nil }

// HasStream implements streaming.StreamProvider.
func (noopStreamProvider) HasStream(string) bool { return false }

// ListStreams implements streaming.StreamProvider.
func (noopStreamProvider) ListStreams() []string { return nil }

// GetStreamReaderCount implements streaming.StreamProvider.
func (noopStreamProvider) GetStreamReaderCount(string) int { return 0 }

// EnsureStreamReady implements streaming.StreamProvider.
func (noopStreamProvider) EnsureStreamReady(string, time.Duration) *streaming.Stream { return nil }

type noopProcessesProvider struct{}

// Snapshot implements api.ProcessesProvider.
func (noopProcessesProvider) Snapshot() []pipeline.ProcessView { return nil }

// RestartProcess implements api.ProcessesProvider.
func (noopProcessesProvider) RestartProcess(string) error { return nil }
