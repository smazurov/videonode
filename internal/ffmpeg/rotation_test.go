package ffmpeg

import (
	"strings"
	"testing"
)

func TestBuildCommand_Rotation_Software(t *testing.T) {
	tests := []struct {
		name     string
		rotation int
		want     string
		notWant  string
	}{
		{"0 emits no transpose", 0, "", "transpose"},
		{"90", 90, "transpose=1", ""},
		{"180", 180, "transpose=1,transpose=1", ""},
		{"270", 270, "transpose=2", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Params{
				DevicePath:  "/dev/video0",
				InputFormat: "yuyv422",
				Resolution:  "1920x1080",
				FPS:         "30",
				Encoder:     "libx264",
				Preset:      "fast",
				Bitrate:     "2M",
				Rotation:    tt.rotation,
				OutputURL:   "rtsp://127.0.0.1:8554/test",
			}
			cmd := BuildCommand(p)

			if tt.want != "" && !strings.Contains(cmd, tt.want) {
				t.Errorf("expected %q in command\ncmd: %s", tt.want, cmd)
			}
			if tt.notWant != "" && strings.Contains(cmd, tt.notWant) {
				t.Errorf("did not expect %q in command\ncmd: %s", tt.notWant, cmd)
			}
			if strings.Contains(cmd, "transpose_vaapi") || strings.Contains(cmd, "vpp_rkrga") {
				t.Errorf("software path must not use hw transpose filters\ncmd: %s", cmd)
			}
		})
	}
}

func TestBuildCommand_Rotation_VAAPI_HWInput(t *testing.T) {
	tests := []struct {
		name     string
		rotation int
		wantDir  string
	}{
		{"90", 90, "clock"},
		{"180", 180, "reversal"},
		{"270", 270, "cclock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Params{
				DevicePath:   "/dev/video0",
				InputFormat:  "h264",
				Resolution:   "1920x1080",
				FPS:          "30",
				Encoder:      "h264_vaapi",
				Bitrate:      "2M",
				GlobalArgs:   []string{"-vaapi_device", "/dev/dri/renderD128", "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi"},
				VideoFilters: "scale_vaapi=format=nv12",
				Rotation:     tt.rotation,
				OutputURL:    "rtsp://127.0.0.1:8554/test",
			}
			cmd := BuildCommand(p)

			// -hwaccel must NOT be stripped — rotation uses a hw filter.
			if !strings.Contains(cmd, "-hwaccel vaapi") {
				t.Errorf("-hwaccel vaapi should be preserved for hw rotation\ncmd: %s", cmd)
			}
			if !strings.Contains(cmd, "-hwaccel_output_format vaapi") {
				t.Errorf("-hwaccel_output_format vaapi should be preserved\ncmd: %s", cmd)
			}
			if !strings.Contains(cmd, "-vaapi_device /dev/dri/renderD128") {
				t.Errorf("-vaapi_device must be retained\ncmd: %s", cmd)
			}

			// Hardware transpose filter present.
			wantFilter := "transpose_vaapi=dir=" + tt.wantDir
			if !strings.Contains(cmd, wantFilter) {
				t.Errorf("expected %q in command\ncmd: %s", wantFilter, cmd)
			}

			// Existing scale_vaapi is preserved (not dropped).
			if !strings.Contains(cmd, "scale_vaapi=format=nv12") {
				t.Errorf("scale_vaapi must be preserved alongside hw transpose\ncmd: %s", cmd)
			}

			// transpose_vaapi must come before scale_vaapi in the -vf chain.
			vfStart := strings.Index(cmd, "-vf ")
			if vfStart == -1 {
				t.Fatalf("expected -vf in command\ncmd: %s", cmd)
			}
			vf := cmd[vfStart:]
			tIdx := strings.Index(vf, "transpose_vaapi")
			sIdx := strings.Index(vf, "scale_vaapi")
			if tIdx == -1 || sIdx == -1 || tIdx >= sIdx {
				t.Errorf("transpose_vaapi must precede scale_vaapi\n-vf: %s", vf)
			}

			// Software transpose must not appear.
			if strings.Contains(cmd, ",transpose=") || strings.Contains(cmd, " transpose=") {
				t.Errorf("software transpose should not appear in hw-input path\ncmd: %s", cmd)
			}
		})
	}
}

func TestBuildCommand_Rotation_VAAPI_SWInput(t *testing.T) {
	p := &Params{
		DevicePath:   "/dev/video0",
		InputFormat:  "yuyv422",
		Resolution:   "1920x1080",
		FPS:          "30",
		Encoder:      "h264_vaapi",
		Bitrate:      "2M",
		GlobalArgs:   []string{"-vaapi_device", "/dev/dri/renderD128"},
		VideoFilters: "format=yuv420p,format=nv12,hwupload",
		Rotation:     90,
		OutputURL:    "rtsp://127.0.0.1:8554/test",
	}
	cmd := BuildCommand(p)

	if strings.Contains(cmd, "transpose_vaapi") {
		t.Errorf("sw-input path must not use transpose_vaapi\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "transpose=1") {
		t.Errorf("expected software transpose=1\ncmd: %s", cmd)
	}
	// The original VideoFilters (ending in hwupload) must still be present.
	if !strings.Contains(cmd, "format=nv12,hwupload") {
		t.Errorf("original hwupload tail must be preserved\ncmd: %s", cmd)
	}
	// Software transpose must run BEFORE hwupload.
	vf := vfSection(t, cmd)
	tIdx := strings.Index(vf, "transpose=1")
	hwIdx := strings.Index(vf, "hwupload")
	if tIdx == -1 || hwIdx == -1 || tIdx >= hwIdx {
		t.Errorf("transpose must precede hwupload\n-vf: %s", vf)
	}
}

// vfSection extracts everything from "-vf " onward. Fails the test if the
// command has no -vf argument.
func vfSection(t *testing.T, cmd string) string {
	t.Helper()
	idx := strings.Index(cmd, "-vf ")
	if idx == -1 {
		t.Fatalf("expected -vf in command\ncmd: %s", cmd)
	}
	return cmd[idx:]
}

func TestBuildCommand_Rotation_RKMPP_HWInput(t *testing.T) {
	tests := []struct {
		name     string
		rotation int
		wantDir  string
	}{
		{"90", 90, "clock"},
		{"180", 180, "reversal"},
		{"270", 270, "cclock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Params{
				DevicePath:   "/dev/video0",
				InputFormat:  "h264",
				Resolution:   "1920x1080",
				FPS:          "30",
				Encoder:      "h264_rkmpp",
				Bitrate:      "2M",
				GlobalArgs:   []string{"-hwaccel", "rkmpp", "-hwaccel_output_format", "drm_prime"},
				VideoFilters: "",
				Rotation:     tt.rotation,
				OutputURL:    "rtsp://127.0.0.1:8554/test",
			}
			cmd := BuildCommand(p)

			if !strings.Contains(cmd, "-hwaccel rkmpp") {
				t.Errorf("-hwaccel rkmpp should be preserved\ncmd: %s", cmd)
			}
			if !strings.Contains(cmd, "-hwaccel_output_format drm_prime") {
				t.Errorf("-hwaccel_output_format drm_prime should be preserved\ncmd: %s", cmd)
			}

			wantFilter := "vpp_rkrga=transpose=" + tt.wantDir
			if !strings.Contains(cmd, wantFilter) {
				t.Errorf("expected %q in command\ncmd: %s", wantFilter, cmd)
			}

			if strings.Contains(cmd, "transpose_vaapi") {
				t.Errorf("rkmpp path must not use transpose_vaapi\ncmd: %s", cmd)
			}
		})
	}
}

func TestBuildCommand_Rotation_RKMPP_SWInput(t *testing.T) {
	p := &Params{
		DevicePath:   "/dev/video0",
		InputFormat:  "yuyv422",
		Resolution:   "1920x1080",
		FPS:          "30",
		Encoder:      "h264_rkmpp",
		Bitrate:      "2M",
		GlobalArgs:   []string{"-init_hw_device", "rkmpp=hw", "-filter_hw_device", "hw"},
		VideoFilters: "hwupload,scale_rkrga=format=nv12:afbc=0",
		Rotation:     90,
		OutputURL:    "rtsp://127.0.0.1:8554/test",
	}
	cmd := BuildCommand(p)

	if strings.Contains(cmd, "vpp_rkrga") {
		t.Errorf("sw-input rkmpp path must not use vpp_rkrga\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "transpose=1") {
		t.Errorf("expected software transpose=1\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "-init_hw_device rkmpp=hw") {
		t.Errorf("-init_hw_device must be retained\ncmd: %s", cmd)
	}
	// Original VideoFilters must be preserved after sw transpose.
	if !strings.Contains(cmd, "hwupload,scale_rkrga=format=nv12:afbc=0") {
		t.Errorf("original scale_rkrga chain must be preserved\ncmd: %s", cmd)
	}
	// Software transpose runs before hwupload/scale_rkrga.
	vf := vfSection(t, cmd)
	tIdx := strings.Index(vf, "transpose=1")
	hwIdx := strings.Index(vf, "hwupload")
	if tIdx == -1 || hwIdx == -1 || tIdx >= hwIdx {
		t.Errorf("transpose must precede hwupload,scale_rkrga\n-vf: %s", vf)
	}
}

func TestBuildCommand_Rotation_WithPerspective_VAAPI(t *testing.T) {
	// Perspective + rotation on a vaapi hw-decode stream. Perspective strips
	// -hwaccel (forcing software decode), rotation uses software transpose, and
	// the tail shim replaces scale_vaapi with format=nv12,hwupload.
	p := &Params{
		DevicePath:   "/dev/video0",
		InputFormat:  "h264",
		Resolution:   "1920x1080",
		FPS:          "30",
		Encoder:      "h264_vaapi",
		Bitrate:      "2M",
		GlobalArgs:   []string{"-vaapi_device", "/dev/dri/renderD128", "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi"},
		VideoFilters: "scale_vaapi=format=nv12",
		Perspective: &PerspectiveConfig{
			Corners: [4][2]int{{100, 50}, {1800, 50}, {1800, 1000}, {100, 1000}},
		},
		Rotation:  90,
		OutputURL: "rtsp://127.0.0.1:8554/test",
	}
	cmd := BuildCommand(p)

	if strings.Contains(cmd, "-hwaccel vaapi") {
		t.Errorf("perspective should strip -hwaccel vaapi\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "-vaapi_device /dev/dri/renderD128") {
		t.Errorf("-vaapi_device must be retained\ncmd: %s", cmd)
	}
	if strings.Contains(cmd, "transpose_vaapi") {
		t.Errorf("sw-forced path must not use transpose_vaapi\ncmd: %s", cmd)
	}
	// Software transpose and final hwupload shim.
	if !strings.Contains(cmd, "transpose=1") {
		t.Errorf("expected software transpose=1\ncmd: %s", cmd)
	}
	if !strings.Contains(cmd, "format=nv12,hwupload") {
		t.Errorf("expected hwupload shim\ncmd: %s", cmd)
	}
	if strings.Contains(cmd, "scale_vaapi") {
		t.Errorf("scale_vaapi should be dropped when perspective forces sw decode\ncmd: %s", cmd)
	}

	// Order: perspective → transpose → hwupload
	vf := vfSection(t, cmd)
	pIdx := strings.Index(vf, "perspective=")
	tIdx := strings.Index(vf, "transpose=1")
	hwIdx := strings.Index(vf, "hwupload")
	if pIdx == -1 || tIdx == -1 || hwIdx == -1 {
		t.Fatalf("expected perspective, transpose, hwupload all present\n-vf: %s", vf)
	}
	if pIdx >= tIdx || tIdx >= hwIdx {
		t.Errorf("expected order perspective < transpose < hwupload; got %d, %d, %d\n-vf: %s",
			pIdx, tIdx, hwIdx, vf)
	}
}
