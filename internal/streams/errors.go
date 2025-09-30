package streams

import "fmt"

// StreamError represents a domain-specific error
type StreamError struct {
	Code    string
	Message string
	Cause   error
}

func (e *StreamError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *StreamError) Unwrap() error {
	return e.Cause
}

// Error codes
const (
	ErrCodeStreamNotFound  = "STREAM_NOT_FOUND"
	ErrCodeDeviceNotFound  = "DEVICE_NOT_FOUND"
	ErrCodeStreamExists    = "STREAM_EXISTS"
	ErrCodeInvalidParams   = "INVALID_PARAMS"
	ErrCodeConfigError     = "CONFIG_ERROR"
	ErrCodeMediaMTXError   = "MEDIAMTX_ERROR"
	ErrCodeMonitoringError = "MONITORING_ERROR"
)

// NewStreamError creates a new stream error
func NewStreamError(code, message string, cause error) *StreamError {
	return &StreamError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}
