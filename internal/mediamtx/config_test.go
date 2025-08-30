package mediamtx

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNewConfigBasicFields(t *testing.T) {
	config := NewConfig()

	if !config.API {
		t.Error("Expected API to be enabled")
	}

	if config.APIAddress != ":9997" {
		t.Errorf("Expected APIAddress to be :9997, got %s", config.APIAddress)
	}

	if config.RTSPAddress != ":8554" {
		t.Errorf("Expected RTSPAddress to be :8554, got %s", config.RTSPAddress)
	}

	if config.WebRTCAddress != ":8889" {
		t.Errorf("Expected WebRTCAddress to be :8889, got %s", config.WebRTCAddress)
	}

	if !config.Metrics {
		t.Error("Expected Metrics to be enabled")
	}

	if config.MetricsAddress != ":9998" {
		t.Errorf("Expected MetricsAddress to be :9998, got %s", config.MetricsAddress)
	}

	if config.Paths == nil {
		t.Error("Expected Paths to be initialized")
	}
}

func TestNewConfigInternalAuth(t *testing.T) {
	config := NewConfig()

	if config.AuthMethod != "internal" {
		t.Errorf("Expected AuthMethod to be 'internal', got %s", config.AuthMethod)
	}

	if len(config.AuthInternalUsers) != 1 {
		t.Fatalf("Expected 1 internal user, got %d", len(config.AuthInternalUsers))
	}

	user := config.AuthInternalUsers[0]
	if user.User != "any" {
		t.Errorf("Expected user to be 'any', got %s", user.User)
	}

	expectedPerms := map[string]bool{
		"publish":  false,
		"read":     false,
		"playback": false,
		"api":      false,
		"metrics":  false,
		"pprof":    false,
	}

	for _, perm := range user.Permissions {
		if _, exists := expectedPerms[perm.Action]; !exists {
			t.Errorf("Unexpected permission action: %s", perm.Action)
		}
		expectedPerms[perm.Action] = true
	}

	for action, found := range expectedPerms {
		if !found {
			t.Errorf("Expected permission action %s not found", action)
		}
	}
}

func TestAddStream(t *testing.T) {
	config := NewConfig()

	err := config.AddStream("test", StreamConfig{
		DevicePath: "/dev/video0",
		Resolution: "1920x1080",
		FPS:        "30",
	})
	if err != nil {
		t.Fatalf("Failed to add stream: %v", err)
	}

	if _, exists := config.Paths["test"]; !exists {
		t.Error("Stream 'test' was not added to paths")
	}

	if !config.Paths["test"].RunOnInitRestart {
		t.Error("Expected RunOnInitRestart to be true")
	}
}

func TestAddStreamEmptyPath(t *testing.T) {
	config := NewConfig()

	err := config.AddStream("", StreamConfig{
		DevicePath: "/dev/video0",
	})
	if err == nil {
		t.Error("Expected error when adding stream with empty path name")
	}
}

func TestRemoveStream(t *testing.T) {
	config := NewConfig()

	config.AddStream("test", StreamConfig{
		DevicePath: "/dev/video0",
	})

	config.RemoveStream("test")

	if _, exists := config.Paths["test"]; exists {
		t.Error("Stream 'test' was not removed from paths")
	}
}

func TestWriteToFileAndLoadFromFile(t *testing.T) {
	config := NewConfig()
	config.AddStream("test", StreamConfig{
		DevicePath: "/dev/video0",
		Resolution: "1920x1080",
	})

	tmpFile := "/tmp/test-mediamtx-config.yml"
	defer os.Remove(tmpFile)

	err := config.WriteToFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	loadedConfig, err := LoadFromFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to load config file: %v", err)
	}

	if loadedConfig.APIAddress != config.APIAddress {
		t.Errorf("Loaded APIAddress mismatch: got %s, want %s", loadedConfig.APIAddress, config.APIAddress)
	}

	if _, exists := loadedConfig.Paths["test"]; !exists {
		t.Error("Stream 'test' was not loaded from file")
	}
}

func TestLoadFromFileNonExistent(t *testing.T) {
	config, err := LoadFromFile("/tmp/non-existent-file.yml")
	if err != nil {
		t.Fatalf("Expected no error for non-existent file, got: %v", err)
	}

	if config == nil {
		t.Error("Expected default config for non-existent file")
	}

	if config.APIAddress != ":9997" {
		t.Errorf("Expected default APIAddress :9997, got %s", config.APIAddress)
	}
}

func TestConfigMarshalYAML(t *testing.T) {
	config := NewConfig()

	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	yamlStr := string(data)
	if !contains(yamlStr, "authMethod: internal") {
		t.Error("Marshaled YAML does not contain authMethod")
	}
	if !contains(yamlStr, "authInternalUsers:") {
		t.Error("Marshaled YAML does not contain authInternalUsers")
	}
	if !contains(yamlStr, "action: metrics") {
		t.Error("Marshaled YAML does not contain metrics permission")
	}
}

func TestGetWebRTCURL(t *testing.T) {
	url := GetWebRTCURL("test-stream")
	expected := ":8889/test-stream"
	if url != expected {
		t.Errorf("Expected WebRTC URL %s, got %s", expected, url)
	}
}

func TestGetRTSPURL(t *testing.T) {
	url := GetRTSPURL("test-stream")
	expected := ":8554/test-stream"
	if url != expected {
		t.Errorf("Expected RTSP URL %s, got %s", expected, url)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}
