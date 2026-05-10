package ffmpeg

import (
	"strings"
	"testing"
)

func baseCompositeParams() *CompositeParams {
	return &CompositeParams{
		Width:          1920,
		Height:         1080,
		FPS:            "30",
		KeyColor:       "0x00ff00",
		Encoder:        "libx264",
		Preset:         "fast",
		Bitrate:        "8.0M",
		BFrames:        0,
		OutputURL:      "rtsp://127.0.0.1:8554/multicam",
		ProgressSocket: "/tmp/ffmpeg-progress-multicam.sock",
	}
}

func TestBuildCompositeCommand_SingleInput(t *testing.T) {
	p := baseCompositeParams()
	p.Inputs = []CompositeInput{
		{
			DevicePath:  "/dev/video0",
			InputFormat: "nv12",
			Resolution:  "1920x1080",
			FPS:         "60",
			X:           0,
			Y:           0,
			Width:       1920,
			Height:      1080,
		},
	}

	cmd := BuildCompositeCommand(p)

	// Check input
	if !strings.Contains(cmd, "-f v4l2") {
		t.Error("missing V4L2 input")
	}
	if !strings.Contains(cmd, "-input_format nv12") {
		t.Error("missing input_format")
	}
	if !strings.Contains(cmd, "-video_size 1920x1080") {
		t.Error("missing video_size")
	}
	if !strings.Contains(cmd, "-framerate 60") {
		t.Error("missing framerate")
	}
	if !strings.Contains(cmd, "-i /dev/video0") {
		t.Error("missing device path")
	}

	// Check filter_complex structure
	if !strings.Contains(cmd, "color=c=0x00ff00:s=1920x1080:r=30[canvas]") {
		t.Error("missing canvas color source")
	}
	if !strings.Contains(cmd, "[0:v]fps=30,scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2:color=0x00ff00[v0]") {
		t.Error("missing input scale+pad filter")
	}
	if !strings.Contains(cmd, "[canvas][v0]overlay=0:0,setpts=PTS-STARTPTS,fps=30[vout]") {
		t.Error("missing overlay with vout setpts+fps")
	}
	if !strings.Contains(cmd, "-map \"[vout]\"") {
		t.Error("missing vout map")
	}
}

func TestBuildCompositeCommand_ThreeInputs(t *testing.T) {
	p := baseCompositeParams()
	p.AudioDevices = []string{"hw:4,0"}
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "60", X: 0, Y: 0, Width: 1152, Height: 1080},
		{DevicePath: "/dev/video2", InputFormat: "mjpeg", Resolution: "1920x1080", FPS: "30", X: 1152, Y: 0, Width: 768, Height: 540},
		{DevicePath: "/dev/video4", InputFormat: "nv12", Resolution: "1280x720", FPS: "30", X: 1152, Y: 540, Width: 768, Height: 540},
	}

	cmd := BuildCompositeCommand(p)

	// Check all three V4L2 inputs
	if strings.Count(cmd, "-f v4l2") != 3 {
		t.Errorf("expected 3 V4L2 inputs, got %d", strings.Count(cmd, "-f v4l2"))
	}

	// Check audio input
	if !strings.Contains(cmd, "-f alsa") {
		t.Error("missing ALSA audio input")
	}
	if !strings.Contains(cmd, "-i hw:4,0") {
		t.Error("missing audio device")
	}

	// Video inputs at sequential indices 0..2; audio follows at index 3.
	if !strings.Contains(cmd, "[0:v]fps=30,scale=1152:1080:force_original_aspect_ratio=decrease,pad=1152:1080:(ow-iw)/2:(oh-ih)/2:color=0x00ff00[v0]") {
		t.Error("missing input 0 filter chain")
	}
	if !strings.Contains(cmd, "[1:v]fps=30,scale=768:540:force_original_aspect_ratio=decrease,pad=768:540:(ow-iw)/2:(oh-ih)/2:color=0x00ff00[v1]") {
		t.Error("missing input 1 filter chain")
	}
	if !strings.Contains(cmd, "[2:v]fps=30,scale=768:540:force_original_aspect_ratio=decrease,pad=768:540:(ow-iw)/2:(oh-ih)/2:color=0x00ff00[v2]") {
		t.Error("missing input 2 filter chain")
	}

	// Check overlay chain
	if !strings.Contains(cmd, "[canvas][v0]overlay=0:0[tmp0]") {
		t.Error("missing first overlay")
	}
	if !strings.Contains(cmd, "[tmp0][v1]overlay=1152:0[tmp1]") {
		t.Error("missing second overlay")
	}
	if !strings.Contains(cmd, "[tmp1][v2]overlay=1152:540,setpts=PTS-STARTPTS,fps=30[vout]") {
		t.Error("missing final overlay with vout setpts+fps")
	}

	// Audio mapping (audio is input 3 after three video inputs)
	if !strings.Contains(cmd, "-map 3:a") {
		t.Error("missing audio map")
	}

	// Audio filter
	if !strings.Contains(cmd, "-af aresample=async=1") {
		t.Error("missing audio resample filter")
	}

	// Audio codec
	if !strings.Contains(cmd, "-c:a libopus -b:a 128k") {
		t.Error("missing audio codec")
	}
}

func TestBuildCompositeCommand_NoSignalInput(t *testing.T) {
	p := baseCompositeParams()
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "60", X: 0, Y: 0, Width: 1152, Height: 1080},
		{OverlayText: "NO SIGNAL", Resolution: "1920x1080", FPS: "30", X: 1152, Y: 0, Width: 768, Height: 540},
	}

	cmd := BuildCompositeCommand(p)

	// First input is V4L2
	if !strings.Contains(cmd, "-f v4l2") {
		t.Error("missing V4L2 input for online device")
	}

	// Second input is test source
	if !strings.Contains(cmd, "-re -f lavfi") {
		t.Error("missing lavfi test source for offline device")
	}
	if !strings.Contains(cmd, "testsrc2=size=1920x1080:rate=30") {
		t.Error("missing test source params")
	}

	// Check NO SIGNAL drawtext in filter chain
	if !strings.Contains(cmd, "drawtext=text='NO SIGNAL'") {
		t.Error("missing NO SIGNAL drawtext")
	}
}

func TestBuildCompositeCommand_Rotation(t *testing.T) {
	tests := []struct {
		name     string
		rotation int
		want     string
	}{
		{"90 degrees", 90, "fps=30,transpose=1,scale=768:540:force_original_aspect_ratio=decrease,pad=768:540:(ow-iw)/2:(oh-ih)/2:color=0x00ff00"},
		{"180 degrees", 180, "fps=30,transpose=1,transpose=1,scale=768:540:force_original_aspect_ratio=decrease,pad=768:540:(ow-iw)/2:(oh-ih)/2:color=0x00ff00"},
		{"270 degrees", 270, "fps=30,transpose=2,scale=768:540:force_original_aspect_ratio=decrease,pad=768:540:(ow-iw)/2:(oh-ih)/2:color=0x00ff00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := baseCompositeParams()
			p.Inputs = []CompositeInput{
				{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30", X: 0, Y: 0, Width: 768, Height: 540, Rotation: tt.rotation},
			}

			cmd := BuildCompositeCommand(p)
			if !strings.Contains(cmd, tt.want) {
				t.Errorf("expected %q in command, got:\n%s", tt.want, cmd)
			}
		})
	}
}

func TestBuildCompositeCommand_Crop(t *testing.T) {
	p := baseCompositeParams()
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30",
			X: 0, Y: 0, Width: 960, Height: 540,
			CropW: 1440, CropH: 810, CropX: 240, CropY: 135,
		},
	}

	cmd := BuildCompositeCommand(p)
	if !strings.Contains(cmd, "crop=1440:810:240:135") {
		t.Error("missing crop filter")
	}
	// Crop should come before scale
	cropIdx := strings.Index(cmd, "crop=1440")
	scaleIdx := strings.Index(cmd, "scale=960:540:force_original_aspect_ratio=decrease")
	if cropIdx > scaleIdx {
		t.Error("crop should come before scale in filter chain")
	}
}

// TestBuildCompositeCommand_VAAPI_RawInputSWFallback covers the case where
// the canvas backend is VAAPI but the input is a raw camera format (nv12) —
// software decode, then hwupload. Overlay is software because no caps probe
// data is supplied.
func TestBuildCompositeCommand_VAAPI_RawInputSWFallback(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_vaapi"
	p.HWBackend = "vaapi"
	p.GlobalArgs = []string{"-vaapi_device", "/dev/dri/renderD128"}
	p.RCMode = "CBR"
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30", X: 0, Y: 0, Width: 1920, Height: 1080},
	}

	cmd := BuildCompositeCommand(p)

	if !strings.Contains(cmd, "-vaapi_device /dev/dri/renderD128") {
		t.Error("missing VAAPI device")
	}
	// Raw nv12 input takes SW decode path: no per-input -hwaccel.
	if strings.Contains(cmd, "-hwaccel vaapi") {
		t.Error("raw nv12 input should not use -hwaccel")
	}
	// SW overlay fallback uploads before encoder. Default (no alignment
	// workaround) emits plain hwupload — see PadsWhenHevcAlignmentBug for
	// the AMD VCN gated fix.
	if !strings.Contains(cmd, "overlay=0:0,setpts=PTS-STARTPTS,fps=30,format=nv12,hwupload[vout]") {
		t.Errorf("missing SW overlay → setpts → fps → hwupload tail, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "-c:v h264_vaapi") {
		t.Error("missing VAAPI encoder")
	}
	if !strings.Contains(cmd, "-rc_mode CBR") {
		t.Error("missing rc_mode")
	}
	if strings.Contains(cmd, "-tune zerolatency") {
		t.Error("should not have zerolatency for hardware encoder")
	}
}

// TestBuildCompositeCommand_VAAPI_HWDecodeAndOverlay covers the full HW path
// when caps report overlay_vaapi available and the input is HW-decodable.
func TestBuildCompositeCommand_VAAPI_HWDecodeAndOverlay(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_vaapi"
	p.HWBackend = "vaapi"
	p.GlobalArgs = []string{"-vaapi_device", "/dev/dri/renderD128"}
	p.HWCaps = HWCapabilities{OverlayVAAPI: true, ScaleVAAPI: true, TransposeVAAPI: true}
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "h264", Resolution: "1280x720", FPS: "30", X: 0, Y: 0, Width: 1920, Height: 1080},
	}

	cmd := BuildCompositeCommand(p)

	if !strings.Contains(cmd, "-hwaccel vaapi -hwaccel_output_format vaapi -f v4l2") {
		t.Errorf("expected per-input -hwaccel, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "color=c=0x00ff00:s=1920x1080:r=30,format=nv12,hwupload[canvas]") {
		t.Errorf("expected HW canvas base, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "scale_vaapi=w=1920:h=1080:format=nv12[v0]") {
		t.Errorf("expected scale_vaapi to slot, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "[canvas][v0]overlay_vaapi=x=0:y=0,setpts=PTS-STARTPTS,fps=30[vout]") {
		t.Errorf("expected overlay_vaapi → setpts → fps → [vout], got:\n%s", cmd)
	}
	if strings.Contains(cmd, "format=nv12,hwupload[vout]") {
		t.Error("HW overlay path should not append hwupload tail")
	}
}

// TestBuildCompositeCommand_RKMPP_HWPath covers RK3588 full HW with all rkrga
// filters available. Includes a rotated input so vpp_rkrga shows up.
func TestBuildCompositeCommand_RKMPP_HWPath(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_rkmpp"
	p.HWBackend = "rkmpp"
	p.HWCaps = HWCapabilities{OverlayRKRGA: true, ScaleRKRGA: true, VppRKRGA: true}
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "h264", Resolution: "1280x720", FPS: "30", X: 0, Y: 0, Width: 960, Height: 540},
		{DevicePath: "/dev/video2", InputFormat: "mjpeg", Resolution: "1920x1080", FPS: "60", X: 960, Y: 0, Width: 960, Height: 540},
		{DevicePath: "/dev/video4", InputFormat: "mjpeg", Resolution: "1920x1080", FPS: "60", X: 480, Y: 540, Width: 960, Height: 540, Rotation: 90},
	}

	cmd := BuildCompositeCommand(p)

	if !strings.Contains(cmd, "-init_hw_device rkmpp=hw -filter_hw_device hw") {
		t.Errorf("missing rkmpp init_hw_device, got:\n%s", cmd)
	}
	if strings.Count(cmd, "-hwaccel rkmpp -hwaccel_output_format drm_prime") != 3 {
		t.Errorf("expected 3 per-input rkmpp hwaccel, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "vpp_rkrga=transpose=clock,scale_rkrga=960:540:format=nv12[v2]") {
		t.Errorf("expected rotated input to use vpp_rkrga + scale_rkrga, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "[canvas][v0]overlay_rkrga=x=0:y=0[tmp0]") {
		t.Errorf("missing first overlay_rkrga, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "[tmp1][v2]overlay_rkrga=x=480:y=540,setpts=PTS-STARTPTS,fps=30[vout]") {
		t.Errorf("missing final overlay_rkrga → setpts → fps, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_RKMPP_OverlayFallback covers RKMPP with no
// overlay_rkrga: per-input HW decode + scale_rkrga + hwdownload, sw overlay,
// final hwupload before encode.
func TestBuildCompositeCommand_RKMPP_OverlayFallback(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_rkmpp"
	p.HWBackend = "rkmpp"
	// VppRKRGA + ScaleRKRGA available, but no OverlayRKRGA.
	p.HWCaps = HWCapabilities{ScaleRKRGA: true, VppRKRGA: true}
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "mjpeg", Resolution: "1920x1080", FPS: "60", X: 0, Y: 0, Width: 1920, Height: 1080},
	}

	cmd := BuildCompositeCommand(p)

	if !strings.Contains(cmd, "-hwaccel rkmpp -hwaccel_output_format drm_prime") {
		t.Error("HW decode should still run when overlay falls back to SW")
	}
	if !strings.Contains(cmd, "scale_rkrga=1920:1080:format=nv12,hwdownload,format=nv12[v0]") {
		t.Errorf("expected per-input scale + hwdownload, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "color=c=0x00ff00:s=1920x1080:r=30[canvas]") {
		t.Errorf("expected SW canvas base in fallback, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "[canvas][v0]overlay=0:0,setpts=PTS-STARTPTS,fps=30,format=nv12,hwupload[vout]") {
		t.Errorf("expected SW overlay → setpts → fps → hwupload tail, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_RKMPP_PerspectiveForcesSW covers the case where
// one input has perspective: that input goes SW (decode + filters), then
// uploads to rejoin the HW canvas; sibling HW-decoded inputs stay on HW.
func TestBuildCompositeCommand_RKMPP_PerspectiveForcesSW(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_rkmpp"
	p.HWBackend = "rkmpp"
	p.HWCaps = HWCapabilities{OverlayRKRGA: true, ScaleRKRGA: true, VppRKRGA: true}
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "h264", Resolution: "1280x720", FPS: "30",
			X: 0, Y: 0, Width: 960, Height: 540,
			Perspective: &PerspectiveConfig{Corners: [4][2]int{{0, 0}, {1280, 0}, {1280, 720}, {0, 720}}},
		},
		{DevicePath: "/dev/video2", InputFormat: "h264", Resolution: "1280x720", FPS: "30", X: 960, Y: 0, Width: 960, Height: 540},
	}

	cmd := BuildCompositeCommand(p)

	// Per-input hwaccel: only the second input gets it.
	if strings.Count(cmd, "-hwaccel rkmpp") != 1 {
		t.Errorf("expected exactly one -hwaccel rkmpp (perspective forces SW), got:\n%s", cmd)
	}
	// Perspective input ends with hwupload to rejoin HW canvas.
	if !strings.Contains(cmd, "format=nv12,hwupload[v0]") {
		t.Errorf("expected SW input to hwupload before [v0], got:\n%s", cmd)
	}
	// Sibling stays on HW.
	if !strings.Contains(cmd, "scale_rkrga=960:540:format=nv12[v1]") {
		t.Errorf("expected sibling on HW, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_RKMPP_BGRAOverlay covers the unified
// BGRA-overlay-on-YUV path on RK3588: per-input chains end in scale_rkrga
// with format=bgra, overlay_rkrga composes onto a HW-NV12 canvas, and the
// final encode reads drm_prime directly (no format=nv12,hwupload tail).
func TestBuildCompositeCommand_RKMPP_BGRAOverlay(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_rkmpp"
	p.HWBackend = "rkmpp"
	p.HWCaps = HWCapabilities{
		OverlayRKRGA:   true,
		ScaleRKRGA:     true,
		ScaleRKRGABGRA: true,
		VppRKRGA:       true,
	}
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "h264", Resolution: "1280x720", FPS: "30",
			X: 0, Y: 0, Width: 960, Height: 540,
		},
		{
			DevicePath: "/dev/video2", InputFormat: "mjpeg", Resolution: "1920x1080", FPS: "60",
			X: 960, Y: 0, Width: 960, Height: 540,
			Rotation: 90,
		},
	}

	cmd := BuildCompositeCommand(p)

	// Both inputs HW-decoded.
	if strings.Count(cmd, "-hwaccel rkmpp -hwaccel_output_format drm_prime") != 2 {
		t.Errorf("expected per-input rkmpp hwaccel on both inputs, got:\n%s", cmd)
	}
	// Per-input chains end in scale_rkrga=...:format=bgra (no hwdownload).
	if !strings.Contains(cmd, "[0:v]scale_rkrga=960:540:format=bgra[v0]") {
		t.Errorf("expected non-rotated input to scale_rkrga to BGRA, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "[1:v]vpp_rkrga=transpose=clock,scale_rkrga=960:540:format=bgra[v1]") {
		t.Errorf("expected rotated input to vpp_rkrga + scale_rkrga to BGRA, got:\n%s", cmd)
	}
	// Canvas base on HW YUV.
	if !strings.Contains(cmd, "color=c=0x00ff00:s=1920x1080:r=30,format=nv12,hwupload[canvas]") {
		t.Errorf("expected HW canvas base, got:\n%s", cmd)
	}
	// Overlay chain runs on RGA all the way to [vout].
	if !strings.Contains(cmd, "[canvas][v0]overlay_rkrga=x=0:y=0[tmp0]") {
		t.Errorf("missing first overlay_rkrga, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "[tmp0][v1]overlay_rkrga=x=960:y=0,setpts=PTS-STARTPTS,fps=30[vout]") {
		t.Errorf("missing final overlay_rkrga → setpts → fps, got:\n%s", cmd)
	}
	// No trailing format=nv12,hwupload before encoder.
	if strings.Contains(cmd, "format=nv12,hwupload[vout]") {
		t.Errorf("BGRA-overlay path should not need hwupload before [vout], got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_VAAPI_BGRAOverlay covers the same unified path
// emitted with VAAPI filter names. Lets us assert command shape on a
// non-RK3588 host.
func TestBuildCompositeCommand_VAAPI_BGRAOverlay(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_vaapi"
	p.HWBackend = "vaapi"
	p.GlobalArgs = []string{"-vaapi_device", "/dev/dri/renderD128"}
	p.HWCaps = HWCapabilities{
		OverlayVAAPI:   true,
		ScaleVAAPI:     true,
		ScaleVAAPIBGRA: true,
		TransposeVAAPI: true,
	}
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "mjpeg", Resolution: "1920x1080", FPS: "60",
			X: 0, Y: 0, Width: 1920, Height: 1080,
		},
	}

	cmd := BuildCompositeCommand(p)

	if !strings.Contains(cmd, "-hwaccel vaapi -hwaccel_output_format vaapi") {
		t.Error("expected per-input vaapi hwaccel")
	}
	if !strings.Contains(cmd, "[0:v]scale_vaapi=w=1920:h=1080:format=bgra[v0]") {
		t.Errorf("expected scale_vaapi to BGRA, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "[canvas][v0]overlay_vaapi=x=0:y=0,setpts=PTS-STARTPTS,fps=30[vout]") {
		t.Errorf("missing overlay_vaapi → setpts → fps → [vout], got:\n%s", cmd)
	}
	if strings.Contains(cmd, "format=nv12,hwupload[vout]") {
		t.Errorf("BGRA-overlay path should not need hwupload before [vout], got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_BGRAOverlay_RawNV12 covers the production case
// on RK3588: V4L2 cameras delivering raw NV12 (no decoder). The per-input
// chain uploads + scales + format-converts in one HW pass.
func TestBuildCompositeCommand_BGRAOverlay_RawNV12(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_rkmpp"
	p.HWBackend = "rkmpp"
	p.HWCaps = HWCapabilities{
		OverlayRKRGA:   true,
		ScaleRKRGA:     true,
		ScaleRKRGABGRA: true,
		VppRKRGA:       true,
	}
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "60",
			X: 0, Y: 0, Width: 960, Height: 540,
		},
	}

	cmd := BuildCompositeCommand(p)

	// Raw NV12 input: no -hwaccel (no decoder), but per-input chain uploads
	// and runs HW scale to BGRA in one shot.
	if strings.Contains(cmd, "-hwaccel rkmpp") {
		t.Error("raw nv12 input should not use -hwaccel (no decoder)")
	}
	if !strings.Contains(cmd, "[0:v]format=nv12,hwupload,scale_rkrga=960:540:format=bgra[v0]") {
		t.Errorf("expected raw NV12 path: hwupload + scale_rkrga to BGRA, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_BGRAOverlay_PerspectiveForceSW covers the case
// where one input has perspective (forces SW chain). It must end with
// format=bgra,hwupload to rejoin the HW canvas; sibling stays on HW.
func TestBuildCompositeCommand_BGRAOverlay_PerspectiveForceSW(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_rkmpp"
	p.HWBackend = "rkmpp"
	p.HWCaps = HWCapabilities{
		OverlayRKRGA:   true,
		ScaleRKRGA:     true,
		ScaleRKRGABGRA: true,
		VppRKRGA:       true,
	}
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "h264", Resolution: "1280x720", FPS: "30",
			X: 0, Y: 0, Width: 960, Height: 540,
			Perspective: &PerspectiveConfig{Corners: [4][2]int{{0, 0}, {1280, 0}, {1280, 720}, {0, 720}}},
		},
		{
			DevicePath: "/dev/video2", InputFormat: "h264", Resolution: "1280x720", FPS: "30",
			X: 960, Y: 0, Width: 960, Height: 540,
		},
	}

	cmd := BuildCompositeCommand(p)

	// Perspective input gets SW chain ending in format=bgra,hwupload.
	if !strings.Contains(cmd, ",format=bgra,hwupload[v0]") {
		t.Errorf("expected perspective input to end SW chain with format=bgra,hwupload, got:\n%s", cmd)
	}
	// Sibling stays on HW BGRA via scale_rkrga.
	if !strings.Contains(cmd, "[1:v]scale_rkrga=960:540:format=bgra[v1]") {
		t.Errorf("expected sibling on HW scale_rkrga to BGRA, got:\n%s", cmd)
	}
	// Overlay chain stays on HW.
	if !strings.Contains(cmd, "overlay_rkrga") {
		t.Errorf("expected overlay_rkrga in chain, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_VAAPI_PartialCaps_TransposeFallsBackSW pins the
// radeonsi shape: HW decode + scale_vaapi work, but transpose_vaapi is
// unavailable. A rotated input must HW-decode + hwdownload + run SW filters
// (incl. SW transpose). A non-rotated sibling stays on HW scale_vaapi.
func TestBuildCompositeCommand_VAAPI_PartialCaps_TransposeFallsBackSW(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_vaapi"
	p.HWBackend = "vaapi"
	p.GlobalArgs = []string{"-vaapi_device", "/dev/dri/renderD128"}
	// Caps mirror radeonsi: scale works, transpose + overlay don't.
	p.HWCaps = HWCapabilities{ScaleVAAPI: true}
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "h264", Resolution: "1280x720", FPS: "30",
			X: 0, Y: 0, Width: 960, Height: 540,
			Rotation: 90,
		},
		{
			DevicePath: "/dev/video2", InputFormat: "mjpeg", Resolution: "1920x1080", FPS: "60",
			X: 960, Y: 0, Width: 960, Height: 540,
		},
	}

	cmd := BuildCompositeCommand(p)

	// Both inputs HW-decode.
	if strings.Count(cmd, "-hwaccel vaapi -hwaccel_output_format vaapi") != 2 {
		t.Errorf("expected per-input HW decode on both inputs, got:\n%s", cmd)
	}
	// Rotated input falls back to SW filters after hwdownload.
	if !strings.Contains(cmd, "[0:v]hwdownload,format=nv12,fps=30,transpose=1,scale=960:540") {
		t.Errorf("expected rotated input to hwdownload + SW transpose, got:\n%s", cmd)
	}
	// Non-rotated sibling stays on HW for scale.
	if !strings.Contains(cmd, "[1:v]scale_vaapi=w=960:h=540:format=nv12") {
		t.Errorf("expected non-rotated input to keep HW scale_vaapi, got:\n%s", cmd)
	}
	// SW overlay (no overlay_vaapi cap), final upload before encoder.
	if !strings.Contains(cmd, "overlay=960:0,setpts=PTS-STARTPTS,fps=30,format=nv12,hwupload[vout]") {
		t.Errorf("expected SW overlay → setpts → fps → hwupload tail, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_VAAPI_PartialCaps_NoScale_VisionDownloads checks
// that the vision branch downloads first when HW scale isn't available.
func TestBuildCompositeCommand_VAAPI_PartialCaps_NoScale_VisionDownloads(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_vaapi"
	p.HWBackend = "vaapi"
	p.GlobalArgs = []string{"-vaapi_device", "/dev/dri/renderD128"}
	// No HW filter caps at all — HW decode only.
	p.HWCaps = HWCapabilities{}
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "h264", Resolution: "1280x720", FPS: "30",
			X: 0, Y: 0, Width: 960, Height: 540,
			VisionEnabled: true, VisionWidth: 320, VisionHeight: 240, VisionFPS: 5,
		},
	}

	cmd := BuildCompositeCommand(p)

	if !strings.Contains(cmd, "-hwaccel vaapi -hwaccel_output_format vaapi") {
		t.Error("expected HW decode")
	}
	if !strings.Contains(cmd, "[raw0]setpts=PTS-STARTPTS,fps=5,hwdownload,format=nv12,scale=320:240,format=nv12[rawout0]") {
		t.Errorf("expected vision branch to hwdownload first, then SW scale, got:\n%s", cmd)
	}
	// Encode branch: HW decode but no scale cap → hwdownload + SW chain.
	if !strings.Contains(cmd, "[enc0]hwdownload,format=nv12,fps=30") {
		t.Errorf("expected encode branch to hwdownload before SW chain, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_VisionFPS covers the new vision FPS throttle.
func TestBuildCompositeCommand_VisionFPS_HWInput(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_rkmpp"
	p.HWBackend = "rkmpp"
	p.HWCaps = HWCapabilities{OverlayRKRGA: true, ScaleRKRGA: true, VppRKRGA: true}
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "h264", Resolution: "1280x720", FPS: "30",
			X: 0, Y: 0, Width: 1920, Height: 1080,
			VisionEnabled: true, VisionWidth: 640, VisionHeight: 480, VisionFPS: 10,
		},
	}

	cmd := BuildCompositeCommand(p)

	if !strings.Contains(cmd, "[raw0]setpts=PTS-STARTPTS,fps=10,scale_rkrga=640:480:format=nv12,hwdownload,format=nv12[rawout0]") {
		t.Errorf("expected vision branch fps=10 + HW scale + hwdownload, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "-map \"[rawout0]\" -f rawvideo -pix_fmt nv12 pipe:3") {
		t.Error("missing rawout pipe map")
	}
}

func TestBuildCompositeCommand_VisionFPS_SWInput(t *testing.T) {
	p := baseCompositeParams() // sw backend
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30",
			X: 0, Y: 0, Width: 1920, Height: 1080,
			VisionEnabled: true, VisionWidth: 320, VisionHeight: 240, VisionFPS: 5,
		},
	}

	cmd := BuildCompositeCommand(p)

	if !strings.Contains(cmd, "[raw0]setpts=PTS-STARTPTS,fps=5,scale=320:240,format=nv12[rawout0]") {
		t.Errorf("expected SW vision branch with fps=5, got:\n%s", cmd)
	}
}

func TestBuildCompositeCommand_PerInputFilterOrder(t *testing.T) {
	p := baseCompositeParams()
	p.Inputs = []CompositeInput{
		{
			DevicePath:  "/dev/video0",
			InputFormat: "nv12",
			Resolution:  "1920x1080",
			FPS:         "30",
			X:           0, Y: 0, Width: 768, Height: 540,
			Rotation: 90,
			CropW:    1440, CropH: 810, CropX: 240, CropY: 135,
			OverlayText: "TEST MODE",
		},
	}

	cmd := BuildCompositeCommand(p)

	// Order: fps, transpose, crop, drawtext, scale+pad, setpts
	fpsIdx := strings.Index(cmd, "fps=30")
	transposeIdx := strings.Index(cmd, "transpose=1")
	cropIdx := strings.Index(cmd, "crop=1440")
	drawtextIdx := strings.Index(cmd, "drawtext=")
	scaleIdx := strings.Index(cmd, "scale=768:540:force_original_aspect_ratio=decrease")
	setptsIdx := strings.Index(cmd, "setpts=PTS-STARTPTS")

	if fpsIdx >= transposeIdx {
		t.Error("fps should come before transpose")
	}
	if transposeIdx >= cropIdx {
		t.Error("transpose should come before crop")
	}
	if cropIdx >= drawtextIdx {
		t.Error("crop should come before drawtext")
	}
	if drawtextIdx >= scaleIdx {
		t.Error("drawtext should come before scale")
	}
	if scaleIdx >= setptsIdx {
		t.Error("scale should come before setpts")
	}
}

// TestBuildCompositeCommand_WallclockOption pins the canvas-level wallclock
// plumbing: when CompositeParams.Options carries OptionWallclockWithGenpts,
// every V4L2 -i is preceded by -use_wallclock_as_timestamps 1 and the
// genpts+igndts fflags. Test/lavfi sources must NOT receive the flag because
// they drive their own clock. Without this, mixed-source canvases would lose
// the shared wallclock baseline overlay relies on.
func TestBuildCompositeCommand_WallclockOption(t *testing.T) {
	p := baseCompositeParams()
	p.Options = []OptionType{OptionWallclockWithGenpts}
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30", X: 0, Y: 0, Width: 960, Height: 540},
		{DevicePath: "/dev/video2", InputFormat: "mjpeg", Resolution: "1920x1080", FPS: "30", X: 960, Y: 0, Width: 960, Height: 540},
		{OverlayText: "NO SIGNAL", Resolution: "1920x1080", FPS: "30", X: 0, Y: 540, Width: 960, Height: 540},
	}

	cmd := BuildCompositeCommand(p)

	if got, want := strings.Count(cmd, "-use_wallclock_as_timestamps 1 -fflags +genpts+igndts"), 2; got != want {
		t.Errorf("expected wallclock flags on %d V4L2 inputs, got %d:\n%s", want, got, cmd)
	}
	if !strings.Contains(cmd, "-use_wallclock_as_timestamps 1 -fflags +genpts+igndts -f v4l2") {
		t.Errorf("expected wallclock flags immediately before -f v4l2, got:\n%s", cmd)
	}
	// Final overlay rebases the composite once.
	if !strings.Contains(cmd, "setpts=PTS-STARTPTS") {
		t.Errorf("expected single setpts on [vout], got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_NoWallclockWithoutOption verifies the wallclock
// flags are absent when the canvas doesn't opt in.
func TestBuildCompositeCommand_NoWallclockWithoutOption(t *testing.T) {
	p := baseCompositeParams()
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30", X: 0, Y: 0, Width: 1920, Height: 1080},
	}
	cmd := BuildCompositeCommand(p)
	if strings.Contains(cmd, "-use_wallclock_as_timestamps") {
		t.Errorf("wallclock flags must not appear without OptionWallclockWithGenpts, got:\n%s", cmd)
	}
}

func TestBuildCompositeCommand_PadColorDefault(t *testing.T) {
	p := baseCompositeParams()
	p.KeyColor = ""
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30", X: 0, Y: 0, Width: 960, Height: 540},
	}
	cmd := BuildCompositeCommand(p)
	if !strings.Contains(cmd, "pad=960:540:(ow-iw)/2:(oh-ih)/2:color=0x000000") {
		t.Errorf("expected default pad color 0x000000 when KeyColor empty, got:\n%s", cmd)
	}
}

func TestEffectiveInputSize(t *testing.T) {
	tests := []struct {
		name        string
		resolution  string
		rotation    int
		perspective *PerspectiveConfig
		cropW       int
		cropH       int
		wantW       int
		wantH       int
	}{
		{"plain landscape", "1920x1080", 0, nil, 0, 0, 1920, 1080},
		{"90 rotation swaps", "1920x1080", 90, nil, 0, 0, 1080, 1920},
		{"180 rotation unchanged", "1920x1080", 180, nil, 0, 0, 1920, 1080},
		{"270 rotation swaps", "1920x1080", 270, nil, 0, 0, 1080, 1920},
		{"crop replaces", "1920x1080", 0, nil, 800, 600, 800, 600},
		{"perspective replaces", "1920x1080", 0, &PerspectiveConfig{Corners: [4][2]int{{0, 0}, {1000, 0}, {1000, 500}, {0, 500}}}, 0, 0, 1000, 500},
		{"perspective then rotation", "1920x1080", 90, &PerspectiveConfig{Corners: [4][2]int{{0, 0}, {1000, 0}, {1000, 500}, {0, 500}}}, 0, 0, 500, 1000},
		{"unparseable resolution", "garbage", 0, nil, 0, 0, 0, 0},
		{"empty resolution", "", 0, nil, 0, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := EffectiveInputSize(tt.resolution, tt.rotation, tt.perspective, tt.cropW, tt.cropH)
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("got %dx%d, want %dx%d", w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestBuildCompositeCommand_TestSourceFallbackResolution(t *testing.T) {
	p := baseCompositeParams()
	p.Inputs = []CompositeInput{
		{OverlayText: "NO SIGNAL", X: 0, Y: 0, Width: 768, Height: 540},
	}

	cmd := BuildCompositeCommand(p)

	// Should use slot dimensions as fallback for test source resolution
	if !strings.Contains(cmd, "testsrc2=size=768x540:rate=30") {
		t.Errorf("expected fallback to slot dimensions for test source, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_HevcVaapi_EmitsDumpExtraBsf verifies that HEVC
// hardware encoders get -bsf:v dump_extra=freq=keyframe so MPEG-TS / SRT
// late-joiners can decode without waiting for the encoder to spontaneously
// re-emit VPS/SPS/PPS.
func TestBuildCompositeCommand_HevcVaapi_EmitsDumpExtraBsf(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "hevc_vaapi"
	p.HWBackend = "vaapi"
	p.GlobalArgs = []string{"-vaapi_device", "/dev/dri/renderD128"}
	p.HWCaps = HWCapabilities{ScaleVAAPI: true}
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30", X: 0, Y: 0, Width: 1920, Height: 1080},
	}

	cmd := BuildCompositeCommand(p)

	if !strings.Contains(cmd, " -bsf:v dump_extra=freq=keyframe") {
		t.Errorf("expected -bsf:v dump_extra=freq=keyframe for hevc_vaapi, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_DumpExtraBsf_SkippedForNonH264OrHevc confirms the
// bsf is gated to H.264/HEVC encoders so we don't break unrelated codecs.
func TestBuildCompositeCommand_DumpExtraBsf_SkippedForNonH264OrHevc(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "av1_nvenc"
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30", X: 0, Y: 0, Width: 1920, Height: 1080},
	}

	cmd := BuildCompositeCommand(p)

	if strings.Contains(cmd, "dump_extra") {
		t.Errorf("did not expect dump_extra bsf for av1_nvenc, got:\n%s", cmd)
	}
}

// TestBuildCommand_LibX265_EmitsDumpExtraBsf verifies the single-stream
// builder also tags HEVC encoders with the dump_extra bsf.
func TestBuildCommand_LibX265_EmitsDumpExtraBsf(t *testing.T) {
	p := &Params{
		DevicePath:  "/dev/video0",
		InputFormat: "mjpeg",
		Resolution:  "1920x1080",
		FPS:         "30",
		Encoder:     "libx265",
		Bitrate:     "5.0M",
		BFrames:     -1,
		OutputURL:   "srt://localhost:6001?streamid=test",
	}

	cmd := BuildCommand(p)

	if !strings.Contains(cmd, " -bsf:v dump_extra=freq=keyframe") {
		t.Errorf("expected -bsf:v dump_extra=freq=keyframe for libx265, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_HevcVaapi_PadsWhenAlignmentBug verifies the
// AMD VCN green-band workaround: when the validator marks
// HevcVaapiNeedsAlignedHeight, the SW canvas is padded to a 16-aligned
// height with black before hwupload so the encoder's CTU-padding region
// carries valid pixel data.
func TestBuildCompositeCommand_HevcVaapi_PadsWhenAlignmentBug(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "hevc_vaapi"
	p.HWBackend = "vaapi"
	p.GlobalArgs = []string{"-vaapi_device", "/dev/dri/renderD128"}
	p.HWCaps = HWCapabilities{HevcVaapiNeedsAlignedHeight: true}
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30", X: 0, Y: 0, Width: 1920, Height: 1080},
	}

	cmd := BuildCompositeCommand(p)

	if !strings.Contains(cmd, "format=nv12,pad=1920:1088:0:0:black,hwupload[vout]") {
		t.Errorf("expected SW pad to 1088 before hwupload, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_HevcVaapi_NoPadByDefault confirms that without
// the alignment cap flag set, the historical hwupload-only shape is preserved
// (so we don't silently emit 1088-tall streams on hosts that don't need it).
func TestBuildCompositeCommand_HevcVaapi_NoPadByDefault(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "hevc_vaapi"
	p.HWBackend = "vaapi"
	p.GlobalArgs = []string{"-vaapi_device", "/dev/dri/renderD128"}
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30", X: 0, Y: 0, Width: 1920, Height: 1080},
	}

	cmd := BuildCompositeCommand(p)

	if strings.Contains(cmd, "pad=1920:1088") {
		t.Errorf("did not expect alignment pad without HevcVaapiNeedsAlignedHeight, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "format=nv12,hwupload[vout]") {
		t.Errorf("expected plain hwupload tail, got:\n%s", cmd)
	}
}

// TestBuildCompositeCommand_PadGate_SkippedForH264 confirms the alignment
// pad fires only for HEVC encoders even when the flag is set — H.264 doesn't
// trip the bug on AMD.
func TestBuildCompositeCommand_PadGate_SkippedForH264(t *testing.T) {
	p := baseCompositeParams()
	p.Encoder = "h264_vaapi"
	p.HWBackend = "vaapi"
	p.GlobalArgs = []string{"-vaapi_device", "/dev/dri/renderD128"}
	p.HWCaps = HWCapabilities{HevcVaapiNeedsAlignedHeight: true}
	p.Inputs = []CompositeInput{
		{DevicePath: "/dev/video0", InputFormat: "nv12", Resolution: "1920x1080", FPS: "30", X: 0, Y: 0, Width: 1920, Height: 1080},
	}

	cmd := BuildCompositeCommand(p)

	if strings.Contains(cmd, "pad=1920:1088") {
		t.Errorf("did not expect alignment pad for h264_vaapi, got:\n%s", cmd)
	}
}

// TestNeedsAlignedHevcHeight covers the capability gate.
func TestNeedsAlignedHevcHeight(t *testing.T) {
	tests := []struct {
		name    string
		caps    HWCapabilities
		backend string
		encoder string
		want    bool
	}{
		{"vaapi hevc bug set", HWCapabilities{HevcVaapiNeedsAlignedHeight: true}, "vaapi", "hevc_vaapi", true},
		{"vaapi hevc bug unset", HWCapabilities{}, "vaapi", "hevc_vaapi", false},
		{"vaapi h264 with bug flag", HWCapabilities{HevcVaapiNeedsAlignedHeight: true}, "vaapi", "h264_vaapi", false},
		{"rkmpp hevc bug set", HWCapabilities{HevcRkmppNeedsAlignedHeight: true}, "rkmpp", "hevc_rkmpp", true},
		{"rkmpp hevc clean", HWCapabilities{}, "rkmpp", "hevc_rkmpp", false},
		{"sw backend", HWCapabilities{HevcVaapiNeedsAlignedHeight: true}, "sw", "libx265", false},
		{"empty encoder", HWCapabilities{HevcVaapiNeedsAlignedHeight: true}, "vaapi", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.caps.NeedsAlignedHevcHeight(tt.backend, tt.encoder); got != tt.want {
				t.Errorf("NeedsAlignedHevcHeight(%q,%q) = %v, want %v", tt.backend, tt.encoder, got, tt.want)
			}
		})
	}
}

// TestIsHevcOrH264 covers the encoder family detector used to gate the
// dump_extra bsf insertion.
func TestIsHevcOrH264(t *testing.T) {
	tests := []struct {
		encoder string
		want    bool
	}{
		{"libx264", true},
		{"libx265", true},
		{"h264_vaapi", true},
		{"hevc_vaapi", true},
		{"h264_rkmpp", true},
		{"hevc_rkmpp", true},
		{"h264_qsv", true},
		{"hevc_nvenc", true},
		{"mjpeg", false},
		{"av1_nvenc", false},
		{"vp9_vaapi", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.encoder, func(t *testing.T) {
			if got := isHevcOrH264(tt.encoder); got != tt.want {
				t.Errorf("isHevcOrH264(%q) = %v, want %v", tt.encoder, got, tt.want)
			}
		})
	}
}
