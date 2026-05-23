package ffmpeg

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Global args (with values) that must be stripped for SW-only filter chains (e.g. perspective).
var hwAccelFlags = map[string]struct{}{
	"-hwaccel":               {},
	"-hwaccel_output_format": {},
}

func stripHWAccelFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if _, drop := hwAccelFlags[args[i]]; drop {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// commandHead returns the ffmpeg base prefix. Strips `-nostdin` when the
// caller is feeding ffmpeg via a stdin pipe (InputPipe), otherwise uses
// Base() as-is so v4l2 / lavfi callers keep their current head.
func commandHead(p *Params) string {
	if p.InputPipe != nil {
		return strings.Replace(Base(), " -nostdin", "", 1)
	}
	return Base()
}

// BuildCommand builds an FFmpeg command from structured parameters.
func BuildCommand(p *Params) string {
	var cmd strings.Builder

	cmd.WriteString(commandHead(p))

	// Perspective is SW-only; strip -hwaccel so decode runs in CPU. -vaapi_device/init_hw_device stay for hwupload.
	args := p.GlobalArgs
	if p.Perspective != nil {
		args = stripHWAccelFlags(args)
	}
	for _, arg := range args {
		cmd.WriteString(" " + arg)
	}

	switch {
	case p.OverlayText != "":
		cmd.WriteString(" -re")
		cmd.WriteString(" -f lavfi")

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
	case p.InputPipe != nil:
		writePipeInput(&cmd, p.InputPipe)
	default:
		cmd.WriteString(" -f v4l2")

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

	// Audio inputs + maps. Two modes:
	//   - AudioInputs (multi): one ALSA -i per entry, filter_complex with
	//     aresample per input, one output track per input
	//   - AudioDevice (single, back-compat): existing behavior verbatim
	switch {
	case len(p.AudioInputs) > 0:
		writeMultiAudioInputs(&cmd, p.AudioInputs)
	case p.AudioDevice != "":
		if p.OverlayText != "" {
			cmd.WriteString(" -f lavfi -i \"sine=frequency=1000:sample_rate=48000\"")
		} else {
			cmd.WriteString(" -thread_queue_size 1024")

			if slices.Contains(p.Options, OptionWallclockWithGenpts) {
				cmd.WriteString(" -use_wallclock_as_timestamps 1 -fflags +genpts+igndts")
			}

			cmd.WriteString(" -f alsa -sample_fmt s16 -ar 48000 -ac 2")
			cmd.WriteString(" -i " + p.AudioDevice)
		}

		if p.VisionEnabled {
			cmd.WriteString(" -map 1:a")
		} else {
			cmd.WriteString(" -map 0:v -map 1:a")
		}
	}

	if slices.Contains(p.Options, OptionVsyncPassthrough) {
		cmd.WriteString(" -fps_mode passthrough")
	}

	if p.AudioFilters != "" {
		cmd.WriteString(" -af " + p.AudioFilters)
	}

	// Multi-audio uses filter_complex (aresample per input + per-track map).
	// Must come after global timing flags but before -c:v so ffmpeg sees
	// the stream selection before the encoder picks them up.
	if len(p.AudioInputs) > 0 {
		writeMultiAudioFilterComplex(&cmd, len(p.AudioInputs))
	}

	var videoFilterChain []string

	if p.OverlayText != "" {
		drawtext := fmt.Sprintf("drawtext=text='%s':x=(w-text_w)/2:y=(h-text_h)/2:fontsize=120:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=5", p.OverlayText)
		videoFilterChain = append(videoFilterChain, drawtext)
	}

	hasPerspective := p.Perspective != nil
	hasRotation := p.Rotation != 0
	isHWFilterChain := strings.Contains(p.VideoFilters, "scale_vaapi") ||
		strings.Contains(p.VideoFilters, "scale_rkrga")

	if hasPerspective {
		videoFilterChain = append(videoFilterChain, "format=yuv420p")
		videoFilterChain = append(videoFilterChain, perspectiveFilterString(p.Perspective))

		natW, natH := perspectiveOutputSize(p.Perspective)
		if natW > 0 && natH > 0 && p.Resolution != "" {
			if parts := strings.SplitN(p.Resolution, "x", 2); len(parts) == 2 {
				videoFilterChain = append(videoFilterChain, fmt.Sprintf("scale=%d:%d", natW, natH))
				videoFilterChain = append(videoFilterChain,
					fmt.Sprintf("scale=%s:%s:force_original_aspect_ratio=decrease", parts[0], parts[1]))
				videoFilterChain = append(videoFilterChain,
					fmt.Sprintf("pad=%s:%s:-1:-1:color=green", parts[0], parts[1]))
			}
		}
	}

	if hasRotation {
		inputIsHW := !hasPerspective && hasHWOutputFormat(p.GlobalArgs)
		backend := backendForEncoder(p.Encoder)
		if inputIsHW && p.HWCaps.HasTranspose(backend) {
			if hwRot := hwTransposeFilter(p.Encoder, p.Rotation); hwRot != "" {
				videoFilterChain = append(videoFilterChain, hwRot)
			}
		} else {
			if inputIsHW {
				videoFilterChain = append(videoFilterChain, "hwdownload", "format=nv12")
			} else if !hasPerspective {
				videoFilterChain = append(videoFilterChain, "format=yuv420p")
			}
			videoFilterChain = append(videoFilterChain, swTransposeFilter(p.Rotation))
			if inputIsHW {
				videoFilterChain = append(videoFilterChain, "format=nv12", "hwupload")
			}
		}
	}

	switch {
	case hasPerspective && isHWFilterChain:
		// HW scaler in VideoFilters can't run on CPU frames after perspective; replace with hwupload shim.
		videoFilterChain = append(videoFilterChain, "format=nv12,hwupload")
	case p.VideoFilters != "":
		videoFilterChain = append(videoFilterChain, p.VideoFilters)
	}

	if p.VisionEnabled {
		vw, vh := p.VisionWidth, p.VisionHeight
		if vw <= 0 {
			vw = 640
		}
		if vh <= 0 {
			vh = 480
		}
		encFilters := strings.Join(videoFilterChain, ",")

		var fc strings.Builder
		fc.WriteString("[0:v]split=2[enc][raw]")
		if encFilters != "" {
			fc.WriteString(fmt.Sprintf("; [enc]%s[encout]", encFilters))
		} else {
			fc.WriteString("; [enc]null[encout]")
		}
		fc.WriteString(fmt.Sprintf("; [raw]scale=%d:%d,format=nv12[rawout]", vw, vh))
		cmd.WriteString(fmt.Sprintf(" -filter_complex \"%s\"", fc.String()))
		cmd.WriteString(" -map \"[encout]\"")
	} else if len(videoFilterChain) > 0 {
		cmd.WriteString(" -vf " + strings.Join(videoFilterChain, ","))
	}

	cmd.WriteString(" -c:v " + p.Encoder)

	if strings.Contains(p.Encoder, "h264") {
		cmd.WriteString(" -profile:v high -level:v 5.2")
	}

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

	if p.Preset != "" {
		cmd.WriteString(" -preset " + p.Preset)
	}
	if p.GOP > 0 {
		cmd.WriteString(fmt.Sprintf(" -g %d", p.GOP))
	} else {
		cmd.WriteString(" -g 60")
	}
	if p.BFrames >= 0 {
		cmd.WriteString(fmt.Sprintf(" -bf %d", p.BFrames))
	}

	if !isHardwareEncoder(p.Encoder) {
		cmd.WriteString(" -tune zerolatency")
		cmd.WriteString(" -keyint_min 15")
		cmd.WriteString(" -sc_threshold 0")
	}

	// Repeat VPS/SPS/PPS at every keyframe so late MPEG-TS/SRT joiners can decode immediately.
	if isHevcOrH264(p.Encoder) {
		cmd.WriteString(" -bsf:v dump_extra=freq=keyframe")
	}

	if p.AudioDevice != "" || len(p.AudioInputs) > 0 {
		cmd.WriteString(" -c:a libopus -b:a 128k -ar 48000")
	}

	if p.ProgressSocket != "" {
		cmd.WriteString(" -progress unix://" + p.ProgressSocket)
	}

	// Outputs: multi-target mode (Outputs slice) supersedes single
	// OutputURL. Both modes pick mux flags by transport type.
	switch {
	case len(p.Outputs) > 0:
		writeOutputs(&cmd, p.Outputs)
	case strings.HasPrefix(p.OutputURL, "rtsp://"):
		cmd.WriteString(" -rtsp_transport tcp -f rtsp " + p.OutputURL)
	default:
		cmd.WriteString(" -muxdelay 0 -muxpreload 0 -flush_packets 1 -f mpegts " + p.OutputURL)
	}

	if p.VisionEnabled {
		cmd.WriteString(" -map \"[rawout]\" -f rawvideo -pix_fmt nv12 pipe:3")
	}

	return cmd.String()
}

// writePipeInput emits the `-f <muxer> [-pix_fmt -s -framerate] -i pipe:0`
// fragment for the InputPipe case. Y4M is self-describing; rawvideo
// needs explicit pix_fmt + dims + framerate.
func writePipeInput(cmd *strings.Builder, pi *PipeInput) {
	switch pi.Format {
	case "rawvideo":
		cmd.WriteString(" -f rawvideo")
		if pi.PixelFormat != "" {
			cmd.WriteString(" -pix_fmt " + pi.PixelFormat)
		}
		if pi.Width > 0 && pi.Height > 0 {
			fmt.Fprintf(cmd, " -s %dx%d", pi.Width, pi.Height)
		}
		if pi.FPS > 0 {
			fmt.Fprintf(cmd, " -framerate %d", pi.FPS)
		}
		cmd.WriteString(" -i pipe:0")
	case "yuv4mpegpipe", "":
		cmd.WriteString(" -f yuv4mpegpipe -i pipe:0")
	default:
		cmd.WriteString(" -f " + pi.Format + " -i pipe:0")
	}
}

// writeMultiAudioInputs emits one `-thread_queue_size ... -f alsa ... -i <dev>`
// fragment per device. Maps are emitted later via writeMultiAudioFilterComplex.
func writeMultiAudioInputs(cmd *strings.Builder, devices []string) {
	for _, dev := range devices {
		cmd.WriteString(" -thread_queue_size 1024")
		cmd.WriteString(" -f alsa -sample_fmt s16 -ar 48000 -ac 2")
		cmd.WriteString(" -i " + dev)
	}
}

// writeMultiAudioFilterComplex emits the filter_complex aresample chain
// plus the per-output map flags for N audio inputs. Inputs are at ffmpeg
// indices [1..N] (video is input 0).
func writeMultiAudioFilterComplex(cmd *strings.Builder, audioCount int) {
	var fc strings.Builder
	for k := range audioCount {
		if k > 0 {
			fc.WriteString(";")
		}
		fmt.Fprintf(&fc, "[%d:a]aresample=async=1:min_hard_comp=0.100000:first_pts=0[a%d]", k+1, k)
	}
	cmd.WriteString(" -filter_complex " + fc.String())
	cmd.WriteString(" -map 0:v")
	for k := range audioCount {
		fmt.Fprintf(cmd, " -map [a%d]", k)
	}
}

// writeOutputs emits one output fragment per OutputTarget. Per-format
// tail flags (rtsp_transport, mpegts mux delays) are applied per output.
func writeOutputs(cmd *strings.Builder, outs []OutputTarget) {
	rtspEmitted := false
	for _, o := range outs {
		switch o.Type {
		case "rtsp":
			if !rtspEmitted {
				cmd.WriteString(" -rtsp_transport tcp")
				rtspEmitted = true
			}
			cmd.WriteString(" -f rtsp " + o.URL)
		case "srt", "mpegts":
			cmd.WriteString(" -muxdelay 0 -muxpreload 0 -flush_packets 1 -f mpegts " + o.URL)
		case "hls":
			cmd.WriteString(" -f hls " + o.URL)
		default:
			cmd.WriteString(" -f " + o.Type + " " + o.URL)
		}
	}
}

// BuildInputArgs returns just the ffmpeg head (base flags + global args +
// input + audio inputs) as a single shell-ready string. The pipeline's
// CustomEncoderArgs path uses this to compose `head + " " + custom_tail`.
func BuildInputArgs(p *Params) string {
	var cmd strings.Builder
	cmd.WriteString(commandHead(p))

	args := p.GlobalArgs
	if p.Perspective != nil {
		args = stripHWAccelFlags(args)
	}
	for _, arg := range args {
		cmd.WriteString(" " + arg)
	}

	switch {
	case p.OverlayText != "":
		cmd.WriteString(" -re -f lavfi")
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
	case p.InputPipe != nil:
		writePipeInput(&cmd, p.InputPipe)
	default:
		cmd.WriteString(" -f v4l2")
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

	switch {
	case len(p.AudioInputs) > 0:
		writeMultiAudioInputs(&cmd, p.AudioInputs)
	case p.AudioDevice != "":
		cmd.WriteString(" -thread_queue_size 1024 -f alsa -sample_fmt s16 -ar 48000 -ac 2 -i " + p.AudioDevice)
	}

	return cmd.String()
}

// perspectiveFilterString generates the FFmpeg perspective filter from corner points.
func perspectiveFilterString(p *PerspectiveConfig) string {
	c := p.Corners
	// Corners stored clockwise [TL,TR,BR,BL]; FFmpeg perspective expects [TL,TR,BL,BR].
	return fmt.Sprintf(
		"perspective=x0=%d:y0=%d:x1=%d:y1=%d:x2=%d:y2=%d:x3=%d:y3=%d:sense=source:interpolation=linear",
		c[0][0], c[0][1], c[1][0], c[1][1], c[3][0], c[3][1], c[2][0], c[2][1],
	)
}

// perspectiveOutputSize returns the output W,H preserving corrected content aspect ratio.
func perspectiveOutputSize(p *PerspectiveConfig) (int, int) {
	c := p.Corners
	dist := func(a, b [2]int) float64 {
		dx, dy := float64(a[0]-b[0]), float64(a[1]-b[1])
		return math.Sqrt(dx*dx + dy*dy)
	}
	w := int(math.Round(math.Max(dist(c[0], c[1]), dist(c[3], c[2]))))
	h := int(math.Round(math.Max(dist(c[0], c[3]), dist(c[1], c[2]))))
	return w, h
}
