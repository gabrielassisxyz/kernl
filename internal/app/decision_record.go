package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
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
//
// This format predates the decision_record artifact's move to agent-authored
// JSON and is unrelated to it: Body is kernl's OWN construction from two
// already-extracted strings (DecisionRecordEntry.OptionsConsidered/TradeOffs),
// never agent-authored markdown, so there is no reason to change it just
// because the artifact the strings originally came from changed shape.
func buildDecisionBody(optionsConsidered, tradeOffs string) string {
	return DecisionBodyOptionsHeading + "\n\n" + strings.TrimSpace(optionsConsidered) +
		"\n\n" + DecisionBodyTradeOffsHeading + "\n\n" + strings.TrimSpace(tradeOffs)
}

// splitDecisionBodyHeadingKeys names the only two headings SplitDecisionBody
// recognizes - the exact two buildDecisionBody writes, with their "## "
// marker stripped (backend.MarkdownSectionsByHeading's keyFn receives a
// heading's bare text, not its marker).
var splitDecisionBodyHeadingKeys = map[string]string{
	strings.TrimPrefix(DecisionBodyOptionsHeading, "## "):   "options_considered",
	strings.TrimPrefix(DecisionBodyTradeOffsHeading, "## "): "trade_offs",
}

// SplitDecisionBody is buildDecisionBody's inverse: it recovers the
// options-considered and trade-offs text a Decision.Body built by this
// package folded together, using the same headings buildDecisionBody wrote.
//
// It re-parses Body with backend.MarkdownSectionsByHeading - the same
// fence- and HTML-comment-aware pass the old decision_record gate's own
// markdown parser used - rather than a naive strings.Index search. A naive
// search finds "## Trade-offs" anywhere in the text, including inside a
// fenced code example the options text quotes verbatim, or as an inline
// mention in prose; either one truncates the real options content and hands
// the real separator's remainder to the wrong half. This matters even now
// that decision_record itself is agent-authored JSON, not markdown: the
// OptionsConsidered/TradeOffs strings folded into Body are still free text a
// human or an agent wrote, and either one could still legitimately contain
// the literal heading text while discussing it - buildDecisionBody's own
// folded format is unaffected by the artifact's authoring format, so this
// split still needs the same defense it always did.
//
// ok is false when Body does not parse into both parts (it was not built by
// buildDecisionBody, or the two recognized headings could not be told apart
// unambiguously), which callers should treat as "not individually
// recoverable" rather than guess at a split.
func SplitDecisionBody(body string) (optionsConsidered, tradeOffs string, ok bool) {
	sections, _ := backend.MarkdownSectionsByHeading(body, func(headingText string) (string, bool) {
		key, recognized := splitDecisionBodyHeadingKeys[strings.TrimSpace(headingText)]
		return key, recognized
	})
	options, hasOptions := sections["options_considered"]
	tradeOffsText, hasTradeOffs := sections["trade_offs"]
	if !hasOptions || !hasTradeOffs {
		return "", "", false
	}
	return options, tradeOffsText, true
}

// fallbackDecisionTitle derives a short label from a decision's own text -
// its first line, capped in length - for the (normal, single-decision) case
// an implementer did not set DecisionRecordEntry.Title explicitly.
func fallbackDecisionTitle(decisionText string) string {
	trimmed := strings.TrimSpace(decisionText)
	title := trimmed
	if idx := strings.IndexByte(trimmed, '\n'); idx != -1 {
		title = strings.TrimSpace(trimmed[:idx])
	}
	const maxTitleRunes = 120
	if r := []rune(title); len(r) > maxTitleRunes {
		title = string(r[:maxTitleRunes]) + "..."
	}
	return title
}

// decisionFromRecordEntry maps one DecisionRecordEntry - already validated
// by backend.ParseDecisionRecordDocument - onto Decision's fields: "decision"
// becomes Context (and, absent an explicit Title, the source of a derived
// one), "rationale" becomes Outcome exactly, and "optionsConsidered" plus
// "tradeOffs" are folded into Body (see buildDecisionBody) since the schema
// has no dedicated field for either. ImpactOnUse is left nil - it is written
// later, by the run's composer at run close, not here.
func decisionFromRecordEntry(e backend.DecisionRecordEntry, now time.Time) nodes.Decision {
	title := strings.TrimSpace(e.Title)
	if title == "" {
		title = fallbackDecisionTitle(e.Decision)
	}
	return nodes.Decision{
		CreatedAt: now,
		UpdatedAt: now,
		Title:     title,
		Context:   strings.TrimSpace(e.Decision),
		Body:      buildDecisionBody(e.OptionsConsidered, e.TradeOffs),
		Outcome:   strings.TrimSpace(e.Rationale),
		DecidedAt: now,
		Tags:      []string{PhaseThreeDecisionTag},
	}
}

// decisionRecordNodeID derives a stable node ID from exactly the facts that
// make two writes "the same decision": which run, which bead, which epic,
// the entry's own position in the record's decisions array, and the entry's
// own content. A re-run of the same bead against the same record within the
// same run - the normal shape of a retry after Backend.Update fails
// following a successful graph write, see WriteDecisionRecordNode - hashes
// every entry to the same ID it hashed to before and so converges on the
// existing nodes instead of minting a fresh UUID and a fresh set of edges
// per attempt.
//
// index is folded into the hash, not derived from content alone: two
// decisions in the same record that happen to carry byte-identical content
// (a pathological but possible case - nothing stops an agent from writing
// the same words twice) must still become two distinct Decision nodes, not
// silently collapse into one. This is the specific failure mode that made
// the withdrawn prefix-matching approach unacceptable (see this file's own
// git history / AGENTS.md): a fix for "more than one decision" that can
// still make one of them vanish is not a fix.
//
// runID is folded in too, not just recorded as an edge, so that two
// DIFFERENT runs can never converge on the same Decision node even when a
// bead somehow revisits the same stage with byte-identical content (a
// reverted-and-reopened bead per the decision model's SS6, a fix-up bead). If
// the ID were content-only, such a case would silently re-link a decision
// that already belongs to an earlier run's report onto a second run,
// reintroducing exactly the cross-run leak WriteDecisionRecordNode's own doc
// comment on the run edge exists to prevent.
func decisionRecordNodeID(runID, beadID, epicID string, index int, e backend.DecisionRecordEntry) string {
	h := sha256.New()
	_, _ = io.WriteString(h, "run:"+runID+"\x00bead:"+beadID+"\x00epic:"+epicID+"\x00index:"+strconv.Itoa(index)+"\x00")
	_, _ = io.WriteString(h, "title:"+e.Title+"\x00decision:"+e.Decision+"\x00options:"+e.OptionsConsidered+"\x00tradeoffs:"+e.TradeOffs+"\x00rationale:"+e.Rationale+"\x00")
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

// WriteDecisionRecordNode creates one Decision node per entry in decisions
// and links each to the run, the bead, and (when different) the epic it was
// written for, all via edges.EdgeTypeHasDecision and all inside ONE write
// transaction covering every entry - a node with no link back to the work it
// explains is not meaningfully queryable later, so all of it is written
// together or not at all, and a record with several decisions is not
// half-written when one of them fails.
//
// One Decision node per entry, not one node carrying the whole array: the
// existing Decision schema (Title/Context/Body/Outcome) already models
// exactly one decision, every existing reader (run_report.go's
// decisionReportFields, the audit API, decision_relevance.go's relevance
// search) already expects one row per decision, and each entry already
// carries everything a Decision node needs. Changing that schema to hold a
// list would be a second, larger migration this fix does not need: N
// entries producing N nodes is additive to every reader that already exists,
// while a single node carrying N would require every one of them to change.
//
// decisions must be non-empty; a caller that reaches here with none is a
// caller bug (backend.ParseDecisionRecordDocument, the only production
// source of this slice, never returns a document with an empty decisions
// array), so this fails loud rather than silently doing nothing.
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
// ensureBeadReferenceNode before their edges are attempted, once for the
// whole call rather than once per entry. The run node is different: it is a
// real nodes.WorkflowRun row StartWorkflowRun already created, never a stub
// minted here, so a runID that does not name an existing run node fails the
// same way any other broken reference would.
//
// Each Decision node's ID is content-addressed over the run id, bead id,
// epic id, the entry's own array index, and the entry's own fields
// (decisionRecordNodeID) - folding runID and index into the hash is what
// makes two different runs, and two different positions within the same
// record, structurally unable to ever converge on the same Decision node;
// see that function's own doc comment. The reference node creates, each
// Decision create, and each edge create are all skipped when they already
// exist (nodes.Exists, ensureBeadReferenceNode, ensureHasDecisionEdge) - a
// caller that retries this same call for the same run, the same bead, and
// the same record (DriveBeadToTerminal does, whenever the graph write
// itself succeeds but the bead's state update fails afterward and the same
// run is retried) converges on one of each rather than accumulating one per
// attempt.
//
// Exported so a test proving the audit API surfaces this path's output (see
// internal/api/audit.go) can call the real write path instead of re-deriving
// fixture data by hand.
func WriteDecisionRecordNode(ctx context.Context, g *graph.Graph, decisions []backend.DecisionRecordEntry, bead, epic BeadRef, runID string) ([]string, error) {
	if runID == "" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: writing decision record node(s) for bead %s: no workflow run id was supplied - Fix: this is a caller bug (DriveBeadDeps.RunID must be set from what StartWorkflowRun returned before DriveBeadToTerminal runs), not a config value to change", bead.ID)
	}
	if len(decisions) == 0 {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: writing decision record node(s) for bead %s: no decisions were supplied - Fix: this is a caller bug (backend.ParseDecisionRecordDocument never returns an empty decisions array), not a config value to change", bead.ID)
	}

	author := nodes.Author{Name: "kernl-dispatch"}
	ids := make([]string, len(decisions))

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if err := ensureBeadReferenceNode(ctx, tx, bead, author); err != nil {
			return err
		}
		if epic.ID != "" && epic.ID != bead.ID {
			if err := ensureBeadReferenceNode(ctx, tx, epic, author); err != nil {
				return err
			}
		}

		for i, entry := range decisions {
			id := decisionRecordNodeID(runID, bead.ID, epic.ID, i, entry)
			ids[i] = id

			exists, err := nodes.Exists(ctx, tx, id)
			if err != nil {
				return err
			}
			if !exists {
				d := decisionFromRecordEntry(entry, time.Now())
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
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: writing decision record node(s) for bead %s: %w", bead.ID, err)
	}
	return ids, nil
}

// readDecisionRecords re-reads the file a decision_record exit gate just
// validated and parses it with backend.ParseDecisionRecordDocument - the
// exact function evaluateSingleExitGate used to decide the gate passed, so
// this read and the gate's own read can never disagree about what counts as
// valid (see AGENTS.md's "two parsers" caution and
// ParseDecisionRecordDocument's own doc comment). A read failure or a parse
// failure here, immediately after the gate reported success against the
// same path, means the file changed between the two reads - either way this
// is a hard stop, not a value to paper over: the whole point of writing this
// node is that the reasoning stays recoverable, and silently dropping a
// decision defeats that.
func readDecisionRecords(gate backend.WorkflowExitGate, gateCtx backend.ExitGateContext, beadID string) ([]backend.DecisionRecordEntry, error) {
	if strings.Contains(gate.Path, "<artifact_dir>") && gateCtx.ArtifactDir == "" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: decision_record gate for bead %s names <artifact_dir> but no artifact directory was resolved for this run - Fix: this indicates a driver bug (artifactDir should already be resolved before gate evaluation), not a config problem", beadID)
	}
	abs := backend.ResolveArtifactFSPath(gate.Path, gateCtx.BeadID, gateCtx.WorktreePath, gateCtx.ArtifactDir)
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: decision_record gate passed for bead %s but the record at %s could not be re-read: %w", beadID, abs, err)
	}
	entries, err := backend.ParseDecisionRecordDocument(string(data))
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: decision_record gate passed for bead %s but the record at %s no longer parses: %w - the gate and this read must never disagree - Fix: investigate what changed the file between the gate's read and this one", beadID, abs, err)
	}
	return entries, nil
}

// recordDecisionIfGateType mirrors a just-passed decision_record exit gate's
// record into the graph as one or more queryable Decision nodes. It is a
// no-op for every other gate type, so DriveBeadToTerminal can call it
// unconditionally right after any gatePassed branch without checking the
// gate type itself first.
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

	entries, err := readDecisionRecords(gate, gateCtx, bead.ID)
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

	_, err = WriteDecisionRecordNode(ctx, g, entries, beadRef, epicRef, deps.RunID)
	return err
}
