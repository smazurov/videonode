package process

import "testing"

func TestParseFFmpegLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLevel string
		wantMsg   string
	}{
		{
			name:      "simple info",
			input:     "[info] Stream mapping:",
			wantLevel: "info",
			wantMsg:   "Stream mapping:",
		},
		{
			name:      "simple warning",
			input:     "[warning] deprecated option",
			wantLevel: "warning",
			wantMsg:   "deprecated option",
		},
		{
			name:      "simple error",
			input:     "[error] failed to open file",
			wantLevel: "error",
			wantMsg:   "failed to open file",
		},
		{
			name:      "component prefix with warning",
			input:     "[swscaler @ 0x7f673c439fc0] [warning] deprecated pixel format used, make sure you did set range correctly",
			wantLevel: "warning",
			wantMsg:   "[swscaler @ 0x7f673c439fc0] deprecated pixel format used, make sure you did set range correctly",
		},
		{
			name:      "component prefix with info",
			input:     "[libx264 @ 0x55f4a8c00000] [info] using cpu capabilities: MMX2 SSE2Fast",
			wantLevel: "info",
			wantMsg:   "[libx264 @ 0x55f4a8c00000] using cpu capabilities: MMX2 SSE2Fast",
		},
		{
			name:      "component prefix without level",
			input:     "[libx264 @ 0x55f4a8c00000] frame=100 fps=30",
			wantLevel: "info",
			wantMsg:   "[libx264 @ 0x55f4a8c00000] frame=100 fps=30",
		},
		{
			name:      "no prefix",
			input:     "frame=100 fps=30 q=28.0 size=1024kB",
			wantLevel: "info",
			wantMsg:   "frame=100 fps=30 q=28.0 size=1024kB",
		},
		{
			name:      "empty line",
			input:     "",
			wantLevel: "info",
			wantMsg:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLevel, gotMsg := parseFFmpegLogLevel(tt.input)
			if gotLevel != tt.wantLevel {
				t.Errorf("parseFFmpegLogLevel() level = %q, want %q", gotLevel, tt.wantLevel)
			}
			if gotMsg != tt.wantMsg {
				t.Errorf("parseFFmpegLogLevel() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}
