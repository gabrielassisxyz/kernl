package prompt_test

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

func sampleShipmentInput() prompt.ShipmentInput {
	return prompt.ShipmentInput{
		EpicID: "e1", EpicTitle: "Test epic", EpicBranch: "feat/e1", BaseBranch: "master",
		RemoteName: "origin", RemoteURL: "git@github.com:owner/repo.git",
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
		"gh pr list --head feat/e1 --json url",
		"pr_already_exists",
		"pr_url:",
		"push_failed",
		"pr_create_failed",
		// The destination is named in the prompt so the agent has nothing to
		// resolve on its own initiative.
		"git push origin feat/e1",
		"git@github.com:owner/repo.git",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("shipment prompt missing %q", want)
		}
	}
}

func TestRenderShipment_EmptyBranches(t *testing.T) {
	cases := []prompt.ShipmentInput{
		{EpicID: "e1", EpicTitle: "x", EpicBranch: "", BaseBranch: "master", RemoteName: "origin", RemoteURL: "u"},
		{EpicID: "e1", EpicTitle: "x", EpicBranch: "feat/e1", BaseBranch: "", RemoteName: "origin", RemoteURL: "u"},
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
	in := sampleShipmentInput()
	in.RemoteURL = ""
	if _, err := prompt.RenderShipment(in); err == nil {
		t.Fatal("expected a refusal when the destination is unresolved")
	}
}
