package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// runRecordAuthor is the author every workflow_run write is attributed to.
// It matches WriteDecisionRecordNode's author: both are kernl's own dispatch
// loop writing on the operator's behalf, not a person, so there is exactly
// one name for this class of write.
var runRecordAuthor = nodes.Author{Name: "kernl-dispatch"}

// StartWorkflowRunInput carries everything StartWorkflowRun needs to create
// a workflow_run node and link it to every bead the dispatch is about to
// drive.
type StartWorkflowRunInput struct {
	// EntryPoint names which CLI verb opened this run - "epic run" or
	// "bead run". It is not the run's Status (see WorkflowRun.Status): it
	// is which door the operator, or the autonomous dispatcher, came in
	// through, so a composer reading this run later can tell "one bead,
	// driven standalone" from "one bead, driven as part of an epic" apart.
	EntryPoint   string
	Title        string
	WorkflowName string
	// Beads is the epic plus every child (EntryPoint "epic run") or the one
	// standalone bead (EntryPoint "bead run") this run drives. Only ID and
	// Title need to be set by the caller: TrackerKind and RepoPath are
	// filled in uniformly from TrackerCommand and RepoPath below, because a
	// single run has exactly one tracker and one repository - asking every
	// caller to repeat both on every entry would just be a chance for one
	// entry to disagree with the rest.
	Beads          []BeadRef
	RepoPath       string
	BaseBranch     string
	VerifyCommand  string
	TrackerCommand string
	DryRun         bool
	StartedAt      time.Time
}

// CloseWorkflowRunInput carries how a run ended.
type CloseWorkflowRunInput struct {
	// Status is the run's terminal state. This package only ever writes
	// "completed" or "failed" - a caller that needs a third value (e.g. an
	// operator-initiated abort) should say so here in a doc-comment update,
	// not pass an arbitrary string through unremarked.
	Status     string
	FinishedAt time.Time
	// Failure is the error text when Status is "failed", empty otherwise.
	Failure string
}

// runData is the JSON shape stored in WorkflowRun.RunData. GET /api/runs
// (internal/api/runs.go) forwards RunData to the web client as-is, so its
// keys are camelCase per AGENTS.md's wire contract even though nothing else
// in this file talks to the wire directly - the contract is set at the
// point data is produced, not at the point it happens to be served.
type runData struct {
	EntryPoint     string     `json:"entryPoint"`
	RepoPath       string     `json:"repoPath"`
	BaseBranch     string     `json:"baseBranch"`
	VerifyCommand  string     `json:"verifyCommand"`
	TrackerCommand string     `json:"trackerCommand"`
	DryRun         bool       `json:"dryRun"`
	BeadIDs        []string   `json:"beadIds"`
	StartedAt      time.Time  `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
	Failure        string     `json:"failure,omitempty"`
}

// StartWorkflowRun creates the run node and links it to every bead the
// dispatch is about to drive, all inside one write transaction - a run node
// with no edges to the beads it drove is not the thing a report composer can
// walk outward from, so the two are written together or not at all (the same
// invariant WriteDecisionRecordNode holds for a Decision and its bead/epic
// links).
//
// The node id is left empty, so nodes.CreateWorkflowRun mints a fresh
// UUIDv7, rather than content-addressed the way WriteDecisionRecordNode's
// Decision id is. A decision record is content-addressed so a retry that
// re-reads the same file converges on the one node instead of minting a
// duplicate; a run is not that shape - it is an event, and every dispatch,
// including a resume of the same epic through the same beads, is a distinct
// occurrence that belongs beside the previous one, not merged into it.
// Hashing a run to a stable id would make a resume overwrite the record of
// the run it is resuming instead of recording that a second attempt
// happened at all.
func StartWorkflowRun(ctx context.Context, g *graph.Graph, in StartWorkflowRunInput) (string, error) {
	if g == nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: no graph is open for this process, so workflow run %q cannot be recorded - Fix: this is an App wiring bug (App.Graph must be set by NewAppForRepo/NewApp), not a config value to change", in.Title)
	}
	if len(in.Beads) == 0 {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: starting workflow run %q with zero beads - a run with nothing to link to is not queryable by anything that walks run -> beads -> decisions - Fix: resolve the epic's children (or the standalone bead) before calling StartWorkflowRun", in.Title)
	}

	trackerKind, err := trackerKindFromCommand(in.TrackerCommand)
	if err != nil {
		return "", err
	}

	refs := make([]BeadRef, len(in.Beads))
	beadIDs := make([]string, len(in.Beads))
	for i, b := range in.Beads {
		refs[i] = BeadRef{ID: b.ID, Title: b.Title, TrackerKind: trackerKind, RepoPath: in.RepoPath}
		beadIDs[i] = b.ID
	}

	raw, err := json.Marshal(runData{
		EntryPoint:     in.EntryPoint,
		RepoPath:       in.RepoPath,
		BaseBranch:     in.BaseBranch,
		VerifyCommand:  in.VerifyCommand,
		TrackerCommand: in.TrackerCommand,
		DryRun:         in.DryRun,
		BeadIDs:        beadIDs,
		StartedAt:      in.StartedAt,
	})
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: marshaling run data for %q: %w - Fix: this is a bug in runData's shape, not something a caller can work around", in.Title, err)
	}

	var runID string
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		for _, ref := range refs {
			if err := ensureBeadReferenceNode(ctx, tx, ref, runRecordAuthor); err != nil {
				return err
			}
		}

		id, err := nodes.CreateWorkflowRun(ctx, tx, nodes.WorkflowRun{
			Title:        in.Title,
			WorkflowName: in.WorkflowName,
			Status:       "running",
			RunData:      string(raw),
		}, runRecordAuthor)
		if err != nil {
			return err
		}
		runID = id

		for _, ref := range refs {
			if _, err := edges.Create(ctx, tx, edges.Edge{Src: runID, Dst: ref.ID, Type: edges.EdgeTypeRanBead}, runRecordAuthor); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: starting workflow run %q: %w", in.Title, err)
	}
	return runID, nil
}

// CloseWorkflowRun records how a run ended, preserving every field
// StartWorkflowRun wrote except the ones a close is responsible for.
//
// The read and the write happen inside one g.DoWrite: graph.WriteTx exposes
// QueryRow/Query directly, and AsReadTx hands GetWorkflowRun that same
// *sql.Tx, so this is a genuine read-modify-write under one transaction, not
// a DoRead followed by a separate DoWrite. That distinction is the whole
// point - two separate transactions would leave a window where a concurrent
// writer's update to this same run node is invisible to the merge computed
// here and then clobbered by this call's UpdateWorkflowRun, which is exactly
// the blind-overwrite bug this function exists to not have.
func CloseWorkflowRun(ctx context.Context, g *graph.Graph, runID string, in CloseWorkflowRunInput) error {
	if g == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no graph is open for this process, so workflow run %s cannot be closed - Fix: this is an App wiring bug (App.Graph must be set by NewAppForRepo/NewApp), not a config value to change", runID)
	}
	if in.Status == "" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: closing workflow run %s with an empty status - Fix: pass \"completed\" or \"failed\"", runID)
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		existing, err := nodes.GetWorkflowRun(ctx, tx.AsReadTx(), runID)
		if err != nil {
			return err
		}

		var data runData
		if err := json.Unmarshal([]byte(existing.RunData), &data); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: run %s's stored run data does not parse as JSON: %w - Fix: this indicates StartWorkflowRun wrote a malformed blob, a bug to fix there, not a value to paper over here", runID, err)
		}
		finishedAt := in.FinishedAt
		data.FinishedAt = &finishedAt
		data.Failure = in.Failure

		raw, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: marshaling closed run data for %s: %w - Fix: this is a bug in runData's shape, not something a caller can work around", runID, err)
		}

		existing.Status = in.Status
		existing.RunData = string(raw)
		return nodes.UpdateWorkflowRun(ctx, tx, *existing, runRecordAuthor)
	})
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: closing workflow run %s: %w", runID, err)
	}
	return nil
}
