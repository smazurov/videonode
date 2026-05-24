package envctl

import (
	"path/filepath"
	"testing"
)

func TestRegisterAndLookupSession(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.db")

	if err := RegisterSession(statePath, "sess-1", "/tmp/worktree-a"); err != nil {
		t.Fatal(err)
	}
	cwd, err := LookupSession(statePath, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if cwd != "/tmp/worktree-a" {
		t.Errorf("got %q, want /tmp/worktree-a", cwd)
	}
}

func TestLookupMissingSession(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.db")

	cwd, err := LookupSession(statePath, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if cwd != "" {
		t.Errorf("expected empty, got %q", cwd)
	}
}

func TestLookupEmptySessionID(t *testing.T) {
	cwd, err := LookupSession("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cwd != "" {
		t.Errorf("expected empty, got %q", cwd)
	}
}

func TestUnregisterSession(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.db")

	RegisterSession(statePath, "sess-1", "/tmp/a")
	UnregisterSession(statePath, "sess-1")

	cwd, _ := LookupSession(statePath, "sess-1")
	if cwd != "" {
		t.Errorf("expected empty after unregister, got %q", cwd)
	}
}

func TestRegisterOverwrites(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.db")

	RegisterSession(statePath, "sess-1", "/tmp/old")
	RegisterSession(statePath, "sess-1", "/tmp/new")

	cwd, _ := LookupSession(statePath, "sess-1")
	if cwd != "/tmp/new" {
		t.Errorf("got %q, want /tmp/new", cwd)
	}
}
