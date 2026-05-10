// Package auth provides authentication backends for the API server.
package auth

// Result contains authentication result.
type Result struct {
	Valid    bool
	Username string
	Error    error // System error (not invalid creds)
}

// Authenticator validates user credentials.
type Authenticator interface {
	Authenticate(username, password string) Result
	Available() bool
	Type() string
}

// Config holds authentication configuration.
type Config struct {
	Type     string
	Username string
	Password string
}
