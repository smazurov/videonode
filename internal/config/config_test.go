package config

import (
	"os"
	"reflect"
	"testing"
)

// TestConfig represents a test configuration structure.
type TestConfig struct {
	Config string `help:"Config file path"`

	// Basic types
	StringField string   `toml:"test.string_field" env:"STRING_FIELD"`
	BoolField   bool     `toml:"test.bool_field" env:"BOOL_FIELD"`
	IntField    int      `toml:"test.int_field" env:"INT_FIELD"`
	SliceField  []string `toml:"test.slice_field" env:"SLICE_FIELD"`

	// Nested config
	NestedString string `toml:"nested.value" env:"NESTED_VALUE"`
}

func TestLoadConfigFromTOML(t *testing.T) {
	// Create a temporary TOML file
	tomlContent := `
[test]
string_field = "hello world"
bool_field = true
int_field = 42
slice_field = ["item1", "item2", "item3"]

[nested]
value = "nested value"
`

	tmpFile, err := os.CreateTemp("", "test_config_*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, writeErr := tmpFile.WriteString(tomlContent); writeErr != nil {
		t.Fatalf("Failed to write to temp file: %v", writeErr)
	}
	tmpFile.Close()

	// Test loading config
	config := &TestConfig{
		Config: tmpFile.Name(),
	}

	err = LoadConfig(config, nil)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify values
	if config.StringField != "hello world" {
		t.Errorf("Expected StringField to be 'hello world', got '%s'", config.StringField)
	}

	if !config.BoolField {
		t.Errorf("Expected BoolField to be true, got %v", config.BoolField)
	}

	if config.IntField != 42 {
		t.Errorf("Expected IntField to be 42, got %d", config.IntField)
	}

	expectedSlice := []string{"item1", "item2", "item3"}
	if !reflect.DeepEqual(config.SliceField, expectedSlice) {
		t.Errorf("Expected SliceField to be %v, got %v", expectedSlice, config.SliceField)
	}

	if config.NestedString != "nested value" {
		t.Errorf("Expected NestedString to be 'nested value', got '%s'", config.NestedString)
	}
}

func TestLoadConfigFromEnvVars(t *testing.T) {
	// Set environment variables
	os.Setenv("VIDEONODE_STRING_FIELD", "env string")
	os.Setenv("VIDEONODE_BOOL_FIELD", "false")
	os.Setenv("VIDEONODE_INT_FIELD", "123")
	os.Setenv("VIDEONODE_SLICE_FIELD", "a,b,c")
	os.Setenv("VIDEONODE_NESTED_VALUE", "env nested")

	defer func() {
		os.Unsetenv("VIDEONODE_STRING_FIELD")
		os.Unsetenv("VIDEONODE_BOOL_FIELD")
		os.Unsetenv("VIDEONODE_INT_FIELD")
		os.Unsetenv("VIDEONODE_SLICE_FIELD")
		os.Unsetenv("VIDEONODE_NESTED_VALUE")
	}()

	config := &TestConfig{}

	err := LoadConfig(config, nil)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify values
	if config.StringField != "env string" {
		t.Errorf("Expected StringField to be 'env string', got '%s'", config.StringField)
	}

	if config.BoolField {
		t.Errorf("Expected BoolField to be false, got %v", config.BoolField)
	}

	if config.IntField != 123 {
		t.Errorf("Expected IntField to be 123, got %d", config.IntField)
	}

	expectedSlice := []string{"a", "b", "c"}
	if !reflect.DeepEqual(config.SliceField, expectedSlice) {
		t.Errorf("Expected SliceField to be %v, got %v", expectedSlice, config.SliceField)
	}

	if config.NestedString != "env nested" {
		t.Errorf("Expected NestedString to be 'env nested', got '%s'", config.NestedString)
	}
}

func TestLoadConfigEnvOverridesToml(t *testing.T) {
	// Create a temporary TOML file
	tomlContent := `
[test]
string_field = "toml value"
bool_field = true
int_field = 100
slice_field = ["toml1", "toml2"]
`

	tmpFile, err := os.CreateTemp("", "test_config_*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, writeErr := tmpFile.WriteString(tomlContent); writeErr != nil {
		t.Fatalf("Failed to write to temp file: %v", writeErr)
	}
	tmpFile.Close()

	// Set environment variables that should override TOML
	os.Setenv("VIDEONODE_STRING_FIELD", "env override")
	os.Setenv("VIDEONODE_BOOL_FIELD", "false")

	defer func() {
		os.Unsetenv("VIDEONODE_STRING_FIELD")
		os.Unsetenv("VIDEONODE_BOOL_FIELD")
	}()

	config := &TestConfig{
		Config: tmpFile.Name(),
	}

	err = LoadConfig(config, nil)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify env vars override TOML values
	if config.StringField != "env override" {
		t.Errorf("Expected StringField to be 'env override', got '%s'", config.StringField)
	}

	if config.BoolField {
		t.Errorf("Expected BoolField to be false (env override), got %v", config.BoolField)
	}

	// Verify TOML values are used when no env override
	if config.IntField != 100 {
		t.Errorf("Expected IntField to be 100 (from TOML), got %d", config.IntField)
	}

	expectedSlice := []string{"toml1", "toml2"}
	if !reflect.DeepEqual(config.SliceField, expectedSlice) {
		t.Errorf("Expected SliceField to be %v (from TOML), got %v", expectedSlice, config.SliceField)
	}
}

func TestGetNestedValue(t *testing.T) {
	data := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"value": "nested_value",
			},
			"simple": "simple_value",
		},
		"root": "root_value",
	}

	tests := []struct {
		path     string
		expected any
	}{
		{"root", "root_value"},
		{"level1.simple", "simple_value"},
		{"level1.level2.value", "nested_value"},
		{"nonexistent", nil},
		{"level1.nonexistent", nil},
	}

	for _, test := range tests {
		result := getNestedValue(data, test.path)
		if result != test.expected {
			t.Errorf("getNestedValue(%q) = %v, expected %v", test.path, result, test.expected)
		}
	}
}

func TestSetFieldValue(t *testing.T) {
	type TestStruct struct {
		StringField string
		BoolField   bool
		IntField    int
		SliceField  []string
	}

	s := &TestStruct{}
	v := reflect.ValueOf(s).Elem()

	// Test string field
	setFieldValue(v.FieldByName("StringField"), "test string")
	if s.StringField != "test string" {
		t.Errorf("Expected StringField to be 'test string', got '%s'", s.StringField)
	}

	// Test bool field
	setFieldValue(v.FieldByName("BoolField"), true)
	if !s.BoolField {
		t.Errorf("Expected BoolField to be true, got %v", s.BoolField)
	}

	// Test int field
	setFieldValue(v.FieldByName("IntField"), int64(42))
	if s.IntField != 42 {
		t.Errorf("Expected IntField to be 42, got %d", s.IntField)
	}

	// Test slice field
	sliceValue := []any{"a", "b", "c"}
	setFieldValue(v.FieldByName("SliceField"), sliceValue)
	expectedSlice := []string{"a", "b", "c"}
	if !reflect.DeepEqual(s.SliceField, expectedSlice) {
		t.Errorf("Expected SliceField to be %v, got %v", expectedSlice, s.SliceField)
	}
}

func TestSetFieldValueFromString(t *testing.T) {
	type TestStruct struct {
		StringField string
		BoolField   bool
		IntField    int
		SliceField  []string
	}

	s := &TestStruct{}
	v := reflect.ValueOf(s).Elem()

	// Test string field
	setFieldValueFromString(v.FieldByName("StringField"), "test string")
	if s.StringField != "test string" {
		t.Errorf("Expected StringField to be 'test string', got '%s'", s.StringField)
	}

	// Test bool field
	setFieldValueFromString(v.FieldByName("BoolField"), "true")
	if !s.BoolField {
		t.Errorf("Expected BoolField to be true, got %v", s.BoolField)
	}

	// Test int field
	setFieldValueFromString(v.FieldByName("IntField"), "123")
	if s.IntField != 123 {
		t.Errorf("Expected IntField to be 123, got %d", s.IntField)
	}

	// Test slice field (comma-separated)
	setFieldValueFromString(v.FieldByName("SliceField"), "x,y,z")
	expectedSlice := []string{"x", "y", "z"}
	if !reflect.DeepEqual(s.SliceField, expectedSlice) {
		t.Errorf("Expected SliceField to be %v, got %v", expectedSlice, s.SliceField)
	}

	// Test slice field with spaces
	setFieldValueFromString(v.FieldByName("SliceField"), " a , b , c ")
	expectedSliceWithSpaces := []string{"a", "b", "c"}
	if !reflect.DeepEqual(s.SliceField, expectedSliceWithSpaces) {
		t.Errorf("Expected SliceField to be %v, got %v", expectedSliceWithSpaces, s.SliceField)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	config := &TestConfig{
		Config: "nonexistent_file.toml",
	}

	// Should not fail when file doesn't exist
	err := LoadConfig(config, nil)
	if err != nil {
		t.Fatalf("LoadConfig should not fail for missing file: %v", err)
	}
}

// LoggingConfig matches the logging fields in main.go Options struct.
type LoggingConfig struct {
	Config           string `help:"Config file path"`
	LoggingLevel     string `toml:"logging.level" env:"LOGGING_LEVEL"`
	LoggingFormat    string `toml:"logging.format" env:"LOGGING_FORMAT"`
	LoggingStreams   string `toml:"logging.streams" env:"LOGGING_STREAMS"`
	LoggingStreaming string `toml:"logging.streaming" env:"LOGGING_STREAMING"`
	LoggingWebRTC    string `toml:"logging.webrtc" env:"LOGGING_WEBRTC"`
	LoggingAPI       string `toml:"logging.api" env:"LOGGING_API"`
}

func TestLoadLoggingModuleLevels(t *testing.T) {
	tomlContent := `
[logging]
level = "info"
format = "text"
streams = "debug"
streaming = "debug"
webrtc = "warn"
api = "error"
`

	tmpFile, err := os.CreateTemp("", "logging_config_*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, writeErr := tmpFile.WriteString(tomlContent); writeErr != nil {
		t.Fatalf("Failed to write to temp file: %v", writeErr)
	}
	tmpFile.Close()

	config := &LoggingConfig{
		Config:           tmpFile.Name(),
		LoggingLevel:     "info", // defaults
		LoggingFormat:    "text",
		LoggingStreams:   "info",
		LoggingStreaming: "info",
		LoggingWebRTC:    "info",
		LoggingAPI:       "info",
	}

	err = LoadConfig(config, nil)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"LoggingLevel", config.LoggingLevel, "info"},
		{"LoggingFormat", config.LoggingFormat, "text"},
		{"LoggingStreams", config.LoggingStreams, "debug"},
		{"LoggingStreaming", config.LoggingStreaming, "debug"},
		{"LoggingWebRTC", config.LoggingWebRTC, "warn"},
		{"LoggingAPI", config.LoggingAPI, "error"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.field, tt.got, tt.want)
		}
	}
}

func TestLoadConfigInvalidTOML(t *testing.T) {
	// Create a temporary file with invalid TOML
	invalidToml := `
[test
invalid toml syntax
`

	tmpFile, err := os.CreateTemp("", "invalid_config_*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, writeErr := tmpFile.WriteString(invalidToml); writeErr != nil {
		t.Fatalf("Failed to write to temp file: %v", writeErr)
	}
	tmpFile.Close()

	config := &TestConfig{
		Config: tmpFile.Name(),
	}

	// Should fail with invalid TOML
	err = LoadConfig(config, nil)
	if err == nil {
		t.Fatalf("LoadConfig should fail for invalid TOML")
	}
}
