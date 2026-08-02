package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// rewindOnlyBackend implements BackendPort by embedding a nil one: every
// method except Rewind panics if called. That is the assertion, not an
// oversight - a rewind must touch the tracker exactly once, and a second call
// creeping in here would be caught rather than silently tolerated.
type rewindOnlyBackend struct {
	backend.BackendPort
	calls   int
	id      string
	target  string
	reason  string
	failErr error
}

func (b *rewindOnlyBackend) Rewind(id, targetState, reason, _ string) error {
	b.calls++
	b.id, b.target, b.reason = id, targetState, reason
	return b.failErr
}

func workerWorkflow() backend.WorkflowDescriptor {
	return backend.WorkflowDescriptor{ID: "worker", RetakeState: "ready_for_implementation"}
}

func TestIsDeliberateRejection(t *testing.T) {
	cases := []struct {
		state, reason string
		want          bool
	}{
		{"implementation_review", "verdict_reject: /tmp/review.md", true},
		// Every other way the verdict gate fails is a stage that did not
		// finish, not a reviewer saying no.
		{"implementation_review", "verdict_not_pass: /tmp/review.md", false},
		{"implementation_review", "artifact_missing: /tmp/review.md", false},
		// integration_review has its own path (the fix-up bead) and must not
		// be diverted into this one.
		{"integration_review", "verdict_reject: /tmp/review.md", false},
		{"implementation", "verdict_reject: /tmp/review.md", false},
	}
	for _, c := range cases {
		if got := isDeliberateRejection(c.state, c.reason); got != c.want {
			t.Errorf("isDeliberateRejection(%q, %q) = %v, want %v", c.state, c.reason, got, c.want)
		}
	}
}

func TestRewindAfterReviewRejection_SendsTheBeadBack(t *testing.T) {
	be := &rewindOnlyBackend{}
	deps := DriveBeadDeps{BeadID: "arch-tws", RepoPath: "/repos/archeion", Backend: be}

	rewound, err := rewindAfterReviewRejection(deps, workerWorkflow(), "verdict_reject: /tmp/review.md", 0)
	if err != nil {
		t.Fatalf("rewindAfterReviewRejection: %v", err)
	}
	if !rewound {
		t.Fatal("rewound = false, want the bead sent back")
	}
	if be.calls != 1 {
		t.Errorf("Rewind called %d times, want 1", be.calls)
	}
	if be.target != "ready_for_implementation" {
		t.Errorf("target = %q, want the workflow's retake state", be.target)
	}
	// The reason is what a human reads in the tracker months later; it has
	// to say what happened, not only that something did.
	for _, want := range []string{"implementation_review", "ready_for_implementation"} {
		if !strings.Contains(be.reason, want) {
			t.Errorf("reason %q does not mention %q", be.reason, want)
		}
	}
}

// One rewind, then the bead blocks. A reviewer rejecting the same work twice
// is describing something another attempt will not fix.
func TestRewindAfterReviewRejection_BudgetIsSpentAfterOne(t *testing.T) {
	be := &rewindOnlyBackend{}
	deps := DriveBeadDeps{BeadID: "arch-tws", RepoPath: "/repos/archeion", Backend: be}

	rewound, err := rewindAfterReviewRejection(deps, workerWorkflow(), "verdict_reject: /tmp/review.md", implementationReviewRewindLimit)
	if err != nil {
		t.Fatalf("rewindAfterReviewRejection: %v", err)
	}
	if rewound {
		t.Error("rewound = true, want the exhausted budget to fall through to blocking")
	}
	if be.calls != 0 {
		t.Errorf("Rewind called %d times, want 0 - an exhausted budget must not touch the tracker", be.calls)
	}
}

// A workflow with no retake state has nowhere to send the work; so does one
// whose retake state is the reviewing stage itself, which would re-review the
// same unchanged work forever.
func TestRewindAfterReviewRejection_NowhereToSendItBlocks(t *testing.T) {
	for _, retake := range []string{"", "   ", "implementation_review"} {
		be := &rewindOnlyBackend{}
		deps := DriveBeadDeps{BeadID: "arch-tws", RepoPath: "/repos/archeion", Backend: be}
		wf := backend.WorkflowDescriptor{ID: "odd", RetakeState: retake}

		rewound, err := rewindAfterReviewRejection(deps, wf, "verdict_reject: /tmp/review.md", 0)
		if err != nil {
			t.Fatalf("retake %q: %v", retake, err)
		}
		if rewound {
			t.Errorf("retake %q: rewound = true, want a fall-through to blocking", retake)
		}
		if be.calls != 0 {
			t.Errorf("retake %q: Rewind called %d times, want 0", retake, be.calls)
		}
	}
}

// A tracker that refuses the rewind must be an error, not a quiet block: the
// bead would otherwise sit at implementation_review while the run reported it
// had been sent back.
func TestRewindAfterReviewRejection_TrackerFailureIsLoud(t *testing.T) {
	be := &rewindOnlyBackend{failErr: errors.New("br: database is locked")}
	deps := DriveBeadDeps{BeadID: "arch-tws", RepoPath: "/repos/archeion", Backend: be}

	rewound, err := rewindAfterReviewRejection(deps, workerWorkflow(), "verdict_reject: /tmp/review.md", 0)
	if err == nil {
		t.Fatal("a refused rewind returned no error")
	}
	if rewound {
		t.Error("rewound = true alongside an error")
	}
	for _, want := range []string{"KERNL DISPATCH FAILURE", "arch-tws", "database is locked", "Fix:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestReviewRejectionCanBeRewound proves the shared predicate finding 4 of
// the fork/decision-gate hardening pass extracted: handleReviewRaisedDecision
// (review_decision_gate.go) calls this exact function BEFORE ever consulting
// the DA, so it must agree, in every case, with what
// rewindAfterReviewRejection itself would do - the two are built from the
// same two booleans precisely so they cannot drift apart.
func TestReviewRejectionCanBeRewound(t *testing.T) {
	cases := []struct {
		name        string
		wf          backend.WorkflowDescriptor
		rewindsUsed int
		want        bool
	}{
		{"budget spent", workerWorkflow(), implementationReviewRewindLimit, false},
		{"no retake state", backend.WorkflowDescriptor{ID: "odd", RetakeState: ""}, 0, false},
		{"retake is the reviewing stage itself", backend.WorkflowDescriptor{ID: "odd", RetakeState: "implementation_review"}, 0, false},
		{"budget available and a real retake state", workerWorkflow(), 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reviewRejectionCanBeRewound(c.wf, c.rewindsUsed); got != c.want {
				t.Errorf("reviewRejectionCanBeRewound(%+v, %d) = %v, want %v", c.wf, c.rewindsUsed, got, c.want)
			}
		})
	}
}

func TestReadRejectedReview(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	rejected := write("rejected.md", "the error handling is missing\n\nVERDICT: REJECT")
	if got := readRejectedReview(rejected); !strings.Contains(got, "the error handling is missing") {
		t.Errorf("a rejecting review returned %q, want its full text", got)
	}

	// A review that passed has nothing for an implementer to answer.
	passed := write("passed.md", "looks right\n\nVERDICT: PASS")
	if got := readRejectedReview(passed); got != "" {
		t.Errorf("a passing review returned %q, want empty", got)
	}

	// Same guard the exit gate itself applies: a trailing substring is not a
	// verdict line, or "NOT A VALID VERDICT: REJECT" would send work back.
	fake := write("fake.md", "NOT A VALID VERDICT: REJECT")
	if got := readRejectedReview(fake); got != "" {
		t.Errorf("a non-verdict line returned %q, want empty", got)
	}

	if got := readRejectedReview(filepath.Join(dir, "absent.md")); got != "" {
		t.Errorf("a missing review returned %q, want empty", got)
	}
	if got := readRejectedReview(""); got != "" {
		t.Errorf("an empty path returned %q, want empty", got)
	}
}

func TestRenderRejectedReview_SilentWhenThereIsNoRejection(t *testing.T) {
	var b strings.Builder
	renderRejectedReview(&b, "")
	renderRejectedReview(&b, "   \n ")
	if b.Len() != 0 {
		t.Errorf("rendered %q, want nothing - a first attempt has no rejection to report", b.String())
	}
}

// The rejection has to come before the bead's own description: an implementer
// who reads the description first starts by re-deriving work that already
// exists, and meets the objection only after choosing how to proceed.
func TestBuildBeadStagePrompt_RejectionComesFirst(t *testing.T) {
	prompt := BuildBeadStagePrompt(StagePromptInput{
		Bead:           &backend.Bead{ID: "arch-tws", Title: "wildcard robots rule", Description: "the crawl ignores it"},
		State:          "implementation",
		RepoPath:       "/repos/archeion",
		Worktree:       "/worktrees/arch-tws",
		VerifyCommand:  "bin/ci",
		TrackerCommand: "br --db /repos/archeion/.beads/beads.db",
		RejectedReview: "the wildcard is matched as a literal\n\nVERDICT: REJECT",
	})

	rejection := strings.Index(prompt, "the wildcard is matched as a literal")
	description := strings.Index(prompt, "the crawl ignores it")
	if rejection < 0 {
		t.Fatal("the rejection text is absent from the implementer's prompt")
	}
	if description >= 0 && rejection > description {
		t.Error("the rejection appears after the bead description, want it first")
	}
	if !strings.Contains(prompt, "not implementing this bead from scratch") {
		t.Error("the prompt does not tell the implementer that an implementation already exists")
	}
}
