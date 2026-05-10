package auth

import "testing"

func TestBasicAuthenticator_Authenticate(t *testing.T) {
	auth := NewBasic("admin", "secret")

	tests := []struct {
		name     string
		username string
		password string
		want     bool
	}{
		{"valid credentials", "admin", "secret", true},
		{"invalid username", "user", "secret", false},
		{"invalid password", "admin", "wrong", false},
		{"both invalid", "user", "wrong", false},
		{"empty credentials", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := auth.Authenticate(tt.username, tt.password)
			if result.Valid != tt.want {
				t.Errorf("Authenticate(%q, %q) = %v, want %v",
					tt.username, tt.password, result.Valid, tt.want)
			}
			if result.Username != tt.username {
				t.Errorf("Username = %q, want %q", result.Username, tt.username)
			}
			if result.Error != nil {
				t.Errorf("Error = %v, want nil", result.Error)
			}
		})
	}
}

func TestBasicAuthenticator_Available(t *testing.T) {
	auth := NewBasic("admin", "secret")
	if !auth.Available() {
		t.Error("Basic auth should always be available")
	}
}

func TestBasicAuthenticator_Type(t *testing.T) {
	auth := NewBasic("admin", "secret")
	if auth.Type() != "basic" {
		t.Errorf("Type() = %q, want %q", auth.Type(), "basic")
	}
}
