package envctl

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

type sessionRegistry struct {
	Sessions map[string]string `json:"sessions"`
}

func sessionsPath(statePath string) string {
	if statePath == "" {
		statePath = defaultStatePath()
	}
	return filepath.Join(filepath.Dir(statePath), "sessions.json")
}

func defaultStatePath() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "testenv", "state.db")
}

// RegisterSession writes a session→cwd mapping to the sessions registry.
func RegisterSession(statePath, sessionID, cwd string) error {
	path := sessionsPath(statePath)
	lk := flock.New(path + ".lock")
	if err := lk.Lock(); err != nil {
		return err
	}
	defer lk.Unlock()

	reg := readRegistry(path)
	reg.Sessions[sessionID] = cwd
	return writeRegistry(path, reg)
}

// LookupSession returns the cwd for a session, or "" if not found.
func LookupSession(statePath, sessionID string) (string, error) {
	if sessionID == "" {
		return "", nil
	}
	reg := readRegistry(sessionsPath(statePath))
	return reg.Sessions[sessionID], nil
}

// UnregisterSession removes a session from the registry.
func UnregisterSession(statePath, sessionID string) error {
	path := sessionsPath(statePath)
	lk := flock.New(path + ".lock")
	if err := lk.Lock(); err != nil {
		return err
	}
	defer lk.Unlock()

	reg := readRegistry(path)
	delete(reg.Sessions, sessionID)
	return writeRegistry(path, reg)
}

func readRegistry(path string) sessionRegistry {
	reg := sessionRegistry{Sessions: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return reg
	}
	json.Unmarshal(data, &reg)
	if reg.Sessions == nil {
		reg.Sessions = map[string]string{}
	}
	return reg
}

func writeRegistry(path string, reg sessionRegistry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(reg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
