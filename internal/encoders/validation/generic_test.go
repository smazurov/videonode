package validation

import (
	"testing"

	"github.com/smazurov/videonode/internal/types"
)

func TestGenericValidator_GetQualityParams(t *testing.T) {
	v := NewGenericValidator()

	tests := []struct {
		name        string
		encoderName string
		params      *types.QualityParams
		wantParams  map[string]string
		wantErr     bool
	}{
		{
			name:        "libx264 CRF mode",
			encoderName: "libx264",
			params: &types.QualityParams{
				Mode:    types.RateControlCRF,
				Quality: intPtr(23),
			},
			wantParams: map[string]string{
				"crf":    "23",
				"preset": "ultrafast",
			},
			wantErr: false,
		},
		{
			name:        "libx264 CRF mode with default",
			encoderName: "libx264",
			params: &types.QualityParams{
				Mode: types.RateControlCRF,
			},
			wantParams: map[string]string{
				"crf":    "23",
				"preset": "ultrafast",
			},
			wantErr: false,
		},
		{
			name:        "libx265 CRF mode with default",
			encoderName: "libx265",
			params: &types.QualityParams{
				Mode: types.RateControlCRF,
			},
			wantParams: map[string]string{
				"crf":    "28",
				"preset": "ultrafast",
			},
			wantErr: false,
		},
		{
			name:        "libx264 CBR mode",
			encoderName: "libx264",
			params: &types.QualityParams{
				Mode:          types.RateControlCBR,
				TargetBitrate: floatPtr(5.0),
				BufferSize:    floatPtr(10.0),
			},
			wantParams: map[string]string{
				"b:v":     "5.0M",
				"minrate": "5.0M",
				"maxrate": "5.0M",
				"bufsize": "10.0M",
				"preset":  "ultrafast",
			},
			wantErr: false,
		},
		{
			name:        "libx264 CBR mode with default buffer",
			encoderName: "libx264",
			params: &types.QualityParams{
				Mode:          types.RateControlCBR,
				TargetBitrate: floatPtr(4.0),
			},
			wantParams: map[string]string{
				"b:v":     "4.0M",
				"minrate": "4.0M",
				"maxrate": "4.0M",
				"bufsize": "8.0M",
				"preset":  "ultrafast",
			},
			wantErr: false,
		},
		{
			name:        "libx265 VBR mode",
			encoderName: "libx265",
			params: &types.QualityParams{
				Mode:          types.RateControlVBR,
				TargetBitrate: floatPtr(5.0),
				MaxBitrate:    floatPtr(8.0),
				BufferSize:    floatPtr(12.0),
			},
			wantParams: map[string]string{
				"b:v":     "5.0M",
				"maxrate": "8.0M",
				"bufsize": "12.0M",
				"preset":  "ultrafast",
			},
			wantErr: false,
		},
		{
			name:        "libx264 VBR mode with default buffer",
			encoderName: "libx264",
			params: &types.QualityParams{
				Mode:          types.RateControlVBR,
				TargetBitrate: floatPtr(5.0),
				MaxBitrate:    floatPtr(10.0),
			},
			wantParams: map[string]string{
				"b:v":     "5.0M",
				"maxrate": "10.0M",
				"bufsize": "20.0M",
				"preset":  "ultrafast",
			},
			wantErr: false,
		},
		{
			name:        "libx264 CQP mode",
			encoderName: "libx264",
			params: &types.QualityParams{
				Mode:    types.RateControlCQP,
				Quality: intPtr(20),
			},
			wantParams: map[string]string{
				"qp":     "20",
				"preset": "ultrafast",
			},
			wantErr: false,
		},
		{
			name:        "libx264 with custom preset",
			encoderName: "libx264",
			params: &types.QualityParams{
				Mode:    types.RateControlCRF,
				Quality: intPtr(23),
				Preset:  stringPtr("medium"),
			},
			wantParams: map[string]string{
				"crf":    "23",
				"preset": "medium",
			},
			wantErr: false,
		},
		{
			name:        "libx264 with invalid preset ignored",
			encoderName: "libx264",
			params: &types.QualityParams{
				Mode:    types.RateControlCRF,
				Quality: intPtr(23),
				Preset:  stringPtr("invalid"),
			},
			wantParams: map[string]string{
				"crf": "23",
				// Invalid preset is ignored, default is only added when preset is nil
			},
			wantErr: false,
		},
		{
			name:        "libx264 with B-frames and GOP",
			encoderName: "libx264",
			params: &types.QualityParams{
				Mode:             types.RateControlCRF,
				Quality:          intPtr(23),
				BFrames:          intPtr(3),
				KeyframeInterval: intPtr(60),
			},
			wantParams: map[string]string{
				"crf":    "23",
				"preset": "ultrafast",
				"bf":     "3",
				"g":      "60",
			},
			wantErr: false,
		},
		{
			name:        "libvpx CBR mode",
			encoderName: "libvpx",
			params: &types.QualityParams{
				Mode:          types.RateControlCBR,
				TargetBitrate: floatPtr(2.0),
			},
			wantParams: map[string]string{
				"b:v":     "2.0M",
				"minrate": "2.0M",
				"maxrate": "2.0M",
			},
			wantErr: false,
		},
		{
			name:        "libvpx-vp9 VBR mode",
			encoderName: "libvpx-vp9",
			params: &types.QualityParams{
				Mode:             types.RateControlVBR,
				Quality:          intPtr(30),
				TargetBitrate:    floatPtr(3.0),
				KeyframeInterval: intPtr(120),
			},
			wantParams: map[string]string{
				"crf": "30",
				"b:v": "3.0M",
				"g":   "120",
			},
			wantErr: false,
		},
		{
			name:        "libvpx-vp9 CRF mode",
			encoderName: "libvpx-vp9",
			params: &types.QualityParams{
				Mode:    types.RateControlCRF,
				Quality: intPtr(35),
			},
			wantParams: map[string]string{
				"crf": "35",
			},
			wantErr: false,
		},
		{
			name:        "unknown encoder CBR fallback",
			encoderName: "unknown_encoder",
			params: &types.QualityParams{
				Mode:          types.RateControlCBR,
				TargetBitrate: floatPtr(3.0),
				MinBitrate:    floatPtr(2.0),
				MaxBitrate:    floatPtr(4.0),
			},
			wantParams: map[string]string{
				"b:v":     "3.0M",
				"minrate": "2.0M",
				"maxrate": "4.0M",
			},
			wantErr: false,
		},
		{
			name:        "unknown encoder CQP fallback",
			encoderName: "some_encoder",
			params: &types.QualityParams{
				Mode:             types.RateControlCQP,
				Quality:          intPtr(25),
				KeyframeInterval: intPtr(90),
			},
			wantParams: map[string]string{
				"qp": "25",
				"g":  "90",
			},
			wantErr: false,
		},
		{
			name:        "libx264 unsupported mode",
			encoderName: "libx264",
			params: &types.QualityParams{
				Mode: "invalid_mode",
			},
			wantParams: nil,
			wantErr:    true,
		},
		{
			name:        "libvpx unsupported mode",
			encoderName: "libvpx",
			params: &types.QualityParams{
				Mode: types.RateControlCQP,
			},
			wantParams: nil,
			wantErr:    true,
		},
		{
			name:        "unknown encoder unsupported mode",
			encoderName: "unknown",
			params: &types.QualityParams{
				Mode: types.RateControlCRF,
			},
			wantParams: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := v.GetQualityParams(tt.encoderName, tt.params)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetQualityParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(result) != len(tt.wantParams) {
					t.Errorf("GetQualityParams() returned %d params, want %d", len(result), len(tt.wantParams))
				}

				for key, wantValue := range tt.wantParams {
					if gotValue, ok := result[key]; !ok {
						t.Errorf("GetQualityParams() missing key %s", key)
					} else if gotValue != wantValue {
						t.Errorf("GetQualityParams() key %s = %v, want %v", key, gotValue, wantValue)
					}
				}
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestGenericValidator_GetProductionSettings(t *testing.T) {
	v := NewGenericValidator()

	tests := []struct {
		name        string
		encoderName string
		inputFormat string
		wantFilters string
		wantCrf     string
		wantPreset  string
		wantErr     bool
	}{
		{
			name:        "libx264 with MJPEG input",
			encoderName: "libx264",
			inputFormat: "mjpeg",
			wantFilters: "format=yuv420p",
			wantCrf:     "18",
			wantPreset:  "ultrafast",
			wantErr:     false,
		},
		{
			name:        "libx264 with YUYV input",
			encoderName: "libx264",
			inputFormat: "yuyv422",
			wantFilters: "format=yuv420p",
			wantCrf:     "18",
			wantPreset:  "ultrafast",
			wantErr:     false,
		},
		{
			name:        "libx264 with empty input",
			encoderName: "libx264",
			inputFormat: "",
			wantFilters: "",
			wantCrf:     "18",
			wantPreset:  "ultrafast",
			wantErr:     false,
		},
		{
			name:        "libx265 with MJPEG input",
			encoderName: "libx265",
			inputFormat: "mjpeg",
			wantFilters: "format=yuv420p",
			wantCrf:     "20",
			wantPreset:  "ultrafast",
			wantErr:     false,
		},
		{
			name:        "unknown encoder with MJPEG input",
			encoderName: "some_encoder",
			inputFormat: "mjpeg",
			wantFilters: "format=yuv420p",
			wantCrf:     "",
			wantPreset:  "",
			wantErr:     false,
		},
		{
			name:        "unknown encoder with empty input",
			encoderName: "random_encoder",
			inputFormat: "",
			wantFilters: "",
			wantCrf:     "",
			wantPreset:  "",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := v.GetProductionSettings(tt.encoderName, tt.inputFormat)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetProductionSettings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if settings.VideoFilters != tt.wantFilters {
					t.Errorf("GetProductionSettings() VideoFilters = %v, want %v", settings.VideoFilters, tt.wantFilters)
				}

				if tt.wantCrf != "" {
					if crf, ok := settings.OutputParams["crf"]; !ok || crf != tt.wantCrf {
						t.Errorf("GetProductionSettings() crf = %v, want %v", crf, tt.wantCrf)
					}
				}

				if tt.wantPreset != "" {
					if preset, ok := settings.OutputParams["preset"]; !ok || preset != tt.wantPreset {
						t.Errorf("GetProductionSettings() preset = %v, want %v", preset, tt.wantPreset)
					}
				}

				// For unknown encoders, check for generic bitrate
				if tt.encoderName != "libx264" && tt.encoderName != "libx265" {
					if bitrate, ok := settings.OutputParams["b:v"]; ok && bitrate != "1M" {
						t.Errorf("GetProductionSettings() generic bitrate = %v, want 1M", bitrate)
					}
				}
			}
		})
	}
}
