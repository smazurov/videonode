package auth

import (
	"os/user"
	"testing"
)

func TestLinuxAuthenticator_ServiceUser(t *testing.T) {
	auth := NewLinux(nil)

	u, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	if auth.ServiceUser() != u.Username {
		t.Errorf("ServiceUser() = %q, want %q", auth.ServiceUser(), u.Username)
	}
}

func TestLinuxAuthenticator_Type(t *testing.T) {
	auth := NewLinux(nil)
	if auth.Type() != "linux" {
		t.Errorf("Type() = %q, want %q", auth.Type(), "linux")
	}
}

func TestLinuxAuthenticator_RejectsWrongUsername(t *testing.T) {
	auth := NewLinux(nil)

	result := auth.Authenticate("nonexistent_user_12345", "anypassword")
	if result.Valid {
		t.Error("Should reject username that doesn't match service user")
	}
	if result.Username != "nonexistent_user_12345" {
		t.Errorf("Username = %q, want %q", result.Username, "nonexistent_user_12345")
	}
}
