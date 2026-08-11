package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sequenceHeadSHAResolver answers a different SHA on every call, which is
// what makes "the epoch was reused" distinguishable from "HEAD happened not
// to move" - a fixed fake would let a resolver that re-reads HEAD on every
// dispatch pass these tests unchanged.
type sequenceHeadSHAResolver struct {
	shas  []string
	calls int
}

func (r *sequenceHeadSHAResolver) HeadSHA(string) string {
	if r.calls >= len(r.shas) {
		r.calls++
		return "exhausted"
	}
	sha := r.shas[r.calls]
	r.calls++
	return sha
}

func TestResolveStageEpochBase_ReusesTheBaseWithinOneStageEntry(t *testing.T) {
	dir := t.TempDir()
	head := &sequenceHeadSHAResolver{shas: []string{"aaaa111", "bbbb222"}}

	first, err := resolveStageEpochBase(dir, "implementation", "/wt", true, head)
	if err != nil {
		t.Fatalf("first entry: %v", err)
	}
	if first != "aaaa111" {
		t.Fatalf("a stage entry captures HEAD; got %q", first)
	}

	// The second dispatch of the same entry - a mechanical-block resume, a
	// fork handover coming back - must measure against the same base even
	// though HEAD has moved, because it moved by the stage's OWN commit.
	second, err := resolveStageEpochBase(dir, "implementation", "/wt", false, head)
	if err != nil {
		t.Fatalf("second dispatch: %v", err)
	}
	if second != "aaaa111" {
		t.Fatalf("a re-entered stage must reuse its epoch base; got %q", second)
	}
	if head.calls != 1 {
		t.Fatalf("the reused epoch must not re-read HEAD at all; HeadSHA called %d times", head.calls)
	}
}

func TestResolveStageEpochBase_AFreshClaimReplacesTheEpoch(t *testing.T) {
	dir := t.TempDir()
	head := &sequenceHeadSHAResolver{shas: []string{"aaaa111", "bbbb222"}}

	if _, err := resolveStageEpochBase(dir, "implementation", "/wt", true, head); err != nil {
		t.Fatalf("first entry: %v", err)
	}
	// The same state name reached by a genuine claim is a new stage entry,
	// not a continuation: work committed before it belongs to whatever ran
	// last, and must not satisfy this entry's gate.
	again, err := resolveStageEpochBase(dir, "implementation", "/wt", true, head)
	if err != nil {
		t.Fatalf("second entry: %v", err)
	}
	if again != "bbbb222" {
		t.Fatalf("a fresh claim must capture HEAD again; got %q", again)
	}

	rec, found, err := readStageEpoch(dir)
	if err != nil || !found {
		t.Fatalf("the replacement must be persisted: found=%v err=%v", found, err)
	}
	if rec.BaseSHA != "bbbb222" {
		t.Fatalf("persisted base = %q, want the freshly captured one", rec.BaseSHA)
	}
}

func TestResolveStageEpochBase_ADifferentStateReplacesTheEpoch(t *testing.T) {
	dir := t.TempDir()
	head := &sequenceHeadSHAResolver{shas: []string{"aaaa111", "bbbb222"}}

	if _, err := resolveStageEpochBase(dir, "implementation", "/wt", true, head); err != nil {
		t.Fatalf("implementation entry: %v", err)
	}
	// A review stage that follows is a different entry even though nothing
	// claimed - and it is what makes a later rewind BACK to implementation
	// start over rather than inherit the implementer's old base.
	review, err := resolveStageEpochBase(dir, "implementation_review", "/wt", false, head)
	if err != nil {
		t.Fatalf("review entry: %v", err)
	}
	if review != "bbbb222" {
		t.Fatalf("a different state must capture its own base; got %q", review)
	}
}

func TestResolveStageEpochBase_ADifferentWorktreeReplacesTheEpoch(t *testing.T) {
	dir := t.TempDir()
	head := &sequenceHeadSHAResolver{shas: []string{"aaaa111", "bbbb222"}}

	if _, err := resolveStageEpochBase(dir, "implementation", "/wt-old", true, head); err != nil {
		t.Fatalf("first entry: %v", err)
	}
	// A worktree recreated under the bead need not carry that SHA at all,
	// and asking the gate about history that is not there fails in a way
	// that reads as the agent's fault.
	moved, err := resolveStageEpochBase(dir, "implementation", "/wt-new", false, head)
	if err != nil {
		t.Fatalf("moved worktree: %v", err)
	}
	if moved != "bbbb222" {
		t.Fatalf("a different worktree must capture its own base; got %q", moved)
	}
}

func TestResolveStageEpochBase_AnUnreadableHeadIsNotPersisted(t *testing.T) {
	dir := t.TempDir()
	head := &sequenceHeadSHAResolver{shas: []string{"", "cccc333"}}

	base, err := resolveStageEpochBase(dir, "implementation", "/wt", true, head)
	if err != nil {
		t.Fatalf("unreadable head: %v", err)
	}
	if base != "" {
		t.Fatalf("an unreadable HEAD has no base to report; got %q", base)
	}
	if _, found, err := readStageEpoch(dir); found || err != nil {
		t.Fatalf("an empty base must not be persisted: found=%v err=%v", found, err)
	}

	// Because nothing was pinned, the next dispatch is free to capture a
	// real base instead of inheriting a permanent commit_marker_unscoped.
	recovered, err := resolveStageEpochBase(dir, "implementation", "/wt", false, head)
	if err != nil {
		t.Fatalf("recovered dispatch: %v", err)
	}
	if recovered != "cccc333" {
		t.Fatalf("the next dispatch must capture a real base; got %q", recovered)
	}
}

func TestResolveStageEpochBase_MalformedRecordFailsLoud(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, stageEpochFile)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	head := &sequenceHeadSHAResolver{shas: []string{"aaaa111"}}

	// Recapturing silently is the exact behaviour the epoch exists to
	// remove, and it would surface three dispatches later as an
	// unexplained commit_marker_missing.
	_, err := resolveStageEpochBase(dir, "implementation", "/wt", false, head)
	if err == nil {
		t.Fatal("a malformed stage epoch must stop the dispatch, not be recaptured silently")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("the error must name the file to delete; got %v", err)
	}
	if head.calls != 0 {
		t.Fatalf("nothing should have been captured; HeadSHA called %d times", head.calls)
	}
}
