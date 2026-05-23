package prompt_test

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

func sampleIntegrationInput() prompt.IntegrationInput {
	return prompt.IntegrationInput{
		EpicID: "e1", EpicTitle: "Test epic", EpicBranch: "feat/e1", BaseBranch: "master",
		Children: []prompt.Child{
			{ID: "c1", Branch: "feat/c1", WorktreePath: "/tmp/c1"},
			{ID: "c2", Branch: "feat/c2", WorktreePath: "/tmp/c2"},
		},
	}
}

func TestRenderIntegration_Content(t *testing.T) {
	out, err := prompt.RenderIntegration(sampleIntegrationInput())
	if err != nil {
		t.Fatal(err)
	}
	mustContain := []string{
		"git merge --no-ff",
		"stage: integration",
		"merge_conflict",
		"feat/e1",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("integration prompt missing %q", want)
		}
	}
	// The integration stage must not push or open a PR.
	for _, banned := range []string{"gh pr create", "git push"} {
		if strings.Contains(out, banned) {
			t.Errorf("integration prompt must NOT contain %q", banned)
		}
	}
}

func TestRenderIntegration_EmptyBranches(t *testing.T) {
	cases := []prompt.IntegrationInput{
		{EpicID: "e1", EpicTitle: "x", EpicBranch: "", BaseBranch: "master"},
		{EpicID: "e1", EpicTitle: "x", EpicBranch: "feat/e1", BaseBranch: ""},
	}
	for _, in := range cases {
		if _, err := prompt.RenderIntegration(in); err == nil {
			t.Errorf("expected error for input %+v", in)
		} else if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
			t.Errorf("expected KERNL DISPATCH FAILURE, got %v", err)
		}
	}
}
