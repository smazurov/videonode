package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// LoadConfig automatically loads configuration from TOML file and environment variables
func LoadConfig(opts interface{}) error {
	v := reflect.ValueOf(opts).Elem()
	t := v.Type()
	
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
			var config map[string]interface{}
			if err := toml.Unmarshal(data, &config); err != nil {
				return fmt.Errorf("failed to parse TOML config: %w", err)
			}
			
			// Apply TOML values using reflection
			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				fieldType := t.Field(i)
				
				if tomlPath := fieldType.Tag.Get("toml"); tomlPath != "" {
					if value := getNestedValue(config, tomlPath); value != nil {
						setFieldValue(field, value)
					}
				}
			}
		}
	}
	
	// Apply environment variable overrides
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		
		if envKey := fieldType.Tag.Get("env"); envKey != "" {
			if envValue := os.Getenv("VIDEONODE_" + envKey); envValue != "" {
				setFieldValueFromString(field, envValue)
			}
		}
	}
	
	return nil
}

// getNestedValue retrieves a value from nested map using dot notation
func getNestedValue(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	current := data
	
	for i, part := range parts {
		if i == len(parts)-1 {
			return current[part]
		}
		if next, ok := current[part].(map[string]interface{}); ok {
			current = next
		} else {
			return nil
		}
	}
	return nil
}

// setFieldValue sets a field value using reflection
func setFieldValue(field reflect.Value, value interface{}) {
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
		} else if i, ok := value.(int); ok {
			field.SetInt(int64(i))
		}
	}
}

// setFieldValueFromString sets a field value from string (for env vars)
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
	}
}