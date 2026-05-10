package ffmpeg

import (
	"fmt"
	"slices"
	"strings"
)

// EffectiveInputSize returns input dimensions after perspective, rotation, and crop.
func EffectiveInputSize(resolution string, rotation int, perspective *PerspectiveConfig, cropW, cropH int) (int, int) {
	w, h, _ := ParseResolution(resolution)
	if perspective != nil {
		if pw, ph := perspectiveOutputSize(perspective); pw > 0 && ph > 0 {
			w, h = pw, ph
		}
	}
	if rotation == 90 || rotation == 270 {
		w, h = h, w
	}
	if cropW > 0 && cropH > 0 {
		w, h = cropW, cropH
	}
	return w, h
}

// CompositeInput represents a single input in a composite layout.
type CompositeInput struct {
	DevicePath  string
	InputFormat string
	Resolution  string
	FPS         string

	// Content rect: HW scalers ignore force_original_aspect_ratio, so callers must size W/H to input AR.
	X      int
	Y      int
	Width  int
	Height int

	Rotation int // 0, 90, 180, 270
	CropW    int
	CropH    int
	CropX    int
	CropY    int

	OverlayText string // non-empty forces lavfi testsrc2

	VisionEnabled bool
	VisionWidth   int
	VisionHeight  int
	VisionFPS     int // 0 = no throttle

	Perspective *PerspectiveConfig
}

// CompositeParams holds everything needed to build a composite FFmpeg command.
type CompositeParams struct {
	Width    int
	Height   int
	FPS      string
	KeyColor string

	Inputs []CompositeInput

	AudioDevices []string // v1 uses at most one entry

	Encoder      string
	GlobalArgs   []string // e.g., -vaapi_device /dev/dri/renderD128
	VideoFilters string

	HWBackend string         // "rkmpp", "vaapi", "sw", or ""
	HWCaps    HWCapabilities // from validator probes

	Bitrate    string
	MinRate    string
	MaxRate    string
	BufferSize string
	CRF        int
	QP         int
	RCMode     string

	Preset  string
	GOP     int
	BFrames int

	ProgressSocket string
	OutputURL      string

	Options []OptionType
}

// BuildCompositeCommand generates the full FFmpeg command for a composite layout.
func BuildCompositeCommand(p *CompositeParams) string {
	backend := p.HWBackend
	if backend == "" {
		backend = "sw"
	}
	hwOverlay := useHWOverlay(backend, p.HWCaps)
	useBGRAOverlay := p.HWCaps.HasOverlayBGRA(backend)
	canvasMode := "sw"
	if hwOverlay {
		canvasMode = "hw"
	}

	var cmd strings.Builder
	cmd.WriteString(Base())

	for _, arg := range p.GlobalArgs {
		cmd.WriteString(" " + arg)
	}
	for _, arg := range extraGlobalHWArgs(backend, p.GlobalArgs) {
		cmd.WriteString(" " + arg)
	}

	modes := make([]inputDecodeMode, len(p.Inputs))
	for i, in := range p.Inputs {
		modes[i] = resolveDecodeMode(backend, in, p.HWCaps)
	}

	// V4L2 inputs share a wallclock baseline so overlay aligns by real time, not first-frame-zero.
	wallclock := slices.Contains(p.Options, OptionWallclockWithGenpts)

	for i, input := range p.Inputs {
		for _, arg := range perInputDecodeArgs(backend, modes[i]) {
			cmd.WriteString(" " + arg)
		}

		if input.OverlayText != "" {
			cmd.WriteString(" -re -f lavfi")
			testSrc := "testsrc2"
			res := input.Resolution
			if res == "" {
				res = fmt.Sprintf("%dx%d", input.Width, input.Height)
			}
			fps := input.FPS
			if fps == "" {
				fps = p.FPS
			}
			testSrc += "=size=" + res + ":rate=" + fps
			cmd.WriteString(fmt.Sprintf(" -i \"%s\"", testSrc))
		} else {
			if wallclock {
				cmd.WriteString(" -use_wallclock_as_timestamps 1 -fflags +genpts+igndts")
			}
			cmd.WriteString(" -f v4l2 -thread_queue_size 1024")
			if input.InputFormat != "" {
				cmd.WriteString(" -input_format " + input.InputFormat)
			}
			if input.Resolution != "" {
				cmd.WriteString(" -video_size " + input.Resolution)
			}
			if input.FPS != "" {
				cmd.WriteString(" -framerate " + input.FPS)
			}
			cmd.WriteString(" -i " + input.DevicePath)
		}
	}

	// First audio input lands at FFmpeg index len(p.Inputs).
	audioInputIdx := -1
	for i, dev := range p.AudioDevices {
		if dev == "" {
			continue
		}
		cmd.WriteString(" -thread_queue_size 1024 -f alsa -sample_fmt s16 -ar 48000 -ac 2")
		cmd.WriteString(" -i " + dev)
		if audioInputIdx < 0 {
			audioInputIdx = len(p.Inputs) + i
		}
	}

	padColor := p.KeyColor
	if padColor == "" {
		padColor = "0x000000"
	}

	// AMD VCN hevc_vaapi green-band workaround: pad SW frame to /16 before hwupload (validator-gated).
	padForAlignment := p.HWCaps.NeedsAlignedHevcHeight(backend, p.Encoder)

	var fc strings.Builder
	fc.WriteString("\n    " + canvasBaseFilter(canvasMode, p.Width, p.Height, p.FPS, padColor, padForAlignment))

	videoIdx := 0
	var rawOutputLabels []string
	for i, input := range p.Inputs {
		mode := modes[i]
		srcLabel := fmt.Sprintf("[%d:v]", i)

		if input.VisionEnabled {
			rawLabel := fmt.Sprintf("[raw%d]", videoIdx)
			encLabel := fmt.Sprintf("[enc%d]", videoIdx)
			rawOutLabel := fmt.Sprintf("[rawout%d]", videoIdx)
			fc.WriteString(fmt.Sprintf(";\n    %ssplit=2%s%s", srcLabel, rawLabel, encLabel))

			visionChain := perInputVisionChain(backend, mode, input, p.HWCaps)
			fc.WriteString(fmt.Sprintf(";\n    %s%s%s",
				rawLabel, joinFilters(visionChain), rawOutLabel))
			rawOutputLabels = append(rawOutputLabels, rawOutLabel)
			srcLabel = encLabel
		}

		var filters []string
		if useBGRAOverlay {
			filters = perInputEncodeChainBGRA(backend, mode, input, p.HWCaps, p.FPS, padColor)
		} else {
			filters = perInputEncodeChain(backend, mode, input, p.HWCaps, p.FPS, padColor)
			if tail := perInputEncodeTail(backend, mode, input, p.HWCaps, canvasMode); tail != "" {
				filters = append(filters, tail)
			}
		}

		fc.WriteString(fmt.Sprintf(";\n    %s%s[v%d]", srcLabel, joinFilters(filters), videoIdx))
		videoIdx++
	}

	prev := "[canvas]"
	for i := range p.Inputs {
		label := fmt.Sprintf("[v%d]", i)
		x := p.Inputs[i].X
		y := p.Inputs[i].Y
		isLast := i == len(p.Inputs)-1
		overlayBackend := backend
		if !hwOverlay {
			overlayBackend = "sw"
		}
		overlayStep := hwOverlayFilter(overlayBackend, x, y)

		if isLast {
			// Single setpts here (not per-input) preserves cross-input alignment when one camera lags.
			// fps= re-asserts canvas rate; setpts wipes frame_rate and the encoder would fall back to 25.
			overlayStep += ",setpts=PTS-STARTPTS,fps=" + p.FPS
			tail := finalOverlayTail(backend, hwOverlay, p.Width, p.Height, padForAlignment)
			if tail != "" {
				overlayStep += "," + tail
			}
			fc.WriteString(fmt.Sprintf(";\n    %s%s%s[vout]", prev, label, overlayStep))
		} else {
			tmpLabel := fmt.Sprintf("[tmp%d]", i)
			fc.WriteString(fmt.Sprintf(";\n    %s%s%s%s", prev, label, overlayStep, tmpLabel))
			prev = tmpLabel
		}
	}

	cmd.WriteString(" -filter_complex \"" + fc.String() + "\n  \"")

	cmd.WriteString(" -map \"[vout]\"")
	if audioInputIdx >= 0 {
		cmd.WriteString(fmt.Sprintf(" -map %d:a", audioInputIdx))
		cmd.WriteString(" -af aresample=async=1:min_hard_comp=0.100000:first_pts=0")
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

	if audioInputIdx >= 0 {
		cmd.WriteString(" -c:a libopus -b:a 128k -ar 48000")
	}

	if p.ProgressSocket != "" {
		cmd.WriteString(" -progress unix://" + p.ProgressSocket)
	}

	if strings.HasPrefix(p.OutputURL, "rtsp://") {
		cmd.WriteString(" -rtsp_transport tcp -f rtsp " + p.OutputURL)
	} else {
		cmd.WriteString(" -muxdelay 0 -muxpreload 0 -flush_packets 1 -f mpegts " + p.OutputURL)
	}

	for i, label := range rawOutputLabels {
		cmd.WriteString(fmt.Sprintf(" -map \"%s\" -f rawvideo -pix_fmt nv12 pipe:%d", label, 3+i))
	}

	return cmd.String()
}
