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

func writeLocalConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, LocalFileName)
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
		"TESTENV_DIR":       "/tmp/data",
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

const baseConfig = `
version = 1
[ports.http]
base = 8090
step = 10
[spawn]
build = "make"
command = "./daemon"
[spawn.env]
X = "1"
[[spawn.files]]
path = "/tmp/a"
content = "a"
`

func TestLoadV1_LocalOverrideScalar(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, baseConfig)
	writeLocalConfig(t, dir, `
[spawn]
build = "go build ."
`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Spawn.Build != "go build ." {
		t.Errorf("build=%q, want %q", c.Spawn.Build, "go build .")
	}
	if c.Spawn.Command != "./daemon" {
		t.Errorf("command=%q, want %q (should be preserved from base)", c.Spawn.Command, "./daemon")
	}
	if c.LocalPath == "" {
		t.Error("LocalPath should be set")
	}
}

func TestLoadV1_LocalOverrideEnvMerge(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, baseConfig)
	writeLocalConfig(t, dir, `
[spawn.env]
Y = "2"
`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Spawn.Env["X"] != "1" {
		t.Errorf("env X=%q, want %q", c.Spawn.Env["X"], "1")
	}
	if c.Spawn.Env["Y"] != "2" {
		t.Errorf("env Y=%q, want %q", c.Spawn.Env["Y"], "2")
	}
}

func TestLoadV1_LocalOverrideEnvReplace(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, baseConfig)
	writeLocalConfig(t, dir, `
[spawn.env]
X = "overridden"
`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Spawn.Env["X"] != "overridden" {
		t.Errorf("env X=%q, want %q", c.Spawn.Env["X"], "overridden")
	}
}

func TestLoadV1_LocalOverrideFilesReplace(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, baseConfig)
	writeLocalConfig(t, dir, `
[[spawn.files]]
path = "/tmp/b"
content = "b"
[[spawn.files]]
path = "/tmp/c"
content = "c"
`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Spawn.Files) != 2 {
		t.Fatalf("files=%d, want 2", len(c.Spawn.Files))
	}
	if c.Spawn.Files[0].Path != "/tmp/b" {
		t.Errorf("files[0].path=%q, want /tmp/b", c.Spawn.Files[0].Path)
	}
}

func TestLoadV1_Binaries(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
version = 1
[ports.http]
base = 8090
step = 10
[spawn]
command = "./daemon"
[[spawn.binaries]]
env = "NATIVE_COMPOSER"
path = "${TESTENV_WORKTREE}/bin/composer"
`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Spawn.Binaries) != 1 {
		t.Fatalf("binaries=%d, want 1", len(c.Spawn.Binaries))
	}
	if c.Spawn.Binaries[0].Env != "NATIVE_COMPOSER" {
		t.Errorf("env=%q", c.Spawn.Binaries[0].Env)
	}
}

func TestValidate_BinaryMissingEnv(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
version = 1
[ports.http]
base = 8090
step = 10
[spawn]
command = "./daemon"
[[spawn.binaries]]
path = "/bin/x"
`)
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "binaries[0].env is required") {
		t.Errorf("expected missing-env error, got %v", err)
	}
}

func TestResolveBinaries_PresentAndMissing(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "videonode-composer")
	if err := os.WriteFile(present, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &V1{Spawn: SpawnV1{Binaries: []SpawnBinary{
		{Env: "COMPOSER", Path: "${TESTENV_WORKTREE}/videonode-composer"},
		{Env: "SOURCE", Path: "${TESTENV_WORKTREE}/videonode-source"},
	}}}
	vars := map[string]string{"TESTENV_WORKTREE": dir}
	got, missing := c.ResolveBinaries(vars)
	if len(got) != 1 || got[0] != "COMPOSER="+present {
		t.Errorf("present=%v, want [COMPOSER=%s]", got, present)
	}
	if len(missing) != 1 || missing[0] != "SOURCE" {
		t.Errorf("missing=%v, want [SOURCE]", missing)
	}
}

func TestLoadV1_LocalOverrideBinariesReplace(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, baseConfig)
	writeLocalConfig(t, dir, `
[[spawn.binaries]]
env = "X_BIN"
path = "/tmp/x"
`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Spawn.Binaries) != 1 || c.Spawn.Binaries[0].Env != "X_BIN" {
		t.Errorf("binaries=%v", c.Spawn.Binaries)
	}
}

func TestLoadV1_NoLocalFile(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, baseConfig)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Spawn.Build != "make" {
		t.Errorf("build=%q, want %q", c.Spawn.Build, "make")
	}
	if c.LocalPath != "" {
		t.Errorf("LocalPath=%q, want empty", c.LocalPath)
	}
}
