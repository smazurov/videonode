package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestOpenAPI_CoreSchema is a smoke test that the openapi command produces a
// document with the expected top-level entity paths. /api/streams is asserted
// today; /api/sources and /api/composers are asserted once units B5 and B6
// have landed (handled as skipped subtests below until then).
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

	required := []string{"/api/streams"}
	for _, p := range required {
		if _, ok := spec.Paths[p]; !ok {
			t.Errorf("missing required path %q in openapi spec", p)
		}
	}

	// These paths land with B5 (/api/sources) and B6 (/api/composers). Skip until merged.
	deferred := []string{"/api/sources", "/api/composers"}
	for _, p := range deferred {
		t.Run("deferred_"+p, func(t *testing.T) {
			if _, ok := spec.Paths[p]; !ok {
				t.Skipf("path %q not in schema yet (lands with B5/B6)", p)
			}
		})
	}
}
