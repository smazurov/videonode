package validation

import (
	"testing"

	"github.com/smazurov/videonode/internal/types"
)

func TestVaapiValidator_GetQualityParams(t *testing.T) {
	v := NewVaapiValidator()

	tests := []struct {
		name        string
		encoderName string
		params      *types.QualityParams
		wantParams  map[string]string
		wantErr     bool
	}{
		{
			name:        "CBR mode with all parameters",
			encoderName: "h264_vaapi",
			params: &types.QualityParams{
				Mode:             types.RateControlCBR,
				TargetBitrate:    floatPtr(5.0),
				BufferSize:       floatPtr(10.0),
				BFrames:          intPtr(2),
				KeyframeInterval: intPtr(60),
			},
			wantParams: map[string]string{
				"rc_mode": "CBR",
				"b:v":     "5.0M",
				"bufsize": "10.0M",
				"bf":      "2",
				"g":       "60",
			},
			wantErr: false,
		},
		{
			name:        "VBR mode with max bitrate",
			encoderName: "hevc_vaapi",
			params: &types.QualityParams{
				Mode:          types.RateControlVBR,
				TargetBitrate: floatPtr(4.0),
				MaxBitrate:    floatPtr(8.0),
				BufferSize:    floatPtr(16.0),
			},
			wantParams: map[string]string{
				"rc_mode": "VBR",
				"b:v":     "4.0M",
				"maxrate": "8.0M",
				"bufsize": "16.0M",
			},
			wantErr: false,
		},
		{
			name:        "CQP mode with quality",
			encoderName: "h264_vaapi",
			params: &types.QualityParams{
				Mode:    types.RateControlCQP,
				Quality: intPtr(20),
			},
			wantParams: map[string]string{
				"rc_mode": "CQP",
				"qp":      "20",
			},
			wantErr: false,
		},
		{
			name:        "CRF mode should fail",
			encoderName: "h264_vaapi",
			params: &types.QualityParams{
				Mode:    types.RateControlCRF,
				Quality: intPtr(23),
			},
			wantParams: nil,
			wantErr:    true,
		},
		{
			name:        "Invalid encoder name",
			encoderName: "invalid_encoder",
			params: &types.QualityParams{
				Mode: types.RateControlCBR,
			},
			wantParams: nil,
			wantErr:    true,
		},
		{
			name:        "Unsupported rate control mode",
			encoderName: "h264_vaapi",
			params: &types.QualityParams{
				Mode: "invalid_mode",
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

func TestVaapiValidator_GetProductionSettings(t *testing.T) {
	v := NewVaapiValidator()

	tests := []struct {
		name           string
		encoderName    string
		inputFormat    string
		wantFilters    string
		wantGlobalArgs []string
		wantErr        bool
	}{
		{
			name:           "MJPEG input format",
			encoderName:    "h264_vaapi",
			inputFormat:    "mjpeg",
			wantFilters:    "format=yuvj420p,format=yuv420p,format=nv12,hwupload",
			wantGlobalArgs: []string{"-vaapi_device", "/dev/dri/renderD128"},
			wantErr:        false,
		},
		{
			name:           "H264 input format",
			encoderName:    "hevc_vaapi",
			inputFormat:    "h264",
			wantFilters:    "format=yuv420p,format=nv12,hwupload",
			wantGlobalArgs: []string{"-vaapi_device", "/dev/dri/renderD128"},
			wantErr:        false,
		},
		{
			name:           "YUYV422 input format",
			encoderName:    "h264_vaapi",
			inputFormat:    "yuyv422",
			wantFilters:    "format=yuv422p,format=yuv420p,format=nv12,hwupload",
			wantGlobalArgs: []string{"-vaapi_device", "/dev/dri/renderD128"},
			wantErr:        false,
		},
		{
			name:           "Empty input format for validation",
			encoderName:    "h264_vaapi",
			inputFormat:    "",
			wantFilters:    "format=nv12,hwupload",
			wantGlobalArgs: []string{"-vaapi_device", "/dev/dri/renderD128"},
			wantErr:        false,
		},
		{
			name:           "Invalid encoder",
			encoderName:    "h264_nvenc",
			inputFormat:    "",
			wantFilters:    "",
			wantGlobalArgs: nil,
			wantErr:        true,
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

				if len(settings.GlobalArgs) != len(tt.wantGlobalArgs) {
					t.Errorf("GetProductionSettings() GlobalArgs length = %v, want %v", len(settings.GlobalArgs), len(tt.wantGlobalArgs))
				}

				for i, arg := range tt.wantGlobalArgs {
					if i < len(settings.GlobalArgs) && settings.GlobalArgs[i] != arg {
						t.Errorf("GetProductionSettings() GlobalArgs[%d] = %v, want %v", i, settings.GlobalArgs[i], arg)
					}
				}
			}
		})
	}
}

func floatPtr(f float64) *float64 {
	return &f
}

func intPtr(i int) *int {
	return &i
}
