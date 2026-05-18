package app

import (
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// BuildBeadStagePrompt produces the prompt sent to the agent for one bead
// at one workflow stage.
//
// nextState is the workflow state the orchestrator will advance to when the
// agent exits cleanly; empty means there is no forward transition.
//
// repoPath is the canonical bd repo (NOT the worktree).
func BuildBeadStagePrompt(bead *backend.Bead, currentState, nextState, repoPath, worktree string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Bead %s — %s\n\n", bead.ID, bead.Title)

	b.WriteString("You are an autonomous engineer executing ONE workflow stage of ONE bead from the kernl orchestrator. ")
	b.WriteString("The cwd you are running in is a git worktree dedicated to this bead. ")
	b.WriteString("Follow the Steps below exactly and stop when this stage is complete.\n\n")

	b.WriteString("## Stage\n\n")
	fmt.Fprintf(&b, "- Current workflow state: `%s`\n", currentState)
	b.WriteString("- The orchestrator will advance the bead when your stage completes.\n")
	b.WriteString("\n")

	b.WriteString("## Steps (verbatim from the bead description)\n\n")
	if strings.TrimSpace(bead.Description) != "" {
		b.WriteString(bead.Description)
		b.WriteString("\n\n")
	} else {
		b.WriteString("_(no description; infer from the title and acceptance criteria)_\n\n")
	}

	if strings.TrimSpace(bead.Acceptance) != "" {
		b.WriteString("## Acceptance criteria\n\n")
		b.WriteString(bead.Acceptance)
		b.WriteString("\n\n")
	}

	b.WriteString("## Operating rules\n\n")
	b.WriteString("1. Edit ONLY files inside this worktree. Do not touch unrelated packages.\n")
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
	b.WriteString("7. Do not run `bd update`, `bd close`, or `bd open`. The orchestrator advances the bead when your stage completes.\n")
	b.WriteString("8. DO NOT push. DO NOT switch branches. DO NOT touch `master`.\n\n")
	b.WriteString("If a tool call is auto-rejected (e.g. 'permission requested: external_directory'), STOP and switch to an in-worktree path immediately — do NOT keep retrying the rejected path; the rejection means opencode will not allow it this session.\n\n")
	b.WriteString("If you cannot proceed because of a missing dependency, fail loud with a descriptive error and stop. Do not invent stubs.\n\n")

	b.WriteString("## Bead metadata\n\n")
	fmt.Fprintf(&b, "- ID: `%s`\n", bead.ID)
	fmt.Fprintf(&b, "- Priority: `P%d`\n", bead.Priority)
	if bead.Type != "" {
		fmt.Fprintf(&b, "- Type: `%s`\n", bead.Type)
	}
	fmt.Fprintf(&b, "- Worktree: `%s`\n", worktree)
	fmt.Fprintf(&b, "- Canonical bd repo: `%s`\n", repoPath)

	return b.String()
}
