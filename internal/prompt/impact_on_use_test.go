package prompt_test

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

func sampleImpactOnUseInput() prompt.ImpactOnUseInput {
	return prompt.ImpactOnUseInput{
		DecisionTitle:     "cache the robots.txt fetch",
		DecisionContext:   "every URL refetched it",
		OptionsConsidered: "per-run cache; per-host cache",
		TradeOffs:         "staleness against request volume",
		Outcome:           "per-host cache",
		RepoPath:          "/repos/archeion",
		BeadTitle:         "a sitemap run refetches robots.txt once per URL",
	}
}

func TestRenderImpactOnUse_AsksFor3To6Sentences(t *testing.T) {
	out := prompt.RenderImpactOnUse(sampleImpactOnUseInput())
	if !strings.Contains(out, "3-6 sentences") {
		t.Errorf("prompt %q does not ask for 3-6 sentences", out)
	}
}

func TestRenderImpactOnUse_ContextPresentRendersUnderItsOwnHeading(t *testing.T) {
	in := sampleImpactOnUseInput()
	in.RepositoryContext = "### README.md\n\nkernl is a knowledge-graph substrate fused with an orchestrator.\n\n"

	out := prompt.RenderImpactOnUse(in)

	if !strings.Contains(out, "## Repository context") {
		t.Errorf("prompt %q has no repository context heading", out)
	}
	headingIdx := strings.Index(out, "## Repository context")
	contentIdx := strings.Index(out, "kernl is a knowledge-graph substrate")
	if contentIdx == -1 || contentIdx < headingIdx {
		t.Errorf("prompt %q does not render the context under its heading", out)
	}
}

// An empty RepositoryContext must render an explicit line, not an empty
// section - an omitted section is indistinguishable, to a reader of the
// prompt, from this feature never having run at all.
func TestRenderImpactOnUse_ContextAbsentRendersAnExplicitLine(t *testing.T) {
	in := sampleImpactOnUseInput()
	in.RepositoryContext = ""

	out := prompt.RenderImpactOnUse(in)

	if !strings.Contains(out, "## Repository context") {
		t.Errorf("prompt %q dropped the heading entirely instead of rendering it with an explicit line under it", out)
	}
	if !strings.Contains(out, "No repository context is available") {
		t.Errorf("prompt %q does not say plainly that no context was available", out)
	}
}

// A truncated file's cut marker is assembled upstream by app.AssembleContext
// and handed to this prompt as part of RepositoryContext - the prompt must
// pass it through rather than swallow it, so a reader (human or model) can
// tell truncation happened.
func TestRenderImpactOnUse_TruncationMarkerSurvives(t *testing.T) {
	in := sampleImpactOnUseInput()
	in.RepositoryContext = "### README.md\n\nsome content\n\n[... README.md truncated: the 32 KB context budget was exceeded ...]\n"

	out := prompt.RenderImpactOnUse(in)

	if !strings.Contains(out, "truncated: the 32 KB context budget was exceeded") {
		t.Errorf("prompt %q lost the truncation marker", out)
	}
}

func TestRenderImpactOnUse_DoesNotInviteReopeningTheDecision(t *testing.T) {
	out := prompt.RenderImpactOnUse(sampleImpactOnUseInput())
	if !strings.Contains(out, "already made") && !strings.Contains(out, "not your job") {
		t.Errorf("prompt %q does not tell the composer the decision is already settled", out)
	}
}
