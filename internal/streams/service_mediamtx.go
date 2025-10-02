package streams

import (
	"context"
	"fmt"

	"github.com/smazurov/videonode/internal/mediamtx"
)

// syncToMediaMTX syncs all streams to MediaMTX using the API
func (s *service) syncToMediaMTX() error {
	// Process all streams to generate FFmpeg commands
	processedStreams, err := s.processor.processAllStreams()
	if err != nil {
		return fmt.Errorf("failed to process streams: %w", err)
	}

	// Restart each stream to ensure fresh FFmpeg processes connect to progress sockets
	for _, stream := range processedStreams {
		if stream.FFmpegCommand != "" {
			// Force restart to ensure FFmpeg connects to socket listeners
			_ = s.mediamtxClient.RestartPath(stream.StreamID, stream.FFmpegCommand)
		}
	}

	s.logger.Info("Synchronized streams with MediaMTX API", "count", len(processedStreams))
	return nil
}

// getProcessedStreamsForSync is a callback for the MediaMTX client to get all streams
func (s *service) getProcessedStreamsForSync() []*mediamtx.ProcessedStream {
	processedStreams, err := s.processor.processAllStreams()
	if err != nil {
		s.logger.Error("Failed to process streams for sync", "error", err)
		return nil
	}

	// Filter out empty commands
	result := make([]*mediamtx.ProcessedStream, 0, len(processedStreams))
	for _, stream := range processedStreams {
		if stream.FFmpegCommand != "" {
			result = append(result, stream)
		}
	}
	return result
}

// GetFFmpegCommand retrieves the FFmpeg command for a stream
// Returns the command and a boolean indicating if it's custom
// Note: Returns unwrapped command for display/editing. Wrapping happens in MediaMTX client when sending.
func (s *service) GetFFmpegCommand(ctx context.Context, streamID string, encoderOverride string) (string, bool, error) {
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