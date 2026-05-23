package prompt_test

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

func sampleShipmentInput() prompt.ShipmentInput {
	return prompt.ShipmentInput{
		EpicID: "e1", EpicTitle: "Test epic", EpicBranch: "feat/e1", BaseBranch: "master",
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
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("shipment prompt missing %q", want)
		}
	}
}

func TestRenderShipment_EmptyBranches(t *testing.T) {
	cases := []prompt.ShipmentInput{
		{EpicID: "e1", EpicTitle: "x", EpicBranch: "", BaseBranch: "master"},
		{EpicID: "e1", EpicTitle: "x", EpicBranch: "feat/e1", BaseBranch: ""},
	}
	for _, in := range cases {
		if _, err := prompt.RenderShipment(in); err == nil {
			t.Errorf("expected error for input %+v", in)
		} else if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
			t.Errorf("expected KERNL DISPATCH FAILURE, got %v", err)
		}
	}
}
