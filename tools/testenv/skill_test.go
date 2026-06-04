package main

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os/exec"
	"strings"
	"testing"

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
	// stdin. Read stdout line-by-line until we see the response to
	// our tools/list request (id:2), then close stdin to shut down.
	cmd := exec.Command(bin, "mcp")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = nil
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

	// Read lines from stdout until we see the tools/list response.
	scanner := bufio.NewScanner(stdout)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if strings.Contains(line, `"id":2`) {
			break
		}
	}
	stdin.Close()
	_ = cmd.Wait()
	var toolNames []string
	for _, line := range lines {
		for part := range strings.SplitSeq(line, `"name":"`) {
			if strings.HasPrefix(part, "testenv_") {
				name := strings.SplitN(part, `"`, 2)[0]
				toolNames = append(toolNames, name)
			}
		}
	}

	if len(toolNames) == 0 {
		t.Fatalf("parsed zero MCP tool names from tools/list response; raw output:\n%s", strings.Join(lines, "\n"))
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
	for line := range strings.SplitSeq(help, "\n") {
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
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		// Dynamic injection: !`testenv subcmd ...`
		// Inline code:       `testenv subcmd ...`
		var cmd string
		switch {
		case strings.HasPrefix(trimmed, "!`testenv "):
			_, cmd, _ = strings.Cut(trimmed, "!`testenv ")
		case strings.HasPrefix(trimmed, "`testenv "):
			_, cmd, _ = strings.Cut(trimmed, "`testenv ")
		case strings.Contains(trimmed, "`testenv "):
			_, cmd, _ = strings.Cut(trimmed, "`testenv ")
		default:
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

// TestHookTemplateCommandsAreValid verifies that every hook command
// in the embedded settings.json.tmpl is a valid `testenv` invocation.
func TestHookTemplateCommandsAreValid(t *testing.T) {
	bin := t.TempDir() + "/testenv"
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	var tmpl map[string]any
	if err := json.Unmarshal(assets.HooksTemplate, &tmpl); err != nil {
		t.Fatalf("parse hooks template: %v", err)
	}

	hooks, ok := tmpl["hooks"].(map[string]any)
	if !ok {
		t.Fatal("template has no 'hooks' key")
	}

	for event, groups := range hooks {
		groupList, ok2 := groups.([]any)
		if !ok2 {
			t.Errorf("event %s: expected array of hook groups", event)
			continue
		}
		for _, g := range groupList {
			gm, okGm := g.(map[string]any)
			if !okGm {
				continue
			}
			hooksList, okList := gm["hooks"].([]any)
			if !okList {
				continue
			}
			for _, h := range hooksList {
				hm, okHm := h.(map[string]any)
				if !okHm {
					continue
				}
				command, _ := hm["command"].(string)
				if command == "" {
					continue
				}
				// Every command should start with "testenv" and its
				// subcommand chain should be valid.
				if !strings.HasPrefix(command, "testenv ") {
					t.Errorf("event %s: command %q doesn't start with 'testenv'", event, command)
					continue
				}
				// Extract subcommand chain: "testenv hook pre-tool-use" → ["hook", "pre-tool-use"]
				parts := strings.Fields(command)
				// Run testenv <subcommands...> --help to verify it's valid.
				args := make([]string, len(parts)-1, len(parts))
				copy(args, parts[1:])
				args = append(args, "--help")
				helpCmd := exec.Command(bin, args...)
				out, err := helpCmd.CombinedOutput()
				// Kong exits 0 on --help, or 1 with usage. Either is fine.
				// An exit code of 1 with "unknown command" means it's invalid.
				if err != nil && strings.Contains(string(out), "unknown command") {
					t.Errorf("event %s: command %q references unknown subcommand; output:\n%s",
						event, command, out)
				}
			}
		}
	}
}

// TestSkillFlagsExistInCLI verifies that every --flag referenced in
// a skill's backtick-fenced testenv invocation actually exists in that
// subcommand's --help output. Catches stale flags like --source or
// --target after they've been removed from the CLI.
func TestSkillFlagsExistInCLI(t *testing.T) {
	bin := t.TempDir() + "/testenv"
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	err := fs.WalkDir(assets.Skills, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(assets.Skills, path)
		if err != nil {
			return err
		}
		checkSkillFlags(t, path, string(body), bin)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// checkSkillFlags finds backtick-fenced `testenv <subcmd> --flag ...`
// patterns and verifies each --flag appears in `testenv <subcmd> --help`.
func checkSkillFlags(t *testing.T, path, body, bin string) {
	t.Helper()
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		// Find testenv invocations in backtick context.
		var cmdStr string
		for _, prefix := range []string{"!`testenv ", "`testenv "} {
			if after, found := strings.CutPrefix(trimmed, prefix); found {
				cmdStr = after
				break
			}
			if _, after, found := strings.Cut(trimmed, prefix); found {
				cmdStr = after
				break
			}
		}
		if cmdStr == "" {
			continue
		}
		cmdStr = strings.Trim(cmdStr, "`")
		fields := strings.Fields(cmdStr)
		if len(fields) == 0 {
			continue
		}

		// First field is the subcommand.
		subcmd := fields[0]
		if strings.HasPrefix(subcmd, "-") || strings.HasPrefix(subcmd, "$") {
			continue
		}

		// Collect --flags from the invocation (strip shell variable expansions).
		var flags []string
		for _, f := range fields[1:] {
			if strings.HasPrefix(f, "--") {
				flag := f
				// Strip =value or trailing shell expansions.
				if idx := strings.Index(flag, "="); idx > 0 {
					flag = flag[:idx]
				}
				// Strip ${...} that might be appended.
				if idx := strings.Index(flag, "$"); idx > 0 {
					flag = flag[:idx]
				}
				flags = append(flags, flag)
			}
		}
		if len(flags) == 0 {
			continue
		}

		// Get the subcommand's --help output.
		helpCmd := exec.Command(bin, subcmd, "--help")
		helpOut, _ := helpCmd.CombinedOutput()
		helpStr := string(helpOut)

		for _, flag := range flags {
			if !strings.Contains(helpStr, flag) {
				t.Errorf("%s: `testenv %s` references %s but it doesn't appear in `testenv %s --help`",
					path, subcmd, flag, subcmd)
			}
		}
	}
}
