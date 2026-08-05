package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRawLedger plants a ledger by hand rather than through
// AppendStageAttempt. That is the point of these tests: the rows they cover
// were written by an older build whose derivation matched verdict_not_pass,
// and the current writer cannot produce them.
func writeRawLedger(t *testing.T, stateDir, epic string, lines ...string) string {
	t.Helper()
	dir := filepath.Join(stateDir, "run", epic)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "attempts.jsonl")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func causedByOf(t *testing.T, line string) *string {
	t.Helper()
	var rec StageAttemptRecord
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("parsing %q: %v", line, err)
	}
	return rec.CausedBy
}

const (
	rowImplPassed  = `{"agentId":"claude","beadId":"b1","stage":"implementation","gatePassed":true,"causedBy":null}`
	rowRejected    = `{"agentId":"codex","beadId":"b1","stage":"implementation_review","gatePassed":false,"gateFailureReason":"verdict_reject: /artifacts/implementation-review.md","causedBy":null}`
	rowRedo        = `{"agentId":"claude","beadId":"b1","stage":"implementation","gatePassed":true,"causedBy":null}`
	rowNoVerdict   = `{"agentId":"codex","beadId":"b2","stage":"implementation_review","gatePassed":false,"gateFailureReason":"verdict_not_pass: /artifacts/b2-review.md","causedBy":null}`
	rowAfterNoVerd = `{"agentId":"claude","beadId":"b2","stage":"implementation","gatePassed":true,"causedBy":"/artifacts/b2-review.md"}`
)

func TestBackfillCausedBy_MarksTheRowsTheOldRuleMissed(t *testing.T) {
	stateDir := t.TempDir()
	path := writeRawLedger(t, stateDir, "epic-a", rowImplPassed, rowRejected, rowRedo)

	result, err := BackfillCausedBy(stateDir, true)
	if err != nil {
		t.Fatalf("BackfillCausedBy: %v", err)
	}
	if len(result.Ledgers) != 1 || result.Ledgers[0].Marked != 1 {
		t.Fatalf("expected one row marked as rework, got %+v", result.Ledgers)
	}
	if result.RowsScanned != 3 {
		t.Errorf("RowsScanned = %d, want 3", result.RowsScanned)
	}

	lines := readLines(t, path)
	got := causedByOf(t, lines[2])
	if got == nil || *got != "/artifacts/implementation-review.md" {
		t.Errorf("the attempt after the rejection must name the rejecting artifact, got %v", got)
	}
}

// The mirror image: a row marked under the old rule, where the review
// produced no verdict at all and sent nothing back, must lose the mark.
func TestBackfillCausedBy_ClearsWhatWasNeverARejection(t *testing.T) {
	stateDir := t.TempDir()
	path := writeRawLedger(t, stateDir, "epic-a", rowNoVerdict, rowAfterNoVerd)

	result, err := BackfillCausedBy(stateDir, true)
	if err != nil {
		t.Fatalf("BackfillCausedBy: %v", err)
	}
	if len(result.Ledgers) != 1 || result.Ledgers[0].Unmarked != 1 {
		t.Fatalf("expected one row unmarked, got %+v", result.Ledgers)
	}
	if got := causedByOf(t, readLines(t, path)[1]); got != nil {
		t.Errorf("causedBy = %q, want null - that review never sent work back", *got)
	}
}

func TestBackfillCausedBy_DryRunReportsWithoutWriting(t *testing.T) {
	stateDir := t.TempDir()
	path := writeRawLedger(t, stateDir, "epic-a", rowImplPassed, rowRejected, rowRedo)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	result, err := BackfillCausedBy(stateDir, false)
	if err != nil {
		t.Fatalf("BackfillCausedBy: %v", err)
	}
	if result.Applied {
		t.Error("Applied must be false on a dry run")
	}
	if len(result.Ledgers) != 1 || result.Ledgers[0].Marked != 1 {
		t.Fatalf("a dry run must still report what it would change, got %+v", result.Ledgers)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a dry run must not touch the ledger")
	}
}

// An unchanged row keeps its exact bytes, so a diff of the ledger after the
// pass shows the rows it decided about and nothing else.
func TestBackfillCausedBy_LeavesUntouchedRowsByteIdentical(t *testing.T) {
	stateDir := t.TempDir()
	path := writeRawLedger(t, stateDir, "epic-a", rowImplPassed, rowRejected, rowRedo)

	if _, err := BackfillCausedBy(stateDir, true); err != nil {
		t.Fatalf("BackfillCausedBy: %v", err)
	}
	lines := readLines(t, path)
	if lines[0] != rowImplPassed {
		t.Errorf("row 0 was rewritten though nothing about it changed:\n got %s\nwant %s", lines[0], rowImplPassed)
	}
	if lines[1] != rowRejected {
		t.Errorf("row 1 was rewritten though nothing about it changed:\n got %s\nwant %s", lines[1], rowRejected)
	}
}

// A rewritten row is re-encoded through a generic map so a field this build
// does not know about survives. These rows were written by older builds,
// which is exactly the population where that can happen.
func TestBackfillCausedBy_KeepsFieldsThisBuildDoesNotKnow(t *testing.T) {
	stateDir := t.TempDir()
	redoWithExtra := `{"agentId":"claude","beadId":"b1","stage":"implementation","gatePassed":true,"causedBy":null,"retiredField":"keep me"}`
	path := writeRawLedger(t, stateDir, "epic-a", rowImplPassed, rowRejected, redoWithExtra)

	if _, err := BackfillCausedBy(stateDir, true); err != nil {
		t.Fatalf("BackfillCausedBy: %v", err)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(readLines(t, path)[2]), &row); err != nil {
		t.Fatal(err)
	}
	if row["retiredField"] != "keep me" {
		t.Errorf("a field this build has no struct member for was dropped: %+v", row)
	}
	if row["causedBy"] != "/artifacts/implementation-review.md" {
		t.Errorf("causedBy was not recomputed: %+v", row)
	}
}

func TestBackfillCausedBy_NoLedgersIsNotAnError(t *testing.T) {
	result, err := BackfillCausedBy(t.TempDir(), true)
	if err != nil {
		t.Fatalf("an empty state dir is the never-ran case, not a failure: %v", err)
	}
	if result.LedgersScanned != 0 || len(result.Ledgers) != 0 {
		t.Errorf("expected nothing scanned, got %+v", result)
	}
}

// A ledger already consistent with the current rule is scanned and reported
// as nothing to do, not listed as work.
func TestBackfillCausedBy_ConsistentLedgerIsNotListed(t *testing.T) {
	stateDir := t.TempDir()
	writeRawLedger(t, stateDir, "epic-a", rowImplPassed)

	result, err := BackfillCausedBy(stateDir, true)
	if err != nil {
		t.Fatalf("BackfillCausedBy: %v", err)
	}
	if result.LedgersScanned != 1 {
		t.Errorf("LedgersScanned = %d, want 1", result.LedgersScanned)
	}
	if len(result.Ledgers) != 0 {
		t.Errorf("a ledger with nothing to change must not be listed, got %+v", result.Ledgers)
	}
}

// The safety copy exists only for the duration of a rewrite that succeeds.
// One left behind means the rewrite failed, and the error says so - so a
// successful pass must not leave one to be mistaken for that.
func TestBackfillCausedBy_RemovesItsSafetyCopy(t *testing.T) {
	stateDir := t.TempDir()
	path := writeRawLedger(t, stateDir, "epic-a", rowImplPassed, rowRejected, rowRedo)

	if _, err := BackfillCausedBy(stateDir, true); err != nil {
		t.Fatalf("BackfillCausedBy: %v", err)
	}
	if _, err := os.Stat(path + ".before-backfill"); !os.IsNotExist(err) {
		t.Errorf("the safety copy must be gone after a successful rewrite, stat gave: %v", err)
	}
}

// A torn trailing row is trimmed exactly as the next append would have
// trimmed it, and reported rather than silently repaired.
func TestBackfillCausedBy_TrimsADanglingTrailingRow(t *testing.T) {
	stateDir := t.TempDir()
	path := writeRawLedger(t, stateDir, "epic-a", rowImplPassed)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"agentId":"claude","bea`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := BackfillCausedBy(stateDir, true)
	if err != nil {
		t.Fatalf("BackfillCausedBy: %v", err)
	}
	if len(result.Ledgers) != 1 || !result.Ledgers[0].DanglingDropped {
		t.Fatalf("the incomplete row must be reported, got %+v", result.Ledgers)
	}
	if lines := readLines(t, path); len(lines) != 1 || lines[0] != rowImplPassed {
		t.Errorf("expected only the complete row to survive, got %q", lines)
	}
}
