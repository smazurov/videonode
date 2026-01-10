package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/types"
)

// setupTestRepo creates a temporary repository for testing.
func setupTestRepo(t *testing.T) (*tomlStore, string) {
	t.Helper()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_streams.toml")

	repo := NewTOML(testFile).(*tomlStore)
	return repo, testFile
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
	if repo.config.Version != 1 {
		t.Errorf("expected version 1, got %d", repo.config.Version)
	}
	if repo.config.Streams == nil {
		t.Error("streams map should be initialized")
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	repo, _ := setupTestRepo(t)

	err := repo.Load()
	if err != nil {
		t.Errorf("Load should not error on non-existent file, got: %v", err)
	}

	if len(repo.config.Streams) != 0 {
		t.Errorf("expected empty streams map, got %d streams", len(repo.config.Streams))
	}
}

func TestSaveAndLoad(t *testing.T) {
	repo, testFile := setupTestRepo(t)

	// Add a test stream
	stream := streams.StreamSpec{
		ID:     "test-stream",
		Name:   "Test Stream",
		Device: "usb-test-device",
	}
	repo.config.Streams["test-stream"] = stream

	// Save
	err := repo.Save()
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, statErr := os.Stat(testFile); os.IsNotExist(statErr) {
		t.Error("Config file was not created")
	}

	// Create new repo and load
	repo2 := NewTOML(testFile).(*tomlStore)
	err = repo2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify loaded data
	if len(repo2.config.Streams) != 1 {
		t.Errorf("expected 1 stream, got %d", len(repo2.config.Streams))
	}

	loadedStream, exists := repo2.config.Streams["test-stream"]
	if !exists {
		t.Fatal("test-stream not found after load")
	}

	if loadedStream.ID != stream.ID {
		t.Errorf("expected ID %s, got %s", stream.ID, loadedStream.ID)
	}
	if loadedStream.Name != stream.Name {
		t.Errorf("expected Name %s, got %s", stream.Name, loadedStream.Name)
	}
	if loadedStream.Device != stream.Device {
		t.Errorf("expected Device %s, got %s", stream.Device, loadedStream.Device)
	}
}

func TestAddStream(t *testing.T) {
	repo, _ := setupTestRepo(t)

	stream := streams.StreamSpec{
		ID:     "new-stream",
		Name:   "New Stream",
		Device: "usb-device-1",
	}

	err := repo.AddStream(stream)
	if err != nil {
		t.Fatalf("AddStream failed: %v", err)
	}

	// Verify stream was added
	stored, exists := repo.config.Streams["new-stream"]
	if !exists {
		t.Fatal("stream was not added to config")
	}

	if stored.ID != stream.ID {
		t.Errorf("expected ID %s, got %s", stream.ID, stored.ID)
	}

	// Verify it was persisted
	repo2 := NewTOML(repo.configPath).(*tomlStore)
	err = repo2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	_, exists = repo2.config.Streams["new-stream"]
	if !exists {
		t.Error("stream was not persisted to file")
	}
}

func TestUpdateStream(t *testing.T) {
	repo, _ := setupTestRepo(t)

	// Add initial stream
	original := streams.StreamSpec{
		ID:     "update-test",
		Name:   "Original Name",
		Device: "device-1",
	}
	repo.config.Streams["update-test"] = original

	// Update stream
	updated := streams.StreamSpec{
		ID:     "update-test",
		Name:   "Updated Name",
		Device: "device-2",
	}

	err := repo.UpdateStream("update-test", updated)
	if err != nil {
		t.Fatalf("UpdateStream failed: %v", err)
	}

	// Verify update
	stored, exists := repo.config.Streams["update-test"]
	if !exists {
		t.Fatal("stream disappeared after update")
	}

	if stored.Name != "Updated Name" {
		t.Errorf("expected Name 'Updated Name', got %s", stored.Name)
	}
	if stored.Device != "device-2" {
		t.Errorf("expected Device 'device-2', got %s", stored.Device)
	}

	// Verify persistence
	repo2 := NewTOML(repo.configPath).(*tomlStore)
	err = repo2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	loaded := repo2.config.Streams["update-test"]
	if loaded.Name != "Updated Name" {
		t.Error("update was not persisted")
	}
}

func TestRemoveStream(t *testing.T) {
	repo, _ := setupTestRepo(t)

	// Add stream
	stream := streams.StreamSpec{
		ID:     "remove-test",
		Name:   "To Be Removed",
		Device: "device-1",
	}
	repo.config.Streams["remove-test"] = stream
	if err := repo.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Remove stream
	err := repo.RemoveStream("remove-test")
	if err != nil {
		t.Fatalf("RemoveStream failed: %v", err)
	}

	// Verify removal
	_, exists := repo.config.Streams["remove-test"]
	if exists {
		t.Error("stream still exists after removal")
	}

	// Verify persistence
	repo2 := NewTOML(repo.configPath).(*tomlStore)
	err = repo2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	_, exists = repo2.config.Streams["remove-test"]
	if exists {
		t.Error("stream removal was not persisted")
	}
}

func TestGetStream(t *testing.T) {
	repo, _ := setupTestRepo(t)

	// Test non-existent stream
	_, exists := repo.GetStream("non-existent")
	if exists {
		t.Error("GetStream should return false for non-existent stream")
	}

	// Add stream
	stream := streams.StreamSpec{
		ID:     "get-test",
		Name:   "Get Test",
		Device: "device-1",
	}
	repo.config.Streams["get-test"] = stream

	// Get existing stream
	retrieved, exists := repo.GetStream("get-test")
	if !exists {
		t.Fatal("GetStream should return true for existing stream")
	}

	if retrieved.ID != stream.ID {
		t.Errorf("expected ID %s, got %s", stream.ID, retrieved.ID)
	}
	if retrieved.Name != stream.Name {
		t.Errorf("expected Name %s, got %s", stream.Name, retrieved.Name)
	}
}

func TestGetAllStreams(t *testing.T) {
	repo, _ := setupTestRepo(t)

	// Empty repository
	allStreams := repo.GetAllStreams()
	if len(allStreams) != 0 {
		t.Errorf("expected 0 streams, got %d", len(allStreams))
	}

	// Add multiple streams
	repo.config.Streams["stream-1"] = streams.StreamSpec{ID: "stream-1", Name: "Stream 1", Device: "dev-1"}
	repo.config.Streams["stream-2"] = streams.StreamSpec{ID: "stream-2", Name: "Stream 2", Device: "dev-2"}
	repo.config.Streams["stream-3"] = streams.StreamSpec{ID: "stream-3", Name: "Stream 3", Device: "dev-3"}

	allStreams = repo.GetAllStreams()
	if len(allStreams) != 3 {
		t.Errorf("expected 3 streams, got %d", len(allStreams))
	}

	// Verify specific streams
	if _, exists := allStreams["stream-1"]; !exists {
		t.Error("stream-1 not found in GetAllStreams")
	}
	if _, exists := allStreams["stream-2"]; !exists {
		t.Error("stream-2 not found in GetAllStreams")
	}
	if _, exists := allStreams["stream-3"]; !exists {
		t.Error("stream-3 not found in GetAllStreams")
	}
}

func TestGetValidation(t *testing.T) {
	repo, _ := setupTestRepo(t)

	// Initially nil
	validation := repo.GetValidation()
	if validation != nil {
		t.Error("expected nil validation for new repo")
	}

	// Set validation
	expected := &types.ValidationResults{
		H264: types.CodecValidation{
			Working: []string{"h264_vaapi", "libx264"},
			Failed:  []string{"h264_qsv"},
		},
		H265: types.CodecValidation{
			Working: []string{"hevc_vaapi"},
			Failed:  []string{"hevc_qsv", "libx265"},
		},
	}
	repo.config.Validation = expected

	// Get validation
	retrieved := repo.GetValidation()
	if retrieved == nil {
		t.Fatal("expected validation data, got nil")
	}

	if len(retrieved.H264.Working) != 2 {
		t.Errorf("expected 2 working H264 encoders, got %d", len(retrieved.H264.Working))
	}
	if len(retrieved.H265.Failed) != 2 {
		t.Errorf("expected 2 failed H265 encoders, got %d", len(retrieved.H265.Failed))
	}
}

func TestUpdateValidation(t *testing.T) {
	repo, _ := setupTestRepo(t)

	validation := &types.ValidationResults{
		H264: types.CodecValidation{
			Working: []string{"h264_vaapi"},
			Failed:  []string{},
		},
		H265: types.CodecValidation{
			Working: []string{},
			Failed:  []string{"hevc_vaapi"},
		},
	}

	err := repo.UpdateValidation(validation)
	if err != nil {
		t.Fatalf("UpdateValidation failed: %v", err)
	}

	// Verify in memory
	stored := repo.GetValidation()
	if stored == nil {
		t.Fatal("validation was not stored")
	}
	if len(stored.H264.Working) != 1 {
		t.Errorf("expected 1 working H264 encoder, got %d", len(stored.H264.Working))
	}

	// Verify persistence
	repo2 := NewTOML(repo.configPath).(*tomlStore)
	err = repo2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	loaded := repo2.GetValidation()
	if loaded == nil {
		t.Fatal("validation was not persisted")
	}
	if len(loaded.H264.Working) != 1 {
		t.Error("validation persistence failed")
	}
}

func TestLoadHandlesNilStreamsMap(t *testing.T) {
	repo, testFile := setupTestRepo(t)

	// Create a config file without streams section
	content := `version = 1
`
	err := os.WriteFile(testFile, []byte(content), 0o644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Load should initialize streams map
	err = repo.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if repo.config.Streams == nil {
		t.Error("Load should initialize nil streams map")
	}

	if repo.config.Version != 1 {
		t.Errorf("expected version 1, got %d", repo.config.Version)
	}
}

func TestLoadSetsDefaultVersion(t *testing.T) {
	repo, testFile := setupTestRepo(t)

	// Create a config file without version
	content := `[streams]
`
	err := os.WriteFile(testFile, []byte(content), 0o644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = repo.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if repo.config.Version != 1 {
		t.Errorf("Load should set default version 1, got %d", repo.config.Version)
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "subdir", "nested", "streams.toml")

	repo := NewTOML(nestedPath).(*tomlStore)
	repo.config.Streams["test"] = streams.StreamSpec{
		ID:     "test",
		Name:   "Test",
		Device: "dev",
	}

	err := repo.Save()
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify nested directories were created
	if _, statErr := os.Stat(nestedPath); os.IsNotExist(statErr) {
		t.Error("Save should create nested directories")
	}
}

func TestStreamTimestamps(t *testing.T) {
	repo, _ := setupTestRepo(t)

	now := time.Now()
	stream := streams.StreamSpec{
		ID:        "timestamp-test",
		Name:      "Timestamp Test",
		Device:    "device-1",
		CreatedAt: now,
		UpdatedAt: now.Add(time.Hour),
	}

	err := repo.AddStream(stream)
	if err != nil {
		t.Fatalf("AddStream failed: %v", err)
	}

	// Reload and verify timestamps persisted
	repo2 := NewTOML(repo.configPath).(*tomlStore)
	err = repo2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	loaded, exists := repo2.GetStream("timestamp-test")
	if !exists {
		t.Fatal("stream not found")
	}

	// Allow small delta for time comparison
	if loaded.CreatedAt.Sub(now).Abs() > time.Second {
		t.Errorf("CreatedAt not preserved correctly")
	}
	if loaded.UpdatedAt.Sub(now.Add(time.Hour)).Abs() > time.Second {
		t.Errorf("UpdatedAt not preserved correctly")
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	repo, testFile := setupTestRepo(t)

	// Write invalid TOML
	invalidContent := `this is not valid toml [[[`
	err := os.WriteFile(testFile, []byte(invalidContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Load should fail
	err = repo.Load()
	if err == nil {
		t.Error("Load should fail with invalid TOML")
	}
	if err != nil && !contains(err.Error(), "parse") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestLoadUnreadableFile(t *testing.T) {
	repo, testFile := setupTestRepo(t)

	// Create file
	err := os.WriteFile(testFile, []byte("version = 1\n"), 0o644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Make file unreadable
	err = os.Chmod(testFile, 0o000)
	if err != nil {
		t.Fatalf("failed to chmod file: %v", err)
	}
	defer func() {
		if err := os.Chmod(testFile, 0o644); err != nil {
			t.Errorf("cleanup chmod failed: %v", err)
		}
	}()

	// Load should fail
	err = repo.Load()
	if err == nil {
		t.Error("Load should fail with unreadable file")
	}
}

func TestSaveToUnwritableDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	unwritableDir := filepath.Join(tmpDir, "unwritable")

	// Create directory and make it unwritable
	err := os.Mkdir(unwritableDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	err = os.Chmod(unwritableDir, 0o444)
	if err != nil {
		t.Fatalf("failed to chmod dir: %v", err)
	}
	defer func() {
		if err := os.Chmod(unwritableDir, 0o755); err != nil {
			t.Errorf("cleanup chmod failed: %v", err)
		}
	}()

	testFile := filepath.Join(unwritableDir, "test.toml")
	repo := NewTOML(testFile)

	// Save should fail
	err = repo.Save()
	if err == nil {
		t.Error("Save should fail with unwritable directory")
	}
}

// Helper function.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsRecursive(s, substr))
}

func containsRecursive(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
