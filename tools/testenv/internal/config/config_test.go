package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadV1_Minimal(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
version = 1
[ports.http]
base = 8090
step = 10
[spawn]
command = "./my-daemon"
`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 1 {
		t.Errorf("version=%d", c.Version)
	}
	if c.MaxSlots != 9 {
		t.Errorf("max_slots=%d (expected default 9)", c.MaxSlots)
	}
	if c.Spawn.Command != "./my-daemon" {
		t.Errorf("command=%q", c.Spawn.Command)
	}
	if c.PortForSlot("http", 3) != 8120 {
		t.Errorf("port for slot 3 = %d", c.PortForSlot("http", 3))
	}
}

func TestLoadV1_Full(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
version = 1
max_slots = 5
[ports.http]
base = 8090
step = 10
[ports.rtsp]
base = 8554
step = 10
[spawn]
build = "go build ."
command = "${TESTENV_DIR}/bin"
health_url = "http://localhost:${TESTENV_PORT_HTTP}/health"
health_timeout = "10s"
health_auth = "user:pass"
[spawn.env]
MY_PORT = ":${TESTENV_PORT_HTTP}"
[[spawn.files]]
path = "${TESTENV_DIR}/config.toml"
content = "port = ${TESTENV_PORT_HTTP}"
[[hooks.block]]
match = "my-daemon.*&"
message = "Use testenv up."
[[hooks.warn]]
match = "pkill.*my-daemon"
message = "Use testenv down."
`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxSlots != 5 {
		t.Errorf("max_slots=%d", c.MaxSlots)
	}
	if len(c.Spawn.Files) != 1 {
		t.Errorf("files=%d", len(c.Spawn.Files))
	}
	if len(c.Hooks.Block) != 1 || len(c.Hooks.Warn) != 1 {
		t.Errorf("hooks block=%d warn=%d", len(c.Hooks.Block), len(c.Hooks.Warn))
	}
}

func TestLoadV1_MissingVersion(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
[ports.http]
base = 8090
step = 10
[spawn]
command = "./x"
`)
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "missing required") {
		t.Errorf("expected missing-version error, got %v", err)
	}
}

func TestLoadV1_UnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `version = 99`)
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "newer than") {
		t.Errorf("expected version-too-new error, got %v", err)
	}
}

func TestValidate_NoPorts(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
version = 1
[spawn]
command = "./x"
`)
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Errorf("expected no-ports error, got %v", err)
	}
}

func TestValidate_NoCommand(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
version = 1
[ports.http]
base = 8090
step = 10
[spawn]
build = "make"
`)
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Errorf("expected no-command error, got %v", err)
	}
}

func TestValidate_PortOverflow(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
version = 1
max_slots = 9
[ports.http]
base = 65000
step = 100
[spawn]
command = "./x"
`)
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), ">65535") {
		t.Errorf("expected port-overflow error, got %v", err)
	}
}

func TestValidate_BadPortRef(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
version = 1
[ports.http]
base = 8090
step = 10
[spawn]
command = "./x"
[spawn.env]
MY_VAR = "${TESTENV_PORT_GRPC}"
`)
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "TESTENV_PORT_GRPC") {
		t.Errorf("expected bad-port-ref error, got %v", err)
	}
}

func TestValidate_BadHookRegex(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
version = 1
[ports.http]
base = 8090
step = 10
[spawn]
command = "./x"
[[hooks.block]]
match = "[invalid"
message = "bad"
`)
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "invalid regex") {
		t.Errorf("expected bad-regex error, got %v", err)
	}
}

func TestBuildVars(t *testing.T) {
	c := &V1{
		Ports: map[string]Port{
			"http": {Base: 8090, Step: 10},
			"rtsp": {Base: 8554, Step: 10},
		},
	}
	vars := c.BuildVars(3, "env-abc", "/tmp/data", "/tmp/wt", []string{"device:/dev/video0"})
	if vars["TESTENV_SLOT"] != "3" {
		t.Errorf("SLOT=%s", vars["TESTENV_SLOT"])
	}
	if vars["TESTENV_PORT_HTTP"] != "8120" {
		t.Errorf("PORT_HTTP=%s", vars["TESTENV_PORT_HTTP"])
	}
	if vars["TESTENV_PORT_RTSP"] != "8584" {
		t.Errorf("PORT_RTSP=%s", vars["TESTENV_PORT_RTSP"])
	}
	if vars["TESTENV_LOCKS"] != "device:/dev/video0" {
		t.Errorf("LOCKS=%s", vars["TESTENV_LOCKS"])
	}
}

func TestExpandVars(t *testing.T) {
	vars := map[string]string{
		"TESTENV_PORT_HTTP": "8120",
		"TESTENV_DIR":      "/tmp/data",
	}
	got := ExpandVars("http://localhost:${TESTENV_PORT_HTTP}/health at ${TESTENV_DIR}", vars)
	want := "http://localhost:8120/health at /tmp/data"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b", "c")
	os.MkdirAll(sub, 0o755)
	writeConfig(t, root, `version = 1
[ports.http]
base = 8090
step = 10
[spawn]
command = "./x"
`)
	path, err := find(sub)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != root {
		t.Errorf("found at %s, expected under %s", path, root)
	}
}
