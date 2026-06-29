// Package planning builds substrate-aware planning context: given a seed
// (the topic the user is about to brainstorm or plan), it pulls the relevant
// notes from the vault so they can be injected into the DA planner's context.
// This is Kernl's keystone seam — notes feed the planner automatically instead
// of being hunted down and pasted by hand.
package planning

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/relate"
	"github.com/gabrielassisxyz/kernl/internal/graph/search"
	"github.com/gabrielassisxyz/kernl/internal/memory"
)

// maxContextClaims caps how many memory claims supplement the notes in a single
// planning context, so claims enrich rather than dominate the retrieved set.
const maxContextClaims = 4

// stopwords are common words dropped from a planning seed before retrieval, so
// the content signal keys on the meaningful terms.
var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "should": true, "with": true,
	"how": true, "what": true, "why": true, "this": true, "that": true,
	"are": true, "was": true, "but": true, "not": true, "you": true,
	"can": true, "our": true, "out": true, "use": true, "into": true,
	"a": true, "an": true, "of": true, "to": true, "in": true, "is": true,
	"do": true, "we": true, "it": true, "on": true, "or": true, "be": true,
}

// salientTerms splits a seed into lowercased, de-duplicated content terms,
// dropping stopwords and very short tokens.
func salientTerms(seed string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range strings.Fields(strings.ToLower(seed)) {
		t := strings.Trim(raw, ".,;:!?\"'()[]{}")
		if len(t) < 3 || stopwords[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// ContextNote is one vault note surfaced as planning context.
type ContextNote struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
	Via     string  `json:"via"` // "content" (FTS) or "linked" (structural)
}

const snippetLen = 240

// BuildContext returns the notes most relevant to seed, newest signal first.
// It fuses two signals: full-text content match against the seed (topical) and,
// when seed is itself a node id, structural relatedness (links/tags/sources).
// Content match is what makes a fresh topic surface the right notes — structural
// relevance alone cannot, since a new topic shares no edges yet.
func BuildContext(ctx context.Context, g *graph.Graph, seed string, limit int) ([]ContextNote, error) {
	if limit <= 0 {
		limit = 8
	}
	seed = strings.TrimSpace(seed)
	if seed == "" {
		return []ContextNote{}, nil
	}

	out := make([]ContextNote, 0, limit)
	seen := map[string]bool{}

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		// 1. Content signal. search.Search treats its argument as an exact
		// phrase, so a natural-language seed would match nothing. Instead, split
		// the seed into salient terms, search each, and rank notes by how many
		// distinct terms they match (OR semantics with a relevance bias).
		type agg struct {
			title    string
			matches  int
			bestRank float64
		}
		scored := map[string]*agg{}
		for _, term := range salientTerms(seed) {
			hits, err := search.Search(ctx, tx, term, search.WithTypes("note"))
			if err != nil {
				continue // a single bad term must not sink the whole retrieval
			}
			for _, h := range hits {
				a := scored[h.NodeID]
				if a == nil {
					a = &agg{title: h.Title, bestRank: h.Rank}
					scored[h.NodeID] = a
				}
				a.matches++
				if h.Rank < a.bestRank {
					a.bestRank = h.Rank
				}
			}
		}

		ranked := make([]ContextNote, 0, len(scored))
		for id, a := range scored {
			ranked = append(ranked, ContextNote{
				ID: id, Title: a.title, Snippet: snippet(tx, id),
				Score: float64(a.matches) - a.bestRank/1000, Via: "content",
			})
		}
		// Most distinct terms matched first; FTS rank breaks ties.
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
		for _, n := range ranked {
			if len(out) >= limit {
				break
			}
			seen[n.ID] = true
			out = append(out, n)
		}

		// 2. Structural signal: if the seed is a node id, fold in its neighbours.
		if isNodeID(tx, seed) {
			ids, err := relate.RelatedTo(ctx, tx, seed, limit)
			if err != nil {
				return fmt.Errorf("planning: relate: %w", err)
			}
			for _, id := range ids {
				if len(out) >= limit || seen[id] {
					continue
				}
				var title, typ string
				if err := tx.QueryRow(
					`SELECT title, type FROM nodes WHERE id = ? AND deleted_at IS NULL`, id,
				).Scan(&title, &typ); err != nil || typ != "note" {
					continue
				}
				seen[id] = true
				out = append(out, ContextNote{ID: id, Title: title, Snippet: snippet(tx, id), Via: "linked"})
			}
		}

		// 3. Memory signal: surface active (non-refuted) claims matching the
		// seed's salient terms. Claims supplement notes — they get their own
		// capped budget so they cannot crowd out the topical/structural notes.
		claims, err := matchClaims(ctx, tx, seed)
		if err != nil {
			return fmt.Errorf("planning: claims: %w", err)
		}
		for _, c := range claims {
			out = append(out, ContextNote{
				ID: c.ID, Title: c.Title, Snippet: truncateSnippet(c.Statement), Via: "claim",
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// matchClaims returns the active memory claims most relevant to seed, ranked by
// how many of the seed's salient terms they match (FTS over claim title,
// statement and tags), with refuted claims filtered out and the result capped
// at maxContextClaims. It mirrors the content-signal aggregation used for notes.
func matchClaims(ctx context.Context, tx *graph.ReadTx, seed string) ([]*nodes.MemoryClaim, error) {
	type agg struct {
		matches  int
		bestRank float64
	}
	scored := map[string]*agg{}
	for _, term := range salientTerms(seed) {
		hits, err := search.Search(ctx, tx, term, search.WithTypes("memory_claim"))
		if err != nil {
			continue // a single bad term must not sink the whole retrieval
		}
		for _, h := range hits {
			a := scored[h.NodeID]
			if a == nil {
				a = &agg{bestRank: h.Rank}
				scored[h.NodeID] = a
			}
			a.matches++
			if h.Rank < a.bestRank {
				a.bestRank = h.Rank
			}
		}
	}
	if len(scored) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(scored))
	for id := range scored {
		ids = append(ids, id)
	}
	// Most distinct terms matched first; FTS rank breaks ties.
	sort.Slice(ids, func(i, j int) bool {
		ai, aj := scored[ids[i]], scored[ids[j]]
		if ai.matches != aj.matches {
			return ai.matches > aj.matches
		}
		return ai.bestRank < aj.bestRank
	})

	ranked := make([]*nodes.MemoryClaim, 0, len(ids))
	for _, id := range ids {
		c, err := nodes.GetMemoryClaim(ctx, tx, id)
		if err != nil {
			continue // tolerate a claim that vanished between FTS hit and load
		}
		ranked = append(ranked, c)
	}

	active, err := memory.FilterRefuted(ctx, tx, ranked)
	if err != nil {
		return nil, err
	}
	if len(active) > maxContextClaims {
		active = active[:maxContextClaims]
	}
	return active, nil
}

func isNodeID(tx *graph.ReadTx, s string) bool {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, s).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

func snippet(tx *graph.ReadTx, nodeID string) string {
	var body string
	if err := tx.QueryRow(
		`SELECT COALESCE(json_extract(attrs, '$.body'), '') FROM nodes WHERE id = ?`, nodeID,
	).Scan(&body); err != nil {
		return ""
	}
	return truncateSnippet(body)
}

// truncateSnippet collapses whitespace and caps text at snippetLen, appending
// an ellipsis when truncated. Shared by note bodies and claim statements.
func truncateSnippet(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > snippetLen {
		return strings.TrimSpace(text[:snippetLen]) + "…"
	}
	return text
}
