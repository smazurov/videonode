package ffmpeg

import (
	"fmt"
	"slices"
	"strings"
)

// BuildCommand builds an FFmpeg command from structured parameters.
func BuildCommand(p *Params) string {
	var cmd strings.Builder

	cmd.WriteString(Base())

	// Global args (hardware devices, etc.)
	for _, arg := range p.GlobalArgs {
		cmd.WriteString(" " + arg)
	}

	// Input configuration
	if p.OverlayText != "" {
		// Generate test pattern input
		// Add -re flag to read at native frame rate (prevents running too fast)
		cmd.WriteString(" -re")
		cmd.WriteString(" -f lavfi")

		// Build test source string with resolution and framerate
		testSrc := "testsrc2"
		if p.Resolution != "" {
			testSrc += "=size=" + p.Resolution
		} else {
			testSrc += "=size=1920x1080"
		}
		if p.FPS != "" {
			testSrc += ":rate=" + p.FPS
		} else {
			testSrc += ":rate=30"
		}
		cmd.WriteString(" -i \"" + testSrc + "\"")
	} else {
		// Normal V4L2 device input
		cmd.WriteString(" -f v4l2")

		// Apply FFmpeg options (before input)
		ApplyOptionsToCommand(p.Options, &cmd)

		if p.InputFormat != "" {
			cmd.WriteString(" -input_format " + p.InputFormat)
		}
		if p.Resolution != "" {
			cmd.WriteString(" -video_size " + p.Resolution)
		}
		if p.FPS != "" {
			cmd.WriteString(" -framerate " + p.FPS)
		}
		cmd.WriteString(" -i " + p.DevicePath)
	}

	// Audio input if specified
	if p.AudioDevice != "" {
		if p.OverlayText != "" {
			// Generate test audio tone for test mode
			cmd.WriteString(" -f lavfi -i \"sine=frequency=1000:sample_rate=48000\"")
			cmd.WriteString(" -map 0:v -map 1:a")
		} else {
			// Normal ALSA audio input
			cmd.WriteString(" -thread_queue_size 1024")

			// Check if wallclock_with_genpts is in options for audio timing
			hasWallclock := slices.Contains(p.Options, OptionWallclockWithGenpts)

			if hasWallclock {
				cmd.WriteString(" -use_wallclock_as_timestamps 1 -fflags +genpts+igndts")
			}

			cmd.WriteString(" -f alsa -sample_fmt s16 -ar 48000 -ac 2")
			cmd.WriteString(" -i " + p.AudioDevice)
			cmd.WriteString(" -map 0:v -map 1:a")
		}
	}

	// Apply fps_mode passthrough if enabled (passes frames as-is without dropping/duplicating)
	if slices.Contains(p.Options, OptionVsyncPassthrough) {
		cmd.WriteString(" -fps_mode passthrough")
	}

	// Audio filters
	if p.AudioFilters != "" {
		cmd.WriteString(" -af " + p.AudioFilters)
	}

	// Video filters
	var videoFilterChain []string

	// Add text overlay for test sources
	if p.OverlayText != "" {
		drawtext := fmt.Sprintf("drawtext=text='%s':x=(w-text_w)/2:y=(h-text_h)/2:fontsize=120:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=5", p.OverlayText)
		videoFilterChain = append(videoFilterChain, drawtext)
	}

	// Add existing video filters
	if p.VideoFilters != "" {
		videoFilterChain = append(videoFilterChain, p.VideoFilters)
	}

	// Apply video filter chain
	if len(videoFilterChain) > 0 {
		cmd.WriteString(" -vf " + strings.Join(videoFilterChain, ","))
	}

	// Encoder
	cmd.WriteString(" -c:v " + p.Encoder)

	// H264 profile and level for WebRTC - High profile for better compression
	// Level 5.2 supports 4K@60fps up to ~25Mbps
	if strings.Contains(p.Encoder, "h264") {
		cmd.WriteString(" -profile:v high -level:v 5.2")
	}

	// Rate control - only add what's set
	if p.RCMode != "" && isHardwareEncoder(p.Encoder) {
		cmd.WriteString(" -rc_mode " + p.RCMode)
	}

	if p.Bitrate != "" {
		cmd.WriteString(" -b:v " + p.Bitrate)
	}
	if p.MinRate != "" {
		cmd.WriteString(" -minrate " + p.MinRate)
	}
	if p.MaxRate != "" {
		cmd.WriteString(" -maxrate " + p.MaxRate)
	}
	if p.BufferSize != "" {
		cmd.WriteString(" -bufsize " + p.BufferSize)
	}
	if p.CRF > 0 {
		cmd.WriteString(fmt.Sprintf(" -crf %d", p.CRF))
	}
	if p.QP > 0 {
		cmd.WriteString(fmt.Sprintf(" -qp %d", p.QP))
	}

	// Encoder options
	if p.Preset != "" {
		cmd.WriteString(" -preset " + p.Preset)
	}
	// GOP settings - default to 60 frames (~2s at 30fps) for WebRTC compatibility
	if p.GOP > 0 {
		cmd.WriteString(fmt.Sprintf(" -g %d", p.GOP))
	} else {
		// Default GOP for all encoders if not specified
		cmd.WriteString(" -g 60")
	}
	if p.BFrames >= 0 {
		cmd.WriteString(fmt.Sprintf(" -bf %d", p.BFrames))
	}

	// Low latency settings for software encoders
	if !isHardwareEncoder(p.Encoder) {
		cmd.WriteString(" -tune zerolatency")
		cmd.WriteString(" -keyint_min 15")
		cmd.WriteString(" -sc_threshold 0")
	}

	// Audio codec
	if p.AudioDevice != "" {
		cmd.WriteString(" -c:a libopus -b:a 128k -ar 48000")
	}

	// Progress monitoring
	if p.ProgressSocket != "" {
		cmd.WriteString(" -progress unix://" + p.ProgressSocket)
	}

	// Output configuration - detect format from URL
	if strings.HasPrefix(p.OutputURL, "rtsp://") {
		// RTSP output for streaming server
		cmd.WriteString(" -rtsp_transport tcp -f rtsp " + p.OutputURL)
	} else {
		// Default: mpegts with low-latency options (for SRT, etc.)
		cmd.WriteString(" -muxdelay 0 -muxpreload 0 -flush_packets 1 -f mpegts " + p.OutputURL)
	}

	return cmd.String()
}
