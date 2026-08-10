package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

// fakeDiffStatter is a named stub for DiffStatter so ledger tests never have
// to shell out to a real git binary (AGENTS.md §4: unit tests must not
// touch the host).
type fakeDiffStatter struct {
	added, removed *int
}

func (f fakeDiffStatter) DiffStat(worktree, baseSHA, commitSHA string) (added, removed *int) {
	return f.added, f.removed
}

func intPtr(v int) *int { return &v }

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
	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktree, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID:   "claude",
		Dialect:   "claude",
		BeadID:    "bead-1",
		Stage:     "implementation",
		StartedAt: time.Now(),
		Duration:  time.Second,
		BaseSHA:   "base-sha",
		CommitSHA: "head-sha",
		Worktree:  worktree,
		DiffStats: fakeDiffStatter{},
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
		if e.Name() != "a.txt" {
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
	worktree := t.TempDir()

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
		ExitCode:        intPtr(0),
		BaseSHA:         "base-sha",
		CommitSHA:       "head-sha",
		Worktree:        worktree,
		GatePassed:      true,
		FollowUpCount:   0,
		Nudged:          false,
		DiffStats:       fakeDiffStatter{added: intPtr(2), removed: intPtr(0)},
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
	if got.Model == nil || *got.Model != "claude-opus-5" || !got.ModelResolved {
		t.Errorf("Model/ModelResolved = %v/%v, want the CLI-reported model marked resolved", got.Model, got.ModelResolved)
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", got.ExitCode)
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
	worktree := t.TempDir()

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
		ExitCode:        intPtr(0),
		BaseSHA:         "base-sha",
		CommitSHA:       "head-sha",
		Worktree:        worktree,
		GatePassed:      true,
		DiffStats:       fakeDiffStatter{},
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
	if got.Model == nil || *got.Model != "gpt-6-codex" {
		t.Errorf("Model = %v, want the configured alias %q to be recorded as the fallback value", got.Model, "gpt-6-codex")
	}
}

// pi and opencode report no num_turns field of their own (Usage.Turns is
// always nil for them - see session.ExtractTokenUsageFromEvent), so the
// ledger's live turn count has to reach StageAttemptRecord.Turns through
// StageAttemptInput.Turns instead, populated from
// session.SessionRuntime.CountedTurns via RunBeadResult.Turns.
func TestAppendStageAttempt_PiRow_PersistsLiveTurnsWhenUsageReportsNone(t *testing.T) {
	stateDir := t.TempDir()
	turns := int64(5)

	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID:    "pi-kimi",
		Dialect:    "pi",
		Pool:       "implementation",
		BeadID:     "bead-pi",
		Stage:      "implementation",
		SessionID:  "sess-pi",
		StartedAt:  time.Now(),
		Duration:   time.Second,
		ExitCode:   intPtr(0),
		GatePassed: true,
		DiffStats:  fakeDiffStatter{},
		Usage: &session.TokenUsageCounts{
			InputTokens:  10,
			OutputTokens: 2,
			TotalTokens:  12,
			// Turns intentionally nil: pi's own usage event carries none.
		},
		Turns: &turns,
	})
	if err := AppendStageAttempt(stateDir, "epic-pi", rec); err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-pi", "attempts.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := lines[0].Turns; got == nil || *got != 5 {
		t.Errorf("Turns = %v, want 5", got)
	}
}

// A dialect that counts no turn boundaries at all (StageAttemptInput.Turns
// left nil, e.g. codex or subprocess) must persist a null Turns column,
// never a fabricated number.
func TestAppendStageAttempt_UnsupportedDialectTurnsStaysNull(t *testing.T) {
	stateDir := t.TempDir()

	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID:    "codex-primary",
		Dialect:    "codex",
		BeadID:     "bead-codex-null",
		Stage:      "implementation",
		StartedAt:  time.Now(),
		Duration:   time.Second,
		GatePassed: true,
		DiffStats:  fakeDiffStatter{},
		// Turns left unset: codex has no counted turn boundary.
	})
	if err := AppendStageAttempt(stateDir, "epic-codex-null", rec); err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-codex-null", "attempts.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := lines[0].Turns; got != nil {
		t.Errorf("Turns = %v, want nil", *got)
	}
}

// A run that genuinely crossed zero counted boundaries (StageAttemptInput.Turns
// pointing at 0, not left nil) must round-trip through the ledger as the
// number 0, not as null - collapsing the two would make "kernl never
// measured this" indistinguishable from "kernl measured zero".
func TestAppendStageAttempt_MeasuredZeroTurnsPersistsAsZeroNotNull(t *testing.T) {
	stateDir := t.TempDir()
	zero := int64(0)

	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID:    "opencode-primary",
		Dialect:    "opencode",
		BeadID:     "bead-opencode-zero",
		Stage:      "implementation",
		StartedAt:  time.Now(),
		Duration:   time.Second,
		GatePassed: true,
		DiffStats:  fakeDiffStatter{},
		Turns:      &zero,
	})
	if err := AppendStageAttempt(stateDir, "epic-opencode-zero", rec); err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-opencode-zero", "attempts.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	got := lines[0].Turns
	if got == nil {
		t.Fatal("Turns = nil, want a measured 0 - not null")
	}
	if *got != 0 {
		t.Errorf("Turns = %d, want 0", *got)
	}
}

// Claude's own num_turns (carried through Usage.Turns) must keep taking
// precedence over StageAttemptInput.Turns's live count - in production the
// two are mutually exclusive (only pi/opencode ever populate Turns, only
// claude ever populates Usage.Turns), but this pins that a live count can
// never silently overwrite a dialect-reported one.
func TestAppendStageAttempt_ClaudeNumTurnsTakesPrecedenceOverLiveTurns(t *testing.T) {
	stateDir := t.TempDir()
	reported := int64(3)
	live := int64(99)

	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID:    "claude-primary",
		Dialect:    "claude",
		BeadID:     "bead-claude-precedence",
		Stage:      "implementation",
		StartedAt:  time.Now(),
		Duration:   time.Second,
		GatePassed: true,
		DiffStats:  fakeDiffStatter{},
		Usage: &session.TokenUsageCounts{
			InputTokens:  10,
			OutputTokens: 2,
			TotalTokens:  12,
			Turns:        &reported,
		},
		Turns: &live,
	})
	if err := AppendStageAttempt(stateDir, "epic-claude-precedence", rec); err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-claude-precedence", "attempts.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := lines[0].Turns; got == nil || *got != 3 {
		t.Errorf("Turns = %v, want 3 (claude's own num_turns, not the live count)", got)
	}
}

// TestAppendStageAttempt_CodexRow_NoConfiguredModelYieldsNilNotEmpty proves
// finding 5's fix: a codex agent with no configured model (settings.agents.<id>.model
// is optional) must not record model:"" - an empty string identifies
// neither the alias nor what ran, which is exactly the comparison the
// ledger exists to enable. It must record null: kernl genuinely does not
// know.
func TestAppendStageAttempt_CodexRow_NoConfiguredModelYieldsNilNotEmpty(t *testing.T) {
	stateDir := t.TempDir()
	worktree := t.TempDir()

	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID:         "codex-default",
		Dialect:         "codex",
		ConfiguredModel: "", // no model configured for this agent
		BeadID:          "bead-codex-nomodel",
		Stage:           "implementation",
		StartedAt:       time.Now(),
		Duration:        time.Second,
		Worktree:        worktree,
		GatePassed:      true,
		DiffStats:       fakeDiffStatter{},
		Usage: &session.TokenUsageCounts{
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
			// Model nil: codex never reports one.
		},
	})
	if err := AppendStageAttempt(stateDir, "epic-nomodel", rec); err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-nomodel", "attempts.jsonl"))
	got := lines[0]
	if got.Model != nil {
		t.Errorf("Model = %v, want nil - neither a resolved model nor a configured alias exists for this row", *got.Model)
	}
	if got.ModelResolved {
		t.Error("ModelResolved = true, want false")
	}
}

// --- AC5: a second attempt caused by a review rejection records
// attemptNumber 2 and a causedBy pointing at the rejecting artifact. ---

func TestAppendStageAttempt_SecondAttemptRecordsCausedByAndAttemptNumber(t *testing.T) {
	stateDir := t.TempDir()
	worktree := t.TempDir()

	// Attempt 1: implementation runs and passes its own gate.
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, BaseSHA: "base-sha", CommitSHA: "head-sha",
		Worktree: worktree, GatePassed: true, DiffStats: fakeDiffStatter{},
	})))

	// Review runs against the implementation and rejects it - the exit gate
	// fails with the exact reason shape backend.evaluateSingleExitGate
	// produces for a deliberate "VERDICT: REJECT" at a state that has
	// somewhere to send the work back to.
	reviewArtifact := filepath.Join(stateDir, "run", "epic-1", "bead-1", "implementation-review.md")
	rejectVerdict := "FAIL"
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "reviewer", Dialect: "claude", BeadID: "bead-1", Stage: "implementation_review",
		StartedAt: time.Now(), Duration: time.Second, BaseSHA: "base-sha", CommitSHA: "head-sha",
		Worktree: worktree, GatePassed: false, DiffStats: fakeDiffStatter{},
		GateFailureReason: "verdict_reject: " + reviewArtifact,
		ReviewVerdict:     &rejectVerdict,
	})))

	// The bead is unblocked and implementation runs again - attempt 2,
	// caused by the rejection above.
	secondAttempt := BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, BaseSHA: "base-sha", CommitSHA: "head-sha",
		Worktree: worktree, GatePassed: true, DiffStats: fakeDiffStatter{},
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
	worktree := t.TempDir()

	passVerdict := "PASS"
	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "reviewer", Dialect: "claude", BeadID: "bead-clean", Stage: "implementation_review",
		StartedAt: time.Now(), Duration: time.Second, BaseSHA: "base-sha", CommitSHA: "head-sha",
		Worktree: worktree, GatePassed: true, ReviewVerdict: &passVerdict, DiffStats: fakeDiffStatter{},
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
	worktree := t.TempDir()

	for i := 0; i < 3; i++ {
		must(t, AppendStageAttempt(stateDir, "epic-raw", BuildStageAttemptRecord(StageAttemptInput{
			AgentID: "claude", Dialect: "claude", BeadID: "bead-raw", Stage: "implementation",
			StartedAt: time.Now(), Duration: time.Second, BaseSHA: "base-sha", CommitSHA: "head-sha",
			Worktree: worktree, GatePassed: false, GateFailureReason: "commit_marker_missing: DONE",
			DiffStats: fakeDiffStatter{},
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

// --- Finding 3: exit codes are real, never fabricated. ---

func TestBuildStageAttemptRecord_ExitCodeIsNilWhenNoProcessRan(t *testing.T) {
	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-spawn-fail", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: t.TempDir(),
		ExitCode:  nil, // spawn failed before any process existed
		DiffStats: fakeDiffStatter{},
	})
	if rec.ExitCode != nil {
		t.Errorf("ExitCode = %v, want nil - no process ever exited to report one", *rec.ExitCode)
	}
}

func TestBuildStageAttemptRecord_ExitCodePreservesRealValue(t *testing.T) {
	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-real-code", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: t.TempDir(),
		ExitCode:  intPtr(42),
		DiffStats: fakeDiffStatter{},
	})
	if rec.ExitCode == nil || *rec.ExitCode != 42 {
		t.Errorf("ExitCode = %v, want 42 - the process's real exit code, not a fabricated 1", rec.ExitCode)
	}
}

// --- Finding 4: concurrent writers never duplicate attempt numbers or
// corrupt the file. ---

// TestAppendStageAttempt_ConcurrentWritersProduceUniqueAttemptNumbers proves
// the flock-based serialization: many goroutines, each opening its own file
// descriptor to the same ledger path (exactly the contention flock(2)
// describes between independently-opened descriptors, which is the same
// contention two separate kernl processes would produce - see the doc
// comment on AppendStageAttempt), append concurrently for the same
// bead+stage. Every row must survive, every row must parse, and every
// AttemptNumber from 1..N must appear exactly once.
func TestAppendStageAttempt_ConcurrentWritersProduceUniqueAttemptNumbers(t *testing.T) {
	stateDir := t.TempDir()
	worktree := t.TempDir()

	const n = 50
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // maximize overlap: every goroutine is ready before any proceeds
			err := AppendStageAttempt(stateDir, "epic-concurrent", BuildStageAttemptRecord(StageAttemptInput{
				AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
				StartedAt: time.Now(), Duration: time.Millisecond, BaseSHA: "base-sha", CommitSHA: "head-sha",
				Worktree: worktree, GatePassed: true, DiffStats: fakeDiffStatter{},
			}))
			errCh <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Errorf("concurrent AppendStageAttempt returned an error: %v", err)
		}
	}

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-concurrent", "attempts.jsonl"))
	if len(lines) != n {
		t.Fatalf("expected %d lines (one per concurrent attempt, none lost or corrupted), got %d", n, len(lines))
	}

	seen := make(map[int]bool, n)
	for _, rec := range lines {
		if seen[rec.AttemptNumber] {
			t.Errorf("duplicate attemptNumber %d - two concurrent writers derived the same number", rec.AttemptNumber)
		}
		seen[rec.AttemptNumber] = true
	}
	for i := 1; i <= n; i++ {
		if !seen[i] {
			t.Errorf("missing attemptNumber %d - expected the contiguous set 1..%d", i, n)
		}
	}
}

// --- Finding 1: a write failure never discards silently, and never leaves
// a partial line that poisons every later append. ---

// shortWriteFile wraps a real *os.File (so Fd() stays valid for flock) and
// reports fewer bytes written than requested, mirroring what Go's writeFile
// loop can observe from a real short write(2) (e.g. ENOSPC mid-write) -
// something a hermetic test cannot trigger by actually exhausting disk
// space.
type shortWriteFile struct {
	*os.File
	failAfter int
}

func (f *shortWriteFile) Write(p []byte) (int, error) {
	if f.failAfter >= 0 && f.failAfter < len(p) {
		n, err := f.File.Write(p[:f.failAfter])
		if err != nil {
			return n, err
		}
		return n, nil // short write: n < len(p), no error - exactly the case os.File.Write can return
	}
	return f.File.Write(p)
}

// closeErrLedgerFile wraps a real *os.File and reports a Close() failure
// after actually closing it, so AppendStageAttempt's handling of that
// failure can be exercised without an unclosable real file descriptor.
type closeErrLedgerFile struct {
	*os.File
	closeErr error
}

func (f *closeErrLedgerFile) Close() error {
	_ = f.File.Close()
	return f.closeErr
}

func TestAppendStageAttempt_ShortWriteIsUndoneNotLeftDangling(t *testing.T) {
	stateDir := t.TempDir()

	// Seed the ledger with one already-valid row, so the test can prove the
	// truncate-back restores exactly that pre-write size, not zero.
	seedRec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-seed", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: t.TempDir(),
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})
	must(t, AppendStageAttempt(stateDir, "epic-fail", seedRec))

	path := filepath.Join(stateDir, "run", "epic-fail", "attempts.jsonl")
	seededInfo, err := os.Stat(path)
	must(t, err)
	seededSize := seededInfo.Size()

	failingOpen := func(p string) (ledgerFile, error) {
		f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			return nil, err
		}
		// 5 bytes is always less than a full JSON line, so every append in
		// this test is guaranteed to short-write.
		return &shortWriteFile{File: f, failAfter: 5}, nil
	}

	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-2", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: t.TempDir(),
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})
	appendErr := appendStageAttempt(stateDir, "epic-fail", rec, failingOpen)
	if appendErr == nil {
		t.Fatal("expected an error from a short write")
	}
	if !strings.Contains(appendErr.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the marker, got: %v", appendErr)
	}

	afterInfo, err := os.Stat(path)
	must(t, err)
	if afterInfo.Size() != seededSize {
		t.Errorf("file size after a failed write = %d, want unchanged at %d (the partial write must be undone)", afterInfo.Size(), seededSize)
	}

	lines := readLedgerLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("expected only the seed row to survive a failed append, got %d lines", len(lines))
	}
}

func TestAppendStageAttempt_CloseErrorIsSurfacedNotDiscarded(t *testing.T) {
	stateDir := t.TempDir()

	failingOpen := func(p string) (ledgerFile, error) {
		f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			return nil, err
		}
		return &closeErrLedgerFile{File: f, closeErr: errors.New("simulated close failure")}, nil
	}

	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: t.TempDir(),
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})
	appendErr := appendStageAttempt(stateDir, "epic-close", rec, failingOpen)
	if appendErr == nil {
		t.Fatal("expected the injected close error to be surfaced")
	}
	if !strings.Contains(appendErr.Error(), "simulated close failure") {
		t.Errorf("error must wrap the underlying close failure, got: %v", appendErr)
	}
}

// TestAppendStageAttempt_RepairsDanglingTrailingRow proves the read-side
// half of finding 1: a ledger file left with a dangling, non-newline-terminated
// fragment at the end (exactly what an interrupted write - one this fix's
// own truncate-back did not get to run for, e.g. the process was killed
// outright) must not disable every future append. The next successful call
// drops the dangling fragment, does not count it, and leaves the file valid.
func TestAppendStageAttempt_RepairsDanglingTrailingRow(t *testing.T) {
	stateDir := t.TempDir()
	path, err := resolveAttemptLedgerPath(stateDir, "epic-dangling")
	must(t, err)

	validLine, err := json.Marshal(BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: t.TempDir(),
		GatePassed: true, DiffStats: fakeDiffStatter{},
	}))
	must(t, err)

	// A dangling fragment with no trailing newline - never something a
	// completed AppendStageAttempt call could have produced.
	dangling := `{"agentId":"claude","beadId":"bead-2","stage":"implementati`
	must(t, os.WriteFile(path, append(append(validLine, '\n'), []byte(dangling)...), 0o644))

	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-3", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: t.TempDir(),
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})
	if err := AppendStageAttempt(stateDir, "epic-dangling", rec); err != nil {
		t.Fatalf("AppendStageAttempt after a dangling row: %v", err)
	}

	lines := readLedgerLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("expected 2 valid rows (the seed + the new append; the dangling fragment must be dropped, not kept or counted), got %d", len(lines))
	}
	if lines[0].BeadID != "bead-1" || lines[1].BeadID != "bead-3" {
		t.Errorf("unexpected bead ids in surviving rows: %q, %q", lines[0].BeadID, lines[1].BeadID)
	}
	// AttemptNumber for the new row must be derived from the 1 valid prior
	// row only - the dangling fragment (bead-2) was never a real attempt.
	if lines[1].AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1 (dangling fragment must not be counted)", lines[1].AttemptNumber)
	}

	raw, err := os.ReadFile(path)
	must(t, err)
	if strings.Contains(string(raw), "bead-2") {
		t.Error("the dangling fragment's bytes must be trimmed from the file, not merely ignored on read")
	}
}

// TestAppendStageAttempt_MidFileCorruptionFailsLoud proves the other half
// of finding 1's boundary: a malformed row that is NOT the trailing
// fragment (something wrote broken JSON in the middle of the file, with a
// valid row after it) is a harder failure than "an interrupted write" and
// must not be silently repaired - it is surfaced, and nothing is appended.
func TestAppendStageAttempt_MidFileCorruptionFailsLoud(t *testing.T) {
	stateDir := t.TempDir()
	path, err := resolveAttemptLedgerPath(stateDir, "epic-corrupt")
	must(t, err)

	validLine, err := json.Marshal(BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-2", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: t.TempDir(),
		GatePassed: true, DiffStats: fakeDiffStatter{},
	}))
	must(t, err)

	corrupt := "{not valid json at all}\n"
	content := corrupt + string(validLine) + "\n"
	must(t, os.WriteFile(path, []byte(content), 0o644))

	rec := BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-3", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: t.TempDir(),
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})
	appendErr := AppendStageAttempt(stateDir, "epic-corrupt", rec)
	if appendErr == nil {
		t.Fatal("expected mid-file corruption to fail loud, not be silently repaired")
	}
	if !strings.Contains(appendErr.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the marker, got: %v", appendErr)
	}

	raw, err := os.ReadFile(path)
	must(t, err)
	if string(raw) != content {
		t.Error("the file must be left exactly as found when the append itself is refused")
	}
}

// --- parseLedgerBytes: the pure parsing/repair logic, tested directly. ---

func TestParseLedgerBytes_EmptyFile(t *testing.T) {
	records, validSize, err := parseLedgerBytes("ledger", []byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 || validSize != 0 {
		t.Errorf("records=%v validSize=%d, want empty/0", records, validSize)
	}
}

func TestParseLedgerBytes_AllValidLines(t *testing.T) {
	data := []byte(`{"beadId":"a"}` + "\n" + `{"beadId":"b"}` + "\n")
	records, validSize, err := parseLedgerBytes("ledger", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if validSize != int64(len(data)) {
		t.Errorf("validSize = %d, want %d (the whole file is valid)", validSize, len(data))
	}
}

func TestParseLedgerBytes_TrailingFragmentWithoutNewlineIsDropped(t *testing.T) {
	valid := `{"beadId":"a"}` + "\n"
	data := []byte(valid + `{"beadId":"b"` /* no closing brace, no newline */)
	records, validSize, err := parseLedgerBytes("ledger", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected only the newline-terminated row, got %d records", len(records))
	}
	if validSize != int64(len(valid)) {
		t.Errorf("validSize = %d, want %d (stop right before the dangling fragment)", validSize, len(valid))
	}
}

func TestParseLedgerBytes_MidFileCorruptionIsAHardError(t *testing.T) {
	data := []byte("{not json}\n" + `{"beadId":"a"}` + "\n")
	_, _, err := parseLedgerBytes("ledger", data)
	if err == nil {
		t.Fatal("expected an error: the corrupt row is not the trailing one")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the marker, got: %v", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLastGateFailure_ReturnsTheReasonOfTheLastFailedAttemptAtThatStage(t *testing.T) {
	dir := t.TempDir()
	reason := `decision_record_invalid_json: invalid character 'd' in string escape code on line 6`

	writeAttempt := func(stage string, passed bool, why string) {
		rec := StageAttemptRecord{BeadID: "kb-1", Stage: stage, GatePassed: passed}
		if why != "" {
			r := why
			rec.GateFailureReason = &r
		}
		if err := AppendStageAttempt(dir, "ep-1", rec); err != nil {
			t.Fatalf("AppendStageAttempt: %v", err)
		}
	}

	writeAttempt("implementation", true, "")
	writeAttempt("implementation", false, reason)

	if got := LastGateFailure(dir, "ep-1", "kb-1", "implementation"); got != reason {
		t.Errorf("LastGateFailure = %q, want the last failure's reason", got)
	}
}

// A review rejection is recorded against the REVIEW stage, and the prompt
// already carries it as RejectedReview. Scoping by stage is what keeps this
// section from repeating it.
func TestLastGateFailure_IgnoresAnotherStagesFailure(t *testing.T) {
	dir := t.TempDir()
	rejected := "verdict_reject: /artifacts/implementation-review.md"

	rec := StageAttemptRecord{BeadID: "kb-1", Stage: "implementation_review", GatePassed: false, GateFailureReason: &rejected}
	if err := AppendStageAttempt(dir, "ep-1", rec); err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}

	if got := LastGateFailure(dir, "ep-1", "kb-1", "implementation"); got != "" {
		t.Errorf("LastGateFailure = %q, want empty - that failure belongs to the review stage", got)
	}
}

// The most recent attempt is what counts. A bead that failed, was fixed, and
// is running again for another reason must not be told about a defect it has
// already corrected.
func TestLastGateFailure_SilentWhenTheLastAttemptPassed(t *testing.T) {
	dir := t.TempDir()
	old := "commit_marker_missing: implementation"

	failed := StageAttemptRecord{BeadID: "kb-1", Stage: "implementation", GatePassed: false, GateFailureReason: &old}
	if err := AppendStageAttempt(dir, "ep-1", failed); err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}
	passed := StageAttemptRecord{BeadID: "kb-1", Stage: "implementation", GatePassed: true}
	if err := AppendStageAttempt(dir, "ep-1", passed); err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}

	if got := LastGateFailure(dir, "ep-1", "kb-1", "implementation"); got != "" {
		t.Errorf("LastGateFailure = %q, want empty - the last attempt passed", got)
	}
}

func TestLastGateFailure_SilentWhenNothingRecorded(t *testing.T) {
	if got := LastGateFailure(t.TempDir(), "ep-1", "kb-1", "implementation"); got != "" {
		t.Errorf("LastGateFailure = %q, want empty for a bead with no attempts", got)
	}
}

// Another bead's failure is not this bead's.
func TestLastGateFailure_ScopedToTheBead(t *testing.T) {
	dir := t.TempDir()
	other := "commit_marker_missing: implementation"

	rec := StageAttemptRecord{BeadID: "kb-2", Stage: "implementation", GatePassed: false, GateFailureReason: &other}
	if err := AppendStageAttempt(dir, "ep-1", rec); err != nil {
		t.Fatalf("AppendStageAttempt: %v", err)
	}

	if got := LastGateFailure(dir, "ep-1", "kb-1", "implementation"); got != "" {
		t.Errorf("LastGateFailure = %q, want empty - that failure belongs to kb-2", got)
	}
}

// verdict_not_pass is not a rejection: it is a review that produced no usable
// verdict (missing artifact, truncated document, a reviewer out of budget).
// Nothing is sent back to an implementer for it, so the attempt that follows
// is not rework and must not be marked as caused by a rejection. This is the
// exact confusion the field carried until the rework rollup needed it: it
// matched this reason and never the one a real rejection emits.
func TestAppendStageAttempt_VerdictNotPassIsNotARejection(t *testing.T) {
	stateDir := t.TempDir()
	worktree := t.TempDir()

	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "reviewer", Dialect: "claude", BeadID: "bead-1", Stage: "implementation_review",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: false, DiffStats: fakeDiffStatter{},
		GateFailureReason: "verdict_not_pass: " + filepath.Join(stateDir, "review.md"),
	})))
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})))

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-1", "attempts.jsonl"))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[1].CausedBy != nil {
		t.Errorf("CausedBy = %q, want nil - a verdict gate that produced no verdict never sent work back", *lines[1].CausedBy)
	}
}

// A review stage re-running straight after its own rejection is a second
// opinion on unchanged work, not a redo of it. Marking it charged the rework
// to the reviewer - the exact attribution the field exists to avoid. Observed
// in a real ledger, where such a re-run rejected and then passed the same code
// with no implementation between the two.
func TestAppendStageAttempt_AReviewRerunAfterItsOwnRejectionIsNotRework(t *testing.T) {
	stateDir := t.TempDir()
	worktree := t.TempDir()
	reject := func() {
		verdict := "REJECT"
		must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
			AgentID: "reviewer", Dialect: "claude", BeadID: "bead-1", Stage: "implementation_review",
			StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
			GatePassed: false, DiffStats: fakeDiffStatter{},
			GateFailureReason: "verdict_reject: " + filepath.Join(stateDir, "implementation-review.md"),
			ReviewVerdict:     &verdict,
		})))
	}

	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})))
	reject()
	reject()

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-1", "attempts.jsonl"))
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[2].CausedBy != nil {
		t.Errorf("CausedBy = %q, want nil - the reviewer re-reviewed, it did not redo the work", *lines[2].CausedBy)
	}
}

// The stage that follows a rejection is not always a retake. An epic whose
// integration_review rejected has been seen recording a shipment attempt
// next, and a stage running for the first time cannot be redoing anything.
func TestAppendStageAttempt_AStageRunningForTheFirstTimeIsNotRework(t *testing.T) {
	stateDir := t.TempDir()
	worktree := t.TempDir()
	verdict := "REJECT"

	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "epic-1", Stage: "integration",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})))
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "reviewer", Dialect: "claude", BeadID: "epic-1", Stage: "integration_review",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: false, DiffStats: fakeDiffStatter{},
		GateFailureReason: "verdict_reject: " + filepath.Join(stateDir, "integration-review.md"),
		ReviewVerdict:     &verdict,
	})))
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "shipper", Dialect: "claude", BeadID: "epic-1", Stage: "shipment",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})))

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-1", "attempts.jsonl"))
	if lines[2].CausedBy != nil {
		t.Errorf("CausedBy = %q, want nil - shipment had never run, so it is redoing nothing", *lines[2].CausedBy)
	}

	// The stage that WAS sent back still counts: integration running a second
	// time after its own review rejected is the rework here.
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "epic-1", Stage: "integration_review",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: false, DiffStats: fakeDiffStatter{},
		GateFailureReason: "verdict_reject: " + filepath.Join(stateDir, "integration-review.md"),
		ReviewVerdict:     &verdict,
	})))
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "epic-1", Stage: "integration",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})))
	lines = readLedgerLines(t, filepath.Join(stateDir, "run", "epic-1", "attempts.jsonl"))
	last := lines[len(lines)-1]
	if last.CausedBy == nil {
		t.Error("integration re-running after its review rejected is rework and must be marked")
	}
}

// The driver knows what a dispatch is answering: it is the code that decided
// to rewind. When it says so, the writer records that and does not go looking
// through the rows for an answer it was already given.
func TestAppendStageAttempt_ADeclaredCauseWinsOverInference(t *testing.T) {
	stateDir := t.TempDir()
	worktree := t.TempDir()
	declared := "/artifacts/the-review-that-rejected.md"

	// Nothing here is inferable as rework: a single first attempt, no prior
	// rejection anywhere. Inference would answer nil.
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: true, DiffStats: fakeDiffStatter{},
		CausedBy: declared,
	})))

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-1", "attempts.jsonl"))
	if lines[0].CausedBy == nil || *lines[0].CausedBy != declared {
		t.Errorf("CausedBy = %v, want the declared %q", lines[0].CausedBy, declared)
	}
}

// And an undeclared attempt still gets the inferred answer, which is what
// keeps a rewind and a retake recorded by different processes connected.
func TestAppendStageAttempt_InferenceStillRunsWhenNothingWasDeclared(t *testing.T) {
	stateDir := t.TempDir()
	worktree := t.TempDir()
	artifact := filepath.Join(stateDir, "implementation-review.md")
	verdict := "REJECT"

	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})))
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "reviewer", Dialect: "claude", BeadID: "bead-1", Stage: "implementation_review",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: false, DiffStats: fakeDiffStatter{},
		GateFailureReason: "verdict_reject: " + artifact,
		ReviewVerdict:     &verdict,
	})))
	// A different process drives the retake, so it declares nothing.
	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})))

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-1", "attempts.jsonl"))
	if lines[2].CausedBy == nil || *lines[2].CausedBy != artifact {
		t.Errorf("CausedBy = %v, want the inferred %q - a retake in another process is still rework", lines[2].CausedBy, artifact)
	}
}

// TestAppendStageAttempt_GateEvidenceRoundTrips proves the evidence a
// failed commit_marker gate collected survives the ledger write/read round
// trip - the whole point of wiring it through: a row that used to say only
// "commit_marker_missing: implementation" now also carries what the gate
// saw when it made that call.
func TestAppendStageAttempt_GateEvidenceRoundTrips(t *testing.T) {
	stateDir := t.TempDir()
	worktree := t.TempDir()

	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: false, GateFailureReason: "commit_marker_missing: implementation", DiffStats: fakeDiffStatter{},
		GateEvidence: backend.GateEvidence{
			GateType:     "commit_marker",
			BaseSHA:      "base-sha",
			HeadSHA:      "head-sha",
			WorktreePath: worktree,
		},
	})))

	lines := readLedgerLines(t, filepath.Join(stateDir, "run", "epic-1", "attempts.jsonl"))
	got := lines[0].GateEvidence
	if got == nil {
		t.Fatal("GateEvidence is nil, want the evidence the gate collected")
	}
	if got.GateType != "commit_marker" || got.BaseSHA != "base-sha" || got.HeadSHA != "head-sha" || got.WorktreePath != worktree {
		t.Errorf("GateEvidence = %+v, want gateType=commit_marker baseSHA=base-sha headSHA=head-sha worktreePath=%s", got, worktree)
	}
}

// TestAppendStageAttempt_NoGateEvidenceStaysNil proves a normal attempt -
// one that never populated GateEvidence, exactly what every attempt looked
// like before this bead - leaves the field nil rather than writing an
// empty struct, so the line carries no "gateEvidence" key at all.
func TestAppendStageAttempt_NoGateEvidenceStaysNil(t *testing.T) {
	stateDir := t.TempDir()
	worktree := t.TempDir()

	must(t, AppendStageAttempt(stateDir, "epic-1", BuildStageAttemptRecord(StageAttemptInput{
		AgentID: "claude", Dialect: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, Worktree: worktree,
		GatePassed: true, DiffStats: fakeDiffStatter{},
	})))

	path := filepath.Join(stateDir, "run", "epic-1", "attempts.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ledger: %v", err)
	}
	if strings.Contains(string(data), "gateEvidence") {
		t.Errorf("line carries a gateEvidence key with no evidence collected: %s", data)
	}

	lines := readLedgerLines(t, path)
	if lines[0].GateEvidence != nil {
		t.Errorf("GateEvidence = %+v, want nil", lines[0].GateEvidence)
	}
}

// TestStageAttemptRecord_OldLedgerLineWithoutGateEvidenceDecodes proves a
// ledger line written before this bead - no "gateEvidence" key at all -
// still decodes cleanly, with GateEvidence left nil rather than failing to
// unmarshal.
func TestStageAttemptRecord_OldLedgerLineWithoutGateEvidenceDecodes(t *testing.T) {
	oldLine := `{"agentId":"claude","dialect":"claude","beadId":"bead-1","stage":"implementation","gatePassed":false}`
	var rec StageAttemptRecord
	if err := json.Unmarshal([]byte(oldLine), &rec); err != nil {
		t.Fatalf("unmarshal pre-existing ledger line: %v", err)
	}
	if rec.GateEvidence != nil {
		t.Errorf("GateEvidence = %+v, want nil for a line that never had the field", rec.GateEvidence)
	}
}
