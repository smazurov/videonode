package streams

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/smazurov/videonode/internal/mediamtx"
)

// getExecutablePath returns the absolute path of the running videonode binary.
// This is needed because MediaMTX runs commands in a separate process without
// videonode in its PATH.
func getExecutablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "videonode"
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "videonode"
	}
	return exe
}

// syncToMediaMTX syncs all streams to MediaMTX using the API.
func (s *service) syncToMediaMTX() error {
	// Generate videonode wrapper commands for all streams
	allStreams := s.store.GetAllStreams()

	// Restart each stream with videonode wrapper command
	for streamID := range allStreams {
		command := fmt.Sprintf("%s stream %s", getExecutablePath(), streamID)
		// Force restart to ensure fresh videonode processes
		_ = s.mediamtxClient.RestartPath(streamID, command)
	}

	s.logger.Info("Synchronized streams with MediaMTX API", "count", len(allStreams))
	return nil
}

// getProcessedStreamsForSync is a callback for the MediaMTX client to get all streams.
func (s *service) getProcessedStreamsForSync() []*mediamtx.ProcessedStream {
	// Generate videonode wrapper commands for all streams
	allStreams := s.store.GetAllStreams()
	result := make([]*mediamtx.ProcessedStream, 0, len(allStreams))

	for streamID := range allStreams {
		// Generate simple videonode wrapper command
		result = append(result, &mediamtx.ProcessedStream{
			StreamID:      streamID,
			FFmpegCommand: fmt.Sprintf("%s stream %s", getExecutablePath(), streamID),
		})
	}

	s.logger.Debug("Generated videonode wrapper commands for MediaMTX", "count", len(result))
	return result
}

// GetFFmpegCommand retrieves the FFmpeg command for a stream
// Returns the command and a boolean indicating if it's custom
// Note: Returns unwrapped command for display/editing. Wrapping happens in MediaMTX client when sending.
func (s *service) GetFFmpegCommand(_ context.Context, streamID string, encoderOverride string) (string, bool, error) {
	// Check if stream exists
	streamConfig, exists := s.store.GetStream(streamID)
	if !exists {
		return "", false, NewStreamError(ErrCodeStreamNotFound, fmt.Sprintf("stream %s not found", streamID), nil)
	}

	// If custom command is set, return it unwrapped
	if streamConfig.CustomFFmpegCommand != "" {
		return streamConfig.CustomFFmpegCommand, true, nil
	}

	// Otherwise, process the stream to generate the command
	processed, err := s.processor.processStreamWithEncoder(streamID, encoderOverride)
	if err != nil {
		return "", false, fmt.Errorf("failed to generate FFmpeg command: %w", err)
	}

	return processed.FFmpegCommand, false, nil
}
