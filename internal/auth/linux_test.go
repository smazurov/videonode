package auth

import (
	"errors"
	"os/user"
	"testing"
	"time"

	"github.com/GehirnInc/crypt/sha512_crypt"
)

// fakeGroupChecker stubs GroupChecker for tests.
type fakeGroupChecker struct {
	users map[string]map[string]bool // username -> set of groups
	err   error
}

func (f fakeGroupChecker) InGroup(username, group string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	groups, ok := f.users[username]
	if !ok {
		return false, user.UnknownUserError(username)
	}
	return groups[group], nil
}

// fakeShadow stubs ShadowSource for tests.
type fakeShadow struct {
	entries map[string]ShadowEntry
	err     error
}

func (f fakeShadow) Lookup(username string) (ShadowEntry, error) {
	if f.err != nil {
		return ShadowEntry{}, f.err
	}
	e, ok := f.entries[username]
	if !ok {
		return ShadowEntry{}, ErrUserNotInShadow
	}
	return e, nil
}

func sha512Hash(t *testing.T, password string) string {
	t.Helper()
	c := sha512_crypt.New()
	hash, err := c.Generate([]byte(password), []byte("$6$testsalt"))
	if err != nil {
		t.Fatalf("generate hash: %v", err)
	}
	return hash
}

func newTestAuth(groups fakeGroupChecker, shadow fakeShadow) *LinuxAuthenticator {
	a := NewLinux(nil)
	a.groups = groups
	a.shadow = shadow
	return a
}

func TestLinuxAuthenticator_Authenticate(t *testing.T) {
	const password = "correct horse battery staple"

	tests := []struct {
		name       string
		username   string
		password   string
		groups     fakeGroupChecker
		shadow     fakeShadow
		wantValid  bool
		wantReason string
		wantErr    bool
	}{
		{
			name:     "accepts valid user in group with correct password",
			username: "alice",
			password: password,
			groups: fakeGroupChecker{users: map[string]map[string]bool{
				"alice": {"videonode": true},
			}},
			shadow: fakeShadow{entries: map[string]ShadowEntry{
				"alice": {Hash: sha512Hash(t, password)},
			}},
			wantValid: true,
		},
		{
			name:     "rejects unknown user",
			username: "ghost",
			password: "anything",
			groups: fakeGroupChecker{users: map[string]map[string]bool{
				"alice": {"videonode": true},
			}},
			shadow:     fakeShadow{},
			wantReason: ReasonUnknownUser,
		},
		{
			name:     "rejects user not in videonode group",
			username: "bob",
			password: password,
			groups: fakeGroupChecker{users: map[string]map[string]bool{
				"bob": {"users": true},
			}},
			shadow:     fakeShadow{},
			wantReason: ReasonNotInGroup,
		},
		{
			name:     "rejects wrong password",
			username: "alice",
			password: "wrong",
			groups: fakeGroupChecker{users: map[string]map[string]bool{
				"alice": {"videonode": true},
			}},
			shadow: fakeShadow{entries: map[string]ShadowEntry{
				"alice": {Hash: sha512Hash(t, password)},
			}},
			wantReason: ReasonInvalidPassword,
		},
		{
			name:     "rejects locked account (leading bang)",
			username: "alice",
			password: password,
			groups: fakeGroupChecker{users: map[string]map[string]bool{
				"alice": {"videonode": true},
			}},
			shadow: fakeShadow{entries: map[string]ShadowEntry{
				"alice": {Hash: "!" + sha512Hash(t, password), Locked: true},
			}},
			wantReason: ReasonAccountLocked,
		},
		{
			name:     "rejects expired account",
			username: "alice",
			password: password,
			groups: fakeGroupChecker{users: map[string]map[string]bool{
				"alice": {"videonode": true},
			}},
			shadow: fakeShadow{entries: map[string]ShadowEntry{
				"alice": {Hash: sha512Hash(t, password), ExpiresAt: time.Now().Add(-24 * time.Hour)},
			}},
			wantReason: ReasonAccountExpired,
		},
		{
			name:     "surfaces shadow read denied as system error",
			username: "alice",
			password: password,
			groups: fakeGroupChecker{users: map[string]map[string]bool{
				"alice": {"videonode": true},
			}},
			shadow:     fakeShadow{err: ErrShadowReadDenied},
			wantReason: ReasonShadowReadDenied,
			wantErr:    true,
		},
		{
			name:     "rejects unsupported hash prefix",
			username: "alice",
			password: password,
			groups: fakeGroupChecker{users: map[string]map[string]bool{
				"alice": {"videonode": true},
			}},
			shadow: fakeShadow{entries: map[string]ShadowEntry{
				"alice": {Hash: "$nope$weird-format"},
			}},
			wantReason: ReasonUnsupportedHash,
		},
		{
			name:       "surfaces group-lookup system error",
			username:   "alice",
			password:   password,
			groups:     fakeGroupChecker{err: errors.New("nss exploded")},
			shadow:     fakeShadow{},
			wantReason: ReasonSystemError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAuth(tt.groups, tt.shadow)
			result := a.Authenticate(tt.username, tt.password)

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if result.Username != tt.username {
				t.Errorf("Username = %q, want %q", result.Username, tt.username)
			}
			if result.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", result.Reason, tt.wantReason)
			}
			if tt.wantErr && result.Error == nil {
				t.Errorf("Error = nil, want non-nil")
			}
			if !tt.wantErr && result.Error != nil {
				t.Errorf("Error = %v, want nil", result.Error)
			}
		})
	}
}

func TestLinuxAuthenticator_Type(t *testing.T) {
	a := NewLinux(nil)
	if got := a.Type(); got != "linux" {
		t.Errorf("Type() = %q, want %q", got, "linux")
	}
}

func TestLinuxAuthenticator_Available(t *testing.T) {
	a := NewLinux(nil)
	if !a.Available() {
		t.Error("Available() should always return true for the pure-Go authenticator")
	}
}

func TestLinuxAuthenticator_LoginGroup(t *testing.T) {
	a := NewLinux(nil)
	if got := a.LoginGroup(); got != "videonode" {
		t.Errorf("LoginGroup() = %q, want %q", got, "videonode")
	}
}
