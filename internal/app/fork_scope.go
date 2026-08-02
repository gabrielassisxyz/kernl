package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// ForkScopeFacts are what a fork handover is judged against before the DA is
// ever asked anything. The one field every layer of DecideForkAction reads
// without asking a model is OpenDependents; RelatedDecisions and
// RepositoryContext are carried here too because they are also measured
// ahead of time - fetched once by the caller via the existing
// FetchRelevantDecisions and AssembleContext, never re-derived by this
// package - and DecideForkAction's own signature takes facts rather than a
// repository path so it stays as easy to test as DecideFixupAction already
// is.
type ForkScopeFacts struct {
	EpicID string
	BeadID string
	// OpenDependents are the beads in this epic that depend on BeadID and
	// are not yet closed - the measured proxy for "something outside this
	// bead has to agree with the choice" (see DecideForkAction). Unlike
	// ReversibilityFacts' fields, this is the one layer here that rests on a
	// proxy rather than a certainty: a bead can depend on another for
	// reasons unrelated to this specific fork. It is named explicitly in
	// the escalation reason (see DecideForkAction) precisely so an operator
	// reading a run's report can tell whether this layer is firing too
	// often and revisit the proxy, not just trust the count.
	OpenDependents []string
	// RelatedDecisions is prose already rendered from
	// app.FetchRelevantDecisions - see prompt.ForkHandoverInput's own field
	// of the same name for why a recorded preference is an input to
	// judgment here, never a short-circuit.
	RelatedDecisions string
	// RepositoryContext is app.AssembleContext's own output - the same
	// context the Oracle sees (Unit A §2.3), reused here rather than a
	// second read path.
	RepositoryContext string
}

// ForkScopeInspector answers the one question the mechanical layer of
// DecideForkAction asks the tracker: which of an epic's own beads depend on
// the one handing over a fork, and have not yet closed. It is an interface
// for the reason every other measurement seam in this package is (see
// BranchInspector): a unit test may not shell out or hit a real tracker, and
// this code decides whether a model is asked anything at all.
type ForkScopeInspector interface {
	OpenDependents(epicID, beadID, repoPath string) ([]string, error)
}

// BackendForkScopeInspector is the production ForkScopeInspector. It adds no
// new tracker call: an epic's children already come back fully hydrated from
// backend.BackendPort.List(&BeadListFilters{Parent: epicID}, ...) - the same
// seam surveyFixupChildren (epic_fixup.go) already reuses - and `br show`
// already reports each child's own dependencies (see brIssue.toRawBead's own
// doc comment: "the issue being shown is the dependent, and each entry names
// what it depends on"). Nothing here talks to br directly; it only reads
// what BackendPort.List already hydrates.
type BackendForkScopeInspector struct {
	Backend backend.BackendPort
}

// blocksDependencyType is the dependency type br's own `dep add` creates by
// default (see AddDependency's own doc comment: "it always creates br's
// default 'blocks' edge"). It is what "sibling depends on this bead" means
// here - "parent-child" is excluded on purpose, since that edge only marks
// epic membership, never an ordering the sibling waits on.
const blocksDependencyType = "blocks"

// OpenDependents implements ForkScopeInspector.
func (i BackendForkScopeInspector) OpenDependents(epicID, beadID, repoPath string) ([]string, error) {
	siblings, err := i.Backend.List(&backend.BeadListFilters{Parent: epicID}, repoPath)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: listing epic %s's children to check whether anything depends on %s failed: %w", epicID, beadID, err)
	}
	var open []string
	for _, sibling := range siblings {
		if sibling.ID == beadID || sibling.ClosedAt != "" {
			continue
		}
		if dependsOn(sibling, beadID) {
			open = append(open, sibling.ID)
		}
	}
	return open, nil
}

// dependsOn reports whether bead carries a "blocks" dependency naming
// targetID - i.e. bead cannot proceed until targetID is done.
func dependsOn(bead backend.Bead, targetID string) bool {
	for _, dep := range bead.Dependencies {
		if dep.TargetID == targetID && dep.Type == blocksDependencyType {
			return true
		}
	}
	return false
}

// GatherForkScopeFactsInput is what the caller already has before the
// tracker is asked anything.
type GatherForkScopeFactsInput struct {
	EpicID   string
	BeadID   string
	RepoPath string
	// RelatedDecisions and RepositoryContext are already-rendered text - the
	// caller fetches them via the existing FetchRelevantDecisions and
	// AssembleContext, exactly as an implementer's own stage prompt already
	// does, rather than this function opening a graph or reading the
	// repository itself. Keeping that reuse literal (call the existing
	// function, hand the result here) is what "reused, not rebuilt" means
	// for this pass - a second internal read path for the same two things
	// is the defect this project's own AGENTS.md warns against.
	RelatedDecisions  string
	RepositoryContext string
	// Inspector measures OpenDependents. No default: unlike
	// GitBranchInspector, a ForkScopeInspector wraps a repository's own
	// BackendPort, which has no natural zero value to fall back to - the
	// caller already carries one (DriveBeadDeps.Backend) and must pass it.
	Inspector ForkScopeInspector
}

// GatherForkScopeFacts measures OpenDependents and carries the two
// already-rendered text fields through unchanged.
func GatherForkScopeFacts(in GatherForkScopeFactsInput) (ForkScopeFacts, error) {
	if in.Inspector == nil {
		return ForkScopeFacts{}, fmt.Errorf("KERNL DISPATCH FAILURE: no ForkScopeInspector was given to measure %s's sibling dependents in epic %s", in.BeadID, in.EpicID)
	}
	open, err := in.Inspector.OpenDependents(in.EpicID, in.BeadID, in.RepoPath)
	if err != nil {
		return ForkScopeFacts{}, err
	}
	return ForkScopeFacts{
		EpicID:            in.EpicID,
		BeadID:            in.BeadID,
		OpenDependents:    open,
		RelatedDecisions:  in.RelatedDecisions,
		RepositoryContext: in.RepositoryContext,
	}, nil
}

// gatherForkScopeFactsForBead is the one place that measures ForkScopeFacts
// for a bead from a DriveBeadDeps and a fresh bead read - the exact sequence
// both callers into DecideForkAction need (the proactive fork gate's own
// decideForkAttempt in fork_gate.go, and the reviewer-raised decision gate's
// handleReviewRaisedDecision in review_decision_gate.go): related decisions
// and repository context rendered the same way an implementer's own stage
// prompt already renders them, and OpenDependents measured from the
// tracker. Factored out so a second caller does not re-inline this
// sequence, which is exactly the kind of duplication AGENTS.md's "reuse it,
// do not write a second policy" already rules out for DecideForkAction
// itself.
func gatherForkScopeFactsForBead(ctx context.Context, deps DriveBeadDeps, bead *backend.Bead, epicID string) (ForkScopeFacts, error) {
	var relatedBuf strings.Builder
	renderRelatedDecisions(&relatedBuf, relatedDecisionsForPrompt(ctx, deps, bead))
	repoContext := AssembleContext(deps.RepoPath, resolveContextDocs(deps.ContextDocs))

	return GatherForkScopeFacts(GatherForkScopeFactsInput{
		EpicID:            epicID,
		BeadID:            deps.BeadID,
		RepoPath:          deps.RepoPath,
		RelatedDecisions:  relatedBuf.String(),
		RepositoryContext: repoContext,
		Inspector:         BackendForkScopeInspector{Backend: deps.Backend},
	})
}
