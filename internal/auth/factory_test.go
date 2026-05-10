package auth

import "testing"

func TestNew_BasicType(t *testing.T) {
	cfg := Config{
		Type:     "basic",
		Username: "admin",
		Password: "secret",
	}

	auth := New(cfg, nil)

	if auth.Type() != "basic" {
		t.Errorf("Type() = %q, want %q", auth.Type(), "basic")
	}

	result := auth.Authenticate("admin", "secret")
	if !result.Valid {
		t.Error("Should authenticate with correct credentials")
	}
}

func TestNew_UnknownTypeFallsBackToBasic(t *testing.T) {
	cfg := Config{
		Type:     "unknown",
		Username: "admin",
		Password: "secret",
	}

	auth := New(cfg, nil)

	if auth.Type() != "basic" {
		t.Errorf("Type() = %q, want %q", auth.Type(), "basic")
	}
}
