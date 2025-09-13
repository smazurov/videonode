package ffmpeg

import (
	"fmt"
	"strings"
)

// BuildCommand builds an FFmpeg command from structured parameters
func BuildCommand(p *Params) string {
	var cmd strings.Builder

	cmd.WriteString(FFmpegBase())

	// Global args (hardware devices, etc.)
	for _, arg := range p.GlobalArgs {
		cmd.WriteString(" " + arg)
	}

	// Input configuration
	cmd.WriteString(" -f v4l2")

	// Apply FFmpeg options (before input)
	appliedOptions := ApplyOptionsToCommand(p.Options, &cmd)
	if len(appliedOptions) > 0 {
		// Options are already applied by ApplyOptionsToCommand
	}

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

	// Audio input if specified
	if p.AudioDevice != "" {
		cmd.WriteString(" -thread_queue_size 10240")
		cmd.WriteString(" -f alsa -sample_fmt s16 -ar 48000 -ac 2")
		cmd.WriteString(" -i " + p.AudioDevice)
		cmd.WriteString(" -map 0:v -map 1:a")
	}

	// Video filters
	if p.VideoFilters != "" {
		cmd.WriteString(" -vf " + p.VideoFilters)
	}

	// Encoder
	cmd.WriteString(" -c:v " + p.Encoder)

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
	if p.GOP > 0 {
		cmd.WriteString(fmt.Sprintf(" -g %d", p.GOP))
	}
	if p.BFrames >= 0 {
		cmd.WriteString(fmt.Sprintf(" -bf %d", p.BFrames))
	}

	// Low latency settings for software encoders
	if !isHardwareEncoder(p.Encoder) {
		cmd.WriteString(" -tune zerolatency")
		if p.GOP == 0 {
			// Set default GOP if not specified
			cmd.WriteString(" -g 20")
		}
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

	// Output
	cmd.WriteString(" -rtsp_transport tcp -f rtsp " + p.OutputURL)

	return cmd.String()
}
