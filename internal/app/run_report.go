package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/dispatch"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

// ImpactComposer writes the decision record's fifth field - impact on using
// the tool. It exists as an interface, not a direct call to
// dispatch.CompleteChat, so a test can inject a named fake instead of
// spawning a real HTTP request (see local/artifacts/orchestrator-autonomy-decision-model.md
// §5: this is deliberately a different actor, at a different time, than the
// implementer who writes the other four fields).
type ImpactComposer interface {
	ComposeImpact(ctx context.Context, in DecisionImpact) (string, error)
}

// DecisionImpact is what the composer needs to translate an implementer's
// decision record into what it changes for someone using the software: the
// four implementer-written fields (OptionsConsidered/TradeOffs recovered by
// SplitDecisionBody, since Body has no dedicated field for either), plus the
// repository and the bead title the decision was recorded against - facts
// available to the composer at run close that were not available to the
// implementer looking at one function in isolation.
type DecisionImpact struct {
	DecisionTitle     string
	DecisionContext   string
	OptionsConsidered string
	TradeOffs         string
	Outcome           string
	RepoPath          string
	BeadTitle         string
	// RepositoryContext is what AssembleContext read out of the repository
	// at RepoPath - the same text for every decision in one run, computed
	// once by ComposeRunReport rather than per decision, since neither the
	// repository nor its configured contextDocs change mid-run.
	RepositoryContext string
}

// impactComposerMaxTokens is a truncation guard, not a design budget: the
// answer's real shape is governed by prompt.RenderImpactOnUse's own
// instruction (3-6 sentences), and this only exists because
// dispatch.CompleteChat marshals max_tokens unconditionally and the
// Anthropic API requires the field - there is no "unbounded" to fall back
// to. 1024 is generous headroom over 3-6 sentences of prose, wide enough
// that the instruction in the prompt is what actually shapes the answer.
const impactComposerMaxTokens = 1024

// LLMImpactComposer is the production ImpactComposer: it renders
// DecisionImpact through prompt.RenderImpactOnUse and asks the configured
// LLM to answer it via dispatch.CompleteChat - the one function in this
// repository that already knows how to reach a provider and unwrap its
// response envelope, so this wrapper carries none of that itself.
type LLMImpactComposer struct {
	LLM config.LLMConfig
}

// ComposeImpact implements ImpactComposer.
//
// A response that is empty only after trimming (whitespace, or a single
// newline) is treated as a failure, not as "the composer ran and had
// nothing to add": CompleteChat itself already rejects a raw-empty
// completion as an error rather than a valid zero-length answer, and a
// model that answered with nothing but whitespace is exhibiting the same
// failure (truncation, a refusal with no text), not making the deliberate
// judgment call *""* is reserved for. Persisting it as "" would tell a
// later reader the composer looked at this decision and had nothing to
// say, which is not what happened.
func (c LLMImpactComposer) ComposeImpact(ctx context.Context, in DecisionImpact) (string, error) {
	return c.Ask(ctx, prompt.RenderImpactOnUse(prompt.ImpactOnUseInput{
		DecisionTitle:     in.DecisionTitle,
		DecisionContext:   in.DecisionContext,
		OptionsConsidered: in.OptionsConsidered,
		TradeOffs:         in.TradeOffs,
		Outcome:           in.Outcome,
		RepoPath:          in.RepoPath,
		BeadTitle:         in.BeadTitle,
		RepositoryContext: in.RepositoryContext,
	}))
}

// Ask implements Oracle: one question to the configured provider, one answer
// back. It carries the same token bound and the same emptiness rule as
// ComposeImpact, because both questions want a few sentences and neither has
// any use for a blank one.
func (c LLMImpactComposer) Ask(ctx context.Context, question string) (string, error) {
	text, err := dispatch.CompleteChat(ctx, c.LLM, question, impactComposerMaxTokens)
	if err != nil {
		return "", err
	}
	return nonEmptyCompletion(text)
}

// nonEmptyCompletion rejects a response that is only whitespace.
//
// dispatch.CompleteChat already refuses a completion that is the empty
// string, but it checks before trimming, so "   \n" reaches here. Passing
// that through would trim to "" and be persisted as a pointer to the empty
// string - which in this design means the composer ran and deliberately had
// nothing to add (see nodes.Decision.ImpactOnUse). A model answering
// whitespace decided nothing of the sort; it truncated or refused. Treating
// it as a failure puts it in the awaiting path, where an absent answer
// belongs, and matches how CompleteChat already treats an empty one.
//
// Split out from ComposeImpact so this rule is testable without a network
// call: everything above it in that method reaches an HTTP endpoint, which
// a unit test may not do (AGENTS.md §4).
func nonEmptyCompletion(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", fmt.Errorf("the model's response was empty after trimming whitespace")
	}
	return trimmed, nil
}

// impactComposeTimeout bounds a single ComposeImpact call. context.Background()
// has no deadline of its own, and dispatch.CompleteChat's http.DefaultClient
// carries none either, so without this an endpoint that accepts the
// connection and never answers hangs the whole run close forever: no report,
// no CloseWorkflowRun, no error, no exit - strictly worse than the
// unreachable-oracle case this design otherwise already tolerates. 60 seconds
// is generous for a 2-4 sentence completion and short enough that one dead
// endpoint costs the operator under a minute of an otherwise-finished run,
// not an indefinite hang.
const impactComposeTimeout = 60 * time.Second

// BeadRunOutcome is one bead a run drove and the terminal state it landed
// on. ComposeRunReport has no tracker client of its own, so the caller
// resolves this - from the tracker for an epic's children, or from a
// standalone bead's own drive result - before calling in.
type BeadRunOutcome struct {
	ID         string
	Title      string
	FinalState string
}

// ComposeRunReportInput carries everything ComposeRunReport needs. RunID is
// the only way it locates its subject: the run's entry point, repository and
// base branch are read back from the workflow_run node itself
// (StartWorkflowRun already wrote them), not re-supplied here, so this
// cannot drift from what the run actually recorded. Status/FinishedAt/Beads
// are NOT yet on that node when this runs - ComposeRunReport is called
// before CloseWorkflowRun writes them - so those remain the caller's own,
// freshly-observed facts.
//
// Only DecisionImpact ever reaches Composer - none of these facts do, per §5
// of the decision model this implements: the composer supplies the
// translation, never the technical record.
type ComposeRunReportInput struct {
	Graph    *graph.Graph
	Composer ImpactComposer

	RunID string
	// Status is the run's terminal status, exactly as the caller is about to
	// pass to CloseWorkflowRun ("completed" or "failed") - the report is
	// composed before the run record closes, so this is the caller's own
	// verdict, not a value read back from the graph.
	Status     string
	FinishedAt time.Time
	Beads      []BeadRunOutcome

	// PRURL is the pull request URL the shipment stage wrote onto the epic
	// bead's description (workflow.SetPRURL), the same field the shipment
	// exit gate and verifyPublishedPullRequest (cmd/kernl/epic.go) already
	// read via workflow.GetPRURL. ComposeRunReport does not parse it itself
	// - it is composed before CloseWorkflowRun, same as Status, so this is
	// the caller's own freshly-read fact, not one this function re-derives.
	// See runEpicRun/runBeadDispatch, which re-fetch the bead rather than
	// reuse the copy taken before the run started, since shipment writes it
	// mid-run.
	//
	// It lands in two places: the run's own report header, and one column of
	// this run's line in the epic's report.md index, so the index can point
	// at a shipped pull request without the reader opening the run's file.
	// Empty whenever shipment never ran or never got that far; both places
	// stay silent about it rather than printing an empty field.
	PRURL string

	// StateDir and EpicID locate this run's report on disk, at
	// <StateDir>/run/<EpicID>/runs/<RunID>.md - the same run root
	// resolveArtifactDir (drive_bead.go) and AppendStageAttempt
	// (attempt_ledger.go) use. EpicID is tracker data this project does not
	// own (see isSafePathComponent's own doc comment): the epic's own id for
	// `epic run`, the standalone bead's own id for `bead run`.
	// <StateDir>/run/<EpicID>/report.md is a second file this same call
	// writes: not this run's report, but the epic's index over every run
	// that ever wrote one - see writeRunReport's own doc comment.
	StateDir string
	EpicID   string

	// ContextDocs is registry.repos[].contextDocs for the repository this run
	// worked in - what AssembleContext reads to give the Oracle something to
	// judge field 4 against. Empty falls back to DefaultContextDocs.
	ContextDocs []string
}

// runDecision is one decision belonging to a run, paired with the title of
// the bead it was recorded against - the bead name a report reader needs
// beside the decision, and BeadReference never carries that in the Decision
// node itself. isFixup mirrors that bead reference's own IsFixup: a decision
// recorded by a Phase 6 fix-up bead's own implementation stage, the one case
// the operator never saw the bead that produced it.
type runDecision struct {
	decision  *nodes.Decision
	beadTitle string
	isFixup   bool
}

// ComposeRunReport is the run-close composer: it finds every decision
// belonging to the run, asks the oracle (in.Composer) to write field 4 for
// whichever ones do not already have it, writes a genuine answer back onto
// the Decision node, and renders one Markdown report for the whole run.
//
// The oracle being unreachable - no llm.provider configured, the call itself
// failing, or the call outliving impactComposeTimeout - never fails this
// function and never writes "" in place of a real answer: an unresolved
// field 4 stays nil on the node (see nodes.Decision.ImpactOnUse's own doc
// comment on why nil and "" must stay distinguishable) and the report says
// plainly, for that decision, that it is awaiting the composer and why.
// This is a deliberate exception to AGENTS.md §2's fail-loud rule: by the
// time a run closes, code is already committed and the tracker is already
// updated, so halting the close over a prose field would fail a run that,
// in every way that matters, succeeded. Everything else here - an unopened
// graph, an unknown run id, a broken traversal, a report that cannot be
// written to disk - is a genuine bug and still fails loud: the caller is
// expected to surface that error as a real dispatch failure, not swallow it
// the way an unresolved field 4 is swallowed.
func ComposeRunReport(ctx context.Context, in ComposeRunReportInput) (string, error) {
	if in.Graph == nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: no graph is open, so run %s's report cannot find its decisions - Fix: this is an App wiring bug (App.Graph must be set by NewAppForRepo/NewApp), not a config value to change", in.RunID)
	}
	if in.RunID == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: composing a run report with no run id")
	}

	var wr *nodes.WorkflowRun
	var found []runDecision
	if err := in.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		wr, err = nodes.GetWorkflowRun(ctx, tx, in.RunID)
		if err != nil {
			return err
		}
		found, err = findRunDecisions(ctx, tx, in.RunID)
		return err
	}); err != nil {
		if errors.Is(err, graph.ErrNotFound) {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: composing a report for run %s: no such workflow run exists in the graph - Fix: this is a caller bug (RunID must be exactly what StartWorkflowRun returned for this dispatch), not a config value to change", in.RunID)
		}
		return "", err
	}

	var rd runData
	if err := json.Unmarshal([]byte(wr.RunData), &rd); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: run %s's stored run data does not parse as JSON: %w - Fix: this indicates StartWorkflowRun wrote a malformed blob, a bug to fix there, not a value to paper over here", in.RunID, err)
	}

	// Assembled once per run, not once per decision: the repository and its
	// configured contextDocs do not change between decisions in the same run,
	// and re-reading the same files from disk once per decision would be
	// pure waste.
	repoContext := AssembleContext(rd.RepoPath, resolveContextDocs(in.ContextDocs))

	fields := make([]decisionReportFields, 0, len(found))
	for _, r := range found {
		impact := resolveImpactField(ctx, in.Graph, in.Composer, rd.RepoPath, repoContext, r.decision, r.beadTitle)
		fields = append(fields, buildDecisionReportFields(r.decision, impact, r.isFixup))
	}
	sortFixupDecisionsFirst(fields)

	return writeRunReport(in, len(fields), renderRunReport(in, rd, fields))
}

// findRunDecisions walks the single edge WriteDecisionRecordNode creates
// from a run to each decision it recorded (workflow_run --has_decision-->
// decision), rather than the two-hop path through the run's beads a
// bead_reference node's own EdgeTypeHasDecision edges would suggest.
//
// bead_reference nodes are persistent and shared across every run that ever
// touches that tracker id - that is the whole point of the bridge - so
// has_decision edges accumulate on one forever, across runs. Reaching
// decisions through a bead would mean a resumed or re-dispatched run over
// the same bead picks up every decision any earlier run ever recorded
// against it, and reports them as its own - the run stops being the report's
// unit. The direct run edge scopes a decision to exactly the run that
// produced it, by construction: decisionRecordNodeID folds the run id into
// the Decision node's own id, so two different runs cannot even converge on
// the same node for byte-identical content, let alone share one by
// traversal.
func findRunDecisions(ctx context.Context, tx *graph.ReadTx, runID string) ([]runDecision, error) {
	hasDecision, err := edges.Outgoing(ctx, tx, runID, edges.WithType(edges.EdgeTypeHasDecision))
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: listing decisions for run %s: %w", runID, err)
	}

	out := make([]runDecision, 0, len(hasDecision))
	for _, de := range hasDecision {
		d, err := nodes.GetDecision(ctx, tx, de.Dst)
		if err != nil {
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: reading decision %s for run %s: %w", de.Dst, runID, err)
		}
		beadTitle, isFixup, err := beadRefForDecision(ctx, tx, de.Dst, runID)
		if err != nil {
			return nil, err
		}
		out = append(out, runDecision{decision: d, beadTitle: beadTitle, isFixup: isFixup})
	}
	return out, nil
}

// beadRefForDecision recovers which bead a decision was recorded against -
// for the report's "Bead:" context handed to the composer - and whether it
// was recorded by a Phase 6 fix-up bead. WriteDecisionRecordNode links a
// decision to its run AND to the bead (and, when different, the epic) that
// produced it, all via EdgeTypeHasDecision, so a child bead's own decision
// has TWO non-run incoming edges: its own bead_reference and its epic's.
//
// title takes the first non-run edge found (unchanged from before this
// function also answered isFixup): when bead and epic are the same tracker
// id there is only one candidate, and when they differ either is an
// accurate answer to "which bead" - the choice between them is not a
// behavior this function pins down, and edges.Incoming makes no ordering
// guarantee between two edges created in the same transaction.
//
// isFixup CANNOT use that same "first one" shortcut: an epic is never
// itself a fix-up bead, so if the query happened to return the epic's edge
// before the child's, reading isFixup off of it would silently read false
// for a decision a fix-up bead genuinely recorded - exactly the
// nondeterminism the title case tolerates becoming a wrong answer instead
// of an equally-valid one. It is instead OR'd across every non-run
// reference this decision links to, which is order-independent and correct
// either way: a real fix-up bead's own reference has IsFixup true regardless
// of which edge is visited first, and an epic's reference never sets it.
func beadRefForDecision(ctx context.Context, tx *graph.ReadTx, decisionID, runID string) (title string, isFixup bool, err error) {
	in, err := edges.Incoming(ctx, tx, decisionID, edges.WithType(edges.EdgeTypeHasDecision))
	if err != nil {
		return "", false, fmt.Errorf("KERNL DISPATCH FAILURE: reading bead links for decision %s: %w", decisionID, err)
	}
	titleSet := false
	for _, e := range in {
		if e.Src == runID {
			continue
		}
		ref, err := nodes.GetBeadReference(ctx, tx, e.Src)
		if err != nil {
			return "", false, fmt.Errorf("KERNL DISPATCH FAILURE: reading bead reference %s for decision %s: %w", e.Src, decisionID, err)
		}
		if !titleSet {
			title = ref.Title
			titleSet = true
		}
		if ref.IsFixup {
			isFixup = true
		}
	}
	return title, isFixup, nil
}

// resolveContextDocs falls back to DefaultContextDocs when a repository
// declares no registry.repos[].contextDocs of its own.
func resolveContextDocs(configured []string) []string {
	if len(configured) == 0 {
		return DefaultContextDocs
	}
	return configured
}

// resolveImpactField answers field 4 for one decision's report entry, and is
// the only place that decides between a real answer and the "awaiting"
// placeholder. It never returns an error: every failure mode (no composer
// configured, the composer erroring or timing out, the graph write failing)
// is folded into the placeholder text, plus a warning to stderr naming what
// happened - see ComposeRunReport's own doc comment for why this path must
// never halt the run.
func resolveImpactField(ctx context.Context, g *graph.Graph, composer ImpactComposer, repoPath, repoContext string, d *nodes.Decision, beadTitle string) string {
	if d.ImpactOnUse != nil {
		if *d.ImpactOnUse == "" {
			return "(the composer already ran for this decision and had nothing to add.)"
		}
		return *d.ImpactOnUse
	}

	text, reason := composeAndPersistImpact(ctx, g, composer, repoPath, repoContext, d, beadTitle)
	if reason == "" {
		return text
	}
	fmt.Fprintf(os.Stderr, "warning: KERNL DISPATCH: decision %s's impact-on-use is still awaiting the composer - %s\n", d.ID, reason)
	return fmt.Sprintf("(awaiting the composer: %s.)", reason)
}

// composeAndPersistImpact asks composer for a decision's field 4 and writes
// a genuine answer back onto the node. reason is empty on success; a
// non-empty reason means text is empty and the decision's ImpactOnUse was
// deliberately left nil, per the "never write \"\" on a failure" rule this
// unit exists to enforce.
//
// The composer call runs under its own bounded context
// (impactComposeTimeout), not the caller's ctx directly: this is the one
// call in the whole close path that leaves the process (an HTTP request to
// whatever kernl.yaml names), and it is the only one with no deadline of its
// own otherwise - see impactComposeTimeout's doc comment. A timed-out call
// is just another composer error and lands in the same awaiting path.
func composeAndPersistImpact(ctx context.Context, g *graph.Graph, composer ImpactComposer, repoPath, repoContext string, d *nodes.Decision, beadTitle string) (text, reason string) {
	if composer == nil {
		return "", "no LLM provider is configured for this run - set llm.provider in kernl.yaml to enable field 4"
	}

	// A false ok means Body was not the exact-length-prefixed shape
	// buildDecisionBody produces (see SplitDecisionBody's own doc comment).
	// The composer is not asked with half its input silently blank in that
	// case - decision %s's own reason, not a generic composer failure,
	// names precisely which decision to look at.
	options, tradeOffs, ok := SplitDecisionBody(d.Body)
	if !ok {
		return "", fmt.Sprintf("decision %s's options-considered and trade-offs could not be recovered from its stored record, so field 4 cannot be composed from them", d.ID)
	}
	composeCtx, cancel := context.WithTimeout(ctx, impactComposeTimeout)
	defer cancel()
	impact, err := composer.ComposeImpact(composeCtx, DecisionImpact{
		DecisionTitle:     d.Title,
		DecisionContext:   d.Context,
		OptionsConsidered: options,
		TradeOffs:         tradeOffs,
		Outcome:           d.Outcome,
		RepoPath:          repoPath,
		BeadTitle:         beadTitle,
		RepositoryContext: repoContext,
	})
	if err != nil {
		return "", fmt.Sprintf("composing this decision's impact failed: %v", err)
	}

	updated := *d
	updated.ImpactOnUse = &impact
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.UpdateDecision(ctx, tx, updated, runRecordAuthor)
	}); err != nil {
		return "", fmt.Sprintf("the composer answered but saving it to decision %s failed: %v", d.ID, err)
	}

	return impact, ""
}

// decisionReportFields is one decision already resolved into the report's
// five headings - renderRunReport only formats these, it never decides
// content, so the two concerns (what to say, how to lay it out) stay
// separable.
type decisionReportFields struct {
	title             string
	whatWasDecided    string
	optionsConsidered string
	tradeOffs         string
	impactOnUse       string
	rationale         string
	// fromFixup marks a decision recorded by a Phase 6 fix-up bead's own
	// implementation stage - see sortFixupDecisionsFirst's own doc comment
	// for why this changes reading order and nothing about execution.
	fromFixup bool
}

// decisionBodyUnrecoverableText is what a report shows for
// optionsConsidered/tradeOffs when SplitDecisionBody cannot recover them -
// the same "say so, don't guess" choice renderRevertedDecisionConstraint
// already makes for the same failure (see decision_record.go). A blank
// string there would read as "nothing was considered", which is a different
// and false claim from "this could not be recovered".
const decisionBodyUnrecoverableText = "(could not be recovered from this decision's stored record.)"

// buildDecisionReportFields maps a Decision node plus its already-resolved
// field 4 onto the decision model's §4.3 five-field shape. SplitDecisionBody
// recovers options-considered and trade-offs from Body using the exact-length
// preamble buildDecisionBody wrote (see decision_record.go); ok is false
// only for a Body this package itself did not build (hand-edited, foreign,
// or predating that format), and the report says so explicitly rather than
// rendering blank options/trade-offs that would read as "none were
// considered".
func buildDecisionReportFields(d *nodes.Decision, impactOnUse string, fromFixup bool) decisionReportFields {
	options, tradeOffs, ok := SplitDecisionBody(d.Body)
	if !ok {
		options, tradeOffs = decisionBodyUnrecoverableText, decisionBodyUnrecoverableText
	}
	return decisionReportFields{
		title:             d.Title,
		whatWasDecided:    d.Context,
		optionsConsidered: options,
		tradeOffs:         tradeOffs,
		impactOnUse:       impactOnUse,
		rationale:         d.Outcome,
		fromFixup:         fromFixup,
	}
}

// sortFixupDecisionsFirst reorders fields so every fix-up-bead decision comes
// before every original-bead decision, preserving each group's own relative
// order (sort.SliceStable) - a fix-up bead is created mid-run with no
// operator involvement (§7: "the one bead that does not satisfy §1's
// premise"), so its decisions are the one place in a run the operator had no
// prior context at all, and that is where attention is worth the most. This
// changes reading order only; it does not change which decisions exist or
// how they were arrived at.
func sortFixupDecisionsFirst(fields []decisionReportFields) {
	sort.SliceStable(fields, func(i, j int) bool {
		return fields[i].fromFixup && !fields[j].fromFixup
	})
}

// renderRunReport composes the report's prose from facts kernl already
// holds - the header (partly read back from the run node itself, via rd;
// partly the caller's own close-time facts, via in) and the summary - plus
// each decision's five resolved fields. Only the decisions' field 4 ever
// passed through an LLM; everything else here is written deterministically,
// per ComposeRunReport's own doc comment on why the header and summary are
// never sent to the composer to be prettified.
func renderRunReport(in ComposeRunReportInput, rd runData, fields []decisionReportFields) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Run Report: %s\n\n", in.RunID)
	fmt.Fprintf(&b, "- **Entry point:** %s\n", rd.EntryPoint)
	fmt.Fprintf(&b, "- **Repository:** %s\n", rd.RepoPath)
	if rd.BaseBranch != "" {
		fmt.Fprintf(&b, "- **Base branch:** %s\n", rd.BaseBranch)
	}
	fmt.Fprintf(&b, "- **Status:** %s\n", in.Status)
	if in.PRURL != "" {
		fmt.Fprintf(&b, "- **Pull request:** %s\n", in.PRURL)
	}
	fmt.Fprintf(&b, "- **Started:** %s\n", rd.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- **Finished:** %s\n", in.FinishedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- **Duration:** %s\n\n", in.FinishedAt.Sub(rd.StartedAt).Truncate(time.Second).String())

	b.WriteString("## Summary\n\n")
	if len(in.Beads) == 0 {
		b.WriteString("No beads were driven by this run.\n\n")
	}
	for _, bead := range in.Beads {
		fmt.Fprintf(&b, "- %s (%q) reached %s\n", bead.ID, bead.Title, bead.FinalState)
	}
	b.WriteString("\n")

	b.WriteString("## Decisions\n\n")
	if len(fields) == 0 {
		b.WriteString("No decisions were recorded for this run.\n")
		return b.String()
	}
	for i, f := range fields {
		fmt.Fprintf(&b, "### %d. %s\n\n", i+1, f.title)
		if f.fromFixup {
			b.WriteString("**This decision was recorded by a fix-up bead the operator never saw created.**\n\n")
		}
		b.WriteString("**What was being decided**\n\n")
		fmt.Fprintf(&b, "%s\n\n", f.whatWasDecided)
		b.WriteString("**Options considered**\n\n")
		fmt.Fprintf(&b, "%s\n\n", f.optionsConsidered)
		b.WriteString("**Trade-offs**\n\n")
		fmt.Fprintf(&b, "%s\n\n", f.tradeOffs)
		b.WriteString("**Impact on using the tool**\n\n")
		fmt.Fprintf(&b, "%s\n\n", f.impactOnUse)
		b.WriteString("**Why the chosen option won**\n\n")
		fmt.Fprintf(&b, "%s\n\n", f.rationale)
	}
	return b.String()
}

// resolveEpicDir is the shared root every artifact this package keeps for
// one epic (or standalone bead) lives under: <StateDir>/run/<EpicID>, the
// same run root resolveArtifactDir (drive_bead.go) and
// resolveAttemptLedgerPath (attempt_ledger.go) use. It applies the two
// path-safety checks every caller building a path from EpicID needs -
// isSafePathComponent and escapesRoot - once, here, because EpicID is
// tracker data this project does not own (see isSafePathComponent's own doc
// comment), rather than once per caller.
func resolveEpicDir(stateDir, epicID string) (string, error) {
	if stateDir == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: no state directory for the run report - Fix: set DriveBeadDeps.StateDir (app.DefaultStateDir() outside tests)")
	}
	if !isSafePathComponent(epicID) {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: unsafe epic id %q for the run report path - Fix: the id must be a single path segment with no '/', '\\', '.', or '..'", epicID)
	}

	runRoot := filepath.Join(stateDir, "run")
	epicDir := filepath.Join(runRoot, epicID)
	if escapesRoot(runRoot, epicDir) {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: run report dir %s for epic %q escapes %s - Fix: epic id %q must resolve to a path beneath it", epicDir, epicID, runRoot, epicID)
	}

	if err := os.MkdirAll(epicDir, 0o755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating run report dir %s: %w", epicDir, err)
	}
	return epicDir, nil
}

// runReportPath resolves where ComposeRunReport writes THIS run's own
// report: <StateDir>/run/<EpicID>/runs/<RunID>.md. RunID is kernl's own
// UUIDv7 (StartWorkflowRun / internal/graph/internal/ids.New), not tracker
// data, so in practice it is already a safe single path segment - but
// isSafePathComponent is applied anyway, at no real cost, rather than
// trusting an id this function did not itself mint.
func runReportPath(stateDir, epicID, runID string) (string, error) {
	epicDir, err := resolveEpicDir(stateDir, epicID)
	if err != nil {
		return "", err
	}
	if !isSafePathComponent(runID) {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: unsafe run id %q for the run report path - Fix: the id must be a single path segment with no '/', '\\', '.', or '..'", runID)
	}

	runsDir := filepath.Join(epicDir, "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating run report dir %s: %w", runsDir, err)
	}
	return filepath.Join(runsDir, runID+".md"), nil
}

// writeRunReport writes body to this run's own report path and returns it
// unchanged (an operator or a test reads this as pointing at the run's own
// report - see ComposeRunReportInput's own doc comment), then folds one line
// describing this run into <StateDir>/run/<EpicID>/report.md, the epic's
// index over every run that ever wrote one.
//
// Before this function existed, report.md WAS the run's report, and every
// run overwrote the previous one's: an epic shipped across several
// invocations lost every earlier invocation's decisions the moment the last
// one closed (see this file's own package doc for the incident that forced
// the change). Splitting the two apart keeps ComposeRunReport's original
// contract - "the run is the unit, one whole report arrives" - while making
// report.md answer a question no single run's file ever could: what did this
// epic do across every run it took.
func writeRunReport(in ComposeRunReportInput, decisionCount int, body string) (string, error) {
	path, err := runReportPath(in.StateDir, in.EpicID, in.RunID)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing run report to %s: %w", path, err)
	}

	if err := updateRunIndex(in, decisionCount); err != nil {
		return "", err
	}
	return path, nil
}
