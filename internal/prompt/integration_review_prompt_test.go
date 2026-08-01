package prompt_test

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

func sampleIntegrationReviewInput() prompt.IntegrationReviewInput {
	return prompt.IntegrationReviewInput{
		EpicID:          "e1",
		EpicTitle:       "Test epic",
		EpicDescription: "Scope: RFC 9309 matching (user-agent groups, Allow and Disallow with longest-match precedence).",
		Children: []prompt.ReviewedChild{
			{ID: "e1.1", Title: "Parse user-agent groups", Acceptance: "All groups matching the agent are combined."},
			{ID: "e1.2", Title: "Longest-match precedence", Acceptance: "The longest matching rule wins."},
		},
		ArtifactPath:   "/home/user/.kernl/run/e1/e1/integration-review.md",
		VerifyCommand:  "bin/ci",
		TrackerCommand: "br --db /repo/.beads/beads.db",
	}
}

func TestRenderIntegrationReview_Content(t *testing.T) {
	out, err := prompt.RenderIntegrationReview(sampleIntegrationReviewInput())
	if err != nil {
		t.Fatal(err)
	}
	mustContain := []string{
		"VERDICT: PASS",
		"VERDICT: REJECT",
		"## Classification",
		"fixup",
		"decision",
		"## What is wrong",
		"## Fix-up acceptance criteria",
		"## Question for the operator",
		"can the acceptance criteria for correcting this be written without choosing anything",
		"/home/user/.kernl/run/e1/e1/integration-review.md",
		"bin/ci",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("integration_review prompt missing %q\n---\n%s\n---", want, out)
		}
	}
	// The reviewer must never be told it can fix what it finds - that is
	// the exit-gate contract (ForbiddenPaths), and the custom prompt
	// replaces the generic contract-rendered text entirely, so it has to
	// restate the rule itself.
	for _, want := range []string{"may NOT modify"} {
		if !strings.Contains(out, want) {
			t.Errorf("integration_review prompt missing %q", want)
		}
	}
}

// The reviewer that rejected an epic for a defect the epic never took on had
// no way to know what the epic was asked for: its prompt carried the merge and
// the diff, and nothing else. These are the two halves that fix that - the
// scope itself, and what the reviewer must do with a defect outside it.
func TestRenderIntegrationReview_CarriesTheEpicScope(t *testing.T) {
	out, err := prompt.RenderIntegrationReview(sampleIntegrationReviewInput())
	if err != nil {
		t.Fatal(err)
	}
	mustContain := []string{
		"Scope: RFC 9309 matching (user-agent groups, Allow and Disallow with longest-match precedence).",
		"e1.1",
		"Parse user-agent groups",
		"All groups matching the agent are combined.",
		"e1.2",
		"Longest-match precedence",
		"The longest matching rule wins.",
		"## Out-of-scope findings",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("integration_review prompt missing %q\n---\n%s\n---", want, out)
		}
	}
}

// Carrying the scope without saying what to do with a defect outside it fixes
// nothing: a reviewer that knows the scope and still rejects is the exact
// behavior this change exists to stop.
func TestRenderIntegrationReview_OutOfScopeDefectDoesNotReject(t *testing.T) {
	out, err := prompt.RenderIntegrationReview(sampleIntegrationReviewInput())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"does not reject",
		"VERDICT: PASS",
		"still REJECT",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("integration_review prompt missing %q\n---\n%s\n---", want, out)
		}
	}
}

// An epic with no description, or a child with no acceptance criteria, is a
// legitimate state of the tracker, not a reason to refuse the render. It is
// said out loud instead: a section that silently disappears is indistinguishable
// from this whole feature never having run.
func TestRenderIntegrationReview_SaysWhenScopeIsMissing(t *testing.T) {
	in := sampleIntegrationReviewInput()
	in.EpicDescription = "  "
	in.Children = []prompt.ReviewedChild{{ID: "e1.1", Title: "Parse user-agent groups"}}
	out, err := prompt.RenderIntegrationReview(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"records no description",
		"records no acceptance criteria",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("integration_review prompt missing %q\n---\n%s\n---", want, out)
		}
	}
}

func TestRenderIntegrationReview_RequiresArtifactPath(t *testing.T) {
	in := sampleIntegrationReviewInput()
	in.ArtifactPath = ""
	if _, err := prompt.RenderIntegrationReview(in); err == nil {
		t.Fatal("expected an error for a missing artifact path")
	} else if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("expected KERNL DISPATCH FAILURE, got %v", err)
	}
}

func TestRenderIntegrationReview_RequiresVerifyCommand(t *testing.T) {
	in := sampleIntegrationReviewInput()
	in.VerifyCommand = ""
	if _, err := prompt.RenderIntegrationReview(in); err == nil {
		t.Fatal("expected an error for a missing verify command")
	} else if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("expected KERNL DISPATCH FAILURE, got %v", err)
	}
}
