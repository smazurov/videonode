package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/types"
)

// setupTestStore creates a temporary store for testing.
func setupTestStore(t *testing.T) (*tomlStore, string) {
	t.Helper()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_streams.toml")

	return NewTOML(testFile).(*tomlStore), testFile
}

func TestNewTOML(t *testing.T) {
	repo := NewTOML("").(*tomlStore)
	if repo.configPath != "streams.toml" {
		t.Errorf("expected default path 'streams.toml', got %s", repo.configPath)
	}

	repo = NewTOML("/custom/path.toml").(*tomlStore)
	if repo.configPath != "/custom/path.toml" {
		t.Errorf("expected custom path '/custom/path.toml', got %s", repo.configPath)
	}

	if repo.config == nil {
		t.Error("config should be initialized")
	}
	if repo.config.Version != schemaVersion {
		t.Errorf("expected version %d, got %d", schemaVersion, repo.config.Version)
	}
}

// TestTOMLStore_ConcurrentAccess exercises the store under simultaneous
// readers and writers. The pipeline reads entity specs through the store
// off the service-layer mutex, so the store must be internally safe. Run
// with -race to surface a missing guard.
func TestTOMLStore_ConcurrentAccess(t *testing.T) {
	repo, _ := setupTestStore(t)
	if err := repo.AddComposer(V2Composer{ID: "c1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			_ = repo.UpdateComposer("c1", V2Composer{ID: "c1", Canvas: V2CanvasDims{W: n, H: n}})
		}(i)
		go func() {
			defer wg.Done()
			_, _ = repo.GetComposerEntity("c1")
		}()
	}
	wg.Wait()
}

func TestLoadNonExistentFile(t *testing.T) {
	repo, _ := setupTestStore(t)

	if err := repo.Load(); err != nil {
		t.Errorf("Load should not error on non-existent file, got: %v", err)
	}

	if len(repo.GetAllV2Streams()) != 0 {
		t.Errorf("expected empty streams, got %d", len(repo.GetAllV2Streams()))
	}
}

func TestSaveAndLoad_V2(t *testing.T) {
	repo, testFile := setupTestStore(t)

	src := V2Source{ID: "cam-1", Device: "usb-1-2"}
	if err := repo.AddSource(src); err != nil {
		t.Fatalf("AddSource failed: %v", err)
	}

	stream := V2Stream{
		ID:       "encoder-1",
		Upstream: "source:cam-1",
	}
	if err := repo.AddV2Stream(stream); err != nil {
		t.Fatalf("AddV2Stream failed: %v", err)
	}

	if _, statErr := os.Stat(testFile); os.IsNotExist(statErr) {
		t.Error("Config file was not created")
	}

	repo2 := NewTOML(testFile).(*tomlStore)
	if err := repo2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(repo2.GetAllSources()) != 1 {
		t.Errorf("expected 1 source, got %d", len(repo2.GetAllSources()))
	}
	if len(repo2.GetAllV2Streams()) != 1 {
		t.Errorf("expected 1 stream, got %d", len(repo2.GetAllV2Streams()))
	}
	if loaded, ok := repo2.GetSource("cam-1"); !ok || loaded.Device != "usb-1-2" {
		t.Errorf("source round-trip failed: %+v ok=%v", loaded, ok)
	}
	if loaded, ok := repo2.GetV2Stream("encoder-1"); !ok || loaded.Upstream != "source:cam-1" {
		t.Errorf("stream round-trip failed: %+v ok=%v", loaded, ok)
	}
}

// TestLoadV2_DropsLegacyPublish verifies that a v2 file persisted by an
// older version (carrying [[streams.publish]] entries) still loads cleanly
// — the now-removed publish field is silently ignored, not an error. The
// encoder output is hardcoded to the local RTSP relay at runtime.
func TestLoadV2_DropsLegacyPublish(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "legacy_publish.toml")

	legacy := `version = 2

[[sources]]
id = "cam-1"
device = "usb-1-2"

[[streams]]
id = "encoder-1"
upstream = "source:cam-1"

  [streams.encoder.encoder]
  codec = "h264"

  [[streams.publish]]
  type = "rtsp"
  url = "rtsp://nas.lan:8554/archive"

  [[streams.publish]]
  type = "srt"
  url = "srt://localhost:6001?streamid=encoder-1"
`
	if err := os.WriteFile(testFile, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	repo := NewTOML(testFile).(*tomlStore)
	if err := repo.Load(); err != nil {
		t.Fatalf("Load of v2 file with legacy publish should not error: %v", err)
	}

	got, ok := repo.GetV2Stream("encoder-1")
	if !ok {
		t.Fatal("stream encoder-1 not loaded")
	}
	if got.Upstream != "source:cam-1" {
		t.Errorf("upstream round-trip failed: %q", got.Upstream)
	}
}

func TestSourceCRUD(t *testing.T) {
	repo, _ := setupTestStore(t)

	src := V2Source{ID: "s1", Device: "dev-1"}
	if err := repo.AddSource(src); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := repo.AddSource(src); err == nil {
		t.Error("AddSource should reject duplicate id")
	}

	src.Device = "dev-2"
	if err := repo.UpdateSource("s1", src); err != nil {
		t.Fatalf("UpdateSource: %v", err)
	}
	got, ok := repo.GetSource("s1")
	if !ok || got.Device != "dev-2" {
		t.Errorf("UpdateSource didn't persist: %+v", got)
	}

	if err := repo.UpdateSource("nope", src); err == nil {
		t.Error("UpdateSource should error on missing id")
	}

	if err := repo.RemoveSource("s1"); err != nil {
		t.Fatalf("RemoveSource: %v", err)
	}
	if _, exists := repo.GetSource("s1"); exists {
		t.Error("RemoveSource didn't delete")
	}
	if err := repo.RemoveSource("s1"); err == nil {
		t.Error("RemoveSource should error on missing id")
	}
}

func TestComposerCRUD(t *testing.T) {
	repo, _ := setupTestStore(t)

	c := V2Composer{ID: "c1", Canvas: V2CanvasDims{W: 1920, H: 1080}}
	if err := repo.AddComposer(c); err != nil {
		t.Fatalf("AddComposer: %v", err)
	}
	if err := repo.AddComposer(c); err == nil {
		t.Error("AddComposer should reject duplicate id")
	}

	c.Canvas.W = 3840
	if err := repo.UpdateComposer("c1", c); err != nil {
		t.Fatalf("UpdateComposer: %v", err)
	}
	got, ok := repo.GetComposer("c1")
	if !ok || got.Canvas.W != 3840 {
		t.Errorf("UpdateComposer didn't persist: %+v", got)
	}

	if err := repo.RemoveComposer("c1"); err != nil {
		t.Fatalf("RemoveComposer: %v", err)
	}
	if _, exists := repo.GetComposer("c1"); exists {
		t.Error("RemoveComposer didn't delete")
	}
}

func TestV2StreamCRUD(t *testing.T) {
	repo, _ := setupTestStore(t)

	st := V2Stream{ID: "st1", Upstream: "source:s1"}
	if err := repo.AddV2Stream(st); err != nil {
		t.Fatalf("AddV2Stream: %v", err)
	}
	if err := repo.AddV2Stream(st); err == nil {
		t.Error("AddV2Stream should reject duplicate id")
	}

	st.Upstream = "composer:c1"
	if err := repo.UpdateV2Stream("st1", st); err != nil {
		t.Fatalf("UpdateV2Stream: %v", err)
	}
	got, ok := repo.GetV2Stream("st1")
	if !ok || got.Upstream != "composer:c1" {
		t.Errorf("UpdateV2Stream didn't persist: %+v", got)
	}

	if err := repo.RemoveV2Stream("st1"); err != nil {
		t.Fatalf("RemoveV2Stream: %v", err)
	}
	if _, exists := repo.GetV2Stream("st1"); exists {
		t.Error("RemoveV2Stream didn't delete")
	}
}

func TestGetValidation(t *testing.T) {
	repo, _ := setupTestStore(t)

	if v := repo.GetValidation(); v != nil {
		t.Error("expected nil validation for new repo")
	}

	expected := &types.ValidationResults{
		H264: types.CodecValidation{Working: []string{"h264_vaapi"}},
		H265: types.CodecValidation{Working: []string{"hevc_vaapi"}},
	}
	if err := repo.UpdateValidation(expected); err != nil {
		t.Fatalf("UpdateValidation: %v", err)
	}
	if v := repo.GetValidation(); v == nil || len(v.H264.Working) != 1 {
		t.Errorf("validation not stored: %+v", v)
	}
}

func TestUpdateValidationPersists(t *testing.T) {
	repo, testFile := setupTestStore(t)

	v := &types.ValidationResults{H264: types.CodecValidation{Working: []string{"libx264"}}}
	if err := repo.UpdateValidation(v); err != nil {
		t.Fatalf("UpdateValidation: %v", err)
	}

	repo2 := NewTOML(testFile).(*tomlStore)
	if err := repo2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := repo2.GetValidation(); got == nil || len(got.H264.Working) != 1 {
		t.Errorf("validation not persisted: %+v", got)
	}
}

func TestSaveSetsVersion(t *testing.T) {
	repo, _ := setupTestStore(t)
	repo.config.Version = 0
	if err := repo.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if repo.config.Version != schemaVersion {
		t.Errorf("expected Save to set version %d, got %d", schemaVersion, repo.config.Version)
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "subdir", "nested", "streams.toml")

	repo := NewTOML(nestedPath).(*tomlStore)
	if err := repo.AddSource(V2Source{ID: "s1", Device: "d1"}); err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	if _, statErr := os.Stat(nestedPath); os.IsNotExist(statErr) {
		t.Error("Save should create nested directories")
	}
}

func TestTimestampsPersist(t *testing.T) {
	repo, testFile := setupTestStore(t)

	now := time.Now()
	src := V2Source{
		ID:        "s1",
		Device:    "d1",
		CreatedAt: now,
		UpdatedAt: now.Add(time.Hour),
	}
	if err := repo.AddSource(src); err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	repo2 := NewTOML(testFile).(*tomlStore)
	if err := repo2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	loaded, ok := repo2.GetSource("s1")
	if !ok {
		t.Fatal("source missing after reload")
	}
	if loaded.CreatedAt.Sub(now).Abs() > time.Second {
		t.Errorf("CreatedAt drift: got %v want %v", loaded.CreatedAt, now)
	}
	if loaded.UpdatedAt.Sub(now.Add(time.Hour)).Abs() > time.Second {
		t.Errorf("UpdatedAt drift: got %v want %v", loaded.UpdatedAt, now.Add(time.Hour))
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	repo, testFile := setupTestStore(t)

	if err := os.WriteFile(testFile, []byte(`this is not valid toml [[[`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := repo.Load()
	if err == nil {
		t.Error("Load should fail with invalid TOML")
	}
}

func TestLoadUnreadableFile(t *testing.T) {
	repo, testFile := setupTestStore(t)

	if err := os.WriteFile(testFile, []byte("version = 2\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(testFile, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer func() { _ = os.Chmod(testFile, 0o644) }()

	if err := repo.Load(); err == nil {
		t.Error("Load should fail with unreadable file")
	}
}

func TestSaveToUnwritableDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	unwritableDir := filepath.Join(tmpDir, "unwritable")

	if err := os.Mkdir(unwritableDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(unwritableDir, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer func() { _ = os.Chmod(unwritableDir, 0o755) }()

	repo := NewTOML(filepath.Join(unwritableDir, "test.toml")).(*tomlStore)
	if err := repo.save(); err == nil {
		t.Error("save should fail with unwritable directory")
	}
}

func TestPipelineDefault(t *testing.T) {
	repo, _ := setupTestStore(t)
	if got := repo.GetPipeline(); !got.Enabled {
		t.Error("default pipeline should be Enabled=true")
	}
}

// TestLoad_V1Rejected verifies that a non-v2 file is refused with a clear
// error now that v1→v2 auto-migration has been removed.
func TestLoad_V1Rejected(t *testing.T) {
	repo, testFile := setupTestStore(t)

	v1 := `version = 1

[streams]
[streams.c920]
id = 'c920'
device = 'usb-046d_HD_Pro_Webcam_C920_D6BA64DF-video-index0'
`
	if err := os.WriteFile(testFile, []byte(v1), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := repo.Load()
	if err == nil {
		t.Fatal("Load: expected error for v1 config, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("Load: expected 'unsupported' version error, got %v", err)
	}
}
