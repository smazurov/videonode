package validation

import (
	"testing"

	"github.com/smazurov/videonode/internal/types"
)

func TestRkmppValidator_GetQualityParams(t *testing.T) {
	v := NewRkmppValidator()

	tests := []struct {
		name        string
		encoderName string
		params      *types.QualityParams
		wantParams  map[string]string
		wantErr     bool
	}{
		{
			name:        "CBR mode",
			encoderName: "h264_rkmpp",
			params: &types.QualityParams{
				Mode:             types.RateControlCBR,
				TargetBitrate:    floatPtr(4.0),
				KeyframeInterval: intPtr(100),
			},
			wantParams: map[string]string{
				"rc_mode": "CBR",
				"b:v":     "4.0M",
				"g":       "100",
			},
			wantErr: false,
		},
		{
			name:        "VBR mode with min/max bitrate",
			encoderName: "hevc_rkmpp",
			params: &types.QualityParams{
				Mode:             types.RateControlVBR,
				TargetBitrate:    floatPtr(5.0),
				MinBitrate:       floatPtr(2.0),
				MaxBitrate:       floatPtr(8.0),
				KeyframeInterval: intPtr(60),
			},
			wantParams: map[string]string{
				"rc_mode": "VBR",
				"b:v":     "5.0M",
				"minrate": "2.0M",
				"maxrate": "8.0M",
				"g":       "60",
			},
			wantErr: false,
		},
		{
			name:        "CQP mode with quality",
			encoderName: "h264_rkmpp",
			params: &types.QualityParams{
				Mode:    types.RateControlCQP,
				Quality: intPtr(26),
			},
			wantParams: map[string]string{
				"rc_mode": "CQP",
				"qp_init": "26",
			},
			wantErr: false,
		},
		{
			name:        "CRF mode mapped to CQP",
			encoderName: "hevc_rkmpp",
			params: &types.QualityParams{
				Mode:    types.RateControlCRF,
				Quality: intPtr(23),
			},
			wantParams: map[string]string{
				"rc_mode": "CQP",
				"qp_init": "23",
			},
			wantErr: false,
		},
		{
			name:        "Invalid encoder name",
			encoderName: "h264_vaapi",
			params: &types.QualityParams{
				Mode: types.RateControlCBR,
			},
			wantParams: nil,
			wantErr:    true,
		},
		{
			name:        "Unsupported rate control mode",
			encoderName: "h264_rkmpp",
			params: &types.QualityParams{
				Mode: "invalid_mode",
			},
			wantParams: nil,
			wantErr:    true,
		},
		{
			name:        "VBR without target bitrate",
			encoderName: "h264_rkmpp",
			params: &types.QualityParams{
				Mode:       types.RateControlVBR,
				MinBitrate: floatPtr(2.0),
				MaxBitrate: floatPtr(8.0),
			},
			wantParams: map[string]string{
				"rc_mode": "VBR",
				"minrate": "2.0M",
				"maxrate": "8.0M",
			},
			wantErr: false,
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

func TestRkmppValidator_GetProductionSettings(t *testing.T) {
	v := NewRkmppValidator()

	tests := []struct {
		name           string
		encoderName    string
		inputFormat    string
		wantFilters    string
		wantGlobalArgs []string
		wantRcMode     string
		wantErr        bool
	}{
		{
			name:           "MJPEG input with hardware decode",
			encoderName:    "h264_rkmpp",
			inputFormat:    "mjpeg",
			wantFilters:    "",
			wantGlobalArgs: []string{"-hwaccel", "rkmpp", "-hwaccel_output_format", "drm_prime"},
			wantRcMode:     "VBR",
			wantErr:        false,
		},
		{
			name:           "H264 input with hardware decode",
			encoderName:    "hevc_rkmpp",
			inputFormat:    "h264",
			wantFilters:    "",
			wantGlobalArgs: []string{"-hwaccel", "rkmpp", "-hwaccel_output_format", "drm_prime"},
			wantRcMode:     "VBR",
			wantErr:        false,
		},
		{
			name:           "YUYV422 with RGA scaler",
			encoderName:    "h264_rkmpp",
			inputFormat:    "yuyv422",
			wantFilters:    "hwupload,scale_rkrga=format=nv12:afbc=0",
			wantGlobalArgs: []string{"-init_hw_device", "rkmpp=hw", "-filter_hw_device", "hw"},
			wantRcMode:     "VBR",
			wantErr:        false,
		},
		{
			name:           "Empty input format for validation",
			encoderName:    "h264_rkmpp",
			inputFormat:    "",
			wantFilters:    "",
			wantGlobalArgs: []string{},
			wantRcMode:     "VBR",
			wantErr:        false,
		},
		{
			name:           "Unknown format with fallback",
			encoderName:    "h264_rkmpp",
			inputFormat:    "rgb24",
			wantFilters:    "format=nv12",
			wantGlobalArgs: []string{},
			wantRcMode:     "VBR",
			wantErr:        false,
		},
		{
			name:           "MJPEG encoder",
			encoderName:    "mjpeg_rkmpp",
			inputFormat:    "",
			wantFilters:    "format=nv12",
			wantGlobalArgs: []string{},
			wantRcMode:     "",
			wantErr:        false,
		},
		{
			name:           "Invalid encoder",
			encoderName:    "h264_vaapi",
			inputFormat:    "",
			wantFilters:    "",
			wantGlobalArgs: nil,
			wantRcMode:     "",
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

				if tt.wantRcMode != "" {
					if rcMode, ok := settings.OutputParams["rc_mode"]; !ok || rcMode != tt.wantRcMode {
						t.Errorf("GetProductionSettings() rc_mode = %v, want %v", rcMode, tt.wantRcMode)
					}
				}
			}
		})
	}
}
