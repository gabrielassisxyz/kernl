package backend

import (
	"fmt"
	"net/http"
	"strings"
)

type BackendErrorCode string

const (
	ErrorCodeNotFound         BackendErrorCode = "NOT_FOUND"
	ErrorCodeAlreadyExists    BackendErrorCode = "ALREADY_EXISTS"
	ErrorCodeInvalidInput     BackendErrorCode = "INVALID_INPUT"
	ErrorCodeLocked           BackendErrorCode = "LOCKED"
	ErrorCodeTimeout          BackendErrorCode = "TIMEOUT"
	ErrorCodeUnavailable      BackendErrorCode = "UNAVAILABLE"
	ErrorCodePermissionDenied BackendErrorCode = "PERMISSION_DENIED"
	ErrorCodeInternal         BackendErrorCode = "INTERNAL"
	ErrorCodeConflict         BackendErrorCode = "CONFLICT"
	ErrorCodeRateLimited      BackendErrorCode = "RATE_LIMITED"
)

var defaultRetryable = map[BackendErrorCode]bool{
	ErrorCodeNotFound:         false,
	ErrorCodeAlreadyExists:    false,
	ErrorCodeInvalidInput:     false,
	ErrorCodeLocked:           true,
	ErrorCodeTimeout:          true,
	ErrorCodeUnavailable:      true,
	ErrorCodePermissionDenied: false,
	ErrorCodeInternal:         false,
	ErrorCodeConflict:         false,
	ErrorCodeRateLimited:      true,
}

type BackendError struct {
	Code      BackendErrorCode
	Message   string
	Retryable bool
	Details   map[string]any
	Cause     error
}

func NewBackendError(code BackendErrorCode, message string, opts ...BackendErrorOption) *BackendError {
	be := &BackendError{
		Code:      code,
		Message:   message,
		Retryable: defaultRetryable[code],
	}
	for _, opt := range opts {
		opt(be)
	}
	return be
}

type BackendErrorOption func(*BackendError)

func WithRetryable(r bool) BackendErrorOption {
	return func(be *BackendError) { be.Retryable = r }
}

func WithDetails(d map[string]any) BackendErrorOption {
	return func(be *BackendError) { be.Details = d }
}

func WithCause(cause error) BackendErrorOption {
	return func(be *BackendError) { be.Cause = cause }
}

func (e *BackendError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("backend error [%s]: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("backend error [%s]: %s", e.Code, e.Message)
}

func (e *BackendError) Unwrap() error {
	return e.Cause
}

func ClassifyErrorMessage(msg string) BackendErrorCode {
	l := strings.ToLower(msg)
	switch {
	case containsAnyLower(l, "not found", "no such file", "does not exist"):
		return ErrorCodeNotFound
	case containsAnyLower(l, "already exists", "duplicate key"):
		return ErrorCodeAlreadyExists
	case containsAnyLower(l, "database is locked", "could not obtain lock"):
		return ErrorCodeLocked
	case containsAnyLower(l, "timed out", "timeout"):
		return ErrorCodeTimeout
	case containsAnyLower(l, "permission denied", "unauthorized", "eacces"):
		return ErrorCodePermissionDenied
	case containsAnyLower(l, "server busy", "service unavailable", "unable to open database"):
		return ErrorCodeUnavailable
	default:
		return ErrorCodeInternal
	}
}

func IsSuppressible(code BackendErrorCode) bool {
	switch code {
	case ErrorCodeLocked, ErrorCodeTimeout, ErrorCodeUnavailable, ErrorCodeRateLimited:
		return true
	}
	return false
}

func HTTPStatusFromCode(code BackendErrorCode) int {
	switch code {
	case ErrorCodeInvalidInput:
		return http.StatusBadRequest
	case ErrorCodeNotFound:
		return http.StatusNotFound
	case ErrorCodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func containsAnyLower(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
