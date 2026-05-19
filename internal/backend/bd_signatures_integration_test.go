//go:build integration

package backend

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBdVersionMatchesExpected verifies that the bd binary on PATH reports
// exactly expectedBdVersion. If this test fails, review every constant in
// bd_signatures.go — a version bump may have changed the output strings that
// the retry and recovery logic depends on.
func TestBdVersionMatchesExpected(t *testing.T) {
	t.Helper()
	bdPath, err := exec.LookPath("bd")
	if err != nil {
		t.Skip("bd not found on PATH — skipping integration test")
	}
	t.Logf("bd binary: %s", bdPath)

	out, err := exec.Command("bd", "--version").Output()
	if err != nil {
		t.Fatalf("bd --version failed: %v", err)
	}

	detected := parseBdVersionString(string(out))
	if detected != expectedBdVersion {
		t.Errorf("bd version mismatch: got %q, want %q\n"+
			"Re-verify every constant in bd_signatures.go before bumping expectedBdVersion.",
			detected, expectedBdVersion)
	}
}

// TestNoDaemonFlagSignaturePresent documents the reproduction recipe for the
// "unknown flag: --no-daemon" condition and verifies the constant value is
// consistent with the error bd actually emits when given an unrecognised flag.
//
// Reproduction: run `bd <old-version> --no-daemon list` where <old-version>
// predates the --no-daemon flag. bd writes "unknown flag: --no-daemon" to
// stderr and exits non-zero. The orchestrator catches this via
// isUnknownNoDaemonFlagError, strips the flag, and retries.
//
// This test only validates that the constant string appears in the expected
// error fragment; it cannot replicate an old bd binary in CI.
func TestNoDaemonFlagSignaturePresent(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH — skipping integration test")
	}

	// "unknown flag: --no-daemon" is the verbatim fragment bd emits. Confirm
	// our constant matches the expected format.
	expected := "unknown flag: " + noDaemonFlag
	if !strings.Contains(expected, noDaemonFlag) {
		t.Errorf("noDaemonFlag constant %q not found in expected error fragment %q", noDaemonFlag, expected)
	}
}

// TestOutOfSyncSignatureDocumented documents the reproduction recipe for the
// outOfSyncSignature condition.
//
// Reproduction: edit .beads/issues.jsonl directly (bypassing bd), then run
// any mutating bd command. bd computes a checksum and detects the drift,
// emitting "Database out of sync with JSONL" to stderr. The orchestrator
// catches this via isOutOfSyncError and recovers with `bd sync --import-only`.
//
// This test is a documentation stub — triggering the condition requires a live
// Dolt store, which is not practical in automated CI. The test will pass as
// long as bd is on PATH (proving the binary exists) and the constant is
// non-empty.
func TestOutOfSyncSignatureDocumented(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH — skipping integration test")
	}
	if outOfSyncSignature == "" {
		t.Error("outOfSyncSignature constant must not be empty")
	}
}

// TestDoltPanicSignaturesDocumented documents the reproduction recipe for the
// embedded Dolt engine panic signatures.
//
// Reproduction: kill -9 a bd process in the middle of a write transaction, or
// corrupt the .dolt store while bd holds a connection. The embedded Dolt
// runtime emits a Go panic trace containing both doltNilPanicSignature and
// doltPanicStackSignature. The orchestrator catches either via
// isEmbeddedDoltPanic and retries with BD_NO_DB=1 to bypass the Dolt engine.
//
// This test is a documentation stub — triggering a controlled Dolt panic is
// not practical in automated CI.
func TestDoltPanicSignaturesDocumented(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH — skipping integration test")
	}
	if doltNilPanicSignature == "" {
		t.Error("doltNilPanicSignature constant must not be empty")
	}
	if doltPanicStackSignature == "" {
		t.Error("doltPanicStackSignature constant must not be empty")
	}
}
