package prompt_test

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

func sampleShipmentInput() prompt.ShipmentInput {
	return prompt.ShipmentInput{
		EpicID: "e1", EpicTitle: "Test epic", EpicBranch: "feat/e1", BaseBranch: "master",
		RemoteName: "origin", RemoteURL: "git@github.com:owner/repo.git", RepoSlug: "github.com/owner/repo",
	}
}

func TestRenderShipment_Content(t *testing.T) {
	out, err := prompt.RenderShipment(sampleShipmentInput())
	if err != nil {
		t.Fatal(err)
	}
	mustContain := []string{
		"git push",
		"gh pr create",
		"--head feat/e1 --json url",
		"pr_already_exists",
		"pr_url:",
		"push_failed",
		"pr_create_failed",
		// The destination is named in the prompt so the agent has nothing to
		// resolve on its own initiative.
		"git push origin feat/e1",
		"git@github.com:owner/repo.git",
		// gh must be pinned to the verified repository: without --repo it picks
		// one out of the working directory's remotes, which is the choice that
		// must not stay open.
		"gh pr create --repo github.com/owner/repo",
		"gh pr list --repo github.com/owner/repo",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("shipment prompt missing %q", want)
		}
	}
}

// Shipment runs under claude and codex too, and neither has opencode's
// permission model. Explaining opencode's rejections to them is noise that
// invites an agent to reason about a sandbox it is not in.
func TestRenderShipment_DoesNotExplainAnotherAgentsSandbox(t *testing.T) {
	out, err := prompt.RenderShipment(sampleShipmentInput())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "opencode") {
		t.Error("the shipment prompt has no dialect and must not name one")
	}
	// The prohibition itself is dialect-neutral and stays.
	if !strings.Contains(out, "/tmp") {
		t.Error("the shipment prompt must still keep scratch files out of /tmp")
	}
}

func TestRenderShipment_EmptyBranches(t *testing.T) {
	cases := []prompt.ShipmentInput{
		{EpicID: "e1", EpicTitle: "x", EpicBranch: "", BaseBranch: "master", RemoteName: "origin", RemoteURL: "u", RepoSlug: "h/o/r"},
		{EpicID: "e1", EpicTitle: "x", EpicBranch: "feat/e1", BaseBranch: "", RemoteName: "origin", RemoteURL: "u", RepoSlug: "h/o/r"},
	}
	for _, in := range cases {
		if _, err := prompt.RenderShipment(in); err == nil {
			t.Errorf("expected error for input %+v", in)
		} else if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
			t.Errorf("expected KERNL DISPATCH FAILURE, got %v", err)
		}
	}
}

// A shipment prompt without a verified destination must not render at all:
// "push to origin" with origin unresolved is the ambiguity that published a
// pull request nobody asked for.
func TestRenderShipment_RefusesWithoutDestination(t *testing.T) {
	for _, blank := range []func(*prompt.ShipmentInput){
		func(in *prompt.ShipmentInput) { in.RemoteURL = "" },
		func(in *prompt.ShipmentInput) { in.RemoteName = "" },
		// A local-path remote produces no repository slug, so there is nothing
		// to open a pull request on and nothing to pin gh to.
		func(in *prompt.ShipmentInput) { in.RepoSlug = "" },
	} {
		in := sampleShipmentInput()
		blank(&in)
		if _, err := prompt.RenderShipment(in); err == nil {
			t.Errorf("expected a refusal for input %+v", in)
		}
	}
}
