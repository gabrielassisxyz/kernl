package prompt_test

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

func sampleIntegrationInput() prompt.IntegrationInput {
	return prompt.IntegrationInput{
		EpicID: "e1", EpicTitle: "Test epic", EpicBranch: "feat/e1", BaseBranch: "main",
		VerifyCommand:  "bin/ci",
		TrackerCommand: "br --db /repo/.beads/beads.db",
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
		"bin/ci",
		// Integration replaces the generic stage prompt wholesale, so the rule
		// that sends an agent to the target repository's own conventions never
		// reaches this stage unless this template carries it - and resolving a
		// merge conflict means writing code under those conventions.
		"AGENTS.md",
		// The tracker is the repository's, not kernl's.
		"br --db /repo/.beads/beads.db update",
	}
	for _, banned := range []string{"bd update"} {
		if strings.Contains(out, banned) {
			t.Errorf("integration prompt must NOT name kernl's own tracker: found %q", banned)
		}
	}
	// The stack of the repository being integrated is not kernl's to assume.
	for _, banned := range []string{"go build", "go test"} {
		if strings.Contains(out, banned) {
			t.Errorf("integration prompt must NOT name a toolchain: found %q", banned)
		}
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

// TestRenderIntegration_NamesEachChildsOwnArtifactDir proves the rendered
// prompt tells the integration agent where to find EACH CHILD's own
// exit-gate artifacts (plan.md, review verdicts) - not the epic's own
// directory, and not silence. Before artifacts moved outside the worktree,
// a child's artifacts arrived for free when its branch was merged; now they
// never travel in a commit, so the only way the integration agent can read
// them is a path named explicitly here.
func TestRenderIntegration_NamesEachChildsOwnArtifactDir(t *testing.T) {
	in := sampleIntegrationInput()
	in.Children = []prompt.Child{
		{ID: "c1", Branch: "feat/c1", WorktreePath: "/tmp/c1", ArtifactDir: "/home/user/.kernl/run/e1/c1"},
		{ID: "c2", Branch: "feat/c2", WorktreePath: "/tmp/c2", ArtifactDir: "/home/user/.kernl/run/e1/c2"},
	}

	out, err := prompt.RenderIntegration(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/home/user/.kernl/run/e1/c1", "/home/user/.kernl/run/e1/c2"} {
		if !strings.Contains(out, want) {
			t.Errorf("integration prompt missing child artifact dir %q\n---\n%s\n---", want, out)
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
