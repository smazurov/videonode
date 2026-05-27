package auth

import (
	"errors"
	"os/exec"
	"os/user"

	"github.com/smazurov/videonode/internal/logging"
)

// LinuxAuthenticator validates credentials using unix_chkpwd.
type LinuxAuthenticator struct {
	serviceUser string
	logger      logging.Logger
}

// NewLinux creates a new Linux authenticator.
func NewLinux(logger logging.Logger) *LinuxAuthenticator {
	var serviceUser string
	if u, err := user.Current(); err == nil {
		serviceUser = u.Username
	}

	return &LinuxAuthenticator{
		serviceUser: serviceUser,
		logger:      logger,
	}
}

// Authenticate validates credentials using unix_chkpwd.
// Only allows authentication for the service user (the user running the process).
func (a *LinuxAuthenticator) Authenticate(username, password string) Result {
	if username != a.serviceUser {
		if a.logger != nil {
			a.logger.Debug("Linux auth rejected: username mismatch",
				logging.KeyProvided, username, logging.KeyExpected, a.serviceUser)
		}
		return Result{Valid: false, Username: username}
	}

	cmd := exec.Command("/sbin/unix_chkpwd", username, "nullok")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		if a.logger != nil {
			a.logger.Error("Failed to create stdin pipe for unix_chkpwd", logging.KeyError, err)
		}
		return Result{Valid: false, Username: username, Error: err}
	}

	if err := cmd.Start(); err != nil {
		if a.logger != nil {
			a.logger.Error("Failed to start unix_chkpwd", logging.KeyError, err)
		}
		return Result{Valid: false, Username: username, Error: err}
	}

	// Write password with null terminator
	_, _ = stdin.Write([]byte(password + "\x00"))
	_ = stdin.Close()

	err = cmd.Wait()
	valid := err == nil

	if a.logger != nil && !valid {
		// Don't log exit status 7 (invalid password) as an error, it's expected
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() != 7 {
			a.logger.Debug("unix_chkpwd returned error", logging.KeyError, err)
		}
	}

	return Result{Valid: valid, Username: username}
}

// Available checks if unix_chkpwd is available on the system.
func (a *LinuxAuthenticator) Available() bool {
	_, err := exec.LookPath("unix_chkpwd")
	if err != nil {
		// Also try the full path
		_, err = exec.LookPath("/sbin/unix_chkpwd")
	}
	return err == nil
}

// Type returns the authenticator type.
func (a *LinuxAuthenticator) Type() string {
	return "linux"
}

// ServiceUser returns the username of the service user.
func (a *LinuxAuthenticator) ServiceUser() string {
	return a.serviceUser
}
