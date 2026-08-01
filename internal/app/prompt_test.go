package app

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
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

	prompt := BuildBeadStagePrompt(StagePromptInput{Bead: bead, State: "implementation", Stages: nil, RepoPath: "/home/user/repo", Worktree: "/home/user/.kernl/worktrees/epic/kernl-eci", VerifyCommand: "bin/ci", TrackerCommand: "bd"})

	mustContain := []string{
		"kernl-eci",
		"Inventory existing module references for reorg",
		"Run `rg -l",
		"/tmp/refs.txt exists and is non-empty",
		"The orchestrator advances the bead",
		"DO NOT push",
		"bin/ci",
	}
	for _, want := range mustContain {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q\n---\n%s\n---", want, prompt)
		}
	}
}

func TestBuildBeadStagePrompt_OmitsEndOfStageProtocol(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Test bead", Description: "do the thing"}
	prompt := BuildBeadStagePrompt(StagePromptInput{Bead: bead, State: "planning", Stages: nil, RepoPath: "/repo", Worktree: "/wt", VerifyCommand: "bin/ci", TrackerCommand: "bd"})

	if strings.Contains(prompt, "END-OF-STAGE PROTOCOL") {
		t.Errorf("prompt must not contain END-OF-STAGE PROTOCOL:\n%s", prompt)
	}
	if strings.Contains(prompt, "bd update --status") {
		t.Errorf("prompt must not contain bd update --status:\n%s", prompt)
	}
}

// The tracker the prompt forbids mutating is the repository's, not kernl's.
func TestBuildBeadStagePrompt_NamesTheRepositorysOwnTracker(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Test bead", Description: "do the thing"}
	prompt := BuildBeadStagePrompt(StagePromptInput{
		Bead: bead, State: "planning", RepoPath: "/repo", Worktree: "/wt",
		VerifyCommand: "bin/ci", TrackerCommand: "br --db /repo/.beads/beads.db",
	})

	if !strings.Contains(prompt, "`br --db /repo/.beads/beads.db update`") {
		t.Errorf("prompt must forbid mutating the tracker this repository uses:\n%s", prompt)
	}
	if strings.Contains(prompt, "`bd update`") {
		t.Error("prompt must not name kernl's own tracker in someone else's repository")
	}
}

func TestBuildBeadStagePrompt_ForbidsBdMutation(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Test bead", Description: "do the thing"}
	prompt := BuildBeadStagePrompt(StagePromptInput{Bead: bead, State: "planning", Stages: nil, RepoPath: "/repo", Worktree: "/wt", VerifyCommand: "bin/ci", TrackerCommand: "bd"})

	if !strings.Contains(prompt, "Do not run `bd update`, `bd close`, or `bd reopen`") {
		t.Error("prompt must forbid bd mutation")
	}
	if !strings.Contains(prompt, "The orchestrator advances the bead") {
		t.Error("prompt must explain the orchestrator handles advancement")
	}
}

func TestBuildBeadStagePrompt_TerminalStageOmitsBdUpdate(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Last stage", Description: "do the thing"}
	prompt := BuildBeadStagePrompt(StagePromptInput{Bead: bead, State: "shipment_review", Stages: nil, RepoPath: "/repo", Worktree: "/wt", VerifyCommand: "bin/ci", TrackerCommand: "bd"})

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

	prompt := BuildBeadStagePrompt(StagePromptInput{Bead: bead, State: "planning", Stages: stages, RepoPath: "/repo", Worktree: "/wt", VerifyCommand: "bin/ci", TrackerCommand: "bd"})

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
			Role:   "Create a plan.",
			Inputs: []string{"bead.description"},
		},
	}

	prompt := BuildBeadStagePrompt(StagePromptInput{Bead: bead, State: "planning", Stages: stages, RepoPath: "/repo", Worktree: "/wt", VerifyCommand: "bin/ci", TrackerCommand: "bd"})

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

	prompt := BuildBeadStagePrompt(StagePromptInput{Bead: bead, State: "implementation", Stages: nil, RepoPath: "/repo", Worktree: "/wt", VerifyCommand: "bin/ci", TrackerCommand: "bd"})

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

// The prompt is sent to an agent working in someone else's repository, so it
// may not carry kernl's stack, kernl's layout or kernl's coding conventions.
// Every string below was in it, shipped verbatim to a Rust repository.
func TestBuildBeadStagePrompt_CarriesNothingAboutKernlsOwnStack(t *testing.T) {
	bead := &backend.Bead{ID: "arch-c9k", Title: "Fix canonical URL handling"}
	prompt := BuildBeadStagePrompt(StagePromptInput{
		Bead: bead, State: "implementation", RepoPath: "/repo", Worktree: "/wt",
		VerifyCommand: "bin/ci", TrackerCommand: "bd",
	})

	for _, banned := range []string{
		"orchestrator/go.mod",
		"go vet",
		"go test",
		"*_test.go",
		"t.TempDir()",
		"KERNL DISPATCH FAILURE",
		"files < 500 lines",
		"git add -A && git commit",
		"cmd/kernl/epic.go",
	} {
		if strings.Contains(prompt, banned) {
			t.Errorf("prompt imposes kernl's own conventions on another repository: found %q", banned)
		}
	}

	// What replaces them: the target repository's own contract, plus the one
	// staging rule that is kernl's business, because `git add -A` is how the
	// orchestrator's own control files end up in a stranger's pull request.
	for _, want := range []string{"AGENTS.md", "bin/ci", "never `git add -A`"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

// The exit-gate artifact directory lives outside the worktree (see
// resolveArtifactDir in drive_bead.go). The agent's cwd is the worktree, so
// a relative path in the prompt would write the file straight back into it -
// exactly the leak PR #40 on archeion shipped. The prompt must name the
// absolute artifact directory instead.
func TestBuildBeadStagePrompt_OutputArtifactIsAbsoluteOutsideWorktree(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Add dark mode"}
	artifactDir := "/home/user/.kernl/run/epic-1/kb-1"

	stages := map[string]backend.StageContract{
		"planning": {
			Role: "Decompose the bead into an actionable plan.",
			Inputs: []string{
				"bead.title",
				"<artifact_dir>/plan.md",
			},
			OutputArtifact: backend.StageArtifact{
				Path: "<artifact_dir>/plan.md",
			},
		},
	}

	prompt := BuildBeadStagePrompt(StagePromptInput{
		Bead: bead, State: "planning", Stages: stages, RepoPath: "/repo", Worktree: "/repo-worktree/kb-1",
		VerifyCommand: "bin/ci", TrackerCommand: "bd", ArtifactDir: artifactDir,
	})

	wantPath := artifactDir + "/plan.md"
	if !strings.Contains(prompt, wantPath) {
		t.Errorf("prompt must name the absolute artifact path %q\n---\n%s\n---", wantPath, prompt)
	}
	if strings.Contains(prompt, "<artifact_dir>") {
		t.Errorf("prompt must not leak the raw <artifact_dir> placeholder:\n%s", prompt)
	}
	if strings.Contains(prompt, "Write the following file: `.kernl/") {
		t.Errorf("prompt must not point the agent back inside the worktree:\n%s", prompt)
	}
}

// The external_directory note describes one CLI's permission model. Sent to
// claude or codex it is an instruction about a rejection they cannot produce.
func TestBuildBeadStagePrompt_OpencodeNoteIsOpencodeOnly(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Test bead"}
	base := StagePromptInput{Bead: bead, State: "implementation", RepoPath: "/repo", Worktree: "/wt", VerifyCommand: "bin/ci", TrackerCommand: "bd"}

	opencode := base
	opencode.Dialect = adapter.DialectOpenCode
	if !strings.Contains(BuildBeadStagePrompt(opencode), "external_directory") {
		t.Error("opencode must still be told how its own rejections behave")
	}

	claude := base
	claude.Dialect = adapter.DialectClaude
	if strings.Contains(BuildBeadStagePrompt(claude), "external_directory") {
		t.Error("claude must not be told about opencode's permission model")
	}
}

// TestBuildBeadStagePrompt_DecisionRecord_NamesAbsolutePathAndFourSections
// proves the rendered implementation prompt tells the agent where to write
// the decision record (as an absolute path outside the worktree, for the
// same reason as any other output artifact) and names all four required
// sections - the parseable shape the decision_record exit gate checks for.
func TestBuildBeadStagePrompt_DecisionRecord_NamesAbsolutePathAndFourSections(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Add dark mode"}
	artifactDir := "/home/user/.kernl/run/epic-1/kb-1"

	stages := map[string]backend.StageContract{
		"implementation": {
			Role: "Implement the plan.",
			OutputArtifact: backend.StageArtifact{
				Kind: "commits",
			},
			DecisionRecord: backend.StageArtifact{
				Path: "<artifact_dir>/decision-record.md",
			},
		},
	}

	prompt := BuildBeadStagePrompt(StagePromptInput{
		Bead: bead, State: "implementation", Stages: stages, RepoPath: "/repo", Worktree: "/repo-worktree/kb-1",
		VerifyCommand: "bin/ci", TrackerCommand: "bd", ArtifactDir: artifactDir,
	})

	wantPath := artifactDir + "/decision-record.md"
	if !strings.Contains(prompt, wantPath) {
		t.Errorf("prompt must name the absolute decision record path %q\n---\n%s\n---", wantPath, prompt)
	}
	for _, heading := range []string{"## Decision", "## Options Considered", "## Trade-offs", "## Rationale"} {
		if !strings.Contains(prompt, heading) {
			t.Errorf("prompt missing required section heading %q\n---\n%s\n---", heading, prompt)
		}
	}
	if strings.Contains(prompt, "<artifact_dir>") {
		t.Errorf("prompt must not leak the raw <artifact_dir> placeholder:\n%s", prompt)
	}
	// The fifth field of a full decision record (impact on using the tool)
	// is out of scope for this stage: it is written later, by a different
	// actor. The prompt must not ask the implementer for it.
	if strings.Contains(prompt, "## Impact") {
		t.Error("prompt must not ask the implementer for the impact-on-usage section")
	}
}

// TestBuildBeadStagePrompt_DecisionRecord_OmittedWhenNotDeclared proves the
// section is only rendered for stages whose contract actually sets
// DecisionRecord - a stage like "planning" that never requires one must not
// carry this instruction.
func TestBuildBeadStagePrompt_DecisionRecord_OmittedWhenNotDeclared(t *testing.T) {
	bead := &backend.Bead{ID: "kb-1", Title: "Add dark mode"}
	stages := map[string]backend.StageContract{
		"planning": {
			Role: "Decompose the bead into an actionable plan.",
			OutputArtifact: backend.StageArtifact{
				Path: "<artifact_dir>/plan.md",
			},
		},
	}

	prompt := BuildBeadStagePrompt(StagePromptInput{
		Bead: bead, State: "planning", Stages: stages, RepoPath: "/repo", Worktree: "/wt",
		VerifyCommand: "bin/ci", TrackerCommand: "bd", ArtifactDir: "/home/user/.kernl/run/epic-1/kb-1",
	})

	if strings.Contains(prompt, "## Decision record") {
		t.Errorf("planning stage must not carry decision record instructions:\n%s", prompt)
	}
}

// reviewStages is the contract shape a review stage carries: one artifact,
// and a required trailing line that is the approval half of the exit gate's
// vocabulary.
func reviewStages() map[string]backend.StageContract {
	return map[string]backend.StageContract{
		"implementation_review": {
			Role: "Review the implementation against the plan and acceptance criteria.",
			OutputArtifact: backend.StageArtifact{
				Path:        "<artifact_dir>/implementation-review.md",
				MustEndWith: "VERDICT: PASS",
			},
		},
		"planning": {
			Role:           "Decompose the bead into an actionable plan.",
			OutputArtifact: backend.StageArtifact{Path: "<artifact_dir>/plan.md"},
		},
	}
}

// A reviewer told only how to spell approval invents a word for the other
// outcome. One wrote "VERDICT: FAIL" over reproducible findings; the gate
// reads exactly one word, so that landed in the bucket for reviews that
// produced no verdict at all - the bead blocked for a human, the findings
// never reached an implementer, and the rewind budget untouched.
func TestBuildBeadStagePrompt_ReviewStageNamesTheRejectionVerdict(t *testing.T) {
	prompt := BuildBeadStagePrompt(StagePromptInput{
		Bead: &backend.Bead{ID: "kb-1", Title: "Add dark mode"}, State: "implementation_review",
		Stages: reviewStages(), RepoPath: "/repo", Worktree: "/repo-worktree/kb-1",
		VerifyCommand: "bin/ci", TrackerCommand: "br", ArtifactDir: "/home/user/.kernl/run/epic-1/kb-1",
	})

	for _, want := range []string{
		"`VERDICT: PASS`",
		"`VERDICT: REJECT`",
		// Naming the word is half of it: a reviewer also has to know that
		// wording it differently costs it its own findings.
		"FAIL",
		"produced no verdict",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("review prompt missing %q\n---\n%s\n---", want, prompt)
		}
	}
}

// A stage whose rejection has nowhere to go must not advertise one. Telling a
// planner to write REJECT produces a verdict the gate reads as a plain
// failure, which is the same outcome by a more confusing route.
func TestBuildBeadStagePrompt_NonReviewStageOffersNoRejectionVerdict(t *testing.T) {
	prompt := BuildBeadStagePrompt(StagePromptInput{
		Bead: &backend.Bead{ID: "kb-1", Title: "Add dark mode"}, State: "planning",
		Stages: reviewStages(), RepoPath: "/repo", Worktree: "/repo-worktree/kb-1",
		VerifyCommand: "bin/ci", TrackerCommand: "br", ArtifactDir: "/home/user/.kernl/run/epic-1/kb-1",
	})

	if strings.Contains(prompt, "VERDICT: REJECT") {
		t.Errorf("a planning stage has no rejection to declare:\n%s", prompt)
	}
}
