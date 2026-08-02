package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
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

// decisionBodyLengthHeaderFormat is the machine-readable preamble
// buildDecisionBody writes ahead of the two heading-labeled sections, and
// SplitDecisionBody reads back before it ever looks at the body text
// itself. It records each section's own byte length (of the trimmed text
// buildDecisionBody actually wrote), so the split point is read off a
// number instead of being re-derived by searching the body for a heading.
//
// This replaces an earlier version of SplitDecisionBody that re-parsed
// Body's own markdown headings via backend.MarkdownSectionsByHeading - a
// reasonable approach when Body might be arbitrary markdown, but Body here
// is built ENTIRELY by this package from two already-extracted strings
// (DecisionRecordEntry.OptionsConsidered/TradeOffs - see buildDecisionBody),
// so nothing stops one of those strings from itself containing a standalone
// line that reads as "## Trade-offs" (an implementer's OptionsConsidered
// field legitimately discussing the trade-offs heading by name, verbatim).
// When that happened, MarkdownSectionsByHeading saw two real, non-fenced
// headings claiming the same key and reported it via its dupKey return -
// which the old SplitDecisionBody discarded, so the ambiguity passed
// unnoticed and the options text silently truncated at the wrong point. Two
// programs, the writer and the reader, control this exact string. There is
// no reason to keep re-deriving the boundary by scanning content for a
// pattern that content itself might also contain, when the writer can just
// tell the reader where the boundary is. Recording the length up front makes
// the round trip unambiguous BY CONSTRUCTION: no string in either field,
// however it is worded, can ever be mistaken for the boundary again, and
// MarkdownSectionsByHeading's dupKey defense - still load-bearing for the
// artifact parsers that read genuinely agent-authored markdown (fork
// handover, the Phase 6 rejection artifacts) - is not needed here at all.
const decisionBodyLengthHeaderFormat = "<!--kernl:decision-body-lengths options=%d tradeoffs=%d-->\n"

// decisionBodyLengthHeaderRe parses decisionBodyLengthHeaderFormat's output
// back into its two byte lengths. Anchored to the start of the string: the
// header is only ever valid as Body's very first line.
var decisionBodyLengthHeaderRe = regexp.MustCompile(`^<!--kernl:decision-body-lengths options=(\d+) tradeoffs=(\d+)-->\n`)

// buildDecisionBody folds options-considered and trade-offs content into one
// string under labeled headings, so the boundary between the two survives
// being stored in Decision's single opaque Body field. See
// decisionBodyLengthHeaderFormat for why the result also carries a hidden
// length preamble ahead of the human-readable headings.
func buildDecisionBody(optionsConsidered, tradeOffs string) string {
	options := strings.TrimSpace(optionsConsidered)
	tradeOffsTrimmed := strings.TrimSpace(tradeOffs)
	header := fmt.Sprintf(decisionBodyLengthHeaderFormat, len(options), len(tradeOffsTrimmed))
	return header + DecisionBodyOptionsHeading + "\n\n" + options +
		"\n\n" + DecisionBodyTradeOffsHeading + "\n\n" + tradeOffsTrimmed
}

// SplitDecisionBody is buildDecisionBody's inverse: it recovers the
// options-considered and trade-offs text a Decision.Body built by this
// package folded together. It reads the exact-length preamble
// buildDecisionBody wrote and slices the two sections off by that recorded
// length, rather than searching the body for the headings that separate
// them - see decisionBodyLengthHeaderFormat for why a search can be made to
// disagree with itself and a length recorded at write time cannot.
//
// ok is false when body does not carry a well-formed preamble matching the
// two heading markers that must immediately follow it at the recorded
// offsets - i.e. body was not built by buildDecisionBody (hand-edited,
// foreign, or predates this format) - which callers should treat as "not
// individually recoverable" rather than guess at a split.
func SplitDecisionBody(body string) (optionsConsidered, tradeOffs string, ok bool) {
	m := decisionBodyLengthHeaderRe.FindStringSubmatch(body)
	if m == nil {
		return "", "", false
	}
	optionsLen, err := strconv.Atoi(m[1])
	if err != nil {
		return "", "", false
	}
	tradeOffsLen, err := strconv.Atoi(m[2])
	if err != nil {
		return "", "", false
	}

	rest := body[len(m[0]):]
	optionsPrefix := DecisionBodyOptionsHeading + "\n\n"
	if !strings.HasPrefix(rest, optionsPrefix) {
		return "", "", false
	}
	rest = rest[len(optionsPrefix):]
	if len(rest) < optionsLen {
		return "", "", false
	}
	options := rest[:optionsLen]
	rest = rest[optionsLen:]

	tradeOffsPrefix := "\n\n" + DecisionBodyTradeOffsHeading + "\n\n"
	if !strings.HasPrefix(rest, tradeOffsPrefix) {
		return "", "", false
	}
	rest = rest[len(tradeOffsPrefix):]
	if len(rest) != tradeOffsLen {
		return "", "", false
	}
	return options, rest, true
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

// decisionContentKey is the string WriteDecisionRecordNode groups entries by
// to compute occurrenceRank for decisionRecordNodeID: the exact Title,
// Context, Body and Outcome an entry resolves to (see
// decisionFromRecordEntry), joined behind NUL separators. Two entries share
// a decisionContentKey exactly when decisionFromRecordEntry would write them
// as byte-identical Decision nodes.
func decisionContentKey(d nodes.Decision) string {
	return d.Title + "\x00" + d.Context + "\x00" + d.Body + "\x00" + d.Outcome
}

// decisionRecordNodeID derives a stable node ID from exactly the facts that
// make two writes "the same decision": which run, which bead, which epic,
// the entry's own resolved content, and occurrenceRank - how many entries
// with that exact same resolved content already preceded this one in the
// SAME record (0 for the first, 1 for the second, and so on; see
// WriteDecisionRecordNode). A re-run of the same bead against the same
// record within the same run - the normal shape of a retry after
// Backend.Update fails following a successful graph write, see
// WriteDecisionRecordNode - hashes every entry to the same ID it hashed to
// before and so converges on the existing nodes instead of minting a fresh
// UUID and a fresh set of edges per attempt.
//
// This is deliberately content-addressed, not position-addressed: an
// earlier version of this function folded the entry's own array INDEX into
// the hash instead of occurrenceRank, which converged on retry only when the
// record came back with every entry in exactly the same array slot. A retry
// that reordered decisions, or inserted a new one before an existing one,
// shifted every following entry's index and so minted a brand-new node for
// each - the old nodes stayed behind, still linked to the run, orphaned but
// never cleaned up. An entry's identity is what it SAYS, not where it sits
// in the array; d already carries that content resolved exactly as it will
// be written (decisionFromRecordEntry has already applied the title
// fallback and trimmed every field), so hashing d's own fields is also
// immune to two changes that used to mint spurious new nodes for an
// otherwise-identical decision: an explicit title that merely repeats what
// fallbackDecisionTitle would already have derived, and a change to a
// field's leading/trailing whitespace alone.
//
// occurrenceRank, not content alone, is what still lets two decisions in the
// same record that happen to carry byte-identical resolved content (a
// pathological but possible case - nothing stops an agent from writing the
// same words twice) become two distinct Decision nodes rather than silently
// collapsing into one. This is the specific failure mode that made the
// withdrawn prefix-matching approach unacceptable (see this file's own git
// history / AGENTS.md): a fix for "more than one decision" that can still
// make one of them vanish is not a fix. Ranking by content occurrence,
// rather than by raw array position, keeps that guarantee while no longer
// requiring the two duplicates to stay in the same relative array slots on
// retry - swapping two byte-identical entries with each other changes
// nothing observable about the record, so it must not mint new nodes either.
//
// runID is folded in too, not just recorded as an edge, so that two
// DIFFERENT runs can never converge on the same Decision node even when a
// bead somehow revisits the same stage with byte-identical content (a
// reverted-and-reopened bead per the decision model's SS6, a fix-up bead). If
// the ID were content-only, such a case would silently re-link a decision
// that already belongs to an earlier run's report onto a second run,
// reintroducing exactly the cross-run leak WriteDecisionRecordNode's own doc
// comment on the run edge exists to prevent.
func decisionRecordNodeID(runID, beadID, epicID string, occurrenceRank int, d nodes.Decision) string {
	h := sha256.New()
	_, _ = io.WriteString(h, "run:"+runID+"\x00bead:"+beadID+"\x00epic:"+epicID+"\x00occurrence:"+strconv.Itoa(occurrenceRank)+"\x00")
	_, _ = io.WriteString(h, "content:"+decisionContentKey(d)+"\x00")
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
// epic id, the entry's own resolved content, and how many prior entries in
// THIS record already resolved to that same content (decisionRecordNodeID)
// - folding runID into the hash is what makes two different runs
// structurally unable to ever converge on the same Decision node, and
// ranking by content occurrence rather than array position is what lets a
// retry that reorders decisions, or inserts a new one, still converge every
// unchanged decision onto the node it already has; see that function's own
// doc comment. The reference node creates, each
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
	now := time.Now()
	// occurrenceRank counts, per distinct resolved content, how many prior
	// entries in THIS record already resolved to it - see
	// decisionRecordNodeID's doc comment for why ranking by content
	// occurrence (rather than raw array index) is what lets a retried
	// record converge on existing nodes even when reordered.
	occurrenceRank := make(map[string]int, len(decisions))

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
			d := decisionFromRecordEntry(entry, now)
			contentKey := decisionContentKey(d)
			rank := occurrenceRank[contentKey]
			occurrenceRank[contentKey] = rank + 1

			id := decisionRecordNodeID(runID, bead.ID, epic.ID, rank, d)
			ids[i] = id
			d.ID = id

			exists, err := nodes.Exists(ctx, tx, id)
			if err != nil {
				return err
			}
			if !exists {
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
