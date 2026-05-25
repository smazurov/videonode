package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/smazurov/videonode/tools/testenv/internal/assets"
	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

// InstallCmd writes the embedded skills + hook entries + .mcp.json
// into a project directory.
type InstallCmd struct {
	ProjectDir string `default:"." help:"Project root that owns .claude/."`
	DryRun     bool   `help:"Print what would change; don't write."`
}

func (c *InstallCmd) Run(ctx *Context) error {
	root, err := filepath.Abs(c.ProjectDir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("project %s has no .claude/ dir — is it a Claude Code project?", root)
		}
		return err
	}

	// Require a valid .testenv.toml before installing hooks/skills.
	if _, err := envctl.Validate(root); err != nil {
		return fmt.Errorf("config invalid (run `testenv init` to create one, then `testenv validate`): %w", err)
	}

	// Walk embedded skills/ and copy each SKILL.md to .claude/skills/<name>/SKILL.md.
	skillsFS := assets.Skills
	err = fs.WalkDir(skillsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		dst := filepath.Join(root, ".claude", "skills", path)
		if c.DryRun {
			fmt.Fprintf(stdout(), "would write %s\n", dst)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		body, err := fs.ReadFile(skillsFS, path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(stdout(), "wrote %s\n", dst)
		return nil
	})
	if err != nil {
		return fmt.Errorf("write skills: %w", err)
	}

	// Merge hooks into .claude/settings.json.
	if err := mergeHooks(root, c.DryRun); err != nil {
		return fmt.Errorf("merge hooks: %w", err)
	}

	// Write .mcp.json registering the stdio MCP server.
	if err := writeMCPJSON(root, c.DryRun); err != nil {
		return fmt.Errorf("write .mcp.json: %w", err)
	}

	return nil
}

func mergeHooks(root string, dryRun bool) error {
	settingsPath := filepath.Join(root, ".claude", "settings.json")
	var settings map[string]any
	if b, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(b, &settings); err != nil {
			return fmt.Errorf("parse existing %s: %w", settingsPath, err)
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}

	var template map[string]any
	if err := json.Unmarshal(assets.HooksTemplate, &template); err != nil {
		return fmt.Errorf("parse embedded hooks template: %w", err)
	}
	tplHooks, _ := template["hooks"].(map[string]any)

	existingHooks, _ := settings["hooks"].(map[string]any)
	if existingHooks == nil {
		existingHooks = map[string]any{}
	}

	for event, group := range tplHooks {
		groupList, _ := group.([]any)
		existing, _ := existingHooks[event].([]any)
		// Append our entries, dedup by JSON-marshalled equality.
		seen := map[string]bool{}
		for _, e := range existing {
			b, _ := json.Marshal(e)
			seen[string(b)] = true
		}
		for _, g := range groupList {
			b, _ := json.Marshal(g)
			if !seen[string(b)] {
				existing = append(existing, g)
			}
		}
		existingHooks[event] = existing
	}
	settings["hooks"] = existingHooks

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if dryRun {
		fmt.Fprintf(stdout(), "would update %s\n", settingsPath)
		return nil
	}
	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout(), "updated %s (SessionStart/SessionEnd hooks merged)\n", settingsPath)
	return nil
}

func writeMCPJSON(root string, dryRun bool) error {
	path := filepath.Join(root, ".mcp.json")
	exe, err := os.Executable()
	if err != nil {
		// Fall back to a $PATH lookup so the .mcp.json stays portable.
		exe = "testenv"
	}
	doc := map[string]any{
		"mcpServers": map[string]any{
			"testenv": map[string]any{
				"command": exe,
				"args":    []string{"mcp"},
				"env":     map[string]string{},
			},
		},
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if dryRun {
		fmt.Fprintf(stdout(), "would write %s with command=%s\n", path, exe)
		return nil
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout(), "wrote %s (command=%s)\n", path, exe)
	return nil
}
