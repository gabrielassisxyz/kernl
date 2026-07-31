package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
}

// impactComposerMaxTokens bounds the mayor's answer to a few sentences - see
// prompt.RenderImpactOnUse's own instruction for the shape it asks for.
const impactComposerMaxTokens = 512

// LLMImpactComposer is the production ImpactComposer: it renders
// DecisionImpact through prompt.RenderImpactOnUse and asks the configured
// LLM to answer it via dispatch.CompleteChat - the one function in this
// repository that already knows how to reach a provider and unwrap its
// response envelope, so this wrapper carries none of that itself.
type LLMImpactComposer struct {
	LLM config.LLMConfig
}

// ComposeImpact implements ImpactComposer.
func (c LLMImpactComposer) ComposeImpact(ctx context.Context, in DecisionImpact) (string, error) {
	p := prompt.RenderImpactOnUse(prompt.ImpactOnUseInput{
		DecisionTitle:     in.DecisionTitle,
		DecisionContext:   in.DecisionContext,
		OptionsConsidered: in.OptionsConsidered,
		TradeOffs:         in.TradeOffs,
		Outcome:           in.Outcome,
		RepoPath:          in.RepoPath,
		BeadTitle:         in.BeadTitle,
	})
	text, err := dispatch.CompleteChat(ctx, c.LLM, p, impactComposerMaxTokens)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

// BeadRunOutcome is one bead a run drove and the terminal state it landed
// on. ComposeRunReport has no tracker client of its own, so the caller
// resolves this - from the tracker for an epic's children, or from a
// standalone bead's own drive result - before calling in.
type BeadRunOutcome struct {
	ID         string
	Title      string
	FinalState string
}

// ComposeRunReportInput carries everything ComposeRunReport needs: which run
// to report on, the composer to hand field 4 to (nil when no LLM is
// configured for this run), and the header/summary facts kernl already holds
// and writes deterministically. Only DecisionImpact ever reaches Composer -
// these facts do not, per §5 of the decision model this implements: the
// composer supplies the translation, never the technical record.
type ComposeRunReportInput struct {
	Graph    *graph.Graph
	Composer ImpactComposer

	RunID      string
	EntryPoint string
	RepoPath   string
	BaseBranch string
	// Status is the run's terminal status, exactly as the caller is about to
	// pass to CloseWorkflowRun ("completed" or "failed") - the report is
	// composed before the run record closes, so this is the caller's own
	// verdict, not a value read back from the graph.
	Status     string
	StartedAt  time.Time
	FinishedAt time.Time
	Beads      []BeadRunOutcome

	// StateDir and EpicID locate the report on disk, at
	// <StateDir>/run/<EpicID>/report.md - the same run root
	// resolveArtifactDir (drive_bead.go) and AppendStageAttempt
	// (attempt_ledger.go) use. EpicID is tracker data this project does not
	// own (see isSafePathComponent's own doc comment): the epic's own id for
	// `epic run`, the standalone bead's own id for `bead run`.
	StateDir string
	EpicID   string
}

// runDecision is one decision reachable from a run's beads, paired with the
// title of whichever bead reference the traversal reached it through - the
// bead name a report reader needs beside the decision, and BeadReference
// never carries that in the Decision node itself.
type runDecision struct {
	decision  *nodes.Decision
	beadTitle string
}

// ComposeRunReport is the run-close composer: it finds every decision the
// run's beads recorded, asks the mayor (in.Composer) to write field 4 for
// whichever ones do not already have it, writes a genuine answer back onto
// the Decision node, and renders one Markdown report for the whole run.
//
// The mayor being unreachable - no llm.provider configured, or the call
// itself failing - never fails this function and never writes "" in place
// of a real answer: an unresolved field 4 stays nil on the node (see
// nodes.Decision.ImpactOnUse's own doc comment on why nil and "" must stay
// distinguishable) and the report says plainly, for that decision, that it
// is awaiting the composer and why. This is a deliberate exception to
// AGENTS.md §2's fail-loud rule: by the time a run closes, code is already
// committed and the tracker is already updated, so halting the close over a
// prose field would fail a run that, in every way that matters, succeeded.
// Everything else here - an unopened graph, a broken traversal, a report
// that cannot be written to disk - is a genuine bug and still fails loud.
func ComposeRunReport(ctx context.Context, in ComposeRunReportInput) (string, error) {
	if in.Graph == nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: no graph is open, so run %s's report cannot find its decisions - Fix: this is an App wiring bug (App.Graph must be set by NewAppForRepo/NewApp), not a config value to change", in.RunID)
	}
	if in.RunID == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: composing a run report with no run id")
	}

	var found []runDecision
	if err := in.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		found, err = findRunDecisions(ctx, tx, in.RunID)
		return err
	}); err != nil {
		return "", err
	}

	fields := make([]decisionReportFields, 0, len(found))
	for _, rd := range found {
		impact := resolveImpactField(ctx, in.Graph, in.Composer, in.RepoPath, rd.decision, rd.beadTitle)
		fields = append(fields, buildDecisionReportFields(rd.decision, impact))
	}

	return writeRunReport(in.StateDir, in.EpicID, renderRunReport(in, fields))
}

// findRunDecisions walks workflow_run --ran_bead--> bead_reference
// --has_decision--> decision - the traversal the decision model's §4.4/§5
// prescribes, and the only one this package uses to locate a run's
// decisions. Re-deriving them any other way (e.g. scanning every Decision
// node for a matching bead id) would silently diverge from what
// WriteDecisionRecordNode actually wrote, and could pick up a decision that
// merely shares a bead id with a different run.
//
// WriteDecisionRecordNode links both a child bead and its epic to the same
// Decision node; since epic run's own bead list includes the epic itself,
// the traversal can reach one decision through two of the run's own beads.
// seen deduplicates so the report lists each decision once, keyed to
// whichever bead's edge reached it first (edges.Outgoing's own deterministic
// ordering).
func findRunDecisions(ctx context.Context, tx *graph.ReadTx, runID string) ([]runDecision, error) {
	ranBead, err := edges.Outgoing(ctx, tx, runID, edges.WithType(edges.EdgeTypeRanBead))
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: listing beads run %s drove: %w", runID, err)
	}

	var out []runDecision
	seen := map[string]bool{}
	for _, be := range ranBead {
		hasDecision, err := edges.Outgoing(ctx, tx, be.Dst, edges.WithType(edges.EdgeTypeHasDecision))
		if err != nil {
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: listing decisions for bead %s: %w", be.Dst, err)
		}
		for _, de := range hasDecision {
			if seen[de.Dst] {
				continue
			}
			seen[de.Dst] = true

			d, err := nodes.GetDecision(ctx, tx, de.Dst)
			if err != nil {
				return nil, fmt.Errorf("KERNL DISPATCH FAILURE: reading decision %s for run %s: %w", de.Dst, runID, err)
			}
			ref, err := nodes.GetBeadReference(ctx, tx, be.Dst)
			if err != nil {
				return nil, fmt.Errorf("KERNL DISPATCH FAILURE: reading bead reference %s for run %s: %w", be.Dst, runID, err)
			}
			out = append(out, runDecision{decision: d, beadTitle: ref.Title})
		}
	}
	return out, nil
}

// resolveImpactField answers field 4 for one decision's report entry, and is
// the only place that decides between a real answer and the "awaiting"
// placeholder. It never returns an error: every failure mode (no composer
// configured, the composer erroring, the graph write failing) is folded into
// the placeholder text, plus a warning to stderr naming what happened - see
// ComposeRunReport's own doc comment for why this path must never halt the
// run.
func resolveImpactField(ctx context.Context, g *graph.Graph, composer ImpactComposer, repoPath string, d *nodes.Decision, beadTitle string) string {
	if d.ImpactOnUse != nil {
		if *d.ImpactOnUse == "" {
			return "(the composer already ran for this decision and had nothing to add.)"
		}
		return *d.ImpactOnUse
	}

	text, reason := composeAndPersistImpact(ctx, g, composer, repoPath, d, beadTitle)
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
func composeAndPersistImpact(ctx context.Context, g *graph.Graph, composer ImpactComposer, repoPath string, d *nodes.Decision, beadTitle string) (text, reason string) {
	if composer == nil {
		return "", "no LLM provider is configured for this run - set llm.provider in kernl.yaml to enable field 4"
	}

	options, tradeOffs, _ := SplitDecisionBody(d.Body)
	impact, err := composer.ComposeImpact(ctx, DecisionImpact{
		DecisionTitle:     d.Title,
		DecisionContext:   d.Context,
		OptionsConsidered: options,
		TradeOffs:         tradeOffs,
		Outcome:           d.Outcome,
		RepoPath:          repoPath,
		BeadTitle:         beadTitle,
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
}

// buildDecisionReportFields maps a Decision node plus its already-resolved
// field 4 onto the decision model's §4.3 five-field shape. SplitDecisionBody
// recovers options-considered and trade-offs from Body using the same
// heading-aware parse buildDecisionBody's own boundary relies on (see
// decision_record.go) - not a second, hand-rolled split.
func buildDecisionReportFields(d *nodes.Decision, impactOnUse string) decisionReportFields {
	options, tradeOffs, _ := SplitDecisionBody(d.Body)
	return decisionReportFields{
		title:             d.Title,
		whatWasDecided:    d.Context,
		optionsConsidered: options,
		tradeOffs:         tradeOffs,
		impactOnUse:       impactOnUse,
		rationale:         d.Outcome,
	}
}

// renderRunReport composes the report's prose from facts kernl already
// holds (the header and the summary) plus each decision's five resolved
// fields. Only the decisions' field 4 ever passed through an LLM; everything
// else here is written deterministically, per ComposeRunReport's own doc
// comment on why the header and summary are never sent to the composer to be
// prettified.
func renderRunReport(in ComposeRunReportInput, fields []decisionReportFields) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Run Report: %s\n\n", in.RunID)
	fmt.Fprintf(&b, "- **Entry point:** %s\n", in.EntryPoint)
	fmt.Fprintf(&b, "- **Repository:** %s\n", in.RepoPath)
	if in.BaseBranch != "" {
		fmt.Fprintf(&b, "- **Base branch:** %s\n", in.BaseBranch)
	}
	fmt.Fprintf(&b, "- **Status:** %s\n", in.Status)
	fmt.Fprintf(&b, "- **Started:** %s\n", in.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- **Finished:** %s\n", in.FinishedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- **Duration:** %s\n\n", in.FinishedAt.Sub(in.StartedAt).Truncate(time.Second).String())

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

// runReportPath resolves where ComposeRunReport writes its output:
// <StateDir>/run/<EpicID>/report.md, the same run root resolveArtifactDir
// (drive_bead.go) and resolveAttemptLedgerPath (attempt_ledger.go) use. It
// reuses their two path-safety checks rather than a third copy -
// isSafePathComponent and escapesRoot - because EpicID is tracker data this
// project does not own (see isSafePathComponent's own doc comment).
func runReportPath(stateDir, epicID string) (string, error) {
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
	return filepath.Join(epicDir, "report.md"), nil
}

// writeRunReport writes body to the run's report path and returns it, so
// the CLI caller can print where the report landed.
func writeRunReport(stateDir, epicID, body string) (string, error) {
	path, err := runReportPath(stateDir, epicID)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing run report to %s: %w", path, err)
	}
	return path, nil
}
