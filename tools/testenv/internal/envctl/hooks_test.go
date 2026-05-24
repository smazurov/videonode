package envctl

import (
	"strings"
	"testing"
)

func TestEvalPreToolUse_ManualDaemonSpawn(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantBlk bool
	}{
		{
			"nohup videonode backgrounded",
			`nohup ./tmp/main > /tmp/videonode-daemon.log 2>&1 &`,
			true,
		},
		{
			"videonode with port flags",
			`/home/stepan/.claude/jobs/xxx/videonode -p :8099 --srt-addr :6099 --streaming-rtsp-port :8559`,
			true,
		},
		{
			"./videonode with config and background",
			`./videonode --config /tmp/vntest/config.toml > /tmp/vntest/daemon.log 2>&1 &`,
			true,
		},
		{
			"env var spawn",
			`VIDEONODE_SERVER_PORT=:8100 ./videonode > /dev/null 2>&1 &`,
			true,
		},
		{
			"go build is fine",
			`go build -o videonode .`,
			false,
		},
		{
			"go test is fine",
			`go test -v ./internal/streams/...`,
			false,
		},
		{
			"testenv up is fine",
			`testenv up --target host --source fake`,
			false,
		},
		{
			"curl is not a spawn",
			`curl -s http://localhost:8090/api/health`,
			false,
		},
		{
			"videonode subcommand is fine",
			`./videonode validate-encoders`,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := `{"tool_name":"Bash","tool_input":{"command":` + jsonStr(tt.cmd) + `},"session_id":"s","cwd":"/tmp"}`
			d := EvalPreToolUse(strings.NewReader(payload))
			if d.Block != tt.wantBlk {
				t.Errorf("Block=%v want %v (msg=%s)", d.Block, tt.wantBlk, d.Message)
			}
		})
	}
}

func TestEvalPreToolUse_ManualKill(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantMsg bool
	}{
		{"pkill videonode", `pkill -f videonode-source`, true},
		{"kill pid", `kill 3839890`, false}, // no "videonode" target
		{"kill with videonode in args", `pkill -f "videonode --config"`, true},
		{"testenv down is fine", `testenv down env-1234`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := `{"tool_name":"Bash","tool_input":{"command":` + jsonStr(tt.cmd) + `},"session_id":"s","cwd":"/tmp"}`
			d := EvalPreToolUse(strings.NewReader(payload))
			if tt.wantMsg && d.Message == "" {
				t.Error("expected a warning message, got empty")
			}
			if !tt.wantMsg && d.Message != "" {
				t.Errorf("expected no message, got: %s", d.Message)
			}
		})
	}
}

func TestEvalPreToolUse_CmakeInstallFromWorktree(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		cwd     string
		wantBlk bool
	}{
		{
			"cmake install from worktree",
			"cmake --install composer/build/dev",
			"/home/user/dev/videonode/.claude/worktrees/my-branch",
			true,
		},
		{
			"cmake install from main checkout is fine",
			"cmake --install composer/build/dev",
			"/home/user/dev/videonode",
			false,
		},
		{
			"cmake build is fine",
			"cmake --build --preset dev",
			"/home/user/dev/videonode/.claude/worktrees/my-branch",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := `{"tool_name":"Bash","tool_input":{"command":` + jsonStr(tt.cmd) + `},"session_id":"s","cwd":` + jsonStr(tt.cwd) + `}`
			d := EvalPreToolUse(strings.NewReader(payload))
			if d.Block != tt.wantBlk {
				t.Errorf("Block=%v want %v", d.Block, tt.wantBlk)
			}
		})
	}
}

func TestEvalPreToolUse_NonBashAllowed(t *testing.T) {
	payload := `{"tool_name":"Read","tool_input":{"file_path":"/etc/passwd"},"session_id":"s","cwd":"/"}`
	d := EvalPreToolUse(strings.NewReader(payload))
	if d.Block || d.Message != "" {
		t.Errorf("non-Bash tool should be allowed; got Block=%v msg=%q", d.Block, d.Message)
	}
}

func TestEvalPreToolUse_MalformedJSON(t *testing.T) {
	d := EvalPreToolUse(strings.NewReader("not json"))
	if d.Block || d.Message != "" {
		t.Errorf("malformed JSON should be allowed; got Block=%v msg=%q", d.Block, d.Message)
	}
}

func TestEvalPostToolUse_EnterWorktree(t *testing.T) {
	dir := t.TempDir()
	statePath := dir + "/state.db"
	payload := `{"tool_name":"EnterWorktree","session_id":"sess-42","cwd":"/home/user/dev/proj/.claude/worktrees/my-branch"}`
	EvalPostToolUse(strings.NewReader(payload), statePath)

	cwd, err := LookupSession(statePath, "sess-42")
	if err != nil {
		t.Fatal(err)
	}
	if cwd != "/home/user/dev/proj/.claude/worktrees/my-branch" {
		t.Errorf("got %q", cwd)
	}
}

func TestEvalPostToolUse_OtherTool(t *testing.T) {
	dir := t.TempDir()
	statePath := dir + "/state.db"
	payload := `{"tool_name":"Bash","session_id":"sess-42","cwd":"/tmp"}`
	EvalPostToolUse(strings.NewReader(payload), statePath)

	cwd, _ := LookupSession(statePath, "sess-42")
	if cwd != "" {
		t.Errorf("non-EnterWorktree should not register; got %q", cwd)
	}
}

func jsonStr(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
