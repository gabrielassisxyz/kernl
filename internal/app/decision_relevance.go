package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/search"
)

// maxRelevantDecisions bounds how many standing decisions ride along in one
// stage prompt. Every decision in the repository is noise against the one or
// two that actually bear on the bead about to run; a cap keeps a repository
// with a long decision history from drowning the signal this section exists
// to surface, the same failure mode the relevance filter itself exists to
// avoid one level up.
const maxRelevantDecisions = 5

// maxRelevanceTerms bounds how many of the bead's own vocabulary words drive
// the search - each term is its own FTS lookup (see FetchRelevantDecisions),
// so this also bounds the query cost for an unusually long bead description.
const maxRelevanceTerms = 8

// RelevantDecision is the sliver of a standing Decision node that reaches
// the implementer's prompt: Title plus Outcome, not the full Body. Body
// holds the options-considered and trade-offs text (see SplitDecisionBody)
// and routinely runs into the thousands of bytes - the revert path pulls
// Body in because a reverted decision needs the implementer to know exactly
// which option is now forbidden, but a standing decision only needs to be
// recognized, not re-litigated, and Outcome is where the winning choice is
// actually named: revert_decision.go's own renderRevertedDecisionConstraint
// documents that Outcome routinely names the winner by an id or word defined
// in Body, which is exactly the "PageCeilingReached... over the more
// ordinary PageLimitReached" sentence that would have stopped the defect
// this bead exists to close.
type RelevantDecision struct {
	ID      string
	Title   string
	Outcome string
}

// FetchRelevantDecisions finds standing (never-reverted) decisions recorded
// in repoPath whose text overlaps bead's own title and description, so an
// implementer sees an answer that was already settled instead of
// re-deriving it and landing on the alternative that was already rejected.
//
// Relevance is full-text overlap: each significant word in the bead's own
// title/description (relevanceTerms) is searched independently against
// Decision nodes, and a hit is kept only if it belongs to this same
// repository (decisionScope) and was not reverted (a reverted decision has
// its own injection route, onto the specific bead it was reverted on - see
// revert_decision.go - and does not need a second one here). This is the
// simplest criterion that catches the production case: two decisions about
// the same enum, in the same file, in the same repository, recorded on two
// different beads days apart - not the same epic (fifteen days apart is
// rarely the same epic), and no per-file signal exists on a Decision node to
// compare against (the attempt ledger records commitSHA, not the decision).
// Shared vocabulary is what both decisions actually have in common: the
// rejected alternative's own name appears verbatim in the first decision's
// Outcome text, which is exactly what a second bead about the same concept
// would also use in its own title.
func FetchRelevantDecisions(ctx context.Context, g *graph.Graph, repoPath string, bead *backend.Bead) ([]RelevantDecision, error) {
	terms := relevanceTerms(bead.Title+" "+bead.Description, maxRelevanceTerms)
	if len(terms) == 0 {
		return nil, nil
	}

	var out []RelevantDecision
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		seen := map[string]bool{}
		for _, term := range terms {
			if len(out) >= maxRelevantDecisions {
				break
			}
			hits, err := search.Search(ctx, tx, term, search.WithTypes("decision"))
			if err != nil {
				if errors.Is(err, graph.ErrFTSQuerySyntax) {
					continue
				}
				return fmt.Errorf("searching decisions for term %q: %w", term, err)
			}
			for _, hit := range hits {
				if len(out) >= maxRelevantDecisions || seen[hit.NodeID] {
					continue
				}
				seen[hit.NodeID] = true

				d, err := nodes.GetDecision(ctx, tx, hit.NodeID)
				if err != nil {
					return fmt.Errorf("reading decision %s: %w", hit.NodeID, err)
				}
				if d.RevertedAt != nil {
					continue
				}
				sameRepo, isSelf, err := decisionScope(ctx, tx, hit.NodeID, repoPath, bead.ID)
				if err != nil {
					return fmt.Errorf("scoping decision %s: %w", hit.NodeID, err)
				}
				if !sameRepo || isSelf {
					continue
				}
				out = append(out, RelevantDecision{ID: d.ID, Title: d.Title, Outcome: d.Outcome})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// relevanceTerms extracts short, deduplicated, lowercase words from s to
// drive FetchRelevantDecisions's per-term search - the same shape as
// internal/inbox's unexported salientTerms, which solves an analogous
// problem (finding graph nodes that share vocabulary with a block of
// prose). Words under 4 characters are dropped as a cheap substitute for a
// stopword list: "the", "and", "for", "not" and the rest of English's short
// function words fall under that length, while words that actually
// distinguish one decision from another ("ceiling", "crawl", "reached") do
// not.
//
// Duplicated here rather than imported: internal/inbox's salientTerms is an
// unexported helper of that package, built for a different caller solving a
// different-shaped relevance problem (bookmarks/notes, not decisions). A
// shared home for the sixteen lines both need is only worth building once a
// third caller needs them too - extracting an abstraction ahead of that
// third, proven use is exactly what this project's own engineering
// principles caution against.
func relevanceTerms(s string, max int) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range strings.Fields(strings.ToLower(s)) {
		t := strings.Trim(raw, ".,;:!?\"'()[]{}")
		if len(t) < 4 || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) >= max {
			break
		}
	}
	return out
}

// decisionScope reports whether decisionID belongs to repoPath - reached by
// walking its has_decision edges back to the bead/epic reference node(s)
// that recorded it, since Decision itself carries no repository field - and
// whether beadID is one of those references. isSelf lets the caller drop a
// decision the SAME bead already recorded: that is the bead's own recent
// work, already visible in its worktree history, not a prior answer it
// risks re-deriving.
//
// A has_decision edge's source is a run, a bead, or an epic (see
// WriteDecisionRecordNode); only the latter two resolve as a bead reference
// node, so ErrNotFound on a run source is expected and skipped rather than
// treated as a failure.
func decisionScope(ctx context.Context, tx *graph.ReadTx, decisionID, repoPath, beadID string) (sameRepo, isSelf bool, err error) {
	in, err := edges.Incoming(ctx, tx, decisionID, edges.WithType(edges.EdgeTypeHasDecision))
	if err != nil {
		return false, false, err
	}
	for _, e := range in {
		ref, err := nodes.GetBeadReference(ctx, tx, e.Src)
		if err != nil {
			if errors.Is(err, graph.ErrNotFound) {
				continue
			}
			return false, false, err
		}
		if ref.Repository == repoPath {
			sameRepo = true
		}
		if e.Src == beadID {
			isSelf = true
		}
	}
	return sameRepo, isSelf, nil
}

// relatedDecisionsForPrompt resolves the standing decisions a stage prompt
// for bead should carry, or nil (with a loud, greppable warning) if the
// graph could not be opened or read.
//
// This degrades rather than halts, unlike recordDecisionIfGateType (the
// write path for this same graph), and the asymmetry is deliberate. The
// write path's job is to guarantee a decision that was actually just made
// gets recorded - losing that silently is permanent: the agent invocation
// that produced it is already spent, and nothing else will ever write it
// down. This read path's job is best-effort context for an agent that has
// not started yet; if the graph cannot be read, the worst case is the
// status quo this bead is improving on (an implementer without prior
// context), not a new failure mode. Halting every bead in the fleet because
// this repository's graph database hiccuped would trade a rare, recoverable
// loss of extra context for certain, repository-wide unavailability of work
// that would otherwise succeed - a worse outcome than the one being fixed.
// The KERNL DISPATCH FAILURE marker keeps the failure greppable in logs
// either way, so a graph that is reliably unreadable does not stay invisible.
func relatedDecisionsForPrompt(ctx context.Context, deps DriveBeadDeps, bead *backend.Bead) []RelevantDecision {
	graphPath, err := graphDBFilePath(deps.Config)
	if err != nil {
		slog.Warn("KERNL DISPATCH FAILURE: resolving graph path to fetch related decisions - continuing without them", "bead", bead.ID, "err", err)
		return nil
	}
	g, err := graph.Open(ctx, graph.Config{Path: graphPath})
	if err != nil {
		slog.Warn("KERNL DISPATCH FAILURE: opening graph to fetch related decisions - continuing without them", "bead", bead.ID, "err", err)
		return nil
	}
	defer g.Close()

	decisions, err := FetchRelevantDecisions(ctx, g, deps.RepoPath, bead)
	if err != nil {
		slog.Warn("KERNL DISPATCH FAILURE: fetching related decisions - continuing without them", "bead", bead.ID, "err", err)
		return nil
	}
	return decisions
}
