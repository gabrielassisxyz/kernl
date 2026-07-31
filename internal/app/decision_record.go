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
// make two writes "the same decision": which run, which bead, which epic,
// and the four section bodies themselves. A re-run of the same bead against
// the same record within the same run - the normal shape of a retry after
// Backend.Update fails following a successful graph write, see
// WriteDecisionRecordNode - hashes to the same ID and so converges on the
// one existing node instead of minting a fresh UUID and a fresh set of
// edges for every attempt.
//
// runID is folded into the hash, not just recorded as an edge, so that two
// DIFFERENT runs can never converge on the same Decision node even when a
// bead somehow revisits the same stage with byte-identical content (a
// reverted-and-reopened bead per the decision model's §6, a fix-up bead). If
// the ID were content-only, such a case would silently re-link a decision
// that already belongs to an earlier run's report onto a second run,
// reintroducing exactly the cross-run leak WriteDecisionRecordNode's own doc
// comment on the run edge exists to prevent. Genuinely different content
// (a real second decision) hashes to a different ID and gets its own node
// regardless.
func decisionRecordNodeID(runID, beadID, epicID string, sections map[string]string) string {
	h := sha256.New()
	_, _ = io.WriteString(h, "run:"+runID+"\x00bead:"+beadID+"\x00epic:"+epicID+"\x00")
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

// BeadRef is the bridge WriteDecisionRecordNode needs to link a Decision to
// an orchestrator bead or epic: the tracker id edges.Create requires to
// already exist as a node, plus the three sourceable facts
// nodes.BeadReference is allowed to carry. It exists so a caller (only
// recordDecisionIfGateType today) can hand over exactly what it already has
// in hand - the bead fetched at the top of the drive loop, and the tracker
// command and repo path resolved once for the whole run - without this
// package re-deriving any of it.
type BeadRef struct {
	ID          string
	Title       string
	TrackerKind string
	RepoPath    string
	// IsFixup marks that ID names a Phase 6 fix-up bead (see
	// IntegrationFixupLabel), so the reference node created for it carries
	// that fact forward into the graph. Only ever meaningful for a bead's
	// own ref, never an epic's - an epic is never itself a fix-up bead.
	IsFixup bool
}

// validBeadReferenceTrackerKinds are the only tracker_kind values a bead
// reference node may ever store. A reference node is never updated after
// creation (see nodes.BeadReference), so a wrong value here is not a
// transient glitch - it is permanent and unfixable without a migration, and
// this check is the last point before it is written where that can be
// prevented. backend.MemoryManagerType also names "knots" as a supported
// tracker, deliberately excluded here: nothing has validated this write path
// against a knots-backed repository's tracker command shape, so rejecting it
// loudly is safer than writing an unverified value into an immutable node.
var validBeadReferenceTrackerKinds = map[string]bool{"br": true, "bd": true}

// trackerKindFromCommand extracts the tracker binary name from the
// invocation string DriveBeadDeps.TrackerCommand already carries, and
// validates it against validBeadReferenceTrackerKinds.
// backend.TrackerInvocation always builds that string as "<binary> <rest>"
// (see its own doc comment), so the first field is the kind - this is string
// surgery on data already resolved for the run, not a second call to
// backend.ResolveMemoryManager, which would mean re-detecting which tracker
// the repository uses.
func trackerKindFromCommand(trackerCommand string) (string, error) {
	fields := strings.Fields(trackerCommand)
	if len(fields) == 0 {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: tracker command is empty, so no tracker kind can be recorded on a bead reference node - Fix: resolve DriveBeadDeps.TrackerCommand with backend.TrackerInvocation before driving a bead")
	}
	kind := fields[0]
	if !validBeadReferenceTrackerKinds[kind] {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: tracker command %q names tracker kind %q, which a bead reference node cannot record - Fix: this write path only supports br and bd; a knots-backed repository is not yet supported here", trackerCommand, kind)
	}
	return kind, nil
}

// ensureBeadReferenceNode creates a reference node for ref, or is a no-op if
// one already exists at that id. It calls nodes.CreateBeadReference
// unconditionally rather than checking nodes.Exists first: every child bead
// of the same epic calls this for the same epic id, each through its own
// *graph.Graph (recordDecisionIfGateType opens one per call), so a
// check-then-insert here would race across those separate connections - two
// callers can both observe absence before either commits, and only one of
// their inserts would then succeed. nodes.CreateBeadReference's own insert
// is atomic and idempotent, so every caller converges on the same row
// without that window.
func ensureBeadReferenceNode(ctx context.Context, tx *graph.WriteTx, ref BeadRef, author nodes.Author) error {
	var missing []string
	if ref.Title == "" {
		missing = append(missing, "title")
	}
	if ref.TrackerKind == "" {
		missing = append(missing, "tracker kind")
	}
	if ref.RepoPath == "" {
		missing = append(missing, "repository path")
	}
	if len(missing) > 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: creating a reference node for bead %s: missing %s - Fix: this data must come from the bead already fetched by DriveBeadToTerminal and the DriveBeadDeps.TrackerCommand/RepoPath resolved for the run; a placeholder must never be substituted here", ref.ID, strings.Join(missing, ", "))
	}
	_, err := nodes.CreateBeadReference(ctx, tx, nodes.BeadReference{
		ID:          ref.ID,
		Title:       ref.Title,
		TrackerKind: ref.TrackerKind,
		Repository:  ref.RepoPath,
		IsFixup:     ref.IsFixup,
	}, author)
	return err
}

// WriteDecisionRecordNode creates a Decision node from a decision record's
// extracted sections and links it to the run, the bead, and (when different)
// the epic it was written for, all via edges.EdgeTypeHasDecision and all
// inside one write transaction - a node with no link back to the work it
// explains is not meaningfully queryable later, so all of it is written
// together or not at all.
//
// runID is mandatory and fails loud when empty, rather than degrading to a
// bead-and-epic-only write: this project's only production caller
// (recordDecisionIfGateType, via DriveBeadDeps.RunID) always has one, since
// both `epic run` and `bead run` open a workflow run before ever driving a
// bead, and a decision written with no run edge would be permanently
// unreachable to app.ComposeRunReport's own traversal (workflow_run
// --has_decision--> decision, one hop from the run) - silently losing part
// of the record the run's report exists to surface.
// EdgeTypeHasDecision is reused for the run edge rather than a new type: the
// direction convention is already "the container points at the contained",
// and a run containing a decision is the same relation a bead containing one
// is.
//
// edges.Create requires every endpoint to already exist as a node row.
// Orchestrator bead/epic ids are never otherwise mirrored into this graph
// (nodes.Task and nodes.Project are a distinct, human-authored concept - see
// task.go's own doc comment), so bead and epic (reference-node stubs, not
// the bead's full tracker state - see nodes.BeadReference) supply exactly
// what is needed to bring those two endpoints into existence via
// ensureBeadReferenceNode before their edges are attempted. The run node is
// different: it is a real nodes.WorkflowRun row StartWorkflowRun already
// created, never a stub minted here, so a runID that does not name an
// existing run node fails the same way any other broken reference would.
//
// The Decision node ID is content-addressed over the run id, bead id, epic
// id, and the four section bodies (decisionRecordNodeID) - folding runID
// into the hash, not just recording it as an edge, is what makes two
// different runs structurally unable to ever converge on the same Decision
// node; see that function's own doc comment. The reference node creates,
// the Decision create, and the edge creates are all skipped when they
// already exist (nodes.Exists, ensureBeadReferenceNode,
// ensureHasDecisionEdge) - a caller that retries this same call for the same
// run, the same bead, and the same record (DriveBeadToTerminal does,
// whenever the graph write itself succeeds but the bead's state update
// fails afterward and the same run is retried) converges on one of each
// rather than accumulating one per attempt.
//
// Exported so a test proving the audit API surfaces this path's output (see
// internal/api/audit.go) can call the real write path instead of re-deriving
// fixture data by hand.
func WriteDecisionRecordNode(ctx context.Context, g *graph.Graph, sections map[string]string, bead, epic BeadRef, runID string) (string, error) {
	if runID == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing decision record node for bead %s: no workflow run id was supplied - Fix: this is a caller bug (DriveBeadDeps.RunID must be set from what StartWorkflowRun returned before DriveBeadToTerminal runs), not a config value to change", bead.ID)
	}

	id := decisionRecordNodeID(runID, bead.ID, epic.ID, sections)
	author := nodes.Author{Name: "kernl-dispatch"}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if err := ensureBeadReferenceNode(ctx, tx, bead, author); err != nil {
			return err
		}
		if epic.ID != "" && epic.ID != bead.ID {
			if err := ensureBeadReferenceNode(ctx, tx, epic, author); err != nil {
				return err
			}
		}

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
		if err := ensureHasDecisionEdge(ctx, tx, runID, id, author); err != nil {
			return err
		}
		if err := ensureHasDecisionEdge(ctx, tx, bead.ID, id, author); err != nil {
			return err
		}
		if epic.ID != "" && epic.ID != bead.ID {
			if err := ensureHasDecisionEdge(ctx, tx, epic.ID, id, author); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing decision record node for bead %s: %w", bead.ID, err)
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
//
// bead, deps and epicTitle are the caller's own loop state, not re-derived
// here: bead is the same object DriveBeadToTerminal already fetched at the
// top of its iteration, deps.RepoPath/TrackerCommand were resolved once for
// the whole run, and epicTitle (when epicID differs from bead.ID) is fetched
// by the same loop, before the agent runs, not here. A second tracker fetch
// at this point - after the agent already ran and the gate already passed -
// would turn a transient lookup failure into a reason to discard work that
// had already succeeded; DriveBeadToTerminal pays that cost earlier, when
// failing costs nothing but a retry of the fetch itself.
func recordDecisionIfGateType(ctx context.Context, wf backend.WorkflowDescriptor, gateCtx backend.ExitGateContext, deps DriveBeadDeps, bead *backend.Bead, epicID, epicTitle string) error {
	gate, ok := backend.FindExitGateByType(wf, gateCtx.FromState, "decision_record")
	if !ok {
		return nil
	}

	sections, err := readDecisionRecordSections(gate, gateCtx, bead.ID)
	if err != nil {
		return err
	}

	trackerKind, err := trackerKindFromCommand(deps.TrackerCommand)
	if err != nil {
		return err
	}
	beadRef := BeadRef{ID: bead.ID, Title: bead.Title, TrackerKind: trackerKind, RepoPath: deps.RepoPath, IsFixup: HasLabel(bead.Labels, IntegrationFixupLabel)}
	epicRef := beadRef
	if epicID != "" && epicID != bead.ID {
		epicRef = BeadRef{ID: epicID, Title: epicTitle, TrackerKind: trackerKind, RepoPath: deps.RepoPath}
	}

	graphPath, err := graphDBFilePath(deps.Config)
	if err != nil {
		return err
	}
	g, err := graph.Open(ctx, graph.Config{Path: graphPath})
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: opening graph to record decision for bead %s: %w", bead.ID, err)
	}
	defer g.Close()

	_, err = WriteDecisionRecordNode(ctx, g, sections, beadRef, epicRef, deps.RunID)
	return err
}
