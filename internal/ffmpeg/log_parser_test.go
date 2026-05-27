package ffmpeg

import (
	"log/slog"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
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
		{
			name:      "absl unknown flag error",
			input:     "ERROR: Unknown command line flag 'test_pattern'",
			wantLevel: "error",
			wantMsg:   "Unknown command line flag 'test_pattern'",
		},
		{
			name:      "glog fatal",
			input:     "FATAL: assertion failed",
			wantLevel: "fatal",
			wantMsg:   "assertion failed",
		},
		{
			name:      "glog warning",
			input:     "WARNING: deprecated",
			wantLevel: "warning",
			wantMsg:   "deprecated",
		},
		{
			name:      "colon in non-level word stays info",
			input:     "Stream mapping: 0 -> 1",
			wantLevel: "info",
			wantMsg:   "Stream mapping: 0 -> 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLevel, gotMsg := ParseLogLevel(tt.input)
			if gotLevel != tt.wantLevel {
				t.Errorf("ParseLogLevel() level = %q, want %q", gotLevel, tt.wantLevel)
			}
			if gotMsg != tt.wantMsg {
				t.Errorf("ParseLogLevel() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestParseLogLine(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLevel string
		wantMsg   string
		wantAttrs []slog.Attr
	}{
		{
			name:      "structured info with tab",
			input:     "[info] consumer connected\tfd=17 total=1",
			wantLevel: "info",
			wantMsg:   "consumer connected",
			wantAttrs: []slog.Attr{slog.String("fd", "17"), slog.String("total", "1")},
		},
		{
			name:      "structured capture ready",
			input:     "[info] capture ready\tw=1920 h=1080 fourcc=MJPG buffers=4",
			wantLevel: "info",
			wantMsg:   "capture ready",
			wantAttrs: []slog.Attr{
				slog.String("w", "1920"), slog.String("h", "1080"),
				slog.String("fourcc", "MJPG"), slog.String("buffers", "4"),
			},
		},
		{
			name:      "no tab falls back to plain",
			input:     "[info] state -> LIVE",
			wantLevel: "info",
			wantMsg:   "state -> LIVE",
			wantAttrs: nil,
		},
		{
			name:      "debug with tab",
			input:     "[debug] EGL initialized\tgpu=/dev/dri/renderD128 glsl=320",
			wantLevel: "debug",
			wantMsg:   "EGL initialized",
			wantAttrs: []slog.Attr{slog.String("gpu", "/dev/dri/renderD128"), slog.String("glsl", "320")},
		},
		{
			name:      "plain text no bracket",
			input:     "frame=100 fps=30",
			wantLevel: "info",
			wantMsg:   "frame=100 fps=30",
			wantAttrs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLevel, gotMsg, gotAttrs := ParseLogLine(tt.input)
			if gotLevel != tt.wantLevel {
				t.Errorf("level = %q, want %q", gotLevel, tt.wantLevel)
			}
			if gotMsg != tt.wantMsg {
				t.Errorf("msg = %q, want %q", gotMsg, tt.wantMsg)
			}
			if len(gotAttrs) != len(tt.wantAttrs) {
				t.Fatalf("attrs len = %d, want %d; got %v", len(gotAttrs), len(tt.wantAttrs), gotAttrs)
			}
			for i, want := range tt.wantAttrs {
				got := gotAttrs[i]
				if got.Key != want.Key || got.Value.String() != want.Value.String() {
					t.Errorf("attr[%d] = %s=%s, want %s=%s", i, got.Key, got.Value, want.Key, want.Value)
				}
			}
		})
	}
}

func TestParseLogLevelAllLevels(t *testing.T) {
	levels := []string{"quiet", "panic", "fatal", "error", "warning", "info", "verbose", "debug", "trace"}
	for _, level := range levels {
		input := "[" + level + "] test message"
		gotLevel, gotMsg := ParseLogLevel(input)
		if gotLevel != level {
			t.Errorf("ParseLogLevel(%q) level = %q, want %q", input, gotLevel, level)
		}
		if gotMsg != "test message" {
			t.Errorf("ParseLogLevel(%q) msg = %q, want 'test message'", input, gotMsg)
		}
	}
}
