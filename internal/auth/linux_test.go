package auth

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"testing"
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

// fakeHelper writes an executable shell script standing in for the
// videonode-session binary and returns its path.
func fakeHelper(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "videonode-session")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("write fake helper: %v", err)
	}
	return path
}

func aliceInGroup() fakeGroupChecker {
	return fakeGroupChecker{users: map[string]map[string]bool{
		"alice": {"videonode": true},
	}}
}

func newTestAuth(groups GroupChecker, helperPath string) *LinuxAuthenticator {
	return NewLinux(nil).WithGroupChecker(groups).WithHelperPath(helperPath)
}

func TestLinuxAuthenticator_Authenticate(t *testing.T) {
	tests := []struct {
		name       string
		username   string
		password   string
		groups     fakeGroupChecker
		helper     string
		wantValid  bool
		wantReason string
		wantErr    bool
	}{
		{
			name:      "accepts when helper says OK",
			username:  "alice",
			password:  "correct horse battery staple",
			groups:    aliceInGroup(),
			helper:    `echo OK`,
			wantValid: true,
		},
		{
			name:     "rejects unknown user before invoking helper",
			username: "ghost",
			password: "anything",
			groups:   aliceInGroup(),
			helper: `echo OK
`,
			wantReason: ReasonUnknownUser,
		},
		{
			name:     "rejects user not in videonode group before invoking helper",
			username: "bob",
			password: "anything",
			groups: fakeGroupChecker{users: map[string]map[string]bool{
				"bob": {"users": true},
			}},
			helper:     `echo OK`,
			wantReason: ReasonNotInGroup,
		},
		{
			name:       "rejects wrong password",
			username:   "alice",
			password:   "wrong",
			groups:     aliceInGroup(),
			helper:     `echo "FAIL invalid_password"; exit 1`,
			wantReason: ReasonInvalidPassword,
		},
		{
			name:       "rejects user unknown to PAM",
			username:   "alice",
			password:   "anything",
			groups:     aliceInGroup(),
			helper:     `echo "FAIL unknown_user"; exit 1`,
			wantReason: ReasonUnknownUser,
		},
		{
			name:       "rejects expired account",
			username:   "alice",
			password:   "anything",
			groups:     aliceInGroup(),
			helper:     `echo "FAIL account_expired"; exit 1`,
			wantReason: ReasonAccountExpired,
		},
		{
			name:       "rejects locked account",
			username:   "alice",
			password:   "anything",
			groups:     aliceInGroup(),
			helper:     `echo "FAIL account_locked"; exit 1`,
			wantReason: ReasonAccountLocked,
		},
		{
			name:       "surfaces unknown FAIL token as system error",
			username:   "alice",
			password:   "anything",
			groups:     aliceInGroup(),
			helper:     `echo "FAIL llama_overflow"; exit 1`,
			wantReason: ReasonSystemError,
			wantErr:    true,
		},
		{
			name:       "surfaces garbage verdict as system error",
			username:   "alice",
			password:   "anything",
			groups:     aliceInGroup(),
			helper:     `echo "segfault lol"`,
			wantReason: ReasonSystemError,
			wantErr:    true,
		},
		{
			name:       "surfaces helper crash without verdict as system error",
			username:   "alice",
			password:   "anything",
			groups:     aliceInGroup(),
			helper:     `exit 3`,
			wantReason: ReasonSystemError,
			wantErr:    true,
		},
		{
			name:       "rejects OK verdict paired with non-zero exit",
			username:   "alice",
			password:   "anything",
			groups:     aliceInGroup(),
			helper:     `echo OK; exit 1`,
			wantReason: ReasonSystemError,
			wantErr:    true,
		},
		{
			name:       "surfaces group-lookup system error",
			username:   "alice",
			password:   "anything",
			groups:     fakeGroupChecker{err: errors.New("nss exploded")},
			helper:     `echo OK`,
			wantReason: ReasonSystemError,
			wantErr:    true,
		},
		{
			name:       "rejects NUL byte in password",
			username:   "alice",
			password:   "pass\x00word",
			groups:     aliceInGroup(),
			helper:     `echo OK`,
			wantReason: ReasonSystemError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAuth(tt.groups, fakeHelper(t, tt.helper))
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

func TestLinuxAuthenticator_Authenticate_MissingHelper(t *testing.T) {
	a := newTestAuth(aliceInGroup(), filepath.Join(t.TempDir(), "no-such-helper"))
	result := a.Authenticate("alice", "anything")
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if result.Reason != ReasonSystemError {
		t.Errorf("Reason = %q, want %q", result.Reason, ReasonSystemError)
	}
	if result.Error == nil {
		t.Error("Error = nil, want non-nil")
	}
}

func TestLinuxAuthenticator_HelperStdinProtocol(t *testing.T) {
	captured := filepath.Join(t.TempDir(), "stdin.bin")
	a := newTestAuth(aliceInGroup(), fakeHelper(t, `cat > "`+captured+`"; echo OK`))

	result := a.Authenticate("alice", "s3cret!")
	if !result.Valid {
		t.Fatalf("Valid = false (reason %q, err %v), want true", result.Reason, result.Error)
	}

	got, err := os.ReadFile(captured)
	if err != nil {
		t.Fatalf("read captured stdin: %v", err)
	}
	want := "alice\x00s3cret!"
	if string(got) != want {
		t.Errorf("helper stdin = %q, want %q", got, want)
	}
}

func TestLinuxAuthenticator_Type(t *testing.T) {
	a := NewLinux(nil)
	if got := a.Type(); got != "linux" {
		t.Errorf("Type() = %q, want %q", got, "linux")
	}
}

func TestLinuxAuthenticator_Available(t *testing.T) {
	nonExec := filepath.Join(t.TempDir(), "videonode-session")
	if err := os.WriteFile(nonExec, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatalf("write non-executable helper: %v", err)
	}

	tests := []struct {
		name   string
		helper string
		want   bool
	}{
		{"executable helper present", fakeHelper(t, `echo OK`), true},
		{"helper missing", filepath.Join(t.TempDir(), "absent"), false},
		{"helper not executable", nonExec, false},
		{"helper path is a directory", t.TempDir(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewLinux(nil).WithHelperPath(tt.helper)
			if got := a.Available(); got != tt.want {
				t.Errorf("Available() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinuxAuthenticator_LoginGroup(t *testing.T) {
	a := NewLinux(nil)
	if got := a.LoginGroup(); got != "videonode" {
		t.Errorf("LoginGroup() = %q, want %q", got, "videonode")
	}
}
