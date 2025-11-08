package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		message string
	}{
		{
			name:    "simple error",
			code:    ErrConfigInvalid,
			message: "invalid configuration",
		},
		{
			name:    "service error",
			code:    ErrServiceFailed,
			message: "service failed to start",
		},
		{
			name:    "test error",
			code:    ErrTestFailed,
			message: "test execution failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, tt.message)

			assert.NotNil(t, err)
			assert.Equal(t, tt.code, err.Code)
			assert.Equal(t, tt.message, err.Message)
			assert.Nil(t, err.Cause)
			assert.NotNil(t, err.Context)
			assert.Empty(t, err.Context)
		})
	}
}

func TestWrap(t *testing.T) {
	originalErr := errors.New("original error")

	tests := []struct {
		name    string
		err     error
		code    string
		message string
	}{
		{
			name:    "wrap with config error",
			err:     originalErr,
			code:    ErrConfigInvalid,
			message: "failed to load config",
		},
		{
			name:    "wrap with service error",
			err:     originalErr,
			code:    ErrServiceFailed,
			message: "service startup failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := Wrap(tt.err, tt.code, tt.message)

			assert.NotNil(t, wrapped)
			assert.Equal(t, tt.code, wrapped.Code)
			assert.Equal(t, tt.message, wrapped.Message)
			assert.Equal(t, tt.err, wrapped.Cause)
		})
	}
}

func TestGTError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *GTError
		expected string
	}{
		{
			name: "error without cause",
			err: &GTError{
				Code:    ErrConfigInvalid,
				Message: "invalid config",
			},
			expected: "[ERR_CONFIG_INVALID] invalid config",
		},
		{
			name: "error with cause",
			err: &GTError{
				Code:    ErrConfigInvalid,
				Message: "invalid config",
				Cause:   errors.New("file not found"),
			},
			expected: "[ERR_CONFIG_INVALID] invalid config: file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGTError_Unwrap(t *testing.T) {
	originalErr := errors.New("original")
	wrapped := Wrap(originalErr, ErrServiceFailed, "service failed")

	unwrapped := wrapped.Unwrap()
	assert.Equal(t, originalErr, unwrapped)
}

func TestGTError_WithContext(t *testing.T) {
	err := New(ErrServiceFailed, "service failed")

	result := err.WithContext("service", "couchbase")
	assert.Equal(t, err, result) // Should return same instance
	assert.Equal(t, "couchbase", err.Context["service"])

	// Add multiple context values
	err.WithContext("port", 8091)
	err.WithContext("timeout", "30s")

	assert.Equal(t, 3, len(err.Context))
	assert.Equal(t, 8091, err.Context["port"])
	assert.Equal(t, "30s", err.Context["timeout"])
}

func TestIs(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     string
		expected bool
	}{
		{
			name:     "matching code",
			err:      New(ErrConfigInvalid, "invalid"),
			code:     ErrConfigInvalid,
			expected: true,
		},
		{
			name:     "non-matching code",
			err:      New(ErrConfigInvalid, "invalid"),
			code:     ErrServiceFailed,
			expected: false,
		},
		{
			name:     "wrapped error matching",
			err:      Wrap(errors.New("cause"), ErrTestFailed, "test failed"),
			code:     ErrTestFailed,
			expected: true,
		},
		{
			name:     "non-GTError",
			err:      errors.New("standard error"),
			code:     ErrConfigInvalid,
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			code:     ErrConfigInvalid,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Is(tt.err, tt.code)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestErrorCodes(t *testing.T) {
	// Test that all error codes are unique
	codes := []string{
		ErrConfigInvalid,
		ErrConfigNotFound,
		ErrServiceTimeout,
		ErrServiceFailed,
		ErrServiceNotReady,
		ErrTestFailed,
		ErrDockerConnection,
		ErrContainerFailed,
		ErrProcessFailed,
		ErrInvalidArgument,
	}

	seen := make(map[string]bool)
	for _, code := range codes {
		assert.False(t, seen[code], "duplicate error code: %s", code)
		seen[code] = true
	}

	assert.Equal(t, len(codes), len(seen))
}

func ExampleNew() {
	err := New(ErrConfigInvalid, "configuration is missing required field")
	fmt.Println(err.Error())
	// Output: [ERR_CONFIG_INVALID] configuration is missing required field
}

func ExampleWrap() {
	originalErr := errors.New("file not found")
	err := Wrap(originalErr, ErrConfigNotFound, "failed to load configuration")
	fmt.Println(err.Error())
	// Output: [ERR_CONFIG_NOT_FOUND] failed to load configuration: file not found
}

func ExampleGTError_WithContext() {
	err := New(ErrServiceFailed, "service failed to start")
	err.WithContext("service", "couchbase")
	err.WithContext("port", 8091)

	fmt.Printf("Code: %s, Message: %s, Service: %s\n",
		err.Code, err.Message, err.Context["service"])
	// Output: Code: ERR_SERVICE_FAILED, Message: service failed to start, Service: couchbase
}

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New(ErrConfigInvalid, "test error")
	}
}

func BenchmarkWrap(b *testing.B) {
	originalErr := errors.New("original")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Wrap(originalErr, ErrServiceFailed, "wrapped error")
	}
}

func BenchmarkWithContext(b *testing.B) {
	err := New(ErrServiceFailed, "service failed")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err.WithContext("iteration", i)
	}
}
