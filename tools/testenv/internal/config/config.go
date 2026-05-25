// Package config loads and validates .testenv.toml project configuration.
//
// The config is versioned: the top-level `version` field selects which
// struct and code path parses the rest. Each version is a completely
// separate type and loader. Currently only version 1 exists.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const FileName = ".testenv.toml"
const LocalFileName = ".testenv.local.toml"

// MaxSupportedVersion is the highest config version this binary knows.
const MaxSupportedVersion = 1

// --- version envelope (parsed first to pick the code path) ---

type versionEnvelope struct {
	Version int `toml:"version"`
}

// Load reads .testenv.toml from dir (walking up to find it) and
// returns the parsed, validated config. Returns an error if the file
// is missing, the version is unsupported, or validation fails.
func Load(dir string) (*V1, error) {
	path, err := find(dir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var env versionEnvelope
	if err := toml.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("parse version from %s: %w", path, err)
	}

	var cfg *V1
	switch env.Version {
	case 0:
		return nil, fmt.Errorf("%s: missing required top-level `version` field", path)
	case 1:
		c, err := loadV1(data, path)
		if err != nil {
			return nil, err
		}
		cfg = c
	default:
		if env.Version > MaxSupportedVersion {
			return nil, fmt.Errorf("%s: config version %d is newer than this binary supports (max %d) — upgrade testenv",
				path, env.Version, MaxSupportedVersion)
		}
		return nil, fmt.Errorf("%s: unsupported config version %d", path, env.Version)
	}

	localPath := filepath.Join(filepath.Dir(path), LocalFileName)
	if localData, err := os.ReadFile(localPath); err == nil {
		var local V1
		if err := toml.Unmarshal(localData, &local); err != nil {
			return nil, fmt.Errorf("parse %s: %w", localPath, err)
		}
		mergeV1(cfg, &local)
		cfg.LocalPath = localPath
		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("merged config invalid: %w", err)
		}
	}

	return cfg, nil
}

// FindProjectRoot walks up from the process cwd looking for .testenv.toml
// and returns the directory that contains it.
func FindProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	p, err := find(dir)
	if err != nil {
		return "", err
	}
	return filepath.Dir(p), nil
}

// find walks from dir upward looking for .testenv.toml.
func find(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, FileName)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%s not found (searched from %s to /)", FileName, dir)
		}
		dir = parent
	}
}

// --- V1 config ---

// V1 is the version-1 config schema.
type V1 struct {
	Version  int               `toml:"version"`
	MaxSlots int               `toml:"max_slots"`
	Ports    map[string]Port   `toml:"ports"`
	Spawn    SpawnV1           `toml:"spawn"`
	Hooks    HooksV1           `toml:"hooks"`
	Path      string            `toml:"-"` // resolved file path
	LocalPath string            `toml:"-"` // resolved local override path, empty if none
}

type Port struct {
	Base int `toml:"base"`
	Step int `toml:"step"`
}

type SpawnV1 struct {
	Build        string            `toml:"build"`
	Command      string            `toml:"command"`
	HealthURL    string            `toml:"health_url"`
	HealthTimeout string           `toml:"health_timeout"`
	HealthAuth   string            `toml:"health_auth"`
	Env          map[string]string `toml:"env"`
	Files        []SpawnFile       `toml:"files"`
}

type SpawnFile struct {
	Path    string `toml:"path"`
	Content string `toml:"content"`
}

type HooksV1 struct {
	Block []HookPattern `toml:"block"`
	Warn  []HookPattern `toml:"warn"`
}

type HookPattern struct {
	Match    string `toml:"match"`
	CwdMatch string `toml:"cwd_match"`
	Message  string `toml:"message"`
}

func loadV1(data []byte, path string) (*V1, error) {
	var c V1
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	c.Path = path
	if c.MaxSlots == 0 {
		c.MaxSlots = 9
	}
	return &c, c.Validate()
}

func mergeV1(base, local *V1) {
	if local.MaxSlots != 0 {
		base.MaxSlots = local.MaxSlots
	}

	for name, p := range local.Ports {
		if base.Ports == nil {
			base.Ports = make(map[string]Port)
		}
		base.Ports[name] = p
	}

	if local.Spawn.Build != "" {
		base.Spawn.Build = local.Spawn.Build
	}
	if local.Spawn.Command != "" {
		base.Spawn.Command = local.Spawn.Command
	}
	if local.Spawn.HealthURL != "" {
		base.Spawn.HealthURL = local.Spawn.HealthURL
	}
	if local.Spawn.HealthTimeout != "" {
		base.Spawn.HealthTimeout = local.Spawn.HealthTimeout
	}
	if local.Spawn.HealthAuth != "" {
		base.Spawn.HealthAuth = local.Spawn.HealthAuth
	}

	for k, v := range local.Spawn.Env {
		if base.Spawn.Env == nil {
			base.Spawn.Env = make(map[string]string)
		}
		base.Spawn.Env[k] = v
	}

	if len(local.Spawn.Files) > 0 {
		base.Spawn.Files = local.Spawn.Files
	}
	if len(local.Hooks.Block) > 0 {
		base.Hooks.Block = local.Hooks.Block
	}
	if len(local.Hooks.Warn) > 0 {
		base.Hooks.Warn = local.Hooks.Warn
	}
}

// Validate checks the V1 config for internal consistency.
func (c *V1) Validate() error {
	var errs []string

	if c.Version != 1 {
		errs = append(errs, fmt.Sprintf("version must be 1, got %d", c.Version))
	}

	if len(c.Ports) == 0 {
		errs = append(errs, "at least one [ports.*] definition required")
	}
	for name, p := range c.Ports {
		if p.Base <= 0 || p.Base > 65535 {
			errs = append(errs, fmt.Sprintf("ports.%s.base=%d out of range", name, p.Base))
		}
		if p.Step <= 0 {
			errs = append(errs, fmt.Sprintf("ports.%s.step must be positive", name))
		}
		maxPort := p.Base + p.Step*c.MaxSlots
		if maxPort > 65535 {
			errs = append(errs, fmt.Sprintf("ports.%s: slot %d would use port %d (>65535)", name, c.MaxSlots, maxPort))
		}
	}

	// Check for port overlap across slots.
	for i := 1; i <= c.MaxSlots; i++ {
		seen := map[int]string{}
		for name, p := range c.Ports {
			port := p.Base + p.Step*i
			if other, ok := seen[port]; ok {
				errs = append(errs, fmt.Sprintf("slot %d: ports.%s and ports.%s both resolve to :%d", i, name, other, port))
			}
			seen[port] = name
		}
	}

	if c.Spawn.Command == "" {
		errs = append(errs, "spawn.command is required")
	}

	// Validate template vars reference defined port names.
	definedPorts := map[string]bool{}
	for name := range c.Ports {
		definedPorts[strings.ToUpper(name)] = true
	}
	allTemplates := collectTemplateStrings(c)
	for _, tmpl := range allTemplates {
		for _, ref := range extractPortRefs(tmpl) {
			if !definedPorts[ref] {
				errs = append(errs, fmt.Sprintf("template references ${TESTENV_PORT_%s} but no [ports.%s] defined", ref, strings.ToLower(ref)))
			}
		}
	}

	// Validate hook regex patterns compile.
	for i, h := range c.Hooks.Block {
		if _, err := regexp.Compile(h.Match); err != nil {
			errs = append(errs, fmt.Sprintf("hooks.block[%d].match: invalid regex: %v", i, err))
		}
	}
	for i, h := range c.Hooks.Warn {
		if _, err := regexp.Compile(h.Match); err != nil {
			errs = append(errs, fmt.Sprintf("hooks.warn[%d].match: invalid regex: %v", i, err))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// PortForSlot returns the port number for a named port at a given slot.
func (c *V1) PortForSlot(name string, slot int) int {
	p, ok := c.Ports[name]
	if !ok {
		return 0
	}
	return p.Base + p.Step*slot
}

// PortNames returns sorted port names for deterministic iteration.
func (c *V1) PortNames() []string {
	names := make([]string, 0, len(c.Ports))
	for name := range c.Ports {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

// ExpandVars replaces ${TESTENV_*} references in s with values from vars.
func ExpandVars(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}

// BuildVars constructs the TESTENV_* variable map for a given slot + env.
func (c *V1) BuildVars(slot int, envID, dataDir, worktree string, locks []string) map[string]string {
	vars := map[string]string{
		"TESTENV_SLOT":     fmt.Sprintf("%d", slot),
		"TESTENV_ENV_ID":   envID,
		"TESTENV_DIR":      dataDir,
		"TESTENV_WORKTREE": worktree,
		"TESTENV_LOCKS":    strings.Join(locks, ","),
	}
	for name := range c.Ports {
		port := c.PortForSlot(name, slot)
		vars["TESTENV_PORT_"+strings.ToUpper(name)] = fmt.Sprintf("%d", port)
	}
	return vars
}

// --- helpers ---

func collectTemplateStrings(c *V1) []string {
	var out []string
	out = append(out, c.Spawn.Build, c.Spawn.Command, c.Spawn.HealthURL)
	for _, v := range c.Spawn.Env {
		out = append(out, v)
	}
	for _, f := range c.Spawn.Files {
		out = append(out, f.Path, f.Content)
	}
	return out
}

func extractPortRefs(s string) []string {
	var refs []string
	prefix := "${TESTENV_PORT_"
	for {
		i := strings.Index(s, prefix)
		if i < 0 {
			break
		}
		s = s[i+len(prefix):]
		j := strings.Index(s, "}")
		if j < 0 {
			break
		}
		refs = append(refs, s[:j])
		s = s[j+1:]
	}
	return refs
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
