package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/store"
)

// getValidatedEncoders returns only encoders that passed validation and are saved in streams.toml
func (s *Server) getValidatedEncoders() (*encoders.EncoderList, error) {
	// Create validation service to load validation results
	streamStore := store.NewTOML("streams.toml")
	validationService := streams.NewValidationService(streamStore)

	// Load validation results from storage
	results, err := encoders.LoadValidationResults(validationService)
	if err != nil {
		// If no validation data exists, return error - system needs to be validated first
		return nil, fmt.Errorf("validation data not found - run encoder validation first: %w", err)
	}

	// Get all available encoders from system
	allEncoders, err := encoders.GetFFmpegEncoders()
	if err != nil {
		return nil, fmt.Errorf("failed to get system encoders: %w", err)
	}

	// Create a map of all available encoders for lookup
	availableMap := make(map[string]encoders.Encoder)
	for _, encoder := range allEncoders.VideoEncoders {
		availableMap[encoder.Name] = encoder
	}
	for _, encoder := range allEncoders.AudioEncoders {
		availableMap[encoder.Name] = encoder
	}

	// Create encoder list to hold only validated working encoders
	validatedList := &encoders.EncoderList{
		VideoEncoders: []encoders.Encoder{},
		AudioEncoders: []encoders.Encoder{},
	}

	// Add working encoders from validation results
	for _, encoderName := range results.H264.Working {
		if encoder, exists := availableMap[encoderName]; exists {
			validatedList.VideoEncoders = append(validatedList.VideoEncoders, encoder)
		} else {
			// Create encoder entry for validated encoder even if not in current system list
			validatedList.VideoEncoders = append(validatedList.VideoEncoders, encoders.Encoder{
				Type:        "V",
				Name:        encoderName,
				Description: "H.264 encoder",
				HWAccel:     !strings.Contains(encoderName, "lib"),
			})
		}
	}

	for _, encoderName := range results.H265.Working {
		if encoder, exists := availableMap[encoderName]; exists {
			validatedList.VideoEncoders = append(validatedList.VideoEncoders, encoder)
		} else {
			validatedList.VideoEncoders = append(validatedList.VideoEncoders, encoders.Encoder{
				Type:        "V",
				Name:        encoderName,
				Description: "H.265/HEVC encoder",
				HWAccel:     !strings.Contains(encoderName, "lib"),
			})
		}
	}

	// Add popular audio encoders
	popularAudioEncoders := []string{"aac", "libopus", "libmp3lame", "ac3"}
	for _, encoderName := range popularAudioEncoders {
		if encoder, exists := availableMap[encoderName]; exists {
			validatedList.AudioEncoders = append(validatedList.AudioEncoders, encoder)
		} else {
			var description string
			switch encoderName {
			case "aac":
				description = "AAC (Advanced Audio Coding)"
			case "libopus":
				description = "Opus audio codec"
			case "libmp3lame":
				description = "MP3 (MPEG Audio Layer 3)"
			case "ac3":
				description = "AC-3 (Dolby Digital)"
			}
			validatedList.AudioEncoders = append(validatedList.AudioEncoders, encoders.Encoder{
				Type:        "A",
				Name:        encoderName,
				Description: description,
				HWAccel:     false,
			})
		}
	}

	return validatedList, nil
}

// convertEncoders converts internal encoder types to API response types
func convertEncoders(encoderList *encoders.EncoderList) models.EncoderData {
	convertEncoder := func(e encoders.Encoder, encoderType models.EncoderType) models.EncoderInfo {
		return models.EncoderInfo{
			Type:        encoderType,
			Name:        e.Name,
			Description: e.Description,
			HWAccel:     e.HWAccel,
		}
	}

	videoEncoders := make([]models.EncoderInfo, len(encoderList.VideoEncoders))
	for i, e := range encoderList.VideoEncoders {
		videoEncoders[i] = convertEncoder(e, models.VideoEncoder)
	}

	audioEncoders := make([]models.EncoderInfo, len(encoderList.AudioEncoders))
	for i, e := range encoderList.AudioEncoders {
		audioEncoders[i] = convertEncoder(e, models.AudioEncoder)
	}

	totalCount := len(videoEncoders) + len(audioEncoders)

	return models.EncoderData{
		VideoEncoders: videoEncoders,
		AudioEncoders: audioEncoders,
		Count:         totalCount,
	}
}

// GetEncodersData fetches the list of validated encoders
func (s *Server) GetEncodersData() (models.EncoderData, error) {
	// Get validated encoders
	encoderList, err := s.getValidatedEncoders()
	if err != nil {
		return models.EncoderData{}, fmt.Errorf("failed to get validated encoders: %w", err)
	}

	return convertEncoders(encoderList), nil
}

// registerEncoderRoutes registers all encoder-related endpoints
func (s *Server) registerEncoderRoutes() {
	// List encoders
	huma.Register(s.api, huma.Operation{
		OperationID: "list-encoders",
		Method:      http.MethodGet,
		Path:        "/api/encoders",
		Summary:     "List Encoders",
		Description: "List validated video and audio encoders available in the system",
		Tags:        []string{"encoders"},
		Security:    withAuth(),
		Errors:      []int{400, 401, 500},
	}, func(ctx context.Context, input *struct{}) (*models.EncodersResponse, error) {
		data, err := s.GetEncodersData()
		if err != nil {
			// Check if error is due to missing validation data
			if strings.Contains(err.Error(), "validation data not found") {
				return nil, huma.Error400BadRequest("Validation required - run encoder validation first", err)
			}
			return nil, huma.Error500InternalServerError("Failed to get encoders", err)
		}

		return &models.EncodersResponse{Body: data}, nil
	})
}
