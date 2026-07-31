package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// PhaseThreeDecisionTag marks a Decision node created from a stage's
// decision_record exit gate - a deliberated, bead-scoped design decision,
// not an auto-approved permission (that is dispatch.LogAutonomousDecision's
// "autonomous" tag, a separate and still-dead path this one does not touch
// or replace). internal/api/audit.go filters on this tag.
const PhaseThreeDecisionTag = "phase3_decision"

// decisionRecordRequiredKeys mirrors the canonical section keys
// backend.DecisionRecordSectionBodies can return - kept here, not derived
// from the backend package, because that list is private to the gate's own
// package and this is only the read side re-checking its own re-read.
var decisionRecordRequiredKeys = []string{"decision", "options_considered", "trade_offs", "rationale"}

// Headings that fold "options considered" and "trade-offs" into
// Decision.Body, the only field the schema offers for content that isn't
// Title, Context, or Outcome. Exported so a reader (the audit API, a future
// composer) can split Body back into its two parts by the same boundary this
// package used to build it, instead of guessing where one ends and the
// other begins.
const (
	DecisionBodyOptionsHeading   = "## Options Considered"
	DecisionBodyTradeOffsHeading = "## Trade-offs"
)

// buildDecisionBody folds options-considered and trade-offs content into one
// string under labeled headings, so the boundary between the two survives
// being stored in Decision's single opaque Body field.
func buildDecisionBody(optionsConsidered, tradeOffs string) string {
	return DecisionBodyOptionsHeading + "\n\n" + strings.TrimSpace(optionsConsidered) +
		"\n\n" + DecisionBodyTradeOffsHeading + "\n\n" + strings.TrimSpace(tradeOffs)
}

// SplitDecisionBody is buildDecisionBody's inverse: it recovers the
// options-considered and trade-offs text a Decision.Body built by this
// package folded together, using the same headings buildDecisionBody wrote.
//
// It re-parses Body with backend.DecisionRecordSectionBodies - the same
// block-aware pass the decision_record gate itself runs - rather than
// string-searching for the heading text. A naive strings.Index search finds
// "## Trade-offs" anywhere in the text, including inside a fenced code
// example the options text quotes verbatim, or as an inline mention in
// prose; either one truncates the real options content and hands the real
// separator's remainder to the wrong half. DecisionRecordSectionBodies
// already knows a heading inside a fence or a comment is not a heading, and
// that an ATX/setext heading must start its own line - exactly the
// distinction this split needs, and exactly the distinction a hand-rolled
// second parser would have to reinvent to get right.
//
// ok is false when Body does not parse into both parts (it was not built by
// buildDecisionBody), which callers should treat as "not individually
// recoverable" rather than guess at a split.
func SplitDecisionBody(body string) (optionsConsidered, tradeOffs string, ok bool) {
	sections := backend.DecisionRecordSectionBodies(body)
	options, hasOptions := sections["options_considered"]
	tradeOffsText, hasTradeOffs := sections["trade_offs"]
	if !hasOptions || !hasTradeOffs {
		return "", "", false
	}
	return options, tradeOffsText, true
}

// decisionTitleAndContext splits the record's "Decision" section (what was
// being decided) across Title and Context: Title is a short label for
// listings, Context is the full section text a reader would actually need to
// understand the decision.
func decisionTitleAndContext(decisionSection string) (title, decisionContext string) {
	trimmed := strings.TrimSpace(decisionSection)
	title = trimmed
	if idx := strings.IndexByte(trimmed, '\n'); idx != -1 {
		title = strings.TrimSpace(trimmed[:idx])
	}
	const maxTitleRunes = 120
	if r := []rune(title); len(r) > maxTitleRunes {
		title = string(r[:maxTitleRunes]) + "..."
	}
	return title, trimmed
}

// decisionFromRecordSections maps the four decision_record parts the gate
// already validated onto Decision's fields: "decision" becomes Title plus
// Context, "rationale" becomes Outcome exactly, and "options_considered"
// plus "trade_offs" are folded into Body (see buildDecisionBody) since the
// schema has no dedicated field for either. ImpactOnUse is left nil - it is
// written later, by the run's composer at run close, not here.
func decisionFromRecordSections(sections map[string]string, now time.Time) nodes.Decision {
	title, decisionContext := decisionTitleAndContext(sections["decision"])
	return nodes.Decision{
		CreatedAt: now,
		UpdatedAt: now,
		Title:     title,
		Context:   decisionContext,
		Body:      buildDecisionBody(sections["options_considered"], sections["trade_offs"]),
		Outcome:   strings.TrimSpace(sections["rationale"]),
		DecidedAt: now,
		Tags:      []string{PhaseThreeDecisionTag},
	}
}

// decisionRecordNodeID derives a stable node ID from exactly the facts that
// make two writes "the same decision": which bead, which epic, and the four
// section bodies themselves. A re-run of the same bead against the same
// record - the normal shape of a retry after Backend.Update fails following
// a successful graph write, see WriteDecisionRecordNode - hashes to the same
// ID and so converges on the one existing node instead of minting a fresh
// UUID and a fresh set of edges for every attempt. Genuinely different
// content (a real second decision, or the same bead revisiting the stage
// with a rewritten record) hashes to a different ID and gets its own node,
// same as it would with a random one.
func decisionRecordNodeID(beadID, epicID string, sections map[string]string) string {
	h := sha256.New()
	_, _ = io.WriteString(h, "bead:"+beadID+"\x00epic:"+epicID+"\x00")
	for _, key := range decisionRecordRequiredKeys {
		_, _ = io.WriteString(h, key+":"+sections[key]+"\x00")
	}
	return "decision-" + hex.EncodeToString(h.Sum(nil))
}

// ensureHasDecisionEdge creates a has_decision edge from src to dst unless
// one already exists - the edges table carries no uniqueness constraint on
// (src, dst, label), so an unconditional Create on every retry would mint a
// duplicate edge each time even once the node itself is deduplicated.
func ensureHasDecisionEdge(ctx context.Context, tx *graph.WriteTx, src, dst string, author nodes.Author) error {
	exists, err := edges.Exists(ctx, tx, src, dst, edges.EdgeTypeHasDecision)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = edges.Create(ctx, tx, edges.Edge{Src: src, Dst: dst, Type: edges.EdgeTypeHasDecision}, author)
	return err
}

// WriteDecisionRecordNode creates a Decision node from a decision record's
// extracted sections and links it to the bead and (when different) the epic
// it was written for via edges.EdgeTypeHasDecision, all inside one write
// transaction - a node with no link back to the work it explains is not
// meaningfully queryable later, so the two are written together or not at
// all.
//
// The node ID is content-addressed (decisionRecordNodeID), and both the node
// create and the edge creates are skipped when they already exist
// (nodes.Exists, ensureHasDecisionEdge) - a caller that retries this same
// call for the same bead and the same record (DriveBeadToTerminal does,
// whenever the graph write itself succeeds but the bead's state update fails
// afterward and the run is retried) converges on one node and its edges
// rather than accumulating one of each per attempt.
//
// Exported so a test proving the audit API surfaces this path's output (see
// internal/api/audit.go) can call the real write path instead of re-deriving
// fixture data by hand.
func WriteDecisionRecordNode(ctx context.Context, g *graph.Graph, sections map[string]string, beadID, epicID string) (string, error) {
	id := decisionRecordNodeID(beadID, epicID, sections)
	author := nodes.Author{Name: "kernl-dispatch"}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		exists, err := nodes.Exists(ctx, tx, id)
		if err != nil {
			return err
		}
		if !exists {
			d := decisionFromRecordSections(sections, time.Now())
			d.ID = id
			if _, err := nodes.CreateDecision(ctx, tx, d, author); err != nil {
				return err
			}
		}
		if err := ensureHasDecisionEdge(ctx, tx, beadID, id, author); err != nil {
			return err
		}
		if epicID != "" && epicID != beadID {
			if err := ensureHasDecisionEdge(ctx, tx, epicID, id, author); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing decision record node for bead %s: %w", beadID, err)
	}
	return id, nil
}

// readDecisionRecordSections re-reads the file a decision_record exit gate
// just validated and extracts its section bodies with
// backend.DecisionRecordSectionBodies - the exact function EvaluateExitGate
// used to decide the gate passed, so this read and the gate's own read can
// never disagree about what counts as a section (see AGENTS.md's
// "two parsers" caution and DecisionRecordSectionBodies' own doc comment).
// A read failure or a section coming back empty here, immediately after the
// gate reported success against the same path, means the file changed
// between the two reads or the two diverged - either way this is a hard
// stop, not a value to paper over: the whole point of writing this node is
// that the reasoning stays recoverable, and silently dropping a section
// defeats that.
func readDecisionRecordSections(gate backend.WorkflowExitGate, gateCtx backend.ExitGateContext, beadID string) (map[string]string, error) {
	if strings.Contains(gate.Path, "<artifact_dir>") && gateCtx.ArtifactDir == "" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: decision_record gate for bead %s names <artifact_dir> but no artifact directory was resolved for this run - Fix: this indicates a driver bug (artifactDir should already be resolved before gate evaluation), not a config problem", beadID)
	}
	abs := backend.ResolveArtifactFSPath(gate.Path, gateCtx.BeadID, gateCtx.WorktreePath, gateCtx.ArtifactDir)
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: decision_record gate passed for bead %s but the record at %s could not be re-read: %w", beadID, abs, err)
	}
	sections := backend.DecisionRecordSectionBodies(string(data))
	for _, key := range decisionRecordRequiredKeys {
		if sections[key] == "" {
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: decision_record gate passed for bead %s but section %q was empty re-reading %s - the gate and this read must never disagree - Fix: investigate what changed the file between the gate's read and this one", beadID, key, abs)
		}
	}
	return sections, nil
}

// recordDecisionIfGateType mirrors a just-passed decision_record exit gate's
// record into the graph as a queryable Decision node. It is a no-op for
// every other gate type, so DriveBeadToTerminal can call it unconditionally
// right after any gatePassed branch without checking the gate type itself
// first.
//
// A record that failed the gate never reaches here: EvaluateExitGate already
// refused to advance the bead, so this function is only ever invoked once
// per stage, after the gate that reads the same file already proved it well
// formed.
func recordDecisionIfGateType(ctx context.Context, wf backend.WorkflowDescriptor, gateCtx backend.ExitGateContext, cfg *config.Config, beadID, epicID string) error {
	gate, ok := wf.ExitGates[gateCtx.FromState]
	if !ok || gate.Type != "decision_record" {
		return nil
	}

	sections, err := readDecisionRecordSections(gate, gateCtx, beadID)
	if err != nil {
		return err
	}

	graphPath, err := graphDBFilePath(cfg)
	if err != nil {
		return err
	}
	g, err := graph.Open(ctx, graph.Config{Path: graphPath})
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: opening graph to record decision for bead %s: %w", beadID, err)
	}
	defer g.Close()

	_, err = WriteDecisionRecordNode(ctx, g, sections, beadID, epicID)
	return err
}
