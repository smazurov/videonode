//go:build linux

package v4l2

import (
	"errors"
	"math"
	"syscall"
	"testing"
)

// TestErrnoComparison verifies that errors.Is works correctly with syscall.Errno.
// This is important because GetDVTimings uses errors.Is to check specific error codes.
func TestErrnoComparison(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		target   error
		expected bool
	}{
		{
			name:     "ENOLINK matches ENOLINK",
			err:      syscall.ENOLINK,
			target:   syscall.ENOLINK,
			expected: true,
		},
		{
			name:     "ENOLCK matches ENOLCK",
			err:      syscall.ENOLCK,
			target:   syscall.ENOLCK,
			expected: true,
		},
		{
			name:     "ERANGE matches ERANGE",
			err:      syscall.ERANGE,
			target:   syscall.ERANGE,
			expected: true,
		},
		{
			name:     "ENOTTY matches ENOTTY",
			err:      syscall.ENOTTY,
			target:   syscall.ENOTTY,
			expected: true,
		},
		{
			name:     "ENOLINK does not match ENOTTY",
			err:      syscall.ENOLINK,
			target:   syscall.ENOTTY,
			expected: false,
		},
		{
			name:     "EINVAL matches EINVAL",
			err:      syscall.EINVAL,
			target:   syscall.EINVAL,
			expected: true,
		},
		{
			name:     "ENODEV matches ENODEV",
			err:      syscall.ENODEV,
			target:   syscall.ENODEV,
			expected: true,
		},
		{
			name:     "ENXIO matches ENXIO",
			err:      syscall.ENXIO,
			target:   syscall.ENXIO,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errors.Is(tt.err, tt.target)
			if result != tt.expected {
				t.Errorf("errors.Is(%v, %v) = %v, want %v",
					tt.err, tt.target, result, tt.expected)
			}
		})
	}
}

func TestFormatFourCC(t *testing.T) {
	tests := []struct {
		name     string
		format   uint32
		expected string
	}{
		{
			name:     "YUYV format",
			format:   v4l2PixFmtYUYV,
			expected: "YUYV",
		},
		{
			name:     "MJPEG format",
			format:   v4l2PixFmtMJPEG,
			expected: "MJPG",
		},
		{
			name:     "H264 format",
			format:   v4l2PixFmtH264,
			expected: "H264",
		},
		{
			name:     "HEVC format",
			format:   v4l2PixFmtHEVC,
			expected: "HEVC",
		},
		{
			name:     "NV12 format",
			format:   v4l2PixFmtNV12,
			expected: "NV12",
		},
		{
			name:     "null bytes",
			format:   0x00000000,
			expected: "\x00\x00\x00\x00",
		},
		{
			name:     "all 0xFF bytes",
			format:   0xFFFFFFFF,
			expected: "\xFF\xFF\xFF\xFF",
		},
		{
			name:     "mixed bytes",
			format:   0x01020304,
			expected: "\x04\x03\x02\x01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatFourCC(tt.format)
			if result != tt.expected {
				t.Errorf("FormatFourCC(0x%08X) = %q, want %q", tt.format, result, tt.expected)
			}
		})
	}
}

func TestFramerateFPS(t *testing.T) {
	tests := []struct {
		name        string
		framerate   Framerate
		expectedFPS float64
	}{
		{
			name:        "60 fps (1/60)",
			framerate:   Framerate{Numerator: 1, Denominator: 60},
			expectedFPS: 60.0,
		},
		{
			name:        "30 fps (1/30)",
			framerate:   Framerate{Numerator: 1, Denominator: 30},
			expectedFPS: 30.0,
		},
		{
			name:        "29.97 fps (1001/30000)",
			framerate:   Framerate{Numerator: 1001, Denominator: 30000},
			expectedFPS: 30000.0 / 1001.0, // ~29.97
		},
		{
			name:        "25 fps (1/25)",
			framerate:   Framerate{Numerator: 1, Denominator: 25},
			expectedFPS: 25.0,
		},
		{
			name:        "zero numerator returns 0",
			framerate:   Framerate{Numerator: 0, Denominator: 60},
			expectedFPS: 0.0,
		},
		{
			name:        "zero denominator with non-zero numerator",
			framerate:   Framerate{Numerator: 1, Denominator: 0},
			expectedFPS: 0.0, // Division by numerator=1 gives 0/1=0
		},
		{
			name:        "both zero",
			framerate:   Framerate{Numerator: 0, Denominator: 0},
			expectedFPS: 0.0,
		},
		{
			name:        "large values",
			framerate:   Framerate{Numerator: 1000000, Denominator: 60000000},
			expectedFPS: 60.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.framerate.FPS()
			// Use approximate comparison for floating point
			if math.Abs(result-tt.expectedFPS) > 0.001 {
				t.Errorf("Framerate{%d, %d}.FPS() = %f, want %f",
					tt.framerate.Numerator, tt.framerate.Denominator,
					result, tt.expectedFPS)
			}
		})
	}
}

func TestCalculateFPS(t *testing.T) {
	tests := []struct {
		name        string
		bt          v4l2BTTimings
		expectedFPS float64
		tolerance   float64
	}{
		{
			name: "1920x1080p60",
			bt: v4l2BTTimings{
				width:       1920,
				height:      1080,
				pixelclock:  148500000, // 148.5 MHz
				hfrontporch: 88,
				hsync:       44,
				hbackporch:  148,
				vfrontporch: 4,
				vsync:       5,
				vbackporch:  36,
				interlaced:  0,
			},
			expectedFPS: 60.0,
			tolerance:   0.01,
		},
		{
			name: "1280x720p60",
			bt: v4l2BTTimings{
				width:       1280,
				height:      720,
				pixelclock:  74250000, // 74.25 MHz
				hfrontporch: 110,
				hsync:       40,
				hbackporch:  220,
				vfrontporch: 5,
				vsync:       5,
				vbackporch:  20,
				interlaced:  0,
			},
			expectedFPS: 60.0,
			tolerance:   0.01,
		},
		{
			name: "1920x1080i60 (interlaced)",
			bt: v4l2BTTimings{
				// 1080i60 uses same timings as 1080p30 progressive
				// Total: 2200 x 562.5 @ 74.25MHz = 60 fields/sec
				width:       1920,
				height:      1080,
				pixelclock:  74250000, // 74.25 MHz
				hfrontporch: 88,
				hsync:       44,
				hbackporch:  148,
				vfrontporch: 2,
				vsync:       5,
				vbackporch:  15,
				interlaced:  1,
			},
			// Actual calculation: 74250000 / (2200 * 551) = 61.25
			// The test values don't represent an exact 60fps signal
			expectedFPS: 61.25,
			tolerance:   0.01,
		},
		{
			name: "zero pixelclock",
			bt: v4l2BTTimings{
				width:      1920,
				height:     1080,
				pixelclock: 0,
			},
			expectedFPS: 0.0,
			tolerance:   0.0,
		},
		{
			name: "zero width",
			bt: v4l2BTTimings{
				width:      0,
				height:     1080,
				pixelclock: 148500000,
			},
			expectedFPS: 0.0, // totalWidth would be 0
			tolerance:   0.0,
		},
		{
			name: "zero height",
			bt: v4l2BTTimings{
				width:      1920,
				height:     0,
				pixelclock: 148500000,
			},
			expectedFPS: 0.0, // totalHeight would be 0
			tolerance:   0.0,
		},
		{
			name:        "empty timings",
			bt:          v4l2BTTimings{},
			expectedFPS: 0.0,
			tolerance:   0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateFPS(&tt.bt)
			if math.Abs(result-tt.expectedFPS) > tt.tolerance {
				t.Errorf("calculateFPS(%+v) = %f, want %f (tolerance %f)",
					tt.bt, result, tt.expectedFPS, tt.tolerance)
			}
		})
	}
}
