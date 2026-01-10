package updater

import "fmt"

// Error codes for update operations.
const (
	ErrCodeInvalidState   = "INVALID_STATE"
	ErrCodeCheckFailed    = "CHECK_FAILED"
	ErrCodeNoUpdate       = "NO_UPDATE"
	ErrCodeDownloadFailed = "DOWNLOAD_FAILED"
	ErrCodeApplyFailed    = "APPLY_FAILED"
	ErrCodeBackupFailed   = "BACKUP_FAILED"
	ErrCodeRollbackFailed = "ROLLBACK_FAILED"
	ErrCodeNoBackup       = "NO_BACKUP"
	ErrCodeDisabled       = "DISABLED"
)

// Error represents an update-specific error with a code.
type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func newError(code, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}
