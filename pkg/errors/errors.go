package errors

import (
	"fmt"
)

// Error codes
const (
	ErrConfigInvalid     = "ERR_CONFIG_INVALID"
	ErrConfigNotFound    = "ERR_CONFIG_NOT_FOUND"
	ErrServiceTimeout    = "ERR_SERVICE_TIMEOUT"
	ErrServiceFailed     = "ERR_SERVICE_FAILED"
	ErrServiceNotReady   = "ERR_SERVICE_NOT_READY"
	ErrServiceNotRunning = "ERR_SERVICE_NOT_RUNNING"
	ErrTestFailed        = "ERR_TEST_FAILED"
	ErrDockerConnection  = "ERR_DOCKER_CONNECTION"
	ErrDockerFailed      = "ERR_DOCKER_FAILED"
	ErrContainerFailed   = "ERR_CONTAINER_FAILED"
	ErrProcessFailed     = "ERR_PROCESS_FAILED"
	ErrInvalidArgument   = "ERR_INVALID_ARGUMENT"
)

// GTError represents a gtool error with code and context
type GTError struct {
	Code    string
	Message string
	Cause   error
	Context map[string]interface{}
}

// Error implements the error interface
func (e *GTError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *GTError) Unwrap() error {
	return e.Cause
}

// New creates a new GTError
func New(code, message string) *GTError {
	return &GTError{
		Code:    code,
		Message: message,
		Context: make(map[string]interface{}),
	}
}

// Wrap wraps an error with a GTError
func Wrap(err error, code, message string) *GTError {
	return &GTError{
		Code:    code,
		Message: message,
		Cause:   err,
		Context: make(map[string]interface{}),
	}
}

// WithContext adds context to the error
func (e *GTError) WithContext(key string, value interface{}) *GTError {
	e.Context[key] = value
	return e
}

// Is checks if an error matches a code
func Is(err error, code string) bool {
	if gtErr, ok := err.(*GTError); ok {
		return gtErr.Code == code
	}
	return false
}
