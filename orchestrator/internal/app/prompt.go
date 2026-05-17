package app

import (
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// BuildBeadStagePrompt produces the prompt sent to the agent for one bead
// at one workflow stage. Mirrors scripts/swarm/swarm_parallel.py:build_prompt
// in spirit — the agent receives the bead's intent plus the operating rules
// it must obey, including the bd status advancement that ends the stage.
//
// nextState is the workflow state the agent must transition to when the
// stage's work is done; empty means there is no forward transition (the
// agent should leave the bead at its current state).
//
// repoPath is the canonical bd repo (NOT the worktree) — that is where
// `bd update --status` will run when the agent completes.
func BuildBeadStagePrompt(bead *backend.Bead, currentState, nextState, repoPath, worktree string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Bead %s — %s\n\n", bead.ID, bead.Title)

	b.WriteString("You are an autonomous engineer executing ONE workflow stage of ONE bead from the kernl orchestrator. ")
	b.WriteString("The cwd you are running in is a git worktree dedicated to this bead. ")
	b.WriteString("Follow the Steps below exactly and stop when this stage is complete.\n\n")

	b.WriteString("## Stage\n\n")
	fmt.Fprintf(&b, "- Current workflow state: `%s`\n", currentState)
	if nextState != "" {
		fmt.Fprintf(&b, "- On success, advance the bead to: `%s`\n", nextState)
	} else {
		b.WriteString("- This stage has no forward transition; finish your work and exit.\n")
	}
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
	b.WriteString("2. Follow `AGENTS.md`: files < 500 lines, funcs 4–40 lines, fail-loud marker `KERNL DISPATCH FAILURE: <problem> — <cause> — Fix: <action>`.\n")
	b.WriteString("3. Tests must be hermetic (`*_test.go`) using fakes/stubs. No real network, no real disk outside `t.TempDir()`.\n")
	b.WriteString("4. The Go module lives at `orchestrator/go.mod`. Before declaring done, run:\n")
	b.WriteString("   ```bash\n")
	b.WriteString("   cd orchestrator && go vet ./... && go test ./...\n")
	b.WriteString("   ```\n")
	b.WriteString("   If ANY test fails, you are NOT done. Fix and re-run.\n")
	b.WriteString("5. Commit your work in this worktree:\n")
	b.WriteString("   ```bash\n")
	b.WriteString("   git add -A && git commit -m \"<conventional message>\"\n")
	b.WriteString("   ```\n")
	b.WriteString("6. DO NOT push. DO NOT switch branches. DO NOT touch `master`. DO NOT run `bd close` — the orchestrator does that.\n")

	if nextState != "" {
		b.WriteString("7. **CRITICAL — advance the bead state when your stage is done:**\n")
		b.WriteString("   ```bash\n")
		fmt.Fprintf(&b, "   bd -C %s update %s --status %s\n", repoPath, bead.ID, nextState)
		b.WriteString("   ```\n")
		b.WriteString("   The orchestrator polls `bd` for the bead status; if you do not advance it, the run will fail loud with `stuck at state` and the work will be marked blocked.\n")
	}
	b.WriteString("8. If you cannot proceed because of a missing dependency, fail loud with a descriptive error and stop. Do not invent stubs.\n\n")

	b.WriteString("## Bead metadata\n\n")
	fmt.Fprintf(&b, "- ID: `%s`\n", bead.ID)
	fmt.Fprintf(&b, "- Priority: `P%d`\n", bead.Priority)
	if bead.Type != "" {
		fmt.Fprintf(&b, "- Type: `%s`\n", bead.Type)
	}
	fmt.Fprintf(&b, "- Worktree: `%s`\n", worktree)
	fmt.Fprintf(&b, "- Canonical bd repo (for `bd update`): `%s`\n", repoPath)

	return b.String()
}
