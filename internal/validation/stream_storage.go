package validation

import (
	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/types"
)

// StreamStorage implements Storage interface using StreamManager
type StreamStorage struct {
	streamManager *config.StreamManager
}

// NewStreamStorage creates a new StreamStorage adapter
func NewStreamStorage(sm *config.StreamManager) *StreamStorage {
	return &StreamStorage{
		streamManager: sm,
	}
}

// Save persists validation data through StreamManager
func (s *StreamStorage) Save(validation *types.ValidationResults) error {
	return s.streamManager.UpdateValidation(validation)
}

// Load retrieves validation data from StreamManager
func (s *StreamStorage) Load() (*types.ValidationResults, error) {
	validation := s.streamManager.GetValidation()
	if validation == nil {
		// Return empty results instead of error for missing data
		return &types.ValidationResults{
			H264: types.CodecValidation{
				Working: []string{},
				Failed:  []string{},
			},
			H265: types.CodecValidation{
				Working: []string{},
				Failed:  []string{},
			},
		}, nil
	}
	return validation, nil
}
