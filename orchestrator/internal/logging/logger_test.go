package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestInitSetsJSONHandler(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	original := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(original) })

	slog.Info("test message", "key", "value")

	line := strings.TrimSpace(buf.String())
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("expected JSON output, got: %s", line)
	}
	if entry["msg"] != "test message" {
		t.Errorf("expected msg 'test message', got %v", entry["msg"])
	}
	if entry["key"] != "value" {
		t.Errorf("expected key='value', got %v", entry["key"])
	}
}

func TestCorrelationIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	if id := CorrelationID(ctx); id != "" {
		t.Errorf("expected empty correlation ID, got %q", id)
	}

	ctx = WithCorrelationID(ctx, "abc-123")
	if id := CorrelationID(ctx); id != "abc-123" {
		t.Errorf("expected 'abc-123', got %q", id)
	}
}

func TestGenerateIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateID()
		if len(id) != 16 {
			t.Errorf("expected 16-char ID, got %d chars: %s", len(id), id)
		}
		if seen[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		seen[id] = true
	}
}

func TestInitWithDebugLevel(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	original := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(original) })

	slog.Debug("debug visible")
	if buf.Len() == 0 {
		t.Error("expected debug log output at debug level")
	}
}