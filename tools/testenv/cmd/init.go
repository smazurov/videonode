package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

// InitCmd writes a commented .testenv.toml template into the project.
type InitCmd struct {
	Dir   string `default:"." help:"Directory to write .testenv.toml into."`
	Force bool   `help:"Overwrite existing .testenv.toml."`
}

func (c *InitCmd) Run(_ *Context) error {
	dir, err := filepath.Abs(c.Dir)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, envctl.ConfigFileName)
	if _, err := os.Stat(path); err == nil && !c.Force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", path)
	}

	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("directory %s does not exist", dir)
	}

	if err := os.WriteFile(path, []byte(configTemplate), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout(), "wrote %s\n", path)
	fmt.Fprintln(stdout(), "edit the file to match your project, then run: testenv validate && testenv install")
	return nil
}

const configTemplate = `# testenv configuration — defines how to spawn isolated test environments.
# See: testenv validate (checks this file) / testenv install (writes hooks+skills)
#
# All values in [spawn] support ${TESTENV_*} variable expansion:
#   TESTENV_SLOT       — allocated slot number (1..max_slots)
#   TESTENV_ENV_ID     — unique env identifier
#   TESTENV_DIR        — per-env data directory
#   TESTENV_WORKTREE   — absolute path to the working tree
#   TESTENV_LOCKS      — comma-separated held lock strings
#   TESTENV_PORT_<NAME> — one per [ports.*] entry (uppercase name)

version = 1

# Maximum number of parallel test environments (slot range 1..N).
max_slots = 9

# --- Port definitions ---
# Each named port gets: base + step * slot_number.
# Add as many as your project needs. Names become TESTENV_PORT_<UPPERCASE>.

[ports.http]
base = 8080     # slot 1 → 8090, slot 2 → 8100, ...
step = 10

# [ports.grpc]
# base = 50050
# step = 1

# --- Spawn configuration ---

[spawn]
# Build step (optional). Runs before the daemon. Use for compilation,
# config generation, etc. Runs in the working tree directory.
# build = "go build -o ${TESTENV_DIR}/myapp ."

# The daemon process to run (required). PID is tracked by testenv.
command = "${TESTENV_DIR}/myapp"

# Health check (optional). Polled until HTTP 200 or timeout.
# health_url = "http://localhost:${TESTENV_PORT_HTTP}/health"
# health_timeout = "15s"
# health_auth = "user:pass"        # basic auth, optional

# Environment variables passed to build + command.
# Values support ${TESTENV_*} expansion.
[spawn.env]
# MY_PORT = ":${TESTENV_PORT_HTTP}"

# Files to write before build/command. Content supports ${TESTENV_*}.
# Use for config files that need per-env port numbers, IDs, etc.
#
# [[spawn.files]]
# path = "${TESTENV_DIR}/config.toml"
# content = """
# port = ${TESTENV_PORT_HTTP}
# """

# --- Hook patterns (optional) ---
# PreToolUse patterns that steer Claude toward testenv instead of
# manual daemon management. "block" exits 2; "warn" prints to stderr.
#
# [[hooks.block]]
# match = "myapp.*--port|nohup.*myapp"
# message = "Use testenv up instead of spawning manually."
#
# [[hooks.warn]]
# match = "pkill.*myapp"
# message = "Consider testenv down to release the env cleanly."
`
