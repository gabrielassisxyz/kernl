package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// composeOneRun drives a single run over epicID and returns the path its own
// report landed at, which is what ComposeRunReport still returns.
func composeOneRun(t *testing.T, g *graph.Graph, stateDir, epicID, beadID, prURL string) string {
	t.Helper()
	runID := seedRunWithBeads(t, g, "epic", []BeadRef{
		{ID: beadID, Title: "epic", TrackerKind: "br", RepoPath: "/repo"},
	})
	in := baseReportInput(g, stateDir, runID, epicID)
	in.PRURL = prURL
	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}
	return path
}

func readIndex(t *testing.T, stateDir, epicID string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(stateDir, "run", epicID, "report.md"))
	if err != nil {
		t.Fatalf("reading the index: %v", err)
	}
	return string(raw)
}

// The defect this file exists for: an epic driven across several
// invocations kept only the last run's report, because every run rewrote
// one file per epic. The run that happened to finish last was often the one
// that recorded the least, so the decisions the earlier invocations made
// were simply gone.
func TestComposeRunReport_TwoRunsOverOneEpicBothSurvive(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	first := composeOneRun(t, g, stateDir, "kb-epic-1", "kb-epic-1", "")
	second := composeOneRun(t, g, stateDir, "kb-epic-1", "kb-epic-1", "")

	if first == second {
		t.Fatalf("both runs wrote the same path %q, so the second overwrote the first", first)
	}
	for _, p := range []string{first, second} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("report %q did not survive: %v", p, err)
		}
	}
}

// report.md keeps the path an operator already reaches for, and becomes the
// one artifact that tells the epic's story across runs: newest first, so the
// most recent run is the first thing read.
func TestComposeRunReport_IndexListsEveryRunNewestFirst(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	firstPath := composeOneRun(t, g, stateDir, "kb-epic-2", "kb-epic-2", "")
	secondPath := composeOneRun(t, g, stateDir, "kb-epic-2", "kb-epic-2", "")

	index := readIndex(t, stateDir, "kb-epic-2")
	firstRun := strings.TrimSuffix(filepath.Base(firstPath), ".md")
	secondRun := strings.TrimSuffix(filepath.Base(secondPath), ".md")

	firstAt := strings.Index(index, firstRun)
	secondAt := strings.Index(index, secondRun)
	if firstAt < 0 || secondAt < 0 {
		t.Fatalf("index must list both runs, got:\n%s", index)
	}
	if secondAt > firstAt {
		t.Errorf("the newer run must come first, got:\n%s", index)
	}
}

// The pull request URL is the one thing still waiting on a person once an
// epic reaches awaiting_pr_review, so the index carries it. A run with no
// pull request must not print a dangling label instead.
func TestComposeRunReport_IndexCarriesThePRURLOnlyWhenThereIsOne(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	const prURL = "https://github.com/gabrielassisxyz/clarity-data-workflow/pull/30"
	composeOneRun(t, g, stateDir, "kb-epic-3", "kb-epic-3", prURL)
	if index := readIndex(t, stateDir, "kb-epic-3"); !strings.Contains(index, prURL) {
		t.Errorf("index must carry the pull request URL, got:\n%s", index)
	}

	otherDir := t.TempDir()
	composeOneRun(t, g, otherDir, "kb-epic-4", "kb-epic-4", "")
	if index := readIndex(t, otherDir, "kb-epic-4"); strings.Contains(index, "PR:") {
		t.Errorf("a run with no pull request must not print a PR column, got:\n%s", index)
	}
}

// An epic swept before this change has a report.md that IS a run's report,
// not an index. Rewriting it in place would destroy the very artifact the
// change exists to stop losing, so it is moved aside and pointed at.
func TestComposeRunReport_ALegacyReportIsArchivedNotOverwritten(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epicDir := filepath.Join(stateDir, "run", "kb-epic-5")
	if err := os.MkdirAll(epicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const legacy = "# Run Report: an-older-run\n\nthe decisions an earlier invocation recorded\n"
	if err := os.WriteFile(filepath.Join(epicDir, "report.md"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	composeOneRun(t, g, stateDir, "kb-epic-5", "kb-epic-5", "")

	archived, err := os.ReadFile(filepath.Join(epicDir, "runs", "legacy.md"))
	if err != nil {
		t.Fatalf("the legacy report must be preserved: %v", err)
	}
	if string(archived) != legacy {
		t.Errorf("the legacy report was altered:\n%s", archived)
	}
	if index := readIndex(t, stateDir, "kb-epic-5"); !strings.Contains(index, "runs/legacy.md") {
		t.Errorf("the index must point at the archived report, got:\n%s", index)
	}
}

// Archiving happens once. A second run must not mistake the index it is
// itself maintaining for another legacy file and keep moving it aside.
func TestComposeRunReport_LegacyIsArchivedOnlyOnce(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epicDir := filepath.Join(stateDir, "run", "kb-epic-6")
	if err := os.MkdirAll(epicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(epicDir, "report.md"), []byte("# Run Report: old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	composeOneRun(t, g, stateDir, "kb-epic-6", "kb-epic-6", "")
	composeOneRun(t, g, stateDir, "kb-epic-6", "kb-epic-6", "")

	if _, err := os.Stat(filepath.Join(epicDir, "runs", "legacy-1.md")); !os.IsNotExist(err) {
		t.Errorf("the index must not be archived as a second legacy file (err=%v)", err)
	}
}

// ComposeRunReport's return value still points at the run's own report, not
// at the index: callers print it for the operator, and pointing them at a
// one-line summary instead of the report they just produced would be a
// silent regression.
func TestComposeRunReport_StillReturnsThisRunsOwnReport(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	path := composeOneRun(t, g, stateDir, "kb-epic-7", "kb-epic-7", "")

	if filepath.Base(path) == "report.md" {
		t.Fatalf("the returned path must be the run's own report, got %q", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.HasPrefix(string(body), "# Run Report: ") {
		t.Errorf("the returned path must hold the run's report, got:\n%s", body)
	}
}
