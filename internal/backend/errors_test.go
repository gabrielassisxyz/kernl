package backend

import (
	"errors"
	"net/http"
	"testing"
)

func TestBackendErrorCodeValues(t *testing.T) {
	codes := map[BackendErrorCode]bool{
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
	for code, wantRetry := range codes {
		got := defaultRetryable[code]
		if got != wantRetry {
			t.Errorf("default retryable for %s: got %v, want %v", code, got, wantRetry)
		}
	}
}

func TestNewBackendError(t *testing.T) {
	be := NewBackendError(ErrorCodeNotFound, "thing missing")
	if be.Code != ErrorCodeNotFound {
		t.Errorf("Code: got %q, want %q", be.Code, ErrorCodeNotFound)
	}
	if be.Message != "thing missing" {
		t.Errorf("Message: got %q, want %q", be.Message, "thing missing")
	}
	if be.Retryable {
		t.Error("Retryable: got true, want false for NOT_FOUND")
	}
	if be.Details != nil {
		t.Error("Details: expected nil")
	}
	if be.Cause != nil {
		t.Error("Cause: expected nil")
	}
}

func TestNewBackendErrorWithOptions(t *testing.T) {
	cause := errors.New("root cause")
	details := map[string]any{"repo": "/tmp/repo"}
	be := NewBackendError(ErrorCodeLocked, "db locked", WithRetryable(false), WithDetails(details), WithCause(cause))
	if be.Retryable {
		t.Error("Retryable: got true, want false (overridden)")
	}
	if be.Details["repo"] != "/tmp/repo" {
		t.Errorf("Details: got %v, want /tmp/repo", be.Details["repo"])
	}
	if be.Cause.Error() != "root cause" {
		t.Errorf("Cause: got %v, want root cause", be.Cause)
	}
}

func TestNewBackendErrorDefaultRetryable(t *testing.T) {
	be := NewBackendError(ErrorCodeTimeout, "timed out")
	if !be.Retryable {
		t.Error("Retryable: got false, want true for TIMEOUT")
	}
}

func TestBackendErrorErrorString(t *testing.T) {
	be := NewBackendError(ErrorCodeNotFound, "missing")
	got := be.Error()
	want := "backend error [NOT_FOUND]: missing"
	if got != want {
		t.Errorf("Error(): got %q, want %q", got, want)
	}
}

func TestBackendErrorErrorStringWithCause(t *testing.T) {
	cause := errors.New("root")
	be := NewBackendError(ErrorCodeNotFound, "missing", WithCause(cause))
	got := be.Error()
	want := "backend error [NOT_FOUND]: missing: root"
	if got != want {
		t.Errorf("Error(): got %q, want %q", got, want)
	}
}

func TestBackendErrorUnwrap(t *testing.T) {
	cause := errors.New("root")
	be := NewBackendError(ErrorCodeInternal, "fail", WithCause(cause))
	if !errors.Is(be, cause) {
		t.Error("Unwrap: expected errors.Is to match cause")
	}
}

func TestClassifyErrorMessage(t *testing.T) {
	tests := []struct {
		input    string
		wantCode BackendErrorCode
	}{
		{"item not found", ErrorCodeNotFound},
		{"no such file or directory", ErrorCodeNotFound},
		{"record does not exist", ErrorCodeNotFound},
		{"already exists in database", ErrorCodeAlreadyExists},
		{"duplicate key value violates", ErrorCodeAlreadyExists},
		{"database is locked", ErrorCodeLocked},
		{"could not obtain lock on resource", ErrorCodeLocked},
		{"operation timed out", ErrorCodeTimeout},
		{"request timeout after 30s", ErrorCodeTimeout},
		{"permission denied for table", ErrorCodePermissionDenied},
		{"unauthorized access", ErrorCodePermissionDenied},
		{"EACCES permission denied", ErrorCodePermissionDenied},
		{"server busy try again later", ErrorCodeUnavailable},
		{"service unavailable", ErrorCodeUnavailable},
		{"unable to open database file", ErrorCodeUnavailable},
		{"something random went wrong", ErrorCodeInternal},
	}
	for _, tt := range tests {
		got := ClassifyErrorMessage(tt.input)
		if got != tt.wantCode {
			t.Errorf("ClassifyErrorMessage(%q): got %q, want %q", tt.input, got, tt.wantCode)
		}
	}
}

func TestClassifyErrorMessageCaseInsensitive(t *testing.T) {
	tests := []struct {
		input    string
		wantCode BackendErrorCode
	}{
		{"NOT FOUND", ErrorCodeNotFound},
		{"Does Not Exist", ErrorCodeNotFound},
		{"No Such File", ErrorCodeNotFound},
		{"ALREADY EXISTS", ErrorCodeAlreadyExists},
		{"DATABASE IS LOCKED", ErrorCodeLocked},
		{"TIMED OUT", ErrorCodeTimeout},
		{"PERMISSION DENIED", ErrorCodePermissionDenied},
		{"UNAUTHORIZED", ErrorCodePermissionDenied},
		{"SERVICE UNAVAILABLE", ErrorCodeUnavailable},
	}
	for _, tt := range tests {
		got := ClassifyErrorMessage(tt.input)
		if got != tt.wantCode {
			t.Errorf("ClassifyErrorMessage(%q): got %q, want %q (case insensitive)", tt.input, got, tt.wantCode)
		}
	}
}

func TestIsSuppressible(t *testing.T) {
	suppressible := []BackendErrorCode{
		ErrorCodeLocked,
		ErrorCodeTimeout,
		ErrorCodeUnavailable,
		ErrorCodeRateLimited,
	}
	for _, code := range suppressible {
		if !IsSuppressible(code) {
			t.Errorf("IsSuppressible(%q): got false, want true", code)
		}
	}
	nonSuppressible := []BackendErrorCode{
		ErrorCodeNotFound,
		ErrorCodeAlreadyExists,
		ErrorCodeInvalidInput,
		ErrorCodePermissionDenied,
		ErrorCodeInternal,
		ErrorCodeConflict,
	}
	for _, code := range nonSuppressible {
		if IsSuppressible(code) {
			t.Errorf("IsSuppressible(%q): got true, want false", code)
		}
	}
}

func TestHTTPStatusFromCode(t *testing.T) {
	tests := []struct {
		code     BackendErrorCode
		wantCode int
	}{
		{ErrorCodeInvalidInput, http.StatusBadRequest},
		{ErrorCodeNotFound, http.StatusNotFound},
		{ErrorCodeUnavailable, http.StatusServiceUnavailable},
		{ErrorCodeInternal, http.StatusInternalServerError},
		{ErrorCodeLocked, http.StatusInternalServerError},
		{ErrorCodeTimeout, http.StatusInternalServerError},
		{ErrorCodeConflict, http.StatusInternalServerError},
		{ErrorCodeRateLimited, http.StatusInternalServerError},
		{ErrorCodePermissionDenied, http.StatusInternalServerError},
		{ErrorCodeAlreadyExists, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		got := HTTPStatusFromCode(tt.code)
		if got != tt.wantCode {
			t.Errorf("HTTPStatusFromCode(%q): got %d, want %d", tt.code, got, tt.wantCode)
		}
	}
}
