package ffmpeg

import (
	"fmt"
	"strings"
	"testing"
)

func TestPerspectiveFilterString(t *testing.T) {
	p := &PerspectiveConfig{
		Corners: [4][2]int{{120, 45}, {1800, 60}, {1850, 1035}, {100, 1020}},
	}
	got := perspectiveFilterString(p)
	// Corners stored clockwise: TL, TR, BR, BL → FFmpeg order: TL, TR, BL, BR
	want := "perspective=x0=120:y0=45:x1=1800:y1=60:x2=100:y2=1020:x3=1850:y3=1035:sense=source:interpolation=linear"
	if got != want {
		t.Errorf("perspectiveFilterString() =\n  %s\nwant\n  %s", got, want)
	}
}

func TestPerspectiveFilterString_Zeros(t *testing.T) {
	p := &PerspectiveConfig{Corners: [4][2]int{}}
	got := perspectiveFilterString(p)
	if !strings.HasPrefix(got, "perspective=x0=0:y0=0") {
		t.Errorf("zero corners should produce valid filter, got: %s", got)
	}
}

func TestBuildCommand_VisionEnabled(t *testing.T) {
	p := &Params{
		DevicePath:    "/dev/video0",
		InputFormat:   "mjpeg",
		Resolution:    "1920x1080",
		FPS:           "30",
		Encoder:       "libx264",
		Preset:        "fast",
		Bitrate:       "2M",
		VisionEnabled: true,
		VisionWidth:   640,
		VisionHeight:  480,
		OutputURL:     "rtsp://127.0.0.1:8554/test",
	}
	cmd := BuildCommand(p)

	checks := []struct {
		name string
		want string
	}{
		{"has filter_complex", "-filter_complex"},
		{"has split", "split=2[enc][raw]"},
		{"has raw scale", "scale=640:480,format=nv12[rawout]"},
		{"maps encout", "-map \"[encout]\""},
		{"maps rawout to pipe:3", "-map \"[rawout]\" -f rawvideo -pix_fmt nv12 pipe:3"},
	}
	for _, tc := range checks {
		if !strings.Contains(cmd, tc.want) {
			t.Errorf("%s: command missing %q\ncmd: %s", tc.name, tc.want, cmd)
		}
	}

	// Should NOT have simple -vf
	if strings.Contains(cmd, " -vf ") {
		t.Errorf("vision command should use filter_complex, not -vf\ncmd: %s", cmd)
	}
}

func TestBuildCommand_VisionDisabled(t *testing.T) {
	p := &Params{
		DevicePath:    "/dev/video0",
		InputFormat:   "mjpeg",
		Resolution:    "1920x1080",
		FPS:           "30",
		Encoder:       "libx264",
		Preset:        "fast",
		Bitrate:       "2M",
		VideoFilters:  "format=nv12",
		VisionEnabled: false,
		OutputURL:     "rtsp://127.0.0.1:8554/test",
	}
	cmd := BuildCommand(p)

	if strings.Contains(cmd, "filter_complex") {
		t.Errorf("non-vision command should not use filter_complex\ncmd: %s", cmd)
	}
	if strings.Contains(cmd, "pipe:3") {
		t.Errorf("non-vision command should not have pipe:3\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "-vf format=nv12") {
		t.Errorf("non-vision command should use simple -vf\ncmd: %s", cmd)
	}
}

func TestBuildCommand_VisionWithVideoFilters(t *testing.T) {
	p := &Params{
		DevicePath:    "/dev/video0",
		InputFormat:   "mjpeg",
		Resolution:    "1920x1080",
		FPS:           "30",
		Encoder:       "h264_rkmpp",
		VideoFilters:  "format=nv12",
		VisionEnabled: true,
		VisionWidth:   320,
		VisionHeight:  240,
		OutputURL:     "rtsp://127.0.0.1:8554/test",
	}
	cmd := BuildCommand(p)

	// Encode branch should have the existing video filters
	if !strings.Contains(cmd, "[enc]format=nv12[encout]") {
		t.Errorf("encode branch should include existing video filters\ncmd: %s", cmd)
	}
	// Raw branch should have the vision dimensions
	if !strings.Contains(cmd, "scale=320:240") {
		t.Errorf("raw branch should use custom vision dimensions\ncmd: %s", cmd)
	}
}

func TestBuildCommand_PerspectiveOnly(t *testing.T) {
	p := &Params{
		DevicePath:  "/dev/video0",
		InputFormat: "mjpeg",
		Resolution:  "1920x1080",
		FPS:         "30",
		Encoder:     "libx264",
		Preset:      "fast",
		Bitrate:     "2M",
		Perspective: &PerspectiveConfig{
			Corners: [4][2]int{{120, 45}, {1800, 60}, {1850, 1035}, {100, 1020}},
		},
		OutputURL: "rtsp://127.0.0.1:8554/test",
	}
	cmd := BuildCommand(p)

	// Without vision, perspective should be in simple -vf chain (with format=yuv420p before it)
	if !strings.Contains(cmd, "-vf format=yuv420p,perspective=") {
		t.Errorf("perspective-only should use -vf with format=yuv420p before perspective\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "x0=120:y0=45") {
		t.Errorf("perspective filter missing correct corners\ncmd: %s", cmd)
	}
	if strings.Contains(cmd, "pipe:3") {
		t.Errorf("perspective-only should not have pipe:3\ncmd: %s", cmd)
	}
}

func TestBuildCommand_VisionWithPerspective(t *testing.T) {
	p := &Params{
		DevicePath:    "/dev/video0",
		InputFormat:   "mjpeg",
		Resolution:    "1920x1080",
		FPS:           "30",
		Encoder:       "h264_rkmpp",
		VideoFilters:  "format=nv12",
		VisionEnabled: true,
		VisionWidth:   640,
		VisionHeight:  480,
		Perspective: &PerspectiveConfig{
			Corners: [4][2]int{{120, 45}, {1800, 60}, {1850, 1035}, {100, 1020}},
		},
		OutputURL: "rtsp://127.0.0.1:8554/test",
	}
	cmd := BuildCommand(p)

	// Perspective should be on the encode branch with format=yuv420p before it
	if !strings.Contains(cmd, "[enc]format=yuv420p,perspective=") {
		t.Errorf("perspective should be on encode branch with format=yuv420p\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "format=nv12[encout]") {
		t.Errorf("existing filters should follow perspective on encode branch\ncmd: %s", cmd)
	}
	// Raw branch should NOT have perspective
	if strings.Contains(cmd, "[raw]perspective") {
		t.Errorf("raw branch should not have perspective filter\ncmd: %s", cmd)
	}
	// Raw pipe should still be present
	if !strings.Contains(cmd, "pipe:3") {
		t.Errorf("vision pipe should be present\ncmd: %s", cmd)
	}
}

func TestBuildCommand_VisionWithAudio(t *testing.T) {
	p := &Params{
		DevicePath:    "/dev/video0",
		InputFormat:   "mjpeg",
		Resolution:    "1920x1080",
		FPS:           "30",
		Encoder:       "libx264",
		Preset:        "fast",
		Bitrate:       "2M",
		AudioDevice:   "hw:4,0",
		VisionEnabled: true,
		VisionWidth:   640,
		VisionHeight:  480,
		OutputURL:     "rtsp://127.0.0.1:8554/test",
	}
	cmd := BuildCommand(p)

	// Audio should map 1:a but NOT map 0:v (video is mapped via [encout])
	if !strings.Contains(cmd, "-map 1:a") {
		t.Errorf("should map audio input\ncmd: %s", cmd)
	}
	if strings.Contains(cmd, "-map 0:v") {
		t.Errorf("should not map 0:v when vision enabled (uses [encout])\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "-map \"[encout]\"") {
		t.Errorf("should map [encout] for video\ncmd: %s", cmd)
	}
}

func TestBuildCommand_VisionDefaultDimensions(t *testing.T) {
	p := &Params{
		DevicePath:    "/dev/video0",
		InputFormat:   "mjpeg",
		Resolution:    "1920x1080",
		FPS:           "30",
		Encoder:       "libx264",
		Preset:        "fast",
		VisionEnabled: true,
		OutputURL:     "rtsp://127.0.0.1:8554/test",
	}
	cmd := BuildCommand(p)

	if !strings.Contains(cmd, "scale=640:480") {
		t.Errorf("should default to 640x480\ncmd: %s", cmd)
	}
}

func TestBuildCommand_PerspectiveStripsHWAccel(t *testing.T) {
	p := &Params{
		DevicePath:   "/dev/video0",
		InputFormat:  "h264",
		Resolution:   "1920x1080",
		FPS:          "30",
		Encoder:      "h264_vaapi",
		GlobalArgs:   []string{"-vaapi_device", "/dev/dri/renderD128", "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi"},
		VideoFilters: "scale_vaapi=format=nv12",
		Perspective: &PerspectiveConfig{
			Corners: [4][2]int{{120, 45}, {1800, 60}, {1850, 1035}, {100, 1020}},
		},
		OutputURL: "rtsp://127.0.0.1:8554/test",
	}
	cmd := BuildCommand(p)

	// Should keep -vaapi_device but strip -hwaccel and -hwaccel_output_format
	if !strings.Contains(cmd, "-vaapi_device /dev/dri/renderD128") {
		t.Errorf("should keep -vaapi_device\ncmd: %s", cmd)
	}
	if strings.Contains(cmd, "-hwaccel vaapi") {
		t.Errorf("should strip -hwaccel when perspective active\ncmd: %s", cmd)
	}
	if strings.Contains(cmd, "-hwaccel_output_format") {
		t.Errorf("should strip -hwaccel_output_format when perspective active\ncmd: %s", cmd)
	}
	// Should replace scale_vaapi with software perspective + hwupload
	if strings.Contains(cmd, "scale_vaapi") {
		t.Errorf("should not have scale_vaapi when perspective active\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "format=nv12,hwupload") {
		t.Errorf("should have format=nv12,hwupload after perspective\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "perspective=x0=120") {
		t.Errorf("should have perspective filter\ncmd: %s", cmd)
	}
}

func TestBuildCompositeCommand_PerInputVision(t *testing.T) {
	p := baseCompositeParams()
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "nv12",
			Resolution: "1920x1080", FPS: "60",
			X: 0, Y: 0, Width: 960, Height: 1080,
			VisionEnabled: true, VisionWidth: 640, VisionHeight: 480,
		},
		{
			DevicePath: "/dev/video1", InputFormat: "mjpeg",
			Resolution: "1920x1080", FPS: "30",
			X: 960, Y: 0, Width: 960, Height: 1080,
		},
	}
	cmd := BuildCompositeCommand(p)

	// Input 0 should have split
	if !strings.Contains(cmd, "split=2[raw0][enc0]") {
		t.Errorf("input 0 should have split\ncmd: %s", cmd)
	}
	// Input 0 should have raw output (setpts rebases V4L2 wallclock PTS so
	// downstream filters can normalize timestamps).
	if !strings.Contains(cmd, "[raw0]setpts=PTS-STARTPTS,scale=640:480,format=nv12[rawout0]") {
		t.Errorf("input 0 should have raw output\ncmd: %s", cmd)
	}
	// Input 0 raw pipe
	if !strings.Contains(cmd, "-map \"[rawout0]\" -f rawvideo -pix_fmt nv12 pipe:3") {
		t.Errorf("should have pipe:3 for input 0\ncmd: %s", cmd)
	}
	// Input 1 should NOT have split
	if strings.Contains(cmd, "[raw1]") {
		t.Errorf("input 1 should not have split\ncmd: %s", cmd)
	}
	// Should NOT have pipe:4
	if strings.Contains(cmd, "pipe:4") {
		t.Errorf("should not have pipe:4 (only input 0 has vision)\ncmd: %s", cmd)
	}
}

func TestBuildCompositeCommand_PerInputPerspective(t *testing.T) {
	p := baseCompositeParams()
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "nv12",
			Resolution: "1920x1080", FPS: "60",
			X: 0, Y: 0, Width: 960, Height: 1080,
			Perspective: &PerspectiveConfig{
				Corners: [4][2]int{{100, 50}, {1820, 50}, {1820, 1030}, {100, 1030}},
			},
		},
		{
			DevicePath: "/dev/video1", InputFormat: "mjpeg",
			Resolution: "1920x1080", FPS: "30",
			X: 960, Y: 0, Width: 960, Height: 1080,
		},
	}
	cmd := BuildCompositeCommand(p)

	// Input 0 should have perspective filter
	if !strings.Contains(cmd, "perspective=x0=100:y0=50") {
		t.Errorf("input 0 should have perspective filter\ncmd: %s", cmd)
	}
	// Input 1 should NOT have perspective
	parts := strings.SplitN(cmd, "perspective=", 2)
	if len(parts) > 1 && strings.Contains(parts[1], "perspective=") {
		t.Errorf("only input 0 should have perspective\ncmd: %s", cmd)
	}
}

func TestBuildCompositeCommand_BothInputsVision(t *testing.T) {
	p := baseCompositeParams()
	p.Inputs = []CompositeInput{
		{
			DevicePath: "/dev/video0", InputFormat: "nv12",
			Resolution: "1920x1080", FPS: "60",
			X: 0, Y: 0, Width: 960, Height: 1080,
			VisionEnabled: true, VisionWidth: 640, VisionHeight: 480,
		},
		{
			DevicePath: "/dev/video1", InputFormat: "mjpeg",
			Resolution: "1920x1080", FPS: "30",
			X: 960, Y: 0, Width: 960, Height: 1080,
			VisionEnabled: true, VisionWidth: 320, VisionHeight: 240,
		},
	}
	cmd := BuildCompositeCommand(p)

	if !strings.Contains(cmd, "pipe:3") {
		t.Errorf("should have pipe:3 for input 0\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "pipe:4") {
		t.Errorf("should have pipe:4 for input 1\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "scale=320:240") {
		t.Errorf("input 1 should use its own vision dimensions\ncmd: %s", cmd)
	}
}

// TestBuildCompositeCommand_FourSourceCanvasEmitsPipe3To6 locks in the
// invariant that streamProcessManager.setupCanvasVisionPipes relies on:
// every vision-enabled source gets its own pipe:N output, indexed from 3.
// If this test ever fails, the per-source pipe readers in process_manager.go
// will block forever on FDs the ffmpeg process never writes to.
func TestBuildCompositeCommand_FourSourceCanvasEmitsPipe3To6(t *testing.T) {
	p := baseCompositeParams()
	p.Inputs = make([]CompositeInput, 4)
	for i := range p.Inputs {
		p.Inputs[i] = CompositeInput{
			DevicePath: "/dev/video0", InputFormat: "nv12",
			Resolution: "1920x1080", FPS: "60",
			X: 0, Y: 0, Width: 960, Height: 540,
			VisionEnabled: true, VisionWidth: 640, VisionHeight: 480,
		}
	}
	cmd := BuildCompositeCommand(p)

	for n := 3; n <= 6; n++ {
		want := fmt.Sprintf("pipe:%d", n)
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %s in command for 4-source canvas\ncmd: %s", want, cmd)
		}
	}
	if strings.Contains(cmd, "pipe:7") {
		t.Errorf("unexpected pipe:7 (only 4 sources)\ncmd: %s", cmd)
	}
}
