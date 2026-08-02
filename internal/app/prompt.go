package app

import (
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/review"
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
	// RejectedReview is the full text of an implementation review that
	// ended in "VERDICT: REJECT", carried into the implementer's prompt when
	// the bead has been sent back to be fixed. Empty is the normal case -
	// a first attempt, or a review that passed - and renders nothing at all,
	// unlike RelevantDecisions: "no reviewer has rejected this" is the
	// default state of the world and does not need saying, while "no prior
	// decision was found" is a search result worth reporting.
	RejectedReview string
	// ForkHandoverPath is the absolute path an implementer may write a fork
	// handover to, when it meets a choice the bead, this repository's own
	// docs and existing precedent do not already determine (see
	// app.ForkHandover). Empty renders nothing at all, and empty is the
	// NORMAL case: it is empty whenever no DA is configured (the fork gate
	// is off) or the active stage is not an implementer's stage (a reviewer
	// stage never carries a decision_record exit gate, and this field is
	// only ever set when one is armed - see app.forkHandoverArmed). A stage
	// that cannot hand a fork over must never be told it can; the field
	// being unset is what enforces that, not a check the prompt text makes
	// on its own.
	ForkHandoverPath string
	// ForkAnswer is EVERY fork decision this same bead has handed over so
	// far, in the order they were decided (see app.readForkAnswerArtifact),
	// carried into the prompt when this stage is being re-entered to answer
	// it. More than one is a real, planned-for case - forkHandoverLimit's own
	// doc comment records why a single stage can genuinely meet more than one
	// distinct fork. Empty is the normal case - a bead that never handed a
	// fork over, or one whose implementer resolved every choice alone - and
	// renders nothing at all, the same silence RejectedReview's own doc
	// comment explains: "no fork was handed over" needs no saying.
	ForkAnswer string
}

// The two verdict lines a review artifact can end with. They are the exit
// gate's own vocabulary (see backend.evaluateSingleExitGate), named here so
// the prompt that asks for them and the gate that reads them cannot drift
// apart in wording.
const (
	passVerdictLine   = "VERDICT: PASS"
	rejectVerdictLine = "VERDICT: REJECT"
)

// BuildBeadStagePrompt produces the prompt sent to the agent for one bead
// at one workflow stage. When in.Stages has a contract for in.State it
// renders a contract-aware prompt; otherwise it falls back to a generic
// engineer prompt.
func BuildBeadStagePrompt(in StagePromptInput) string {
	contract, hasContract := in.Stages[in.State]

	var b strings.Builder
	fmt.Fprintf(&b, "# Bead %s - %s\n\n", in.Bead.ID, in.Bead.Title)

	renderRole(&b, hasContract, contract)
	renderRejectedReview(&b, in.RejectedReview)
	renderForkAnswer(&b, in.ForkAnswer)
	renderRelatedDecisions(&b, in.RelevantDecisions)
	renderInputs(&b, hasContract, contract, in.Bead.ID, in.ArtifactDir)
	renderOutput(&b, hasContract, contract, in.State, in.Bead.ID, in.ArtifactDir)
	renderDecisionRecord(&b, hasContract, contract, in.Bead.ID, in.ArtifactDir)
	renderForkHandover(&b, in.ForkHandoverPath)
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

func renderOutput(b *strings.Builder, hasContract bool, contract backend.StageContract, state, beadID, artifactDir string) {
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
		renderRejectionVerdict(b, state, artifact.MustEndWith)
	}
	b.WriteString("\n")
}

// renderRejectionVerdict tells a reviewer how to spell the other outcome.
//
// A stage told only how to write approval writes whatever it likes for the
// opposite, and the exit gate reads exactly one word. A review that ended
// "VERDICT: FAIL" - accurate English, with reproducible findings under it -
// was read as a review that produced no verdict at all, so the bead was
// blocked for a human instead of going back to the implementer with those
// findings, and the rewind budget was never touched. The gate's own word had
// only ever been written down in the custom integration_review prompt, which
// is why that stage never hit this.
func renderRejectionVerdict(b *strings.Builder, state, mustEndWith string) {
	if !backend.IsRejectableVerdictState(state) || mustEndWith != passVerdictLine {
		return
	}
	fmt.Fprintf(b, "If it does NOT pass, end with this line instead, exactly: `%s`\n", rejectVerdictLine)
	b.WriteString("That is the only wording read as a deliberate rejection, and it is what sends the work back to be fixed with your review as the brief. Anything else - FAIL, NEEDS WORK, CHANGES REQUESTED, a verdict line you word yourself - is read as a review that produced no verdict, which blocks the bead for a human and throws your findings away.\n")
	renderRejectionClassification(b, state)
}

// renderRejectionClassification teaches a reviewer it may classify a
// rejection, using review.All() to render the literal vocabulary so this
// text and the parser that reads it back (ParseImplementationRejection) can
// never drift apart - the same reuse internal/prompt's integration_review
// template already makes for the OTHER reviewer's own classification.
//
// Gated on state == "implementation_review" specifically, narrower than
// renderRejectionVerdict's own IsRejectableVerdictState check: the routing
// this teaches (handleGateFailure, review_decision_gate.go) only ever acts
// on that one state - integration_review has its own separate classification,
// already taught by internal/prompt's custom template, and a hypothetical
// custom workflow naming a generic-path stage "integration_review" would
// never be rewound here at all (isDeliberateRejection requires the literal
// state name), so teaching it this vocabulary would promise a mechanism that
// does not fire for it.
//
// This is the section a stage that is only told how to spell REJECT would
// never think to write on its own - see renderRejectionVerdict's own doc
// comment for the shipped precedent of exactly that failure (a stage told
// only the approval word invented its own word for the other outcome, and
// the gate never saw it). A reviewer never told it MAY classify a rejection
// will never classify one, and this whole pass becomes dead code.
func renderRejectionClassification(b *strings.Builder, state string) {
	if state != "implementation_review" {
		return
	}
	b.WriteString("\nIf you reject it, you may ALSO classify what the rejection needs, in a `## Classification` section written before the trailing VERDICT line. Exactly one word, one of:")
	for _, k := range review.All() {
		fmt.Fprintf(b, " `%s`", k)
	}
	b.WriteString(".\n\nAsk yourself: can the implementer answer this alone, or does answering it mean choosing something nothing already determined? If the implementer can answer it alone, classify it `fixup` - this is exactly today's behavior, unchanged: the work is rewound to the implementer with this review as its brief. If answering it means choosing between real alternatives nothing here already settles, classify it `decision`, and add a `## Question for the operator` section stating exactly what must be decided and why the answer is not already determined - this routes the choice to the same DA a proactive fork handover reaches, instead of the implementer choosing alone.\n\n")
	b.WriteString("Classifying is OPTIONAL, and the default is deliberately the safe one: omitting the `## Classification` section, or writing anything this cannot read as one of the words above, is read exactly like `fixup` - the work is rewound to the implementer, nothing escalates and nobody is asked anything. Only an explicit, well-formed `decision` (with its `## Question for the operator` section) ever reaches the operator's own decision-making assistant.\n")
}

// renderDecisionRecord tells the agent about the second, independent
// required output a stage's contract can declare via DecisionRecord: a
// document, not commits and not a review verdict. Only stages that set it
// (today, only "implementation") render this section.
//
// The field names and the JSON shape named here are exactly what
// backend.ParseDecisionRecordDocument requires - the prompt and the gate
// must keep naming the same fields, or an agent that followed this text to
// the letter would still fail the gate. The array shape exists because a
// fixed, single-decision document left an implementer facing more than one
// decision to invent its own way of fitting a second one in (measured: three
// different, gate-incompatible syntaxes from three agents in one afternoon,
// each rejected with a misleading "missing: decision") - telling the agent
// to add another array entry instead removes the need to invent anything.
func renderDecisionRecord(b *strings.Builder, hasContract bool, contract backend.StageContract, beadID, artifactDir string) {
	if !hasContract || contract.DecisionRecord.Path == "" {
		return
	}
	resolved := backend.ResolveArtifactPath(contract.DecisionRecord.Path, beadID, artifactDir)
	b.WriteString("## Decision record\n\n")
	b.WriteString("Any decision this stage makes that was not already settled for you - a choice between approaches, a new dependency, a changed default, anything a future reader would ask \"why this and not that\" about - gets written down BEFORE you write the code for it, not after. A record written afterward is a justification, not a decision; the ordering is the point.\n\n")
	fmt.Fprintf(b, "Enumerate the options you considered for each such decision, then write the following file as JSON: `%s`\n\n", resolved)
	b.WriteString("It must be a single JSON object with a `decisions` array containing at least one entry - one entry per decision, so if this stage makes more than one, add another entry rather than inventing a way to fit both into one. Each entry needs all four of these fields, each a non-empty string:\n\n")
	b.WriteString("- `decision` - what was being decided\n")
	b.WriteString("- `optionsConsidered` - the options you weighed\n")
	b.WriteString("- `tradeOffs` - what each option costs and gains\n")
	b.WriteString("- `rationale` - why the winner won\n\n")
	b.WriteString("`title` is optional on each entry - a short label, worth adding once the record holds more than one decision so each is easy to tell apart.\n\n")
	b.WriteString("Example:\n\n```json\n{\n  \"decisions\": [\n    {\n      \"title\": \"short label\",\n      \"decision\": \"what was decided\",\n      \"optionsConsidered\": \"the options weighed\",\n      \"tradeOffs\": \"what each option costs and gains\",\n      \"rationale\": \"why the winner won\"\n    }\n  ]\n}\n```\n\n")
	b.WriteString("Do not add a field about this record's impact on how the tool gets used day to day - that is written separately, later, by a different step, not by you.\n\n")
}

// renderForkHandover tells an implementer it may hand a genuine fork over to
// the DA instead of choosing alone - see app.ForkHandover and
// local/artifacts/plans/2026-08-01-composer-context-and-fork-gate-plan.md
// §3.1-§3.2.
//
// Empty renders nothing at all - see StagePromptInput.ForkHandoverPath on why
// this section is silent whenever the gate is off or this is not an
// implementer's stage: a stage that cannot hand a fork over must never be
// told it can.
func renderForkHandover(b *strings.Builder, path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	b.WriteString("## A genuine fork may be handed over instead of decided alone\n\n")
	b.WriteString("Most choices while implementing this bead are already determined by the bead itself, this repository's own tests and docs, and existing precedent in its code - those are lookups, not forks, and handing one over wastes a round. This is for a genuine fork only: the same standard your own decision record already asks for, where the options are both real and nothing already settles which one wins.\n\n")
	fmt.Fprintf(b, "When you meet one, write the following file instead of deciding alone: `%s`\n\n", path)
	b.WriteString("It must contain exactly these three sections as markdown headings, each followed by real, non-empty content:\n\n")
	b.WriteString("- `## Fork` - the choice you cannot make alone\n")
	b.WriteString("- `## Options Considered` - the real options you weighed\n")
	b.WriteString("- `## What Would Have To Agree` - answered as a FACT, not an opinion: does anything OUTSIDE this bead have to agree with the choice? This is explicitly NOT blast radius. A large rewrite entirely behind an interface can be huge in lines and need agreement from nobody; a one-line change to a stored data format can be tiny in lines and need everyone downstream to agree. If nothing outside this bead has to agree, say so explicitly rather than leaving this section vague.\n\n")
	b.WriteString("Then STOP. Do not commit, do not implement either option, do not guess which one is right. The stage will be re-entered with the DA's answer once it has decided.\n\n")
	b.WriteString("THIS OVERRIDES THE OPERATING RULES BELOW FOR THIS CASE ONLY: the Operating Rules section below tells you to commit your work and run this repository's verify command before declaring done. Neither applies once you have written the fork file above - do not commit, and do not run the verify command, until the stage is re-entered with the DA's own answer. The operating rules are still correct for every OTHER path through this stage; they are suspended only for the fork you just handed over.\n\n")
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
// renderRejectedReview puts a reviewer's rejection at the top of the
// implementer's prompt, above everything else the stage says.
//
// Position is the point. This bead has been implemented once already and sent
// back; an implementer who reads the bead description first will start by
// re-deriving work that exists, and find the objection only after deciding
// how to proceed. The objection IS the task now.
//
// Empty renders nothing at all - see StagePromptInput.RejectedReview on why
// this section is silent by default while "related decisions" is not.
func renderRejectedReview(b *strings.Builder, review string) {
	if strings.TrimSpace(review) == "" {
		return
	}
	b.WriteString("## This work was reviewed and rejected - read this first\n\n")
	b.WriteString("You are not implementing this bead from scratch. A previous implementation exists on this branch, a reviewer rejected it, and the rejection below is what you have to answer. Fix what it names. Do not start over, and do not re-litigate the bead's scope: if you believe the reviewer is wrong, say so in your decision record and explain why, rather than ignoring the objection.\n\n")
	b.WriteString("```\n")
	b.WriteString(strings.TrimSpace(review))
	b.WriteString("\n```\n\n")
}

// renderForkAnswer puts the DA's decision(s) at the top of the implementer's
// prompt, immediately after renderRejectedReview and for the same reason
// that function documents: the answer IS the task now, and an implementer
// that reads the bead description first will start by re-deriving a question
// that has already been settled.
//
// answer may carry MORE THAN ONE decided fork (see
// StagePromptInput.ForkAnswer, app.readForkAnswerArtifact) - the wording
// below deliberately speaks in the plural and says "every" rather than "the",
// so an implementer reading it cannot mistake the instruction as covering
// only the last entry in the block below.
//
// Empty renders nothing at all - see StagePromptInput.ForkAnswer on why this
// section is silent by default, the same rule RejectedReview follows.
func renderForkAnswer(b *strings.Builder, answer string) {
	if strings.TrimSpace(answer) == "" {
		return
	}
	b.WriteString("## A fork you handed over has been decided - read this first\n\n")
	b.WriteString("You stopped mid-stage and handed one or more forks to the DA instead of choosing alone. Every one below has been decided, and EACH of them, individually, is not open for re-litigation: do not re-derive any of these choices, do not weigh their options again, and do not pick a different one because you would have gone another way. Proceed with every option named below exactly as decided, and write each into your decision record as the decision that was made and why - the DA's own reason is what belongs there, not a reconstruction of your own.\n\n")
	b.WriteString("```\n")
	b.WriteString(strings.TrimSpace(answer))
	b.WriteString("\n```\n\n")
}

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
