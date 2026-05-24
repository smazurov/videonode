package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestInstallIdempotent verifies that running `testenv install` twice
// produces identical output — no duplicate hooks, skills, or MCP entries.
func TestInstallIdempotent(t *testing.T) {
	bin := t.TempDir() + "/testenv"
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	projDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projDir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	// install requires a valid .testenv.toml.
	writeTestenvToml(t, projDir)

	// Run install twice.
	for i := range 2 {
		cmd := exec.Command(bin, "install", "--project-dir", projDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("install #%d: %v\n%s", i+1, err, out)
		}
	}

	// Verify: no duplicate hook entries.
	settingsPath := filepath.Join(projDir, ".claude", "settings.json")
	b, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(b, &settings); err != nil {
		t.Fatal(err)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	for event, groups := range hooks {
		groupList, ok := groups.([]any)
		if !ok {
			continue
		}
		seen := map[string]bool{}
		for _, g := range groupList {
			key, _ := json.Marshal(g)
			if seen[string(key)] {
				t.Errorf("duplicate hook entry in %s: %s", event, key)
			}
			seen[string(key)] = true
		}
	}

	// Verify: skills exist.
	for _, name := range []string{"testenv-up", "testenv-down", "testenv-list"} {
		path := filepath.Join(projDir, ".claude", "skills", name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("skill %s missing: %v", name, err)
		}
	}

	// Verify: .mcp.json exists and has testenv entry.
	mcpPath := filepath.Join(projDir, ".mcp.json")
	mcpBytes, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("mcp.json: %v", err)
	}
	var mcpDoc map[string]any
	if err := json.Unmarshal(mcpBytes, &mcpDoc); err != nil {
		t.Fatal(err)
	}
	servers, ok := mcpDoc["mcpServers"].(map[string]any)
	if !ok || servers["testenv"] == nil {
		t.Error("mcp.json missing testenv server entry")
	}

	// Verify: settings.json has all four hook events.
	expectedEvents := []string{"SessionStart", "SessionEnd", "PreToolUse", "PostToolUse", "WorktreeRemove"}
	for _, event := range expectedEvents {
		if hooks[event] == nil {
			t.Errorf("settings.json missing hook event %s", event)
		}
	}
}

// TestInstallPreservesExistingSettings verifies that install doesn't
// clobber existing permissions or other settings.
func TestInstallPreservesExistingSettings(t *testing.T) {
	bin := t.TempDir() + "/testenv"
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	projDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projDir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestenvToml(t, projDir)

	// Write a pre-existing settings.json with permissions.
	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []string{"Bash(ls:*)"},
		},
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "echo pre-existing-hook",
						},
					},
				},
			},
		},
	}
	b, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(projDir, ".claude", "settings.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "install", "--project-dir", projDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}

	// Read back.
	result, err := os.ReadFile(filepath.Join(projDir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(result, &s); err != nil {
		t.Fatal(err)
	}

	// Permissions preserved.
	perms, ok := s["permissions"].(map[string]any)
	if !ok {
		t.Fatal("permissions key missing")
	}
	allowList, ok := perms["allow"].([]any)
	if !ok || len(allowList) == 0 {
		t.Error("permissions.allow was clobbered")
	}

	// Pre-existing SessionStart hook preserved.
	hooks, _ := s["hooks"].(map[string]any)
	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok || len(sessionStart) < 2 {
		t.Errorf("SessionStart should have pre-existing + testenv entries; got %d", len(sessionStart))
	}
}

func writeTestenvToml(t *testing.T, dir string) {
	t.Helper()
	cfg := `version = 1
[ports.http]
base = 8090
step = 10
[spawn]
command = "./myapp"
`
	if err := os.WriteFile(filepath.Join(dir, ".testenv.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}
