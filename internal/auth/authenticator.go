// Package auth provides authentication backends for the API server.
package auth

// Result contains authentication result.
type Result struct {
	Valid    bool
	Username string
	Reason   string // Machine-readable rejection reason (empty on success)
	Error    error  // System error (not invalid creds)
}

// Rejection reasons surfaced through Result.Reason for logging and diagnostics.
const (
	ReasonUnknownUser     = "unknown_user"
	ReasonNotInGroup      = "not_in_group"
	ReasonInvalidPassword = "invalid_password"
	ReasonAccountLocked   = "account_locked"
	ReasonAccountExpired  = "account_expired"
	ReasonSystemError     = "system_error"
)

func reasonText(reason string) string {
	switch reason {
	case ReasonUnknownUser:
		return "user not found"
	case ReasonNotInGroup:
		return "user not in login group"
	case ReasonInvalidPassword:
		return "invalid password"
	case ReasonAccountLocked:
		return "account locked"
	case ReasonAccountExpired:
		return "account expired"
	default:
		return "rejected"
	}
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
