package app

import (
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// BuildBeadStagePrompt produces the prompt sent to the agent for one bead
// at one workflow stage. When wf.Stages has a contract for currentState it
// renders a contract-aware prompt; otherwise it falls back to a generic
// engineer prompt.
//
// repoPath is the canonical bd repo (NOT the worktree).
func BuildBeadStagePrompt(bead *backend.Bead, currentState string, stages map[string]backend.StageContract, repoPath, worktree string) string {
	contract, hasContract := stages[currentState]

	var b strings.Builder
	fmt.Fprintf(&b, "# Bead %s — %s\n\n", bead.ID, bead.Title)

	renderRole(&b, hasContract, contract)
	renderInputs(&b, hasContract, contract, bead.ID)
	renderOutput(&b, hasContract, contract, bead.ID)
	renderForbidden(&b, hasContract, contract)
	renderOperatingRules(&b)

	if !hasContract {
		b.WriteString("## Steps (from the bead description)\n\n")
	}
	renderBeadData(&b, bead, hasContract)

	b.WriteString("## Bead metadata\n\n")
	fmt.Fprintf(&b, "- ID: `%s`\n", bead.ID)
	fmt.Fprintf(&b, "- Priority: `P%d`\n", bead.Priority)
	if bead.Type != "" {
		fmt.Fprintf(&b, "- Type: `%s`\n", bead.Type)
	}
	fmt.Fprintf(&b, "- Worktree (this IS your current working directory — reference files by paths relative to it, e.g. `cmd/kernl/epic.go`; do not retype the absolute path): `%s`\n", worktree)
	fmt.Fprintf(&b, "- Canonical bd repo (read-only, lives outside your worktree — never cd or write here): `%s`\n", repoPath)

	return b.String()
}

func renderRole(b *strings.Builder, hasContract bool, contract backend.StageContract) {
	b.WriteString("## Role\n\n")
	if hasContract && strings.TrimSpace(contract.Role) != "" {
		b.WriteString(strings.TrimSpace(contract.Role))
		b.WriteString("\n\n")
		return
	}
	b.WriteString("You are an autonomous engineer executing ONE workflow stage of ONE bead from the kernl orchestrator. ")
	b.WriteString("The cwd you are running in is a git worktree dedicated to this bead. ")
	b.WriteString("Complete this stage's work and stop.\n\n")
}

func renderInputs(b *strings.Builder, hasContract bool, contract backend.StageContract, beadID string) {
	if !hasContract || len(contract.Inputs) == 0 {
		return
	}
	b.WriteString("## Inputs available to you\n\n")
	for _, inp := range contract.Inputs {
		resolved := strings.ReplaceAll(inp, "<bead_id>", beadID)
		fmt.Fprintf(b, "- %s\n", resolved)
	}
	b.WriteString("\nSome inputs may not exist this run — e.g. planning was skipped, so there is no `plan.md`. If a listed file is absent, proceed WITHOUT it: review against the committed changes in your worktree (`git log -p`, `git diff`) and the acceptance criteria below. NEVER search for a missing input outside your worktree (the canonical repo, other beads); it is not there and the access will be auto-rejected.\n\n")
}

func renderOutput(b *strings.Builder, hasContract bool, contract backend.StageContract, beadID string) {
	if !hasContract {
		return
	}
	artifact := contract.OutputArtifact
	if artifact.Path == "" && artifact.Kind == "" {
		return
	}
	b.WriteString("## Required output\n\n")
	if artifact.Path != "" {
		resolved := strings.ReplaceAll(artifact.Path, "<bead_id>", beadID)
		fmt.Fprintf(b, "Write the following file: `%s`\n", resolved)
	}
	if artifact.Kind == "commits" && artifact.CommitMarker != "" {
		fmt.Fprintf(b, "Commit your work with the marker: `%s`\n", artifact.CommitMarker)
	}
	if artifact.MustEndWith != "" {
		fmt.Fprintf(b, "The output must end with: `%s`\n", artifact.MustEndWith)
	}
	b.WriteString("\n")
}

func renderForbidden(b *strings.Builder, hasContract bool, contract backend.StageContract) {
	b.WriteString("## You may NOT\n\n")
	if hasContract {
		for _, fp := range contract.ForbiddenPaths {
			fmt.Fprintf(b, "- Modify `%s`\n", fp)
		}
	}
	b.WriteString("- Do not run `bd update`, `bd close`, or `bd open`. The orchestrator advances the bead when your stage completes.\n")
	b.WriteString("\n")
}

func renderOperatingRules(b *strings.Builder) {
	b.WriteString("## Operating rules\n\n")
	b.WriteString("1. Your cwd IS this bead's worktree. Reference files by paths relative to cwd (e.g. `cmd/kernl/epic.go`), not absolute paths — hand-retyping the absolute worktree path (and dropping a hidden segment like `.kernl`) is the #1 cause of auto-rejected `external_directory` errors. Edit ONLY files inside this worktree; do not touch unrelated packages.\n")
	b.WriteString("2. Scratch files (rg output, inventory lists, anything intermediate): write them INSIDE the worktree (e.g. `./_scratch/<name>`) — NEVER `/tmp/*`. The orchestrator allow-lists `/tmp/**` for reads but several observed bails came from agents trying to write outside the worktree.\n")
	b.WriteString("3. Follow `AGENTS.md`: files < 500 lines, funcs 4–40 lines, fail-loud marker `KERNL DISPATCH FAILURE: <problem> — <cause> — Fix: <action>`.\n")
	b.WriteString("4. Tests must be hermetic (`*_test.go`) using fakes/stubs. No real network, no real disk outside `t.TempDir()`.\n")
	b.WriteString("5. The Go module lives at `orchestrator/go.mod`. Before declaring done, run:\n")
	b.WriteString("   ```bash\n")
	b.WriteString("   cd orchestrator && go vet ./... && go test ./...\n")
	b.WriteString("   ```\n")
	b.WriteString("   If ANY test fails, you are NOT done. Fix and re-run.\n")
	b.WriteString("6. Commit your work in this worktree:\n")
	b.WriteString("   ```bash\n")
	b.WriteString("   git add -A && git commit -m \"<conventional message>\"\n")
	b.WriteString("   ```\n")
	b.WriteString("7. DO NOT push. DO NOT switch branches. DO NOT touch `master`.\n\n")
	b.WriteString("If a tool call is auto-rejected (e.g. 'permission requested: external_directory'), STOP and switch to an in-worktree path immediately — do NOT keep retrying the rejected path; the rejection means opencode will not allow it this session.\n\n")
	b.WriteString("If you cannot proceed because of a missing dependency, fail loud with a descriptive error and stop. Do not invent stubs.\n\n")
}

func renderBeadData(b *strings.Builder, bead *backend.Bead, hasContract bool) {
	if hasContract {
		b.WriteString("## Bead data\n\n")
	} else {
		fmt.Fprintf(b, "%s\n\n", bead.Description)
		if strings.TrimSpace(bead.Acceptance) != "" {
			b.WriteString("## Acceptance criteria\n\n")
			b.WriteString(bead.Acceptance)
			b.WriteString("\n\n")
		}
		return
	}
	if strings.TrimSpace(bead.Description) != "" {
		fmt.Fprintf(b, "Description:\n%s\n\n", bead.Description)
	} else {
		b.WriteString("Description: _(none; infer from the title)_\n\n")
	}
	if strings.TrimSpace(bead.Acceptance) != "" {
		fmt.Fprintf(b, "Acceptance criteria:\n%s\n\n", bead.Acceptance)
	}
}
