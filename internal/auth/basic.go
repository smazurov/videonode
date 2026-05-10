package auth

// BasicAuthenticator validates against configured credentials.
type BasicAuthenticator struct {
	username string
	password string
}

// NewBasic creates a new basic authenticator.
func NewBasic(username, password string) *BasicAuthenticator {
	return &BasicAuthenticator{
		username: username,
		password: password,
	}
}

// Authenticate validates the provided credentials against configured values.
func (a *BasicAuthenticator) Authenticate(username, password string) Result {
	valid := username == a.username && password == a.password
	return Result{
		Valid:    valid,
		Username: username,
	}
}

// Available returns true since basic auth is always available.
func (a *BasicAuthenticator) Available() bool {
	return true
}

// Type returns the authenticator type.
func (a *BasicAuthenticator) Type() string {
	return "basic"
}
