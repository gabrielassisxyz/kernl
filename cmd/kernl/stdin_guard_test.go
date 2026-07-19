package main

import (
	"strings"
	"testing"
)

// R2-005: reading a note body / paste text from stdin when stdin is a terminal
// used to block forever on io.ReadAll with no prompt — the worst agent trap in
// the CLI. The guard turns that deadlock into a fast exit-2 usage error. The
// stdinIsTerminal seam is overridden so the terminal branch is exercised without
// a pty; if the guard regressed, the real io.ReadAll would hang and the test
// would time out rather than fail — which is itself the signal.
func withTerminalStdin(t *testing.T) {
	t.Helper()
	orig := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdinIsTerminal = orig })
}

func TestNoteBodyFromTerminalStdinFailsFast(t *testing.T) {
	withTerminalStdin(t)
	// No --file source, stdin is a terminal → must not reach io.ReadAll.
	_, err := readNoteBody("write", "--file", "", false)
	if err == nil {
		t.Fatal("expected a usage error when stdin is a terminal, got nil")
	}
	if code := exitCode(err); code != 2 {
		t.Errorf("terminal-stdin guard must be a usage error (exit 2), got %d: %v", code, err)
	}
	if !strings.Contains(err.Error(), "terminal") {
		t.Errorf("error should explain stdin is a terminal, got: %v", err)
	}
}

func TestIngestPasteFromTerminalStdinFailsFast(t *testing.T) {
	withTerminalStdin(t)
	_, err := ingestPasteText("")
	if err == nil {
		t.Fatal("expected a usage error when stdin is a terminal, got nil")
	}
	if code := exitCode(err); code != 2 {
		t.Errorf("terminal-stdin guard must be a usage error (exit 2), got %d: %v", code, err)
	}
	if !strings.Contains(err.Error(), "terminal") {
		t.Errorf("error should explain stdin is a terminal, got: %v", err)
	}
}

func TestInboxAddFromTerminalStdinFailsFast(t *testing.T) {
	withTerminalStdin(t)
	err := inboxAddText(verbContext{}, "", false)
	if err == nil || exitCode(err) != 2 || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("inbox add must fail fast on terminal stdin (exit 2, names terminal), got: %v", err)
	}
}

func TestInboxBatchFromTerminalStdinFailsFast(t *testing.T) {
	withTerminalStdin(t)
	_, err := readInboxBatchText("apply", "")
	if err == nil || exitCode(err) != 2 || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("inbox batch must fail fast on terminal stdin (exit 2, names terminal), got: %v", err)
	}
}
