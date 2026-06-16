package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCorrelationMiddlewareSetsHeader(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		id := CorrelationID(r.Context())
		if id == "" {
			t.Error("expected non-empty correlation ID in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := CorrelationMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not called")
	}
	gotID := rec.Header().Get("X-Request-ID")
	if gotID == "" {
		t.Error("expected X-Request-ID header to be set")
	}
}

func TestCorrelationMiddlewarePreservesExisting(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := CorrelationID(r.Context())
		if id != "existing-id" {
			t.Errorf("expected 'existing-id', got %q", id)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := CorrelationMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "existing-id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	gotID := rec.Header().Get("X-Request-ID")
	if gotID != "existing-id" {
		t.Errorf("expected 'existing-id', got %q", gotID)
	}
}

func TestLoggingMiddlewareEmitsStructuredLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	original := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(original) })

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	handler := LoggingMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/api/beads", nil)
	ctx := WithCorrelationID(req.Context(), "test-req-1")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	line := strings.TrimSpace(buf.String())
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("expected JSON log, got: %s", line)
	}
	if entry["msg"] != "request" {
		t.Errorf("expected msg 'request', got %v", entry["msg"])
	}
	if entry["correlation_id"] != "test-req-1" {
		t.Errorf("expected correlation_id 'test-req-1', got %v", entry["correlation_id"])
	}
	if entry["method"] != "GET" {
		t.Errorf("expected method 'GET', got %v", entry["method"])
	}
	if entry["path"] != "/api/beads" {
		t.Errorf("expected path '/api/beads', got %v", entry["path"])
	}
}

func TestLoggingMiddlewareCapturesStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	original := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(original) })

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	handler := LoggingMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	line := strings.TrimSpace(buf.String())
	var entry map[string]interface{}
	_ = json.Unmarshal([]byte(line), &entry)
	status, ok := entry["status"].(float64)
	if !ok || int(status) != 404 {
		t.Errorf("expected status 404, got %v", entry["status"])
	}
}

func TestTimingMiddlewareSetsServerTimingHeader(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = Measure(r.Context(), "db_query", func() error {
			return nil
		})
		w.WriteHeader(http.StatusOK)
	})

	handler := TimingMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("expected Server-Timing header to be set")
	}
	if !strings.Contains(header, "db_query;dur=") {
		t.Errorf("expected 'db_query;dur=' in Server-Timing, got %q", header)
	}
	if !strings.Contains(header, "total;dur=") {
		t.Errorf("expected 'total;dur=' in Server-Timing, got %q", header)
	}
}

func TestTimingMiddlewareWithNoMeasures(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := TimingMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	header := rec.Header().Get("Server-Timing")
	if !strings.Contains(header, "total;dur=") {
		t.Errorf("expected 'total;dur=' in Server-Timing, got %q", header)
	}
}

func TestTimingMiddlewareMultipleMeasures(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = Measure(r.Context(), "read", func() error { return nil })
		_ = Measure(r.Context(), "process", func() error { return nil })
		w.WriteHeader(http.StatusOK)
	})

	handler := TimingMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	header := rec.Header().Get("Server-Timing")
	if !strings.Contains(header, "read;dur=") {
		t.Errorf("expected 'read;dur=' in Server-Timing, got %q", header)
	}
	if !strings.Contains(header, "process;dur=") {
		t.Errorf("expected 'process;dur=' in Server-Timing, got %q", header)
	}
	if !strings.Contains(header, "total;dur=") {
		t.Errorf("expected 'total;dur=' in Server-Timing, got %q", header)
	}
}

func TestTimingMiddlewareSlowRequestLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	original := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(original) })

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(600 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	handler := TimingMiddlewareWithConfig(next, TimingConfig{
		Route:  "GET /api/test",
		SlowMs: 500,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected slow request warning log, got none")
	}
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("expected JSON log, got: %s", line)
	}
	if entry["msg"] != "slow request" {
		t.Errorf("expected msg 'slow request', got %v", entry["msg"])
	}
	if entry["route"] != "GET /api/test" {
		t.Errorf("expected route 'GET /api/test', got %v", entry["route"])
	}
}

func TestTimingMiddlewareNoSlowLogBelowThreshold(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	original := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(original) })

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := TimingMiddlewareWithConfig(next, TimingConfig{
		Route:  "GET /api/fast",
		SlowMs: 5000,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/fast", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if buf.Len() > 0 {
		t.Errorf("expected no slow request log, got: %s", buf.String())
	}
}

func TestRoundMetric(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{1.234, 1.2},
		{1.256, 1.3},
		{0.0, 0.0},
		{100.0, 100.0},
		{99.95, 100.0},
	}
	for _, tt := range tests {
		got := roundMetric(tt.input)
		if got != tt.expected {
			t.Errorf("roundMetric(%v) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestMeasureWithoutTracker(t *testing.T) {
	err := Measure(context.Background(), "test", func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestMeasurePropagatesError(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := Measure(r.Context(), "fail_op", func() error {
			return fmt.Errorf("test error")
		})
		if err == nil {
			t.Error("expected error from Measure")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := TimingMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	header := rec.Header().Get("Server-Timing")
	if !strings.Contains(header, "fail_op;dur=") {
		t.Errorf("expected 'fail_op;dur=' in Server-Timing even on error, got %q", header)
	}
}

func TestTimingConfigDefaultSlowMs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	original := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(original) })

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := TimingMiddlewareWithConfig(next, TimingConfig{
		SlowMs: 0,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if buf.Len() > 0 {
		t.Errorf("expected no slow request log for a fast request, got: %s", buf.String())
	}
}
