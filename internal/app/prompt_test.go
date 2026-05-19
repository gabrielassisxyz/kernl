package app

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

func TestBuildBeadStagePrompt_IncludesStageInstruction(t *testing.T) {
	bead := &backend.Bead{
		ID:          "kernl-eci",
		Title:       "Inventory existing module references for reorg",
		Description: "Run `rg -l 'orchestrator/internal'` and write the result to /tmp/refs.txt",
		Acceptance:  "/tmp/refs.txt exists and is non-empty",
		Priority:    0,
		Type:        "task",
	}

	prompt := BuildBeadStagePrompt(bead, "implementation", nil, "/home/user/repo", "/home/user/.kernl/worktrees/epic/kernl-eci")

	mustContain := []string{
		"kernl-eci",
		"Inventory existing module references for reorg",
		"Run `rg -l",
		"/tmp/refs.txt exists and is non-empty",
		"The orchestrator advances the bead",
		"DO NOT push",
		"go vet ./... && go test ./...",
	}
	for _, want := range mustContain {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q\n---\n%s\n---", want, prompt)
		}
	}
}

func TestBuildBeadStagePrompt_OmitsEndOfStageProtocol(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Test bead", Description: "do the thing"}
	prompt := BuildBeadStagePrompt(bead, "planning", nil, "/repo", "/wt")

	if strings.Contains(prompt, "END-OF-STAGE PROTOCOL") {
		t.Errorf("prompt must not contain END-OF-STAGE PROTOCOL:\n%s", prompt)
	}
	if strings.Contains(prompt, "bd update --status") {
		t.Errorf("prompt must not contain bd update --status:\n%s", prompt)
	}
}

func TestBuildBeadStagePrompt_ForbidsBdMutation(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Test bead", Description: "do the thing"}
	prompt := BuildBeadStagePrompt(bead, "planning", nil, "/repo", "/wt")

	if !strings.Contains(prompt, "Do not run `bd update`, `bd close`, or `bd open`") {
		t.Error("prompt must forbid bd mutation")
	}
	if !strings.Contains(prompt, "The orchestrator advances the bead") {
		t.Error("prompt must explain the orchestrator handles advancement")
	}
}

func TestBuildBeadStagePrompt_TerminalStageOmitsBdUpdate(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Last stage", Description: "do the thing"}
	prompt := BuildBeadStagePrompt(bead, "shipment_review", nil, "/repo", "/wt")

	if strings.Contains(prompt, "bd -C") {
		t.Errorf("terminal stage should not include `bd update` instruction; got:\n%s", prompt)
	}
	if strings.Contains(prompt, "bd update --status") {
		t.Errorf("terminal stage should not include `bd update --status`; got:\n%s", prompt)
	}
}

func TestAppendOpencodeStageFlags_AddsDirTitleAndPrompt(t *testing.T) {
	args := []string{"run", "--format", "json", "--model", "litellm/m"}
	out := appendOpencodeStageFlags(args, "kb-1", "/tmp/wt", "", "PROMPT_BODY")

	for i, a := range args {
		if out[i] != a {
			t.Errorf("arg %d mutated: want %q got %q", i, a, out[i])
		}
	}
	if out[len(out)-1] != "PROMPT_BODY" {
		t.Errorf("prompt must be last arg, got %q", out[len(out)-1])
	}
	joined := strings.Join(out, " ")
	if !strings.Contains(joined, "--dir /tmp/wt") {
		t.Errorf("missing --dir <worktree>: %s", joined)
	}
	if !strings.Contains(joined, "--title kernl:kb-1") {
		t.Errorf("missing --title kernl:<id>: %s", joined)
	}
}

func TestAppendOpencodeStageFlags_IdempotentWhenDirAlreadySet(t *testing.T) {
	args := []string{"run", "--dir", "/preconfigured", "--title", "preset"}
	out := appendOpencodeStageFlags(args, "kb-1", "/tmp/wt", "", "PROMPT")

	dirCount, titleCount := 0, 0
	for _, a := range out {
		if a == "--dir" {
			dirCount++
		}
		if a == "--title" {
			titleCount++
		}
	}
	if dirCount != 1 {
		t.Errorf("--dir should appear exactly once, got %d", dirCount)
	}
	if titleCount != 1 {
		t.Errorf("--title should appear exactly once, got %d", titleCount)
	}
}

func TestBuildBeadStagePrompt_RendersStageContract(t *testing.T) {
	bead := &backend.Bead{
		ID:          "kb-1",
		Title:       "Add dark mode",
		Description: "Implement dark mode toggle",
		Acceptance:  "Toggle works in all components",
	}

	stages := map[string]backend.StageContract{
		"planning": {
			Role: "Decompose the bead into an actionable plan.",
			Inputs: []string{
				"bead.title",
				"bead.description",
			},
			OutputArtifact: backend.StageArtifact{
				Path: ".kernl/<bead_id>/plan.md",
			},
			ForbiddenPaths: []string{
				"**/*.go",
				"**/*.ts",
			},
		},
	}

	prompt := BuildBeadStagePrompt(bead, "planning", stages, "/repo", "/wt")

	mustContain := []string{
		"Decompose the bead into an actionable plan.",
		".kernl/kb-1/plan.md",
		"**/*.go",
		"**/*.ts",
	}
	for _, want := range mustContain {
		if !strings.Contains(prompt, want) {
			t.Errorf("contract prompt missing %q\n---\n%s\n---", want, prompt)
		}
	}
}

func TestBuildBeadStagePrompt_BeadIsInputNotInstruction(t *testing.T) {
	bead := &backend.Bead{
		ID:          "kb-1",
		Title:       "Build feature",
		Description: "Write a function that sorts arrays",
		Acceptance:  "Tests must pass",
	}

	stages := map[string]backend.StageContract{
		"planning": {
			Role: "Create a plan.",
			Inputs: []string{"bead.description"},
		},
	}

	prompt := BuildBeadStagePrompt(bead, "planning", stages, "/repo", "/wt")

	if strings.Contains(prompt, "## Steps") || strings.Contains(prompt, "## Instructions") {
		t.Errorf("contract prompt must not contain Steps/Instructions heading. Bead data should appear under 'Bead data':\n%s", prompt)
	}
	if !strings.Contains(prompt, "## Bead data") {
		t.Errorf("contract prompt missing '## Bead data' heading:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Write a function") {
		t.Errorf("contract prompt should still contain bead description text:\n%s", prompt)
	}
}

func TestBuildBeadStagePrompt_FallbackWhenNoStageBlock(t *testing.T) {
	bead := &backend.Bead{
		ID:          "kb-1",
		Title:       "Fallback bead",
		Description: "do the work",
		Acceptance:  "work is done",
	}

	prompt := BuildBeadStagePrompt(bead, "implementation", nil, "/repo", "/wt")

	if !strings.Contains(prompt, "do the work") {
		t.Errorf("fallback prompt must contain description; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Operating rules") {
		t.Error("fallback prompt must include operating rules")
	}
	if strings.Contains(prompt, "END-OF-STAGE") {
		t.Error("fallback prompt must not contain END-OF-STAGE")
	}
}
