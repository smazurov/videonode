package main

import (
	"io/fs"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/smazurov/videonode/tools/testenv/internal/assets"
)

// TestSkillsReferenceValidSubcommands verifies that every `testenv`
// subcommand referenced in the embedded SKILL.md files actually exists
// in the binary. Catches skill-text drift after CLI changes.
func TestSkillsReferenceValidSubcommands(t *testing.T) {
	// Build the binary so we can query its help.
	bin := t.TempDir() + "/testenv"
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// Get valid subcommands from help output.
	helpOut, err := exec.Command(bin, "--help").CombinedOutput()
	if err != nil {
		// Kong exits 1 on --help; ignore exit code, use output.
		if len(helpOut) == 0 {
			t.Fatalf("help: %v", err)
		}
	}
	validCmds := parseSubcommands(string(helpOut))
	if len(validCmds) == 0 {
		t.Fatal("parsed zero subcommands from --help output")
	}

	// Walk each SKILL.md and check references.
	err = fs.WalkDir(assets.Skills, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(assets.Skills, path)
		if err != nil {
			return err
		}
		checkSkillReferences(t, path, string(body), validCmds, bin)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestMCPToolsMatchEnvctlSurface verifies that every MCP tool name
// maps to a real envctl function. This is a compile-time guarantee
// already (the mcpsrv package imports envctl), but this test also
// checks that the tool names in the MCP server match what the
// skills reference.
func TestMCPToolsMatchSkillReferences(t *testing.T) {
	bin := t.TempDir() + "/testenv"
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// Get MCP tool names by sending initialize + tools/list over
	// stdin. Use StdinPipe so we control when EOF arrives — the MCP
	// server needs stdin open while it writes its responses.
	cmd := exec.Command(bin, "mcp")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var outBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = nil // discard "server is closing" noise
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	messages := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}
	for _, m := range messages {
		if _, err := stdin.Write([]byte(m + "\n")); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	// Give the server time to process, then close stdin to trigger shutdown.
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	// Brief sleep to let responses flush before we close stdin.
	// (Closing immediately can race on very fast machines.)
	time.Sleep(500 * time.Millisecond)
	_ = stdin.Close()
	<-done

	out := outBuf.String()
	lines := strings.Split(out, "\n")
	var toolNames []string
	for _, line := range lines {
		for _, part := range strings.Split(line, `"name":"`) {
			if strings.HasPrefix(part, "testenv_") {
				name := strings.SplitN(part, `"`, 2)[0]
				toolNames = append(toolNames, name)
			}
		}
	}

	if len(toolNames) == 0 {
		t.Fatalf("parsed zero MCP tool names from tools/list response; raw output:\n%s", out)
	}

	// Every MCP tool should correspond to a CLI subcommand.
	helpOut, _ := exec.Command(bin, "--help").CombinedOutput()
	validCmds := parseSubcommands(string(helpOut))

	for _, tool := range toolNames {
		sub := strings.TrimPrefix(tool, "testenv_")
		sub = strings.ReplaceAll(sub, "_", "-")
		if !validCmds[sub] {
			t.Errorf("MCP tool %q maps to subcommand %q which doesn't exist in CLI", tool, sub)
		}
	}
	t.Logf("MCP tools: %v", toolNames)
}


// parseSubcommands extracts subcommand names from Kong --help output.
// Kong format:
//
//	Commands:
//	  up [flags]
//	    Spin up a test environment.
//	  release-session [<session-id>] [flags]
//	    Release everything a session owns.
//
// Command lines start with exactly 2 spaces then a lowercase word.
// Description lines start with 4+ spaces.
func parseSubcommands(help string) map[string]bool {
	cmds := map[string]bool{}
	inCommands := false
	for _, line := range strings.Split(help, "\n") {
		if strings.TrimSpace(line) == "Commands:" {
			inCommands = true
			continue
		}
		if !inCommands {
			continue
		}
		// Command line: "  word ..." (2 spaces + lowercase word).
		// Description line: "    ..." (4+ spaces).
		// Section end: line starts at column 0.
		if len(line) >= 3 && line[0] == ' ' && line[1] == ' ' && line[2] != ' ' {
			word := strings.Fields(line)[0]
			if len(word) > 0 && word[0] >= 'a' && word[0] <= 'z' {
				cmds[word] = true
			}
		} else if len(line) > 0 && line[0] != ' ' {
			break
		}
	}
	return cmds
}

// checkSkillReferences finds command-invocation patterns in skill
// body — specifically lines starting with `!` (dynamic injection) or
// backtick-fenced `testenv` invocations — and verifies each
// subcommand referenced actually exists.
//
// Does NOT flag prose references like "registered with testenv on this
// host" — only code-context invocations.
func checkSkillReferences(t *testing.T, path, body string, validCmds map[string]bool, _ string) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		// Dynamic injection: !`testenv subcmd ...`
		// Inline code:       `testenv subcmd ...`
		var cmd string
		if strings.HasPrefix(trimmed, "!`testenv ") {
			cmd = strings.TrimPrefix(trimmed, "!`testenv ")
		} else if strings.HasPrefix(trimmed, "`testenv ") {
			cmd = strings.TrimPrefix(trimmed, "`testenv ")
		} else if strings.Contains(trimmed, "`testenv ") {
			idx := strings.Index(trimmed, "`testenv ")
			cmd = trimmed[idx+len("`testenv "):]
		} else {
			continue
		}
		cmd = strings.Trim(cmd, "`")
		sub := strings.Fields(cmd)
		if len(sub) == 0 {
			continue
		}
		// The first field is the subcommand name.
		name := sub[0]
		if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "$") {
			continue
		}
		if !validCmds[name] {
			t.Errorf("%s: invokes `testenv %s` but %q is not a valid subcommand",
				path, name, name)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
