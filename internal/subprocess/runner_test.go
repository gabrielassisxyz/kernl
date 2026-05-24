package subprocess_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/subprocess"
)

func createScript(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	err := os.WriteFile(path, []byte(content), 0755)
	if err != nil {
		t.Fatalf("failed to create script: %v", err)
	}
	return path
}

func TestRunSubprocessStage_HappyPath(t *testing.T) {
	scriptPath := createScript(t, `#!/bin/bash
read -r stdin
clean=$(echo "$stdin" | tr -d '"{}')
echo "{\"context_payload\": \"echo: $clean\"}"
`)
	spec := backend.StageContract{
		Subprocess: &backend.SubprocessSpec{
			Command: []string{scriptPath},
		},
	}
	req := subprocess.HandoffRequest{
		EpicID:         "epic-1",
		BeadID:         "bead-1",
		WorktreePath:   t.TempDir(),
		ContextPayload: "hello world",
	}
	resp, err := subprocess.RunSubprocessStage(context.Background(), spec, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prefix := "echo: "
	if !strings.HasPrefix(resp.ContextPayload, prefix) {
		t.Fatalf("unexpected response context payload: %q", resp.ContextPayload)
	}
	content := resp.ContextPayload[len(prefix):]
	if !strings.Contains(content, "context_payload:hello world") {
		t.Errorf("expected echoed context to contain context_payload:hello world, got %q", content)
	}
}

func TestRunSubprocessStage_EmptyStdout(t *testing.T) {
	scriptPath := createScript(t, `#!/bin/bash
exit 0
`)
	spec := backend.StageContract{
		Subprocess: &backend.SubprocessSpec{
			Command: []string{scriptPath},
		},
	}
	req := subprocess.HandoffRequest{
		EpicID:       "epic-1",
		BeadID:       "bead-1",
		WorktreePath: t.TempDir(),
	}
	resp, err := subprocess.RunSubprocessStage(context.Background(), spec, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ContextPayload != "" {
		t.Errorf("expected empty context payload, got %q", resp.ContextPayload)
	}
}

func TestRunSubprocessStage_NonZeroExit(t *testing.T) {
	scriptPath := createScript(t, `#!/bin/bash
echo "some error message" >&2
exit 3
`)
	spec := backend.StageContract{
		Subprocess: &backend.SubprocessSpec{
			Command: []string{scriptPath},
		},
	}
	req := subprocess.HandoffRequest{
		EpicID:       "epic-1",
		BeadID:       "bead-1",
		WorktreePath: t.TempDir(),
	}
	_, err := subprocess.RunSubprocessStage(context.Background(), spec, req)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	var subErr *subprocess.SubprocessError
	if !errors.As(err, &subErr) {
		t.Fatalf("expected SubprocessError, got %T: %v", err, err)
	}
	if subErr.Cause != subprocess.CauseNonZeroExit {
		t.Errorf("expected cause %q, got %q", subprocess.CauseNonZeroExit, subErr.Cause)
	}
	if !strings.Contains(subErr.Stderr, "some error message") {
		t.Errorf("expected stderr to contain %q, got %q", "some error message", subErr.Stderr)
	}
	if !errors.Is(err, subprocess.ErrNonZeroExit) {
		t.Errorf("expected errors.Is(err, ErrNonZeroExit) to be true")
	}
}

func TestRunSubprocessStage_Timeout(t *testing.T) {
	scriptPath := createScript(t, `#!/bin/bash
sleep 2
`)
	spec := backend.StageContract{
		Subprocess: &backend.SubprocessSpec{
			Command: []string{scriptPath},
			Timeout: "100ms",
		},
	}
	req := subprocess.HandoffRequest{
		EpicID:       "epic-1",
		BeadID:       "bead-1",
		WorktreePath: t.TempDir(),
	}
	_, err := subprocess.RunSubprocessStage(context.Background(), spec, req)
	if err == nil {
		t.Fatal("expected error due to timeout, got nil")
	}
	var subErr *subprocess.SubprocessError
	if !errors.As(err, &subErr) {
		t.Fatalf("expected SubprocessError, got %T: %v", err, err)
	}
	if subErr.Cause != subprocess.CauseTimeout {
		t.Errorf("expected cause %q, got %q", subprocess.CauseTimeout, subErr.Cause)
	}
	if !errors.Is(err, subprocess.ErrTimeout) {
		t.Errorf("expected errors.Is(err, ErrTimeout) to be true")
	}
}

func TestRunSubprocessStage_MalformedJSON(t *testing.T) {
	scriptPath := createScript(t, `#!/bin/bash
echo "not json"
`)
	spec := backend.StageContract{
		Subprocess: &backend.SubprocessSpec{
			Command: []string{scriptPath},
		},
	}
	req := subprocess.HandoffRequest{
		EpicID:       "epic-1",
		BeadID:       "bead-1",
		WorktreePath: t.TempDir(),
	}
	_, err := subprocess.RunSubprocessStage(context.Background(), spec, req)
	if err == nil {
		t.Fatal("expected error due to malformed JSON, got nil")
	}
	var subErr *subprocess.SubprocessError
	if !errors.As(err, &subErr) {
		t.Fatalf("expected SubprocessError, got %T: %v", err, err)
	}
	if subErr.Cause != subprocess.CauseParseError {
		t.Errorf("expected cause %q, got %q", subprocess.CauseParseError, subErr.Cause)
	}
	if !errors.Is(err, subprocess.ErrParseError) {
		t.Errorf("expected errors.Is(err, ErrParseError) to be true")
	}
}

func TestRunSubprocessStage_OutputTooLarge(t *testing.T) {
	scriptPath := createScript(t, `#!/bin/bash
printf 'a%.0s' {1..70000}
`)
	spec := backend.StageContract{
		Subprocess: &backend.SubprocessSpec{
			Command: []string{scriptPath},
		},
	}
	req := subprocess.HandoffRequest{
		EpicID:       "epic-1",
		BeadID:       "bead-1",
		WorktreePath: t.TempDir(),
	}
	_, err := subprocess.RunSubprocessStage(context.Background(), spec, req)
	if err == nil {
		t.Fatal("expected error due to oversized output, got nil")
	}
	var subErr *subprocess.SubprocessError
	if !errors.As(err, &subErr) {
		t.Fatalf("expected SubprocessError, got %T: %v", err, err)
	}
	if subErr.Cause != subprocess.CauseOutputTooLarge {
		t.Errorf("expected cause %q, got %q", subprocess.CauseOutputTooLarge, subErr.Cause)
	}
	if !errors.Is(err, subprocess.ErrOutputTooLarge) {
		t.Errorf("expected errors.Is(err, ErrOutputTooLarge) to be true")
	}
}

func TestRunSubprocessStage_StderrTruncation(t *testing.T) {
	scriptPath := createScript(t, `#!/bin/bash
printf 'b%.0s' {1..70000} >&2
exit 1
`)
	spec := backend.StageContract{
		Subprocess: &backend.SubprocessSpec{
			Command: []string{scriptPath},
		},
	}
	req := subprocess.HandoffRequest{
		EpicID:       "epic-1",
		BeadID:       "bead-1",
		WorktreePath: t.TempDir(),
	}
	_, err := subprocess.RunSubprocessStage(context.Background(), spec, req)
	if err == nil {
		t.Fatal("expected error due to non-zero exit, got nil")
	}
	var subErr *subprocess.SubprocessError
	if !errors.As(err, &subErr) {
		t.Fatalf("expected SubprocessError, got %T: %v", err, err)
	}
	if subErr.Cause != subprocess.CauseNonZeroExit {
		t.Errorf("expected cause %q, got %q", subprocess.CauseNonZeroExit, subErr.Cause)
	}
	if !strings.Contains(subErr.Stderr, "(truncated at 65536 bytes)") {
		t.Errorf("expected stderr to contain truncation marker, got:\n%s", subErr.Stderr)
	}
}
