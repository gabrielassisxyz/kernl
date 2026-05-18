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

	prompt := BuildBeadStagePrompt(bead, "implementation", "ready_for_implementation_review", "/home/user/repo", "/home/user/.kernl/worktrees/epic/kernl-eci")

	mustContain := []string{
		"kernl-eci",
		"Inventory existing module references for reorg",
		"Run `rg -l",
		"/tmp/refs.txt exists and is non-empty",
		"Current workflow state: `implementation`",
		"On success, advance the bead to: `ready_for_implementation_review`",
		"bd -C /home/user/repo update kernl-eci --status ready_for_implementation_review",
		"DO NOT push",
		"DO NOT run `bd close`",
		"go vet ./... && go test ./...",
	}
	for _, want := range mustContain {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q\n---\n%s\n---", want, prompt)
		}
	}
}

func TestBuildBeadStagePrompt_TerminalStageOmitsBdUpdate(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Last stage", Description: "do the thing"}
	prompt := BuildBeadStagePrompt(bead, "shipment_review", "", "/repo", "/wt")

	if strings.Contains(prompt, "bd -C") {
		t.Errorf("terminal stage should not include `bd update` instruction; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "no forward transition") {
		t.Errorf("expected terminal-stage hint, got:\n%s", prompt)
	}
}

func TestAppendOpencodeStageFlags_AddsDirTitleAndPrompt(t *testing.T) {
	args := []string{"run", "--format", "json", "--model", "litellm/m"}
	out := appendOpencodeStageFlags(args, "kb-1", "/tmp/wt", "", "PROMPT_BODY")

	// Original args preserved in order
	for i, a := range args {
		if out[i] != a {
			t.Errorf("arg %d mutated: want %q got %q", i, a, out[i])
		}
	}
	// Prompt is the LAST positional
	if out[len(out)-1] != "PROMPT_BODY" {
		t.Errorf("prompt must be last arg, got %q", out[len(out)-1])
	}
	// --dir and --title present
	joined := strings.Join(out, " ")
	if !strings.Contains(joined, "--dir /tmp/wt") {
		t.Errorf("missing --dir <worktree>: %s", joined)
	}
	if !strings.Contains(joined, "--title kernl:kb-1") {
		t.Errorf("missing --title kernl:<id>: %s", joined)
	}
}

func TestAppendOpencodeStageFlags_IdempotentWhenDirAlreadySet(t *testing.T) {
	// If kernl.yaml already includes --dir / --title, do not double them.
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
