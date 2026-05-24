package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/streaming"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// CreateOpenAPICmd creates the openapi command that dumps the OpenAPI spec to stdout.
func CreateOpenAPICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "openapi",
		Short: "Dump OpenAPI spec to stdout",
		RunE: func(c *cobra.Command, _ []string) error {
			server := api.NewServer(&api.Options{
				EventBus:          events.New(),
				StreamProvider:    noopStreamProvider{},
				RecordingDir:      "/tmp",
				WebRTCManager:     &streaming.WebRTCManager{},
				ProcessesProvider: noopProcessesProvider{},
			})

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

type noopProcessesProvider struct{}

// Snapshot implements api.ProcessesProvider.
func (noopProcessesProvider) Snapshot() []pipeline.ProcessView { return nil }
