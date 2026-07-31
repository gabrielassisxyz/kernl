package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// seedRunAtRepo is seedRunWithBeads (run_report_test.go) with the
// repository made a parameter instead of the hardcoded "/repo": this file's
// own tests need two DISTINCT repositories to prove decisionScope's
// same-repository filter actually excludes a same-vocabulary decision that
// belongs to a different one, which the hardcoded helper cannot produce -
// StartWorkflowRun stamps every bead reference it creates with its own
// single RepoPath (see that function's own doc comment on refs[i]), so the
// repository has to be set correctly at THIS call, before writeTestDecision
// ever runs; a bead_reference row already created with one Repository value
// is never updated by a later call naming a different one
// (ensureBeadReferenceNode is insert-if-absent, not upsert).
func seedRunAtRepo(t *testing.T, g *graph.Graph, title, repoPath string, beads []BeadRef) string {
	t.Helper()
	runID, err := StartWorkflowRun(context.Background(), g, StartWorkflowRunInput{
		EntryPoint:     "epic run",
		Title:          title,
		WorkflowName:   "worker",
		Beads:          beads,
		RepoPath:       repoPath,
		TrackerCommand: "br --db '" + repoPath + "/.beads/beads.db'",
		StartedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	return runID
}

// pageCeilingDecisionRecord is a stand-in for the real defect this bead
// exists to close: bead arch-f40 named a crawl-stop enum variant
// PageCeilingReached and, in doing so, explicitly considered and rejected
// PageLimitReached - the exact name a second, unrelated bead (arch-hkk,
// fifteen days later, in the production incident) went on to pick anyway,
// because nothing carried the first decision forward into its prompt.
const pageCeilingDecisionRecord = "## Decision\n\n" +
	"Name the crawl-stop variant for a page-count bound.\n\n" +
	"## Options Considered\n\n" +
	"1. PageCeilingReached.\n2. PageLimitReached.\n\n" +
	"## Trade-offs\n\n" +
	"PageLimitReached reads fine in isolation, but a second word for a bound this project already calls a ceiling elsewhere makes the report harder to read, not easier.\n\n" +
	"## Rationale\n\n" +
	"PageCeilingReached is chosen over the more ordinary PageLimitReached because this project already calls a bound on a count a ceiling, in code and in prose, and a second word for the same idea makes the report harder to read than either word alone.\n"

// unrelatedDecisionRecord shares no vocabulary with the crawl-stop naming
// question - the decoy that proves the relevance filter is not just "every
// standing decision in the repository".
const unrelatedDecisionRecord = "## Decision\n\n" +
	"Use TOML for the export manifest.\n\n" +
	"## Options Considered\n\n" +
	"1. JSON.\n2. TOML.\n\n" +
	"## Trade-offs\n\n" +
	"TOML reads better hand-edited; JSON has wider tooling.\n\n" +
	"## Rationale\n\n" +
	"No existing precedent either way; TOML matches the config file already in the repo.\n"

// TestFetchRelevantDecisions_ReproducesRealCase is the reproduction the
// defect report asked for: two decisions, the second on a bead that would
// touch the same concept as the first, and the second bead's own fetch
// carries the first decision forward.
func TestFetchRelevantDecisions_ReproducesRealCase(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	firstBead := BeadRef{ID: "arch-f40", Title: "Add a page ceiling to the crawl stop enum", TrackerKind: "br", RepoPath: "/repo/archeion"}
	runID := seedRunAtRepo(t, g, "archeion crawler", "/repo/archeion", []BeadRef{firstBead})
	decisionID := writeTestDecision(t, g, runID, firstBead, firstBead, pageCeilingDecisionRecord)

	secondBead := &backend.Bead{
		ID:          "arch-hkk",
		Title:       "Enforce a page limit when the crawl loop stops",
		Description: "Add PageLimitReached handling wherever the crawl loop checks its page count.",
	}

	got, err := FetchRelevantDecisions(ctx, g, "/repo/archeion", secondBead)
	if err != nil {
		t.Fatalf("FetchRelevantDecisions: %v", err)
	}
	if len(got) != 1 || got[0].ID != decisionID {
		t.Fatalf("FetchRelevantDecisions = %+v, want exactly the arch-f40 decision (%s)", got, decisionID)
	}
	if !strings.Contains(got[0].Outcome, "PageLimitReached") {
		t.Errorf("Outcome = %q, want it to still name the rejected alternative", got[0].Outcome)
	}

	// The acceptance criterion is about the RENDERED PROMPT, not the
	// storage layer: build the second bead's stage prompt exactly as
	// DriveBeadToTerminal would and check the text an implementer actually
	// reads.
	prompt := BuildBeadStagePrompt(StagePromptInput{
		Bead:              secondBead,
		Stages:            map[string]backend.StageContract{},
		RelevantDecisions: got,
	})
	if !strings.Contains(prompt, "PageCeilingReached") {
		t.Errorf("rendered prompt does not carry the prior decision's title:\n%s", prompt)
	}
	if !strings.Contains(prompt, "PageLimitReached") {
		t.Errorf("rendered prompt does not carry the prior decision's outcome naming the rejected alternative:\n%s", prompt)
	}
}

// TestFetchRelevantDecisions_UnrelatedDecisionExcluded is the negative side
// of the same criterion: a standing decision in the same repository that
// shares no vocabulary with the bead about to run must not ride along -
// otherwise every prompt would carry every decision ever recorded.
func TestFetchRelevantDecisions_UnrelatedDecisionExcluded(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	manifestBead := BeadRef{ID: "arch-manifest", Title: "Pick an export manifest format", TrackerKind: "br", RepoPath: "/repo/archeion"}
	runID := seedRunAtRepo(t, g, "archeion crawler", "/repo/archeion", []BeadRef{manifestBead})
	writeTestDecision(t, g, runID, manifestBead, manifestBead, unrelatedDecisionRecord)

	secondBead := &backend.Bead{
		ID:          "arch-hkk",
		Title:       "Enforce a page limit when the crawl loop stops",
		Description: "Add PageLimitReached handling wherever the crawl loop checks its page count.",
	}

	got, err := FetchRelevantDecisions(ctx, g, "/repo/archeion", secondBead)
	if err != nil {
		t.Fatalf("FetchRelevantDecisions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FetchRelevantDecisions = %+v, want none: the manifest decision shares no vocabulary with this bead", got)
	}
}

// TestFetchRelevantDecisions_OtherRepositoryExcluded proves the repository
// scope is enforced even when the vocabulary matches exactly: the
// orchestrator serves more than one repository from a single graph, and a
// decision made in one repository is not a constraint on a same-named
// concept in a different one.
func TestFetchRelevantDecisions_OtherRepositoryExcluded(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	otherRepoBead := BeadRef{ID: "other-1", Title: "Add a page ceiling to the crawl stop enum", TrackerKind: "br", RepoPath: "/repo/a-different-project"}
	runID := seedRunAtRepo(t, g, "a different project", "/repo/a-different-project", []BeadRef{otherRepoBead})
	writeTestDecision(t, g, runID, otherRepoBead, otherRepoBead, pageCeilingDecisionRecord)

	secondBead := &backend.Bead{
		ID:          "arch-hkk",
		Title:       "Enforce a page limit when the crawl loop stops",
		Description: "Add PageLimitReached handling wherever the crawl loop checks its page count.",
	}

	got, err := FetchRelevantDecisions(ctx, g, "/repo/archeion", secondBead)
	if err != nil {
		t.Fatalf("FetchRelevantDecisions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FetchRelevantDecisions = %+v, want none: the matching decision belongs to a different repository", got)
	}
}

// TestFetchRelevantDecisions_RevertedDecisionExcluded: a reverted decision
// already has its own injection route - the constraint
// RevertDecisionAndReopenBead writes directly into the reverted bead's own
// description (see revert_decision.go). Surfacing it here too, on an
// unrelated bead with no context on why it was reverted, would be
// confusing rather than useful.
func TestFetchRelevantDecisions_RevertedDecisionExcluded(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	firstBead := BeadRef{ID: "arch-f40", Title: "Add a page ceiling to the crawl stop enum", TrackerKind: "br", RepoPath: "/repo/archeion"}
	runID := seedRunAtRepo(t, g, "archeion crawler", "/repo/archeion", []BeadRef{firstBead})
	decisionID := writeTestDecision(t, g, runID, firstBead, firstBead, pageCeilingDecisionRecord)

	if err := markDecisionReverted(ctx, g, decisionID, "the ceiling wording was confusing to operators after all"); err != nil {
		t.Fatalf("markDecisionReverted: %v", err)
	}

	secondBead := &backend.Bead{
		ID:          "arch-hkk",
		Title:       "Enforce a page limit when the crawl loop stops",
		Description: "Add PageLimitReached handling wherever the crawl loop checks its page count.",
	}

	got, err := FetchRelevantDecisions(ctx, g, "/repo/archeion", secondBead)
	if err != nil {
		t.Fatalf("FetchRelevantDecisions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FetchRelevantDecisions = %+v, want none: the only matching decision was reverted", got)
	}
}

// TestFetchRelevantDecisions_SameBeadExcluded: a decision the bead about to
// run already recorded itself (an earlier stage of the same bead) is not a
// prior answer it risks re-deriving - it is that bead's own recent work,
// already visible in its own worktree history.
func TestFetchRelevantDecisions_SameBeadExcluded(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	bead := BeadRef{ID: "arch-hkk", Title: "Enforce a page limit when the crawl loop stops", TrackerKind: "br", RepoPath: "/repo/archeion"}
	runID := seedRunAtRepo(t, g, "archeion crawler", "/repo/archeion", []BeadRef{bead})
	writeTestDecision(t, g, runID, bead, bead, pageCeilingDecisionRecord)

	sameBead := &backend.Bead{
		ID:          "arch-hkk",
		Title:       "Enforce a page limit when the crawl loop stops",
		Description: "Add PageLimitReached handling wherever the crawl loop checks its page count.",
	}

	got, err := FetchRelevantDecisions(ctx, g, "/repo/archeion", sameBead)
	if err != nil {
		t.Fatalf("FetchRelevantDecisions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FetchRelevantDecisions = %+v, want none: the only matching decision belongs to this same bead", got)
	}
}

// TestRenderRelatedDecisions_NoneFoundIsExplicit proves the "silence vs an
// explicit line" contract decision: an empty RelevantDecisions still
// renders a stated line, rather than omitting the section - so a reader of
// the prompt cannot mistake "nothing relevant" for "this never ran".
func TestRenderRelatedDecisions_NoneFoundIsExplicit(t *testing.T) {
	prompt := BuildBeadStagePrompt(StagePromptInput{
		Bead:   &backend.Bead{ID: "kb-1", Title: "unrelated bead"},
		Stages: map[string]backend.StageContract{},
	})
	if !strings.Contains(prompt, "## Related decisions already made") {
		t.Errorf("prompt is missing the related-decisions section entirely:\n%s", prompt)
	}
	if !strings.Contains(prompt, "No related decisions were found") {
		t.Errorf("prompt does not say explicitly that none were found:\n%s", prompt)
	}
}
