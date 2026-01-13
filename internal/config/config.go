package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// LoadConfig loads configuration with proper precedence: CLI args > env vars > config file.
// If cmd is provided, flags explicitly set via CLI will not be overwritten.
func LoadConfig(opts any, cmd *cobra.Command) error {
	v := reflect.ValueOf(opts).Elem()
	t := v.Type()

	// Build set of flags explicitly changed via CLI
	changedFlags := make(map[string]bool)
	if cmd != nil {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Changed {
				changedFlags[f.Name] = true
			}
		})
	}

	// Get config file path
	var configPath string
	for i := 0; i < v.NumField(); i++ {
		fieldType := t.Field(i)
		if fieldType.Name == "Config" {
			configPath = v.Field(i).String()
			break
		}
	}

	// Load TOML file if it exists
	if configPath != "" {
		if data, err := os.ReadFile(configPath); err == nil {
			var config map[string]any
			if err := toml.Unmarshal(data, &config); err != nil {
				return fmt.Errorf("failed to parse TOML config: %w", err)
			}

			// Apply TOML values using reflection
			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				fieldType := t.Field(i)

				// Skip if this flag was explicitly set via CLI
				flagName := fieldNameToFlag(fieldType.Name)
				if changedFlags[flagName] {
					continue
				}

				if tomlPath := fieldType.Tag.Get("toml"); tomlPath != "" {
					if value := getNestedValue(config, tomlPath); value != nil {
						setFieldValue(field, value)
					}
				}
			}
		}
	}

	// Apply environment variable overrides (skip CLI-set flags)
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Skip if this flag was explicitly set via CLI
		flagName := fieldNameToFlag(fieldType.Name)
		if changedFlags[flagName] {
			continue
		}

		if envKey := fieldType.Tag.Get("env"); envKey != "" {
			if envValue := os.Getenv("VIDEONODE_" + envKey); envValue != "" {
				setFieldValueFromString(field, envValue)
			}
		}
	}

	return nil
}

// fieldNameToFlag converts a struct field name to a CLI flag name.
// Example: "LoggingLevel" -> "logging-level", "Port" -> "port".
func fieldNameToFlag(fieldName string) string {
	var result []rune
	for i, r := range fieldName {
		if i > 0 && unicode.IsUpper(r) {
			result = append(result, '-')
		}
		result = append(result, unicode.ToLower(r))
	}
	return string(result)
}

// getNestedValue retrieves a value from nested map using dot notation.
func getNestedValue(data map[string]any, path string) any {
	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		if i == len(parts)-1 {
			return current[part]
		}
		if next, ok := current[part].(map[string]any); ok {
			current = next
		} else {
			return nil
		}
	}
	return nil
}

// setFieldValue sets a field value using reflection.
func setFieldValue(field reflect.Value, value any) {
	if !field.CanSet() {
		return
	}

	switch field.Kind() {
	case reflect.String:
		if s, ok := value.(string); ok {
			field.SetString(s)
		}
	case reflect.Bool:
		if b, ok := value.(bool); ok {
			field.SetBool(b)
		}
	case reflect.Int:
		if i, ok := value.(int64); ok {
			field.SetInt(i)
		} else if i, intOk := value.(int); intOk {
			field.SetInt(int64(i))
		}
	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			if arr, ok := value.([]any); ok {
				slice := make([]string, len(arr))
				for i, v := range arr {
					if s, strOk := v.(string); strOk {
						slice[i] = s
					}
				}
				field.Set(reflect.ValueOf(slice))
			}
		}
	}
}

// setFieldValueFromString sets a field value from string (for env vars).
func setFieldValueFromString(field reflect.Value, value string) {
	if !field.CanSet() {
		return
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Bool:
		if b, err := strconv.ParseBool(value); err == nil {
			field.SetBool(b)
		}
	case reflect.Int:
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			field.SetInt(i)
		}
	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			// Parse comma-separated values for env vars
			parts := strings.Split(value, ",")
			slice := make([]string, len(parts))
			for i, part := range parts {
				slice[i] = strings.TrimSpace(part)
			}
			field.Set(reflect.ValueOf(slice))
		}
	}
}

// LoadLoggingConfig loads logging configuration from a TOML config file.
// Returns default config if file doesn't exist or can't be parsed.
func LoadLoggingConfig(configPath string) logging.Config {
	cfg := logging.Config{
		Level:   "info",
		Format:  "text",
		Modules: make(map[string]string),
	}

	if configPath == "" {
		return cfg
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}

	var rawConfig struct {
		Logging map[string]string `toml:"logging"`
	}
	if err := toml.Unmarshal(data, &rawConfig); err != nil {
		return cfg
	}

	if rawConfig.Logging == nil {
		return cfg
	}

	// Extract level and format, rest are module-specific levels
	for key, value := range rawConfig.Logging {
		switch key {
		case "level":
			cfg.Level = value
		case "format":
			cfg.Format = value
		default:
			cfg.Modules[key] = value
		}
	}

	return cfg
}
