package obs

import (
	"fmt"
)

// ErrorCode represents specific error types in the obs package
type ErrorCode string

const (
	ErrCollectorExists   ErrorCode = "COLLECTOR_EXISTS"
	ErrCollectorNotFound ErrorCode = "COLLECTOR_NOT_FOUND"
	ErrCollectorStart    ErrorCode = "COLLECTOR_START_FAILED"
	ErrCollectorStop     ErrorCode = "COLLECTOR_STOP_FAILED"
	ErrStoreNotFound     ErrorCode = "STORE_NOT_FOUND"
	ErrStoreFull         ErrorCode = "STORE_FULL"
	ErrInvalidQuery      ErrorCode = "INVALID_QUERY"
	ErrExporterFailed    ErrorCode = "EXPORTER_FAILED"
	ErrInvalidConfig     ErrorCode = "INVALID_CONFIG"
	ErrDataPointInvalid  ErrorCode = "DATA_POINT_INVALID"
)

// ObsError represents an error in the obs package
type ObsError struct {
	Code    ErrorCode              `json:"code"`
	Message string                 `json:"message"`
	Context map[string]interface{} `json:"context,omitempty"`
	Cause   error                  `json:"cause,omitempty"`
}

// NewObsError creates a new obs error
func NewObsError(code ErrorCode, message string, context map[string]interface{}) *ObsError {
	return &ObsError{
		Code:    code,
		Message: message,
		Context: context,
	}
}

// NewObsErrorWithCause creates a new obs error with a cause
func NewObsErrorWithCause(code ErrorCode, message string, cause error, context map[string]interface{}) *ObsError {
	return &ObsError{
		Code:    code,
		Message: message,
		Context: context,
		Cause:   cause,
	}
}

// Error implements the error interface
func (e *ObsError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause
func (e *ObsError) Unwrap() error {
	return e.Cause
}

// Is checks if the error matches a specific code
func (e *ObsError) Is(code ErrorCode) bool {
	return e.Code == code
}
