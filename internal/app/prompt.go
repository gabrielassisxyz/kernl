package app

import (
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// StagePromptInput is everything one stage's prompt is rendered from.
//
// It is a struct rather than a parameter list because two of its fields are
// facts about the repository being worked on (VerifyCommand) and the agent
// running (Dialect) that the prompt used to answer for itself, on kernl's
// behalf, for every repository at once.
type StagePromptInput struct {
	Bead   *backend.Bead
	State  string
	Stages map[string]backend.StageContract
	// RepoPath is the canonical tracker repo (NOT the worktree).
	RepoPath string
	Worktree string
	// VerifyCommand is how this repository says "this works". Resolved per
	// repository by epic.ResolveVerifyCommand.
	VerifyCommand string
	// Dialect selects the notes that apply to one agent CLI and are noise to
	// the rest.
	Dialect adapter.AgentDialect
	// TrackerCommand is how this repository's tracker is typed from inside a
	// worktree, binary and store-pinning flags together. The prompt names the
	// tracker in prose, and which one it is is a property of the repository:
	// "bd" was written into these strings when kernl was the only repository
	// the orchestrator ever ran against.
	TrackerCommand string
	// ArtifactDir is where kernl writes this bead's exit-gate artifacts -
	// absolute, and deliberately outside Worktree, so a stage's own
	// `git add <files>` can never sweep kernl's control files into the
	// target repository's commits. A stage contract's Path/Inputs entries
	// that use the <artifact_dir> placeholder are expanded against it.
	ArtifactDir string
	// RelevantDecisions carries standing decisions from earlier beads in
	// this repository whose vocabulary overlaps this bead's own title and
	// description (see FetchRelevantDecisions), so this bead's implementer
	// sees an answer that was already settled instead of re-deriving it and
	// landing on the alternative that was already rejected. A nil or empty
	// slice is a legitimate value - nothing relevant was found, or the graph
	// could not be read - and renderRelatedDecisions renders an explicit
	// "none found" line for it either way, rather than silently omitting
	// the section (see that function's own doc comment for why).
	RelevantDecisions []RelevantDecision
}

// BuildBeadStagePrompt produces the prompt sent to the agent for one bead
// at one workflow stage. When in.Stages has a contract for in.State it
// renders a contract-aware prompt; otherwise it falls back to a generic
// engineer prompt.
func BuildBeadStagePrompt(in StagePromptInput) string {
	contract, hasContract := in.Stages[in.State]

	var b strings.Builder
	fmt.Fprintf(&b, "# Bead %s - %s\n\n", in.Bead.ID, in.Bead.Title)

	renderRole(&b, hasContract, contract)
	renderRelatedDecisions(&b, in.RelevantDecisions)
	renderInputs(&b, hasContract, contract, in.Bead.ID, in.ArtifactDir)
	renderOutput(&b, hasContract, contract, in.Bead.ID, in.ArtifactDir)
	renderDecisionRecord(&b, hasContract, contract, in.Bead.ID, in.ArtifactDir)
	renderForbidden(&b, hasContract, contract, in.TrackerCommand)
	renderOperatingRules(&b, in.VerifyCommand, in.Dialect)

	if !hasContract {
		b.WriteString("## Steps (from the bead description)\n\n")
	}
	renderBeadData(&b, in.Bead, hasContract)

	b.WriteString("## Bead metadata\n\n")
	fmt.Fprintf(&b, "- ID: `%s`\n", in.Bead.ID)
	fmt.Fprintf(&b, "- Priority: `P%d`\n", in.Bead.Priority)
	if in.Bead.Type != "" {
		fmt.Fprintf(&b, "- Type: `%s`\n", in.Bead.Type)
	}
	fmt.Fprintf(&b, "- Worktree (this IS your current working directory - reference files by paths relative to it; do not retype the absolute path): `%s`\n", in.Worktree)
	fmt.Fprintf(&b, "- Canonical tracker repo (read-only, lives outside your worktree - never cd or write here): `%s`\n", in.RepoPath)

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

func renderInputs(b *strings.Builder, hasContract bool, contract backend.StageContract, beadID, artifactDir string) {
	if !hasContract || len(contract.Inputs) == 0 {
		return
	}
	b.WriteString("## Inputs available to you\n\n")
	for _, inp := range contract.Inputs {
		resolved := backend.ResolveArtifactPath(inp, beadID, artifactDir)
		fmt.Fprintf(b, "- %s\n", resolved)
	}
	b.WriteString("\nSome inputs may not exist this run - e.g. planning was skipped, so there is no `plan.md`. If a listed file is absent, proceed WITHOUT it: review against the committed changes in your worktree (`git log -p`, `git diff`) and the acceptance criteria below. An input path outside your worktree is kernl's own artifact directory for this bead, named above as an absolute path - it is allowed. NEVER search for a missing input anywhere else outside your worktree (the canonical repo, other beads); it is not there and the access will be auto-rejected.\n\n")
}

func renderOutput(b *strings.Builder, hasContract bool, contract backend.StageContract, beadID, artifactDir string) {
	if !hasContract {
		return
	}
	artifact := contract.OutputArtifact
	// A "commits" stage (implementation, integration, shipment) used to also
	// carry a required marker string for this section to print - the same
	// vocabulary the commit_marker exit gate scanned commit messages for.
	// Now that the gate only asks whether the stage left any new commit, kind
	// alone has nothing left to instruct here: skip the section rather than
	// print an empty "## Required output" header.
	if artifact.Path == "" && artifact.MustEndWith == "" {
		return
	}
	b.WriteString("## Required output\n\n")
	if artifact.Path != "" {
		// The destination may be outside your worktree (kernl's own
		// artifact directory) - resolved to an absolute path here, not left
		// relative, because your cwd is the worktree and a relative path
		// would write the file back into it, exactly the leak this
		// directory exists to prevent.
		resolved := backend.ResolveArtifactPath(artifact.Path, beadID, artifactDir)
		fmt.Fprintf(b, "Write the following file: `%s`\n", resolved)
	}
	if artifact.MustEndWith != "" {
		fmt.Fprintf(b, "The output must end with: `%s`\n", artifact.MustEndWith)
	}
	b.WriteString("\n")
}

// renderDecisionRecord tells the agent about the second, independent
// required output a stage's contract can declare via DecisionRecord: a
// document, not commits and not a review verdict. Only stages that set it
// (today, only "implementation") render this section.
//
// The four section headings named here are exactly what
// missingDecisionRecordSections in internal/backend/state_machine.go looks
// for (case-insensitively, punctuation-insensitively) - the prompt and the
// gate must keep naming the same four things, or an agent that followed this
// text to the letter would still fail the gate.
func renderDecisionRecord(b *strings.Builder, hasContract bool, contract backend.StageContract, beadID, artifactDir string) {
	if !hasContract || contract.DecisionRecord.Path == "" {
		return
	}
	resolved := backend.ResolveArtifactPath(contract.DecisionRecord.Path, beadID, artifactDir)
	b.WriteString("## Decision record\n\n")
	b.WriteString("Any decision this stage makes that was not already settled for you - a choice between approaches, a new dependency, a changed default, anything a future reader would ask \"why this and not that\" about - gets written down BEFORE you write the code for it, not after. A record written afterward is a justification, not a decision; the ordering is the point.\n\n")
	fmt.Fprintf(b, "Enumerate the options you considered for each such decision, then write the following file: `%s`\n\n", resolved)
	b.WriteString("It must contain all four of these sections as markdown headings, each followed by real, non-empty content:\n\n")
	b.WriteString("- `## Decision` - what was being decided\n")
	b.WriteString("- `## Options Considered` - the options you weighed\n")
	b.WriteString("- `## Trade-offs` - what each option costs and gains\n")
	b.WriteString("- `## Rationale` - why the winner won\n\n")
	b.WriteString("Do not add a section about this record's impact on how the tool gets used day to day - that is written separately, later, by a different step, not by you.\n\n")
}

// renderRelatedDecisions surfaces standing decisions from earlier work in
// this repository whose vocabulary overlaps this bead - see
// FetchRelevantDecisions for the relevance criterion - so an implementer
// sees an answer that was already settled instead of re-deriving it and
// landing on the alternative that was already rejected. That exact failure
// is what motivated this section: two beads, days apart, independently
// named the same enum variant and picked opposite names, one of them the
// name the other had already considered and rejected in a decision record
// nothing downstream ever read.
//
// Rendered unconditionally, even when decisions is empty: omitting the
// section entirely when nothing was found is indistinguishable, to a reader
// of the prompt, from this feature never having run at all - which is
// exactly the silent failure mode that let the original defect through. One
// explicit line costs nothing and removes that ambiguity.
func renderRelatedDecisions(b *strings.Builder, decisions []RelevantDecision) {
	b.WriteString("## Related decisions already made\n\n")
	if len(decisions) == 0 {
		b.WriteString("No related decisions were found recorded in this repository for this bead's own title and description.\n\n")
		return
	}
	b.WriteString("These were already decided in this repository, on other beads. Do not silently re-derive or contradict them: if you disagree, say so in your own decision record instead of quietly choosing differently.\n\n")
	for _, d := range decisions {
		fmt.Fprintf(b, "- **%s** - %s\n", d.Title, strings.TrimSpace(d.Outcome))
	}
	b.WriteString("\n")
}

func renderForbidden(b *strings.Builder, hasContract bool, contract backend.StageContract, trackerCommand string) {
	b.WriteString("## You may NOT\n\n")
	if hasContract {
		for _, fp := range contract.ForbiddenPaths {
			fmt.Fprintf(b, "- Modify `%s`\n", fp)
		}
	}
	fmt.Fprintf(b, "- Do not run `%s update`, `%s close`, or `%s reopen`. The orchestrator advances the bead when your stage completes.\n", trackerCommand, trackerCommand, trackerCommand)
	b.WriteString("\n")
}

// renderOperatingRules states the contract of the stage: where the agent may
// work, how the work is checked, and how the stage ends.
//
// It deliberately says nothing about a language, a directory layout or a
// coding convention. It used to say all three - Go tests, a module at
// `orchestrator/go.mod` that no longer exists, and kernl's own file and
// function size limits - which is a description of this repository being read
// out to an agent working in someone else's. The conventions come from the
// target repository's own AGENTS.md, and the check comes from its own verify
// command.
func renderOperatingRules(b *strings.Builder, verifyCommand string, dialect adapter.AgentDialect) {
	b.WriteString("## Operating rules\n\n")
	b.WriteString("1. Your cwd IS this bead's worktree. Reference files by paths relative to cwd, not absolute paths - hand-retyping the absolute worktree path (and dropping a hidden segment) is the #1 cause of rejected out-of-worktree file access. Edit ONLY files inside this worktree; do not touch unrelated parts of the tree.\n")
	b.WriteString("2. Scratch files (search output, inventory lists, anything intermediate): write them INSIDE the worktree (e.g. `./_scratch/<name>`) - NEVER `/tmp/*`. Several observed bails came from agents trying to write outside the worktree.\n")
	b.WriteString("3. This repository has its own conventions and they are the ones that apply. Read `AGENTS.md` in your worktree (or `CONTRIBUTING.md`, or the README) and follow what it says about style, testing and commit messages. Nothing about how any other project works applies here.\n")
	fmt.Fprintf(b, "4. Before declaring done, run this repository's own check:\n   ```bash\n   %s\n   ```\n   If it fails, you are NOT done. Fix and re-run.\n", verifyCommand)
	b.WriteString("5. Commit your work in this worktree. Stage the files you changed BY NAME; never `git add -A`, which sweeps up files that are not yours - including the orchestrator's own control files and anything a build left behind.\n")
	b.WriteString("6. DO NOT push. DO NOT switch branches. Commit only onto the branch already checked out in this worktree; leave every other branch alone.\n\n")
	if dialect == adapter.DialectOpenCode {
		b.WriteString("If a tool call is auto-rejected (e.g. 'permission requested: external_directory'), STOP and switch to an in-worktree path immediately - do NOT keep retrying the rejected path; the rejection means opencode will not allow it this session.\n\n")
	}
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
