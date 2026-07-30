package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/session"
)

// initGitRepo creates a throwaway git repo with two commits so diffLineStats
// and the ledger's CommitSHA/BaseSHA fields have something real to scope
// against, mirroring how commit_marker gates scope their own scan.
func initGitRepo(t *testing.T) (dir, baseSHA, headSHA string) {
	t.Helper()
	dir = t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.invalid",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.invalid",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-q", "-m", "base")
	baseSHA = strings.TrimSpace(gitOutput(t, dir, "rev-parse", "--short", "HEAD"))

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-q", "-m", "stage work")
	headSHA = strings.TrimSpace(gitOutput(t, dir, "rev-parse", "--short", "HEAD"))
	return dir, baseSHA, headSHA
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func readLedgerLines(t *testing.T, path string) []StageAttemptRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ledger: %v", err)
	}
	var recs []StageAttemptRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec StageAttemptRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal ledger line %q: %v", line, err)
		}
		recs = append(recs, rec)
	}
	return recs
}

// --- AC2: nothing is written inside a worktree or the target repository;
// the resolved path is asserted directly. ---

func TestResolveAttemptLedgerPath_OutsideWorktree(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "worktree")
	stateDir := t.TempDir()

	path, err := resolveAttemptLedgerPath(stateDir, "epic-1")
	if err != nil {
		t.Fatalf("resolveAttemptLedgerPath: %v", err)
	}
	if strings.HasPrefix(path, worktree) {
		t.Errorf("ledger path %q must not be inside the worktree %q", path, worktree)
	}
	if !strings.HasPrefix(path, stateDir) {
		t.Errorf("ledger path %q must be under StateDir %q", path, stateDir)
	}
	want := filepath.Join(stateDir, "run", "epic-1", "attempts.jsonl")
	if path != want {
		t.Errorf("ledger path = %q, want %q", path, want)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
		t.Errorf("resolveAttemptLedgerPath must create the epic's run directory: %v", err)
	}
}

func TestAppendStageAttempt_WritesOutsideWorktreeAndTargetRepo(t *testing.T) {
	stateDir := t.TempDir()
	worktree, base, head := initGitRepo(t)

	err := AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID:   "claude",
		Dialect:   "claude",
		BeadID:    "bead-1",
		Stage:     "implementation",
		StartedAt: time.Now(),
		Duration:  time.Second,
		BaseSHA:   base,
		CommitSHA: head,
		Worktree:  worktree,
	}))
	if err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}

	ledgerPath := filepath.Join(stateDir, "run", "epic-1", "attempts.jsonl")
	if _, err := os.Stat(ledgerPath); err != nil {
		t.Fatalf("expected ledger file at %s: %v", ledgerPath, err)
	}
	// The worktree (the target repository's own working copy) must contain
	// nothing kernl wrote - a `git status` in it would be clean.
	entries, err := os.ReadDir(worktree)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != ".git" && e.Name() != "a.txt" {
			t.Errorf("unexpected file %q written inside the worktree", e.Name())
		}
	}
}

// --- AC3: unsafe/empty epicID or empty StateDir fails loud. ---

func TestResolveAttemptLedgerPath_FailsLoudWithoutStateDir(t *testing.T) {
	_, err := resolveAttemptLedgerPath("", "epic-1")
	if err == nil {
		t.Fatal("expected a loud failure when StateDir is empty")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") || !strings.Contains(err.Error(), "StateDir") {
		t.Errorf("error must name the field that fixes it, got: %v", err)
	}
}

func TestResolveAttemptLedgerPath_RefusesUnsafeEpicID(t *testing.T) {
	stateDir := t.TempDir()
	unsafe := []string{"..", ".", "", "../../../..", "a/../../b", "sub/dir", `sub\dir`}

	for _, epicID := range unsafe {
		t.Run("epicID="+epicID, func(t *testing.T) {
			path, err := resolveAttemptLedgerPath(stateDir, epicID)
			if err == nil {
				t.Fatalf("expected refusal for unsafe epic id %q, got path %q", epicID, path)
			}
			if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
				t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
			}
			if path != "" {
				t.Errorf("a refused id must not return any path, got %q", path)
			}
		})
	}
}

func TestAppendStageAttempt_PropagatesPathFailure(t *testing.T) {
	err := AppendStageAttempt("", "epic-1", StageAttemptRecord{BeadID: "bead-1", Stage: "implementation"})
	if err == nil {
		t.Fatal("expected AppendStageAttempt to fail loud when StateDir is empty")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
	}
}

// --- AC1 + AC4: one line per attempt, every field the dialect reported and
// null for every field it did not - one test per dialect. ---

func TestAppendStageAttempt_ClaudeRow_CarriesEveryReportedField(t *testing.T) {
	stateDir := t.TempDir()
	worktree, base, head := initGitRepo(t)

	cost := 0.0456
	turns := int64(3)
	cacheWrite := int64(300)
	cacheRead := int64(200)
	model := "claude-opus-5"

	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID:         "claude-primary",
		Dialect:         "claude",
		ConfiguredModel: "opus",
		Pool:            "implementation",
		BeadID:          "bead-claude",
		Stage:           "implementation",
		SessionID:       "sess-abc",
		StartedAt:       time.Now(),
		Duration:        4200 * time.Millisecond,
		ExitCode:        0,
		BaseSHA:         base,
		CommitSHA:       head,
		Worktree:        worktree,
		GatePassed:      true,
		FollowUpCount:   0,
		Nudged:          false,
		Usage: &session.TokenUsageCounts{
			InputTokens:      500,
			OutputTokens:     120,
			TotalTokens:      620,
			CacheReadTokens:  &cacheRead,
			CacheWriteTokens: &cacheWrite,
			CostUSD:          &cost,
			Turns:            &turns,
			Model:            &model,
			// ReasoningTokens intentionally left nil: claude does not report it.
		},
	})
	if err := AppendStageAttempt(stateDir, "epic-claude", rec); err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-claude", "attempts.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	got := lines[0]

	// Reported fields are present.
	if got.Model != "claude-opus-5" || !got.ModelResolved {
		t.Errorf("Model/ModelResolved = %q/%v, want the CLI-reported model marked resolved", got.Model, got.ModelResolved)
	}
	if got.InputTokens == nil || *got.InputTokens != 500 {
		t.Errorf("InputTokens = %v, want 500", got.InputTokens)
	}
	if got.OutputTokens == nil || *got.OutputTokens != 120 {
		t.Errorf("OutputTokens = %v, want 120", got.OutputTokens)
	}
	if got.CacheReadTokens == nil || *got.CacheReadTokens != 200 {
		t.Errorf("CacheReadTokens = %v, want 200", got.CacheReadTokens)
	}
	if got.CacheWriteTokens == nil || *got.CacheWriteTokens != 300 {
		t.Errorf("CacheWriteTokens = %v, want 300", got.CacheWriteTokens)
	}
	if got.CostUSD == nil || *got.CostUSD != 0.0456 {
		t.Errorf("CostUSD = %v, want 0.0456", got.CostUSD)
	}
	if got.Turns == nil || *got.Turns != 3 {
		t.Errorf("Turns = %v, want 3", got.Turns)
	}

	// Unreported field stays null, never a fabricated estimate.
	if got.ReasoningTokens != nil {
		t.Errorf("ReasoningTokens = %v, want nil (claude does not report it)", got.ReasoningTokens)
	}

	if got.DiffLinesAdded == nil || *got.DiffLinesAdded != 2 {
		t.Errorf("DiffLinesAdded = %v, want 2", got.DiffLinesAdded)
	}
	if got.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1", got.AttemptNumber)
	}
}

func TestAppendStageAttempt_CodexRow_CarriesEveryReportedField(t *testing.T) {
	stateDir := t.TempDir()
	worktree, base, head := initGitRepo(t)

	cacheWrite := int64(15)
	cacheRead := int64(40)
	reasoning := int64(8)

	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID:         "codex-primary",
		Dialect:         "codex",
		ConfiguredModel: "gpt-6-codex",
		Pool:            "implementation",
		BeadID:          "bead-codex",
		Stage:           "implementation",
		SessionID:       "sess-def",
		StartedAt:       time.Now(),
		Duration:        3100 * time.Millisecond,
		ExitCode:        0,
		BaseSHA:         base,
		CommitSHA:       head,
		Worktree:        worktree,
		GatePassed:      true,
		Usage: &session.TokenUsageCounts{
			InputTokens:      100,
			OutputTokens:     20,
			TotalTokens:      120,
			CacheReadTokens:  &cacheRead,
			CacheWriteTokens: &cacheWrite,
			ReasoningTokens:  &reasoning,
			// Model, CostUSD, Turns intentionally nil: codex never reports them.
		},
	})
	if err := AppendStageAttempt(stateDir, "epic-codex", rec); err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-codex", "attempts.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	got := lines[0]

	if got.CacheReadTokens == nil || *got.CacheReadTokens != 40 {
		t.Errorf("CacheReadTokens = %v, want 40", got.CacheReadTokens)
	}
	if got.CacheWriteTokens == nil || *got.CacheWriteTokens != 15 {
		t.Errorf("CacheWriteTokens = %v, want 15", got.CacheWriteTokens)
	}
	if got.ReasoningTokens == nil || *got.ReasoningTokens != 8 {
		t.Errorf("ReasoningTokens = %v, want 8", got.ReasoningTokens)
	}

	// Fields codex never reports stay null.
	if got.CostUSD != nil {
		t.Errorf("CostUSD = %v, want nil (codex does not report cost)", got.CostUSD)
	}
	if got.Turns != nil {
		t.Errorf("Turns = %v, want nil (codex does not report turns)", got.Turns)
	}

	// --- AC6: the row makes the alias fallback unambiguous. ---
	if got.ModelResolved {
		t.Errorf("ModelResolved = true, want false - codex never reports a resolved model")
	}
	if got.Model != "gpt-6-codex" {
		t.Errorf("Model = %q, want the configured alias %q to be recorded as the fallback value", got.Model, "gpt-6-codex")
	}
}

// --- AC5: a second attempt caused by a review rejection records
// attemptNumber 2 and a causedBy pointing at the rejecting artifact. ---

func TestAppendStageAttempt_SecondAttemptRecordsCausedByAndAttemptNumber(t *testing.T) {
	stateDir := t.TempDir()
	worktree, base, head := initGitRepo(t)

	// Attempt 1: implementation runs and passes its own gate.
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, BaseSHA: base, CommitSHA: head,
		Worktree: worktree, GatePassed: true,
	})))

	// Review runs against the implementation and rejects it - the exit gate
	// fails with the exact reason shape backend.EvaluateExitGate produces
	// for artifact_verdict gates.
	reviewArtifact := filepath.Join(stateDir, "run", "epic-1", "bead-1", "implementation-review.md")
	rejectVerdict := "FAIL"
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "reviewer", Dialect: "claude", BeadID: "bead-1", Stage: "implementation_review",
		StartedAt: time.Now(), Duration: time.Second, BaseSHA: base, CommitSHA: head,
		Worktree: worktree, GatePassed: false,
		GateFailureReason: "verdict_not_pass: " + reviewArtifact,
		ReviewVerdict:     &rejectVerdict,
	})))

	// The bead is unblocked and implementation runs again - attempt 2,
	// caused by the rejection above.
	secondAttempt := BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, BaseSHA: base, CommitSHA: head,
		Worktree: worktree, GatePassed: true,
	})
	must(t, AppendStageAttempt(stateDir, "epic-1", secondAttempt))

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-1", "attempts.jsonl"))
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	last := lines[2]

	if last.Stage != "implementation" {
		t.Fatalf("sanity: last row stage = %q, want implementation", last.Stage)
	}
	if last.AttemptNumber != 2 {
		t.Errorf("AttemptNumber = %d, want 2", last.AttemptNumber)
	}
	if last.CausedBy == nil || *last.CausedBy != reviewArtifact {
		t.Errorf("CausedBy = %v, want %q (the rejecting review artifact)", last.CausedBy, reviewArtifact)
	}

	// The review row itself is attempt 1 of its own stage, and its
	// firstPassApproved must be false (it was rejected).
	review := lines[1]
	if review.AttemptNumber != 1 {
		t.Errorf("review AttemptNumber = %d, want 1", review.AttemptNumber)
	}
	if review.FirstPassApproved == nil || *review.FirstPassApproved {
		t.Errorf("review FirstPassApproved = %v, want false", review.FirstPassApproved)
	}
}

func TestAppendStageAttempt_FirstPassApprovedTrueOnCleanReview(t *testing.T) {
	stateDir := t.TempDir()
	worktree, base, head := initGitRepo(t)

	passVerdict := "PASS"
	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "reviewer", Dialect: "claude", BeadID: "bead-clean", Stage: "implementation_review",
		StartedAt: time.Now(), Duration: time.Second, BaseSHA: base, CommitSHA: head,
		Worktree: worktree, GatePassed: true, ReviewVerdict: &passVerdict,
	})
	must(t, AppendStageAttempt(stateDir, "epic-clean", rec))

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-clean", "attempts.jsonl"))
	got := lines[0]
	if got.FirstPassApproved == nil || !*got.FirstPassApproved {
		t.Errorf("FirstPassApproved = %v, want true", got.FirstPassApproved)
	}
}

// --- No pre-aggregation: the ledger holds raw rows only. ---

func TestAppendStageAttempt_NeverAggregates(t *testing.T) {
	stateDir := t.TempDir()
	worktree, base, head := initGitRepo(t)

	for i := 0; i < 3; i++ {
		must(t, AppendStageAttempt(stateDir, "epic-raw", BuildStageAttemptRecord(StageAttemptInput{
			AgentID: "claude", Dialect: "claude", BeadID: "bead-raw", Stage: "implementation",
			StartedAt: time.Now(), Duration: time.Second, BaseSHA: base, CommitSHA: head,
			Worktree: worktree, GatePassed: false, GateFailureReason: "commit_marker_missing: DONE",
		})))
	}

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-raw", "attempts.jsonl"))
	if len(lines) != 3 {
		t.Fatalf("expected 3 raw rows (one per attempt), got %d - the ledger must never collapse rows into a summary", len(lines))
	}
	for i, rec := range lines {
		if rec.AttemptNumber != i+1 {
			t.Errorf("row %d AttemptNumber = %d, want %d", i, rec.AttemptNumber, i+1)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
