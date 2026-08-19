// Package planning builds substrate-aware planning context: given a seed
// (the topic the user is about to brainstorm or plan), it pulls the relevant
// notes from the vault so they can be injected into the DA planner's context.
// This is Kernl's keystone seam - notes feed the planner automatically instead
// of being hunted down and pasted by hand.
package planning

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
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

// contextTypes are the node types BuildContext retrieves: notes, tasks and
// projects. A task's reasoning lives in its description and a project's in its
// description, both FTS-indexed, so they are planning context alongside notes.
var contextTypes = []string{"note", "task", "project"}

// stopwords are common words dropped from a planning seed before retrieval, so
// the content signal keys on the meaningful terms.
//
// Both languages, and the Portuguese half is not symmetry for its own sake. A note is
// scored by HOW MANY distinct seed terms it matches, so every word left in the seed counts
// the same whether it is "navegação" or "sobre". The vault and the questions asked of it are
// mostly Portuguese, so an English-only list left four or five junk terms in a typical
// question and handed the top slots to whichever notes happened to contain them. Measured on
// 2026-08-19: "o que já foi decidido sobre navegação de notas" kept que, já, foi and sobre as
// salient terms and did not return its target at any depth up to 40; the same question with
// those words removed returned it at rank 2.
var stopwords = map[string]bool{
	// en
	"the": true, "and": true, "for": true, "should": true, "with": true,
	"how": true, "what": true, "why": true, "this": true, "that": true,
	"are": true, "was": true, "but": true, "not": true, "you": true,
	"can": true, "our": true, "out": true, "use": true, "into": true,
	"a": true, "an": true, "of": true, "to": true, "in": true, "is": true,
	"do": true, "we": true, "it": true, "on": true, "or": true, "be": true,
	// pt
	"que": true, "qual": true, "quais": true, "quando": true, "onde": true,
	"como": true, "por": true, "para": true, "com": true, "sem": true,
	"uma": true, "uns": true, "umas": true, "dos": true, "das": true,
	"nos": true, "nas": true, "aos": true, "ele": true, "ela": true,
	"eles": true, "elas": true, "isso": true, "esse": true, "essa": true,
	"este": true, "esta": true, "isto": true, "aquele": true, "aquela": true,
	"foi": true, "ser": true, "sao": true, "são": true,
	"est": true, "está": true, "estao": true, "estão": true, "tem": true,
	"têm": true, "ter": true, "havia": true, "sobre": true, "entre": true,
	"mais": true, "menos": true, "muito": true, "pouco": true, "todo": true,
	"toda": true, "todos": true, "todas": true, "não": true, "nao": true,
	"sim": true, "meu": true, "minha": true, "seu": true, "sua": true,
	"nosso": true, "nossa": true, "vai": true, "vou": true, "fica": true,
	"ficar": true, "fazer": true, "faz": true, "pode": true, "deve": true,
	"já": true, "jah": true, "ainda": true, "agora": true, "depois": true,
	"antes": true, "aqui": true, "ali": true, "lá": true, "eu": true,
	"coisa": true, "coisas": true,
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
	// matches is the number of distinct seed terms the node matched. It is
	// unexported because it is a ranking detail, not part of the wire contract:
	// the sort ranks by matches first, and Score (matches minus a bestRank
	// adjustment) cannot recover that raw count.
	matches int
	Via     string `json:"via"` // "content" (FTS), "linked" (structural), or "claim" (memory claim)
	// Type is the node's type: "note", "task", "project", or "memory_claim".
	// It lets a consumer tell a note from a task without guessing from a path.
	Type string `json:"type"`
	// Path is the note's vault path, resolved from the reconciler's cache. It
	// is nil - serialized as JSON null - when the node has no file on disk. A
	// claim is not a note that lost its file; it is a different kind of thing
	// that never had one. nil is deliberate: an empty string would be
	// indistinguishable from "I failed to resolve the path", a real failure
	// mode in this system, and an omitted key would break consumers that
	// branch on the field's presence.
	Path *string `json:"path"`
}

const snippetLen = 240

// BuildContext returns the notes most relevant to seed, newest signal first.
// It fuses two signals: full-text content match against the seed (topical) and,
// when seed is itself a node id, structural relatedness (links/tags/sources).
// Content match is what makes a fresh topic surface the right notes - structural
// relevance alone cannot, since a new topic shares no edges yet.
//
// Notes the DA authored (origin "da" - its own briefings) are never returned.
// They are derivatives of the user's captures, and feeding them back as "what
// the user knows" closes a loop the knowledge base does not survive: the inbox
// classifier began proposing that captures be merged into "Briefing:" notes the
// DA had generated from those very captures. The exclusion lives here, at the
// one seam where a note is retrieved as knowledge, rather than at each caller  -
// the rule is a property of the note, not of who is asking.
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
			hits, err := search.Search(ctx, tx, term, search.WithTypes(contextTypes...))
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
			surface, err := fetchNodeSurface(tx, id)
			if err != nil {
				return fmt.Errorf("planning: node surface: %w", err)
			}
			// Notes the DA authored (origin "prep") are never returned: see
			// BuildContext. Tasks and projects carry no origin, so the check
			// applies to notes only.
			if surface.typ == "note" && surface.origin == nodes.OriginPrep {
				continue
			}
			if excludedByState(surface.typ, surface.status) {
				continue
			}
			path, err := notePath(tx, id)
			if err != nil {
				return fmt.Errorf("planning: note path: %w", err)
			}
			ranked = append(ranked, ContextNote{
				ID: id, Title: a.title, Snippet: surface.snippet,
				Score: float64(a.matches) - a.bestRank/1000, Via: "content",
				Type: surface.typ, Path: path, matches: a.matches,
			})
		}
		// Most distinct terms matched first. On a tie, a note beats an entity:
		// a task/project and its companion note are two nodes about one thing,
		// and the note is the canonical representation whose body carries the
		// fuller reasoning. FTS rank breaks the remaining ties.
		sort.Slice(ranked, func(i, j int) bool {
			if ranked[i].matches != ranked[j].matches {
				return ranked[i].matches > ranked[j].matches
			}
			if (ranked[i].Type == "note") != (ranked[j].Type == "note") {
				return ranked[i].Type == "note"
			}
			return ranked[i].Score > ranked[j].Score
		})
		for _, n := range ranked {
			if len(out) >= limit {
				break
			}
			if seen[n.ID] {
				continue
			}
			// A task/project and its companion note are two nodes about one
			// thing and must not occupy two slots. Iterating in score order,
			// the first of the pair reached is the higher-scoring; mark its
			// companion seen so the lower-scoring half is skipped.
			partner, err := companionPartner(tx, n.ID, n.Type)
			if err != nil {
				return fmt.Errorf("planning: companion: %w", err)
			}
			seen[n.ID] = true
			if partner != "" {
				seen[partner] = true
			}
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
				surface, err := fetchNodeSurface(tx, id)
				if errors.Is(err, sql.ErrNoRows) {
					continue // a neighbour deleted between relate and here
				}
				if err != nil {
					return fmt.Errorf("planning: node surface: %w", err)
				}
				if !isContextType(surface.typ) {
					continue
				}
				if surface.typ == "note" && surface.origin == nodes.OriginPrep {
					continue
				}
				if excludedByState(surface.typ, surface.status) {
					continue
				}
				partner, err := companionPartner(tx, id, surface.typ)
				if err != nil {
					return fmt.Errorf("planning: companion: %w", err)
				}
				seen[id] = true
				if partner != "" {
					seen[partner] = true
				}
				path, err := notePath(tx, id)
				if err != nil {
					return fmt.Errorf("planning: note path: %w", err)
				}
				out = append(out, ContextNote{ID: id, Title: surface.title, Snippet: surface.snippet, Via: "linked", Type: surface.typ, Path: path})
			}
		}

		// 3. Memory signal: surface active (non-refuted) claims matching the
		// seed's salient terms. Claims supplement notes - they get their own
		// capped budget so they cannot crowd out the topical/structural notes.
		claims, err := matchClaims(ctx, tx, seed)
		if err != nil {
			return fmt.Errorf("planning: claims: %w", err)
		}
		for _, c := range claims {
			out = append(out, ContextNote{
				ID: c.ID, Title: c.Title, Snippet: truncateSnippet(c.Statement), Via: "claim",
				Type: "memory_claim",
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

// nodeSurface is the planning-relevant surface of a node: its title, type,
// status (empty for notes), origin (empty for non-notes), and snippet text -
// the body for a note, the description for a task or project, since that is
// the text FTS indexed for each.
type nodeSurface struct {
	title   string
	typ     string
	status  string
	origin  string
	snippet string
}

// fetchNodeSurface reads a node's planning surface in one query. It returns
// sql.ErrNoRows when the node does not exist (deleted between discovery and
// here), which the structural signal treats as "skip" and the content signal
// never sees because search already filters deleted nodes.
func fetchNodeSurface(tx *graph.ReadTx, nodeID string) (nodeSurface, error) {
	var title, typ string
	var status, body, desc, origin sql.NullString
	err := tx.QueryRow(`
		SELECT title, type,
		       json_extract(attrs, '$.status'),
		       json_extract(attrs, '$.body'),
		       json_extract(attrs, '$.description'),
		       json_extract(attrs, '$.origin')
		FROM nodes WHERE id = ? AND deleted_at IS NULL`, nodeID,
	).Scan(&title, &typ, &status, &body, &desc, &origin)
	if err != nil {
		return nodeSurface{}, err
	}
	text := body.String
	if typ != "note" {
		text = desc.String
	}
	return nodeSurface{
		title: title, typ: typ, status: status.String,
		origin: origin.String, snippet: truncateSnippet(text),
	}, nil
}

// isContextType reports whether a node type is one BuildContext retrieves.
func isContextType(typ string) bool {
	return slices.Contains(contextTypes, typ)
}

// excludedByState reports whether a node's status marks it as finished work
// that must not feed planning context: a done or closed task, or an archived
// project. Finished work is not context for planning the next thing; it would
// arrive as pure noise.
func excludedByState(typ, status string) bool {
	switch typ {
	case "task":
		return status == nodes.TaskStatusDone || status == nodes.TaskStatusClosed
	case "project":
		return status == "archived"
	}
	return false
}

// companionPartner returns the id of the node's companion - the entity a
// companion note describes, or the note describing an entity - or "" when the
// node has none. The relationship is the links_to edge the wikilink resolver
// writes when it resolves the [[entity|title]] link in a companion note's body
// (see internal/vault/reconcile): a note links to its entity, an entity is
// linked from its companion note.
func companionPartner(tx *graph.ReadTx, nodeID, typ string) (string, error) {
	var partner string
	var err error
	if typ == "note" {
		err = tx.QueryRow(`
			SELECT dst FROM edges
			WHERE src = ? AND label = 'links_to'
			  AND dst IN (SELECT id FROM nodes WHERE type IN ('task','project') AND deleted_at IS NULL)
			LIMIT 1`, nodeID).Scan(&partner)
	} else {
		err = tx.QueryRow(`
			SELECT src FROM edges
			WHERE dst = ? AND label = 'links_to'
			  AND src IN (SELECT id FROM nodes WHERE type = 'note' AND deleted_at IS NULL)
			LIMIT 1`, nodeID).Scan(&partner)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return partner, nil
}

func isNodeID(tx *graph.ReadTx, s string) bool {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, s).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// notePath resolves a note's vault path from the reconciler's cache. It
// returns (nil, nil) when the node has no file on disk (a claim, or a note
// node created without a file), so the JSON contract can distinguish "no file
// by design" (null) from "failed to resolve" (an error). Any other query
// failure is returned to the caller rather than silently serialized as null.
func notePath(tx *graph.ReadTx, nodeID string) (*string, error) {
	var p string
	err := tx.QueryRow(`SELECT path FROM note_paths WHERE uuid = ?`, nodeID).Scan(&p)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
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
