package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"slices"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/logging"
)

const (
	defaultLoginGroup = "videonode"
	defaultHelperPath = "/usr/bin/videonode-session"
	helperTimeout     = 30 * time.Second
)

// helperReasons maps FAIL tokens printed by videonode-session to Reason*
// values. Tokens outside this allowlist are treated as system errors.
var helperReasons = map[string]string{
	"invalid_password": ReasonInvalidPassword,
	"unknown_user":     ReasonUnknownUser,
	"account_expired":  ReasonAccountExpired,
	"account_locked":   ReasonAccountLocked,
}

// GroupChecker resolves group membership; injectable for tests.
type GroupChecker interface {
	InGroup(username, group string) (bool, error)
}

type osGroupChecker struct{}

// InGroup reports whether username is a member of the named Unix group.
func (osGroupChecker) InGroup(username, group string) (bool, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return false, err
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return false, fmt.Errorf("lookup group %q: %w", group, err)
	}
	gids, err := u.GroupIds()
	if err != nil {
		return false, fmt.Errorf("group ids for %s: %w", u.Username, err)
	}
	return slices.Contains(gids, g.Gid), nil
}

// LinuxAuthenticator validates credentials by delegating the PAM conversation
// to the setuid-root videonode-session helper, gated on membership in a Unix
// group (default "videonode"). The daemon itself never reads /etc/shadow.
type LinuxAuthenticator struct {
	group  string
	helper string
	groups GroupChecker
	logger logging.Logger
}

// NewLinux creates an authenticator backed by the videonode-session helper
// and the videonode login group.
func NewLinux(logger logging.Logger) *LinuxAuthenticator {
	return &LinuxAuthenticator{
		group:  defaultLoginGroup,
		helper: defaultHelperPath,
		groups: osGroupChecker{},
		logger: logger,
	}
}

// WithGroup overrides the required login group (used by tests).
func (a *LinuxAuthenticator) WithGroup(group string) *LinuxAuthenticator {
	a.group = group
	return a
}

// WithHelperPath overrides the videonode-session binary path (used by tests).
func (a *LinuxAuthenticator) WithHelperPath(path string) *LinuxAuthenticator {
	a.helper = path
	return a
}

// WithGroupChecker overrides the group-membership resolver (used by tests).
func (a *LinuxAuthenticator) WithGroupChecker(g GroupChecker) *LinuxAuthenticator {
	a.groups = g
	return a
}

// Authenticate validates the user's password via the videonode-session helper
// after confirming the user is in the configured login group. Every accept and
// every reject is logged at Info level with a structured "reason".
func (a *LinuxAuthenticator) Authenticate(username, password string) Result {
	inGroup, err := a.groups.InGroup(username, a.group)
	if err != nil {
		var unknownUser user.UnknownUserError
		if errors.Is(err, user.UnknownUserError(username)) || errors.As(err, &unknownUser) {
			return a.reject(username, ReasonUnknownUser)
		}
		a.logError("group lookup failed", username, err)
		return Result{Valid: false, Username: username, Reason: ReasonSystemError, Error: err}
	}
	if !inGroup {
		return a.reject(username, ReasonNotInGroup, logging.KeyGroup, a.group)
	}

	verdict, err := a.runHelper(username, password)
	if err != nil {
		a.logError("session helper failed — is videonode-session installed setuid root?",
			username, err)
		return Result{Valid: false, Username: username, Reason: ReasonSystemError, Error: err}
	}

	if verdict == "OK" {
		if a.logger != nil {
			a.logger.Debug("auth accepted", logging.KeyUsername, username)
		}
		return Result{Valid: true, Username: username}
	}
	if token, ok := strings.CutPrefix(verdict, "FAIL "); ok {
		if reason, known := helperReasons[token]; known {
			return a.reject(username, reason)
		}
	}
	err = fmt.Errorf("unexpected helper verdict %q", verdict)
	a.logError("session helper returned unexpected verdict", username, err)
	return Result{Valid: false, Username: username, Reason: ReasonSystemError, Error: err}
}

// runHelper execs videonode-session, writes "username\0password" to its stdin
// and returns the first line of its stdout. Credentials never touch argv or
// the environment. A non-zero exit with a FAIL verdict is the expected
// rejection path, not an error.
func (a *LinuxAuthenticator) runHelper(username, password string) (string, error) {
	if strings.IndexByte(username, 0) >= 0 || strings.IndexByte(password, 0) >= 0 {
		return "", errors.New("credentials contain NUL byte")
	}

	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, a.helper)
	cmd.Stdin = strings.NewReader(username + "\x00" + password)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	runErr := cmd.Run()
	verdict, _, _ := strings.Cut(strings.TrimSpace(stdout.String()), "\n")

	var exitErr *exec.ExitError
	expectedRejection := errors.As(runErr, &exitErr) && strings.HasPrefix(verdict, "FAIL ")
	if runErr != nil && !expectedRejection {
		return "", fmt.Errorf("run %s: %w", a.helper, runErr)
	}
	return verdict, nil
}

// Available reports whether the videonode-session helper is present and
// executable. PAM/shadow access is the helper's concern, not the daemon's.
func (a *LinuxAuthenticator) Available() bool {
	info, err := os.Stat(a.helper)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// Type returns the authenticator type.
func (a *LinuxAuthenticator) Type() string { return "linux" }

// ServiceUser returns the user the daemon process runs as. Informational only.
func (a *LinuxAuthenticator) ServiceUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// LoginGroup returns the group required for authentication.
func (a *LinuxAuthenticator) LoginGroup() string { return a.group }

func (a *LinuxAuthenticator) reject(username, reason string, extra ...any) Result {
	if a.logger != nil {
		args := append([]any{logging.KeyUsername, username, logging.KeyReason, reason}, extra...)
		a.logger.Info("authentication rejected: "+reasonText(reason), args...)
	}
	return Result{Valid: false, Username: username, Reason: reason}
}

func (a *LinuxAuthenticator) logError(msg, username string, err error) {
	if a.logger == nil {
		return
	}
	a.logger.Error(msg, logging.KeyUsername, username, logging.KeyError, err)
}
