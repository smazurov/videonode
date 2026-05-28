package auth

import (
	"bufio"
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"os/user"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GehirnInc/crypt"
	_ "github.com/GehirnInc/crypt/apr1_crypt"
	_ "github.com/GehirnInc/crypt/md5_crypt"
	_ "github.com/GehirnInc/crypt/sha256_crypt"
	_ "github.com/GehirnInc/crypt/sha512_crypt"
	yescrypt "github.com/openwall/yescrypt-go"

	"github.com/smazurov/videonode/internal/logging"
)

const (
	defaultLoginGroup = "videonode"
	defaultShadowPath = "/etc/shadow"
)

// ShadowEntry is a parsed /etc/shadow record (the fields we care about).
type ShadowEntry struct {
	Hash      string    // password hash field
	Locked    bool      // hash is "!"/"*"/empty or has a leading "!"
	ExpiresAt time.Time // zero if no expiry
}

// ShadowSource abstracts /etc/shadow access for testability.
type ShadowSource interface {
	Lookup(username string) (ShadowEntry, error)
}

// Sentinel errors returned by ShadowSource implementations.
var (
	ErrShadowReadDenied = errors.New("shadow read denied")
	ErrUserNotInShadow  = errors.New("user not found in /etc/shadow")
)

// fileShadow reads entries from a colon-delimited shadow(5) file.
type fileShadow struct{ path string }

// Lookup implements ShadowSource by scanning the configured shadow file.
func (s *fileShadow) Lookup(username string) (ShadowEntry, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return ShadowEntry{}, ErrShadowReadDenied
		}
		return ShadowEntry{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), ":", 9)
		if len(fields) < 2 || fields[0] != username {
			continue
		}
		entry := ShadowEntry{Hash: fields[1]}
		entry.Locked = entry.Hash == "" || entry.Hash == "*" || strings.HasPrefix(entry.Hash, "!")
		if len(fields) >= 8 && fields[7] != "" {
			if days, err := strconv.ParseInt(fields[7], 10, 64); err == nil && days > 0 {
				entry.ExpiresAt = time.Unix(days*86400, 0).UTC()
			}
		}
		return entry, nil
	}
	if err := scanner.Err(); err != nil {
		return ShadowEntry{}, err
	}
	return ShadowEntry{}, ErrUserNotInShadow
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

// LinuxAuthenticator validates credentials against /etc/shadow, gated on
// membership in a Unix group (default "videonode").
type LinuxAuthenticator struct {
	group  string
	shadow ShadowSource
	groups GroupChecker
	logger logging.Logger
}

// NewLinux creates an authenticator using /etc/shadow and the videonode group.
func NewLinux(logger logging.Logger) *LinuxAuthenticator {
	return &LinuxAuthenticator{
		group:  defaultLoginGroup,
		shadow: &fileShadow{path: defaultShadowPath},
		groups: osGroupChecker{},
		logger: logger,
	}
}

// WithGroup overrides the required login group (used by tests).
func (a *LinuxAuthenticator) WithGroup(group string) *LinuxAuthenticator {
	a.group = group
	return a
}

// WithShadowSource overrides the shadow file reader (used by tests).
func (a *LinuxAuthenticator) WithShadowSource(s ShadowSource) *LinuxAuthenticator {
	a.shadow = s
	return a
}

// WithGroupChecker overrides the group-membership resolver (used by tests).
func (a *LinuxAuthenticator) WithGroupChecker(g GroupChecker) *LinuxAuthenticator {
	a.groups = g
	return a
}

// Authenticate validates the user's password against /etc/shadow after
// confirming the user is in the configured login group. Every accept and
// every reject is logged at Info level with a structured "reason".
func (a *LinuxAuthenticator) Authenticate(username, password string) Result {
	inGroup, err := a.groups.InGroup(username, a.group)
	if err != nil {
		if errors.Is(err, user.UnknownUserError(username)) {
			return a.reject(username, ReasonUnknownUser)
		}
		var unknownUser user.UnknownUserError
		if errors.As(err, &unknownUser) {
			return a.reject(username, ReasonUnknownUser)
		}
		a.logError("group lookup failed", username, err)
		return Result{Valid: false, Username: username, Reason: ReasonSystemError, Error: err}
	}
	if !inGroup {
		return a.reject(username, ReasonNotInGroup, logging.KeyGroup, a.group)
	}

	entry, err := a.shadow.Lookup(username)
	switch {
	case errors.Is(err, ErrShadowReadDenied):
		a.logError("shadow read denied — add the daemon to the 'shadow' group",
			username, err)
		return Result{Valid: false, Username: username, Reason: ReasonShadowReadDenied, Error: err}
	case errors.Is(err, ErrUserNotInShadow):
		return a.reject(username, ReasonUnknownUser)
	case err != nil:
		a.logError("shadow lookup failed", username, err)
		return Result{Valid: false, Username: username, Reason: ReasonSystemError, Error: err}
	}

	if entry.Locked {
		return a.reject(username, ReasonAccountLocked)
	}
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		return a.reject(username, ReasonAccountExpired)
	}

	matched, verifyErr := verifyHash(entry.Hash, password)
	if verifyErr != nil {
		a.logError("hash verification error", username, verifyErr)
		return a.reject(username, ReasonUnsupportedHash, logging.KeyError, verifyErr)
	}
	if !matched {
		return a.reject(username, ReasonInvalidPassword)
	}

	if a.logger != nil {
		a.logger.Info("auth accepted", logging.KeyUsername, username)
	}
	return Result{Valid: true, Username: username}
}

func (a *LinuxAuthenticator) reject(username, reason string, extra ...any) Result {
	if a.logger != nil {
		args := append([]any{logging.KeyUsername, username, logging.KeyReason, reason}, extra...)
		a.logger.Info("auth rejected", args...)
	}
	return Result{Valid: false, Username: username, Reason: reason}
}

func (a *LinuxAuthenticator) logError(msg, username string, err error) {
	if a.logger == nil {
		return
	}
	a.logger.Error(msg, logging.KeyUsername, username, logging.KeyError, err)
}

// verifyHash dispatches to the right crypt scheme based on the hash prefix.
// Supports SHA-256, SHA-512, MD5/APR1 (via GehirnInc/crypt) and yescrypt
// (via openwall/yescrypt-go).
func verifyHash(hash, password string) (bool, error) {
	if strings.HasPrefix(hash, "$y$") {
		out, err := yescrypt.Hash([]byte(password), []byte(hash))
		if err != nil {
			return false, fmt.Errorf("yescrypt: %w", err)
		}
		return subtle.ConstantTimeCompare(out, []byte(hash)) == 1, nil
	}
	if crypt.IsHashSupported(hash) {
		c := crypt.NewFromHash(hash)
		if err := c.Verify(hash, []byte(password)); err != nil {
			if errors.Is(err, crypt.ErrKeyMismatch) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("unsupported hash prefix in %.4q", hash)
}

// Available always returns true — no external binaries needed.
func (a *LinuxAuthenticator) Available() bool { return true }

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
