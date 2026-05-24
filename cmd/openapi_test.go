package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestOpenAPI_CoreSchema is a smoke test that the openapi command produces a
// document with the expected top-level entity paths.
func TestOpenAPI_CoreSchema(t *testing.T) {
	c := CreateOpenAPICmd()
	var buf bytes.Buffer
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs(nil)
	if err := c.Execute(); err != nil {
		t.Fatalf("openapi: %v", err)
	}

	var spec struct {
		Paths map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(buf.Bytes(), &spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}

	required := []string{"/api/streams", "/api/sources", "/api/composers"}
	for _, p := range required {
		if _, ok := spec.Paths[p]; !ok {
			t.Errorf("missing required path %q in openapi spec", p)
		}
	}
}
