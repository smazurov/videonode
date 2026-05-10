package streams

import (
	"strings"
	"testing"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/types"
)

// canvasProcessorWithSources builds a canvasProcessor wired against a mock
// store that already contains both the canvas stream and its sources, plus a
// fixed encoderSelector and deviceResolver. Used by tests to assert how the
// processor maps stream config into ffmpeg.CompositeParams.
func canvasProcessorWithSources(t *testing.T, canvasFPS int, sources []StreamSpec, canvas StreamSpec, defaultVisionFPS int, sel encoderSelector) *canvasProcessor {
	t.Helper()
	repo := &mockStore{streams: make(map[string]StreamSpec)}
	for _, s := range sources {
		if err := repo.AddStream(s); err != nil {
			t.Fatalf("AddStream(%s): %v", s.ID, err)
		}
	}
	if err := repo.AddStream(canvas); err != nil {
		t.Fatalf("AddStream(canvas): %v", err)
	}
	cp := newCanvasProcessor(repo)
	cp.encoderSelector = sel
	cp.deviceResolver = func(d string) string { return d }
	cp.getStreamState = func(id string) (*Stream, bool) {
		enabled := map[string]bool{}
		for _, s := range sources {
			enabled[s.ID] = true
		}
		return &Stream{ID: id, Enabled: true, InputsEnabled: enabled}, true
	}
	cp.defaultVisionFPS = defaultVisionFPS
	_ = canvasFPS
	return cp
}

func TestCanvasProcessor_HWBackendAndVisionFPS_Defaults(t *testing.T) {
	src := StreamSpec{
		ID:     "cam0",
		Device: "/dev/video0",
		FFmpeg: FFmpegConfig{
			Codec:       "h264",
			InputFormat: "h264",
			Resolution:  "1280x720",
			FPS:         "30",
		},
	}
	canvas := StreamSpec{
		ID: "canvas0",
		FFmpeg: FFmpegConfig{
			Codec: "h264",
			FPS:   "30",
		},
		Canvas: &CanvasConfig{
			Width:         1920,
			Height:        1080,
			FPS:           "30",
			KeyColor:      "0x000000",
			SourceStreams: []string{"cam0"},
		},
	}
	sel := func(_ string, _ string, _ *types.QualityParams, _ string) *ffmpeg.Params {
		return &ffmpeg.Params{Encoder: "h264_rkmpp", HWBackend: "rkmpp"}
	}

	cp := canvasProcessorWithSources(t, 30, []StreamSpec{src}, canvas, 10, sel)
	processed, err := cp.processStream("canvas0")
	if err != nil {
		t.Fatalf("processStream: %v", err)
	}

	cmd := processed.FFmpegCommand
	if !strings.Contains(cmd, "fps=10,") {
		t.Errorf("expected default vision fps=10 to appear in vision branch, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "-c:v h264_rkmpp") {
		t.Errorf("expected h264_rkmpp encoder, got:\n%s", cmd)
	}
}

// TestCanvasProcessor_HWCaps_PartialVAAPI pins the radeonsi case: validation
// reports scale_vaapi + transpose_vaapi working, overlay_vaapi failed. The
// canvas builder must use HW scale/transpose on per-input chains and fall
// back to SW overlay (followed by hwupload) before encode.
func TestCanvasProcessor_HWCaps_PartialVAAPI(t *testing.T) {
	src := StreamSpec{
		ID:     "cam0",
		Device: "/dev/video0",
		FFmpeg: FFmpegConfig{
			Codec:       "h264",
			InputFormat: "h264",
			Resolution:  "1280x720",
			FPS:         "30",
		},
	}
	canvas := StreamSpec{
		ID:     "canvas0",
		FFmpeg: FFmpegConfig{Codec: "h264", FPS: "30"},
		Canvas: &CanvasConfig{
			Width:         1920,
			Height:        1080,
			FPS:           "30",
			SourceStreams: []string{"cam0"},
		},
	}
	sel := func(_ string, _ string, _ *types.QualityParams, _ string) *ffmpeg.Params {
		return &ffmpeg.Params{
			Encoder:    "h264_vaapi",
			HWBackend:  "vaapi",
			GlobalArgs: []string{"-vaapi_device", "/dev/dri/renderD128"},
		}
	}

	cp := canvasProcessorWithSources(t, 30, []StreamSpec{src}, canvas, 10, sel)
	// Plumb validation results into the same mockStore the processor uses.
	repo := cp.store.(*mockStore)
	repo.validation = &types.ValidationResults{
		Backends: map[string]types.BackendValidation{
			"vaapi": {
				Filters: types.CodecValidation{
					Working: []string{"scale_vaapi", "transpose_vaapi"},
					Failed:  []string{"overlay_vaapi"},
				},
			},
		},
	}

	processed, err := cp.processStream("canvas0")
	if err != nil {
		t.Fatalf("processStream: %v", err)
	}
	cmd := processed.FFmpegCommand

	// Per-input chain stays on HW for scale.
	if !strings.Contains(cmd, "scale_vaapi=w=1920:h=1080:format=nv12") {
		t.Errorf("expected scale_vaapi on per-input chain, got:\n%s", cmd)
	}
	// Overlay falls back to SW.
	if strings.Contains(cmd, "overlay_vaapi") {
		t.Errorf("overlay_vaapi should have fallen back to SW, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "overlay=0:0,setpts=PTS-STARTPTS,fps=30,format=nv12,hwupload[vout]") {
		t.Errorf("expected SW overlay → setpts → fps → hwupload tail before encoder, got:\n%s", cmd)
	}
	// Canvas base is SW (no hwupload on the color source) since the overlay
	// runs in system memory.
	if !strings.Contains(cmd, "color=c=0x000000:s=1920x1080:r=30[canvas]") {
		t.Errorf("expected SW canvas base, got:\n%s", cmd)
	}
}

// TestCanvasProcessor_HWCaps_NoValidation pins the fresh-install case: the
// store has no validation data, so every HW flag is off and the canvas
// runs the all-software path.
func TestCanvasProcessor_HWCaps_NoValidation(t *testing.T) {
	src := StreamSpec{
		ID:     "cam0",
		Device: "/dev/video0",
		FFmpeg: FFmpegConfig{Codec: "h264", InputFormat: "h264", Resolution: "1280x720", FPS: "30"},
	}
	canvas := StreamSpec{
		ID:     "canvas0",
		FFmpeg: FFmpegConfig{Codec: "h264", FPS: "30"},
		Canvas: &CanvasConfig{
			Width:         1920,
			Height:        1080,
			FPS:           "30",
			SourceStreams: []string{"cam0"},
		},
	}
	sel := func(_ string, _ string, _ *types.QualityParams, _ string) *ffmpeg.Params {
		return &ffmpeg.Params{
			Encoder:   "h264_vaapi",
			HWBackend: "vaapi",
		}
	}

	cp := canvasProcessorWithSources(t, 30, []StreamSpec{src}, canvas, 10, sel)
	processed, err := cp.processStream("canvas0")
	if err != nil {
		t.Fatalf("processStream: %v", err)
	}
	cmd := processed.FFmpegCommand
	if strings.Contains(cmd, "scale_vaapi") || strings.Contains(cmd, "overlay_vaapi") {
		t.Errorf("no validation data → no HW filters, got:\n%s", cmd)
	}
}

// arPreservingCanvasSpec builds a 3-source canvas where the layout solver
// picks "asym-port-right": three 1920×1080 inputs with the third rotated 90°
// so its effective AR becomes 9:16. The resulting slot geometry is
//
//	a (idx 0): slot 1312×738 at (0,0)     — 16:9 fits perfectly, content == slot
//	b (idx 1): slot 1312×342 at (0,738)   — 16:9 letterboxed → content 608×342 at (352,738)
//	c (idx 2): slot 608×1080 at (1312,0)  — 9:16 fits perfectly, content == slot
//
// Input b is the AR-mismatch case: its 1312×342 slot has AR ~3.84:1 but the
// input is 16:9. A correct HW filter chain must scale to the content size
// (608×342) and overlay at the content position (352,738) — never to the
// stretched slot dimensions.
func arPreservingCanvasSpec() ([]StreamSpec, StreamSpec) {
	rot90 := 90
	sources := []StreamSpec{
		{ID: "a", Device: "/dev/videoa", FFmpeg: FFmpegConfig{Codec: "h264", InputFormat: "h264", Resolution: "1920x1080", FPS: "30"}},
		{ID: "b", Device: "/dev/videob", FFmpeg: FFmpegConfig{Codec: "h264", InputFormat: "h264", Resolution: "1920x1080", FPS: "30"}},
		{ID: "c", Device: "/dev/videoc", FFmpeg: FFmpegConfig{Codec: "h264", InputFormat: "h264", Resolution: "1920x1080", FPS: "30"}},
	}
	canvas := StreamSpec{
		ID:     "canvas0",
		FFmpeg: FFmpegConfig{Codec: "h264", FPS: "30"},
		Canvas: &CanvasConfig{
			Width:         1920,
			Height:        1080,
			FPS:           "30",
			SourceStreams: []string{"a", "b", "c"},
			SourceOverrides: []CanvasSourceOverride{
				{}, {}, {Rotation: &rot90},
			},
		},
	}
	return sources, canvas
}

// TestCanvasProcessor_VAAPIPath_PreservesAspectRatio pins the bug fix: HW
// filter chains must scale each input to its AR-correct content rectangle,
// not to the raw slot dimensions. Before the fix, scale_vaapi targeted the
// 1312×342 slot and stretched the 16:9 input to ~3.84:1.
func TestCanvasProcessor_VAAPIPath_PreservesAspectRatio(t *testing.T) {
	sources, canvas := arPreservingCanvasSpec()
	sel := func(_ string, _ string, _ *types.QualityParams, _ string) *ffmpeg.Params {
		return &ffmpeg.Params{
			Encoder:    "h264_vaapi",
			HWBackend:  "vaapi",
			GlobalArgs: []string{"-vaapi_device", "/dev/dri/renderD128"},
		}
	}

	cp := canvasProcessorWithSources(t, 30, sources, canvas, 10, sel)
	repo := cp.store.(*mockStore)
	repo.validation = &types.ValidationResults{
		Backends: map[string]types.BackendValidation{
			"vaapi": {
				Filters: types.CodecValidation{
					Working: []string{"scale_vaapi", "transpose_vaapi", "overlay_vaapi"},
				},
			},
		},
	}

	processed, err := cp.processStream("canvas0")
	if err != nil {
		t.Fatalf("processStream: %v", err)
	}
	cmd := processed.FFmpegCommand

	// Stretched form must NOT appear: scaling to slot dims squashes AR.
	if strings.Contains(cmd, "scale_vaapi=w=1312:h=342") {
		t.Errorf("HW chain scales to slot dims (1312×342), stretching the 16:9 input.\ngot:\n%s", cmd)
	}
	// AR-correct content size MUST appear for input b.
	if !strings.Contains(cmd, "scale_vaapi=w=608:h=342") {
		t.Errorf("expected AR-correct scale_vaapi=w=608:h=342 for input b.\ngot:\n%s", cmd)
	}
	// Overlay must use the centered content position (352, 738), not slot (0, 738).
	if strings.Contains(cmd, "overlay_vaapi=x=0:y=738") {
		t.Errorf("overlay_vaapi positioned at slot origin (0, 738) — should be content origin (352, 738).\ngot:\n%s", cmd)
	}
	if !strings.Contains(cmd, "overlay_vaapi=x=352:y=738") {
		t.Errorf("expected overlay_vaapi=x=352:y=738 for letterboxed input b.\ngot:\n%s", cmd)
	}
}

// TestCanvasProcessor_RKMPPPath_PreservesAspectRatio is the RK3588 mirror of
// the VAAPI test: scale_rkrga and overlay_rkrga must use content geometry,
// not slot geometry.
func TestCanvasProcessor_RKMPPPath_PreservesAspectRatio(t *testing.T) {
	sources, canvas := arPreservingCanvasSpec()
	sel := func(_ string, _ string, _ *types.QualityParams, _ string) *ffmpeg.Params {
		return &ffmpeg.Params{
			Encoder:   "h264_rkmpp",
			HWBackend: "rkmpp",
		}
	}

	cp := canvasProcessorWithSources(t, 30, sources, canvas, 10, sel)
	repo := cp.store.(*mockStore)
	repo.validation = &types.ValidationResults{
		Backends: map[string]types.BackendValidation{
			"rkmpp": {
				Filters: types.CodecValidation{
					Working: []string{"scale_rkrga", "vpp_rkrga", "overlay_rkrga"},
				},
			},
		},
	}

	processed, err := cp.processStream("canvas0")
	if err != nil {
		t.Fatalf("processStream: %v", err)
	}
	cmd := processed.FFmpegCommand

	if strings.Contains(cmd, "scale_rkrga=1312:342") {
		t.Errorf("HW chain scales to slot dims (1312×342), stretching the 16:9 input.\ngot:\n%s", cmd)
	}
	if !strings.Contains(cmd, "scale_rkrga=608:342") {
		t.Errorf("expected AR-correct scale_rkrga=608:342 for input b.\ngot:\n%s", cmd)
	}
	if strings.Contains(cmd, "overlay_rkrga=x=0:y=738") {
		t.Errorf("overlay_rkrga positioned at slot origin (0, 738) — should be content origin (352, 738).\ngot:\n%s", cmd)
	}
	if !strings.Contains(cmd, "overlay_rkrga=x=352:y=738") {
		t.Errorf("expected overlay_rkrga=x=352:y=738 for letterboxed input b.\ngot:\n%s", cmd)
	}
}

func TestCanvasProcessor_VisionFPS_PerSourceOverride(t *testing.T) {
	src := StreamSpec{
		ID:     "cam0",
		Device: "/dev/video0",
		FFmpeg: FFmpegConfig{
			Codec:       "h264",
			InputFormat: "h264",
			Resolution:  "1280x720",
			FPS:         "30",
		},
		Vision: &ffmpeg.VisionConfig{Enabled: true, Width: 320, Height: 240, FPS: 25},
	}
	canvas := StreamSpec{
		ID:     "canvas0",
		FFmpeg: FFmpegConfig{Codec: "h264", FPS: "30"},
		Canvas: &CanvasConfig{
			Width:         1920,
			Height:        1080,
			FPS:           "30",
			SourceStreams: []string{"cam0"},
		},
	}
	sel := func(_ string, _ string, _ *types.QualityParams, _ string) *ffmpeg.Params {
		return &ffmpeg.Params{Encoder: "libx264"}
	}

	cp := canvasProcessorWithSources(t, 30, []StreamSpec{src}, canvas, 10, sel)
	processed, err := cp.processStream("canvas0")
	if err != nil {
		t.Fatalf("processStream: %v", err)
	}
	if !strings.Contains(processed.FFmpegCommand, "fps=25,scale=320:240,format=nv12") {
		t.Errorf("expected per-source vision fps=25 to win, got:\n%s", processed.FFmpegCommand)
	}
}
