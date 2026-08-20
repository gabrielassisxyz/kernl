// Package linksuggest turns a note being written into link candidates drawn
// from the vault, so a writer (the CLI assistant) can connect a fresh note to
// what it is about instead of leaving it disconnected.
package linksuggest

import (
	"context"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/planning"
	"github.com/gabrielassisxyz/kernl/internal/vault/companion"
	"github.com/gabrielassisxyz/kernl/internal/vault/wikilink"
)

// Suggest returns the notes most relevant to seed as link candidates. It is
// BuildContextForLinking minus the memory claims: a claim is not a note and has
// no file, so it can never be a link target.
//
// The linking variant rather than plain BuildContext because the seed here is a
// whole note rather than a question, and against a seed that long every large
// document matches something. See BuildContextForLinking for the measurement.
//
// excludeID, when non-empty, drops the note being written from the candidates:
// from the second write of a note onward it is already in the graph, matches
// its own body, and would otherwise be offered as a link to itself. An empty
// excludeID excludes nothing, so the first write of a note (which has no id
// yet) is unaffected.
func Suggest(ctx context.Context, g *graph.Graph, seed string, limit int, excludeID string) ([]nodes.LinkCandidate, error) {
	seed = withEntityContext(ctx, g, seed, excludeID)
	// One slot over when there is something to exclude, then trimmed back. The
	// point of dropping the self-candidate is that it was costing a real one;
	// filtering a list already closed at limit removes the bad link and keeps
	// the loss, which is half the fix.
	fetch := limit
	if excludeID != "" && limit > 0 {
		fetch = limit + 1
	}
	notes, err := planning.BuildContextForLinking(ctx, g, seed, fetch)
	if err != nil {
		return nil, err
	}
	out := make([]nodes.LinkCandidate, 0, len(notes))
	for _, n := range notes {
		// A link candidate must be a note: a claim (memory_claim) has no file,
		// and a task or project is an entity, not a wikilink target. The linking
		// retrieval is notes-only at the source, so this is the backstop for
		// anything it grows later - filtering here is not enough on its own,
		// because an entity that reaches the ranking takes the slot of its own
		// companion note before this can drop it.
		if n.Type != "note" {
			continue
		}
		if excludeID != "" && n.ID == excludeID {
			continue
		}
		out = append(out, nodes.LinkCandidate{
			ID:      n.ID,
			Title:   n.Title,
			Path:    n.Path,
			Snippet: n.Snippet,
		})
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// DeriveReceipts splits previously-offered candidates into accepted and
// rejected by whether the body now links to them. A candidate is accepted when
// the body carries a wikilink the RESOLVER would turn into an edge to it, and
// nothing else is: this function's answer and the graph must not disagree.
// Pure: it reads only its arguments.
func DeriveReceipts(prev []nodes.LinkCandidate, body string) (accepted, rejected []nodes.LinkCandidate) {
	links := wikilink.Parse(body)
	targets := make(map[string]bool, len(links))
	for _, l := range links {
		targets[l.Target] = true
	}
	for _, c := range prev {
		if linked(c, targets) {
			accepted = append(accepted, c)
		} else {
			rejected = append(rejected, c)
		}
	}
	return accepted, rejected
}

// withEntityContext adds what a companion note is ABOUT to its seed.
//
// A companion is created holding one generated line, "Notes for [[id|title]].",
// and nothing else until somebody writes in it. Measured 2026-08-20: 468 of 636
// companions still have under 200 bytes of indexed body. Seeding link suggestion
// on that body asks the ranker to find neighbours for a template, and it answers
// accordingly - a companion for the homelab project was offered notes about the
// editor UI, matched on the word "Notes".
//
// The subject is not missing, it is one edge away. The entity's title and
// description are what the companion is for, and they are already indexed on the
// entity's own node. Appending rather than replacing, so a companion somebody has
// written in keeps what they wrote and gains the context around it.
//
// Everything is best-effort: a note with no entity, a deleted entity or a failed
// read leaves the seed exactly as it came in. A worse seed is a worse suggestion,
// never a failed write.
func withEntityContext(ctx context.Context, g *graph.Graph, seed, noteID string) string {
	if noteID == "" || g == nil {
		return seed
	}
	var title, description string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`
			SELECT COALESCE(e.title, ''), COALESCE(json_extract(e.attrs, '$.description'), '')
			FROM edges ed
			JOIN nodes e ON e.id = ed.dst AND e.deleted_at IS NULL
			WHERE ed.src = ? AND ed.label = ?
			LIMIT 1`, noteID, companion.EdgeLabel).Scan(&title, &description)
	})
	if err != nil {
		return seed
	}
	extra := strings.TrimSpace(title + "\n" + description)
	if extra == "" {
		return seed
	}
	return strings.TrimSpace(seed + "\n" + extra)
}

// linked mirrors the three routes wikilink.Resolver takes, in the same order:
// the uuid, the vault-relative path with .md dropped, and the title.
//
// The stem is the WHOLE path and not its base name, because that is the key the
// resolver looks up (`note_paths WHERE path = target || '.md'`). Accepting the
// base name here made this function claim an accepted link the graph never grew:
// measured 2026-08-20 over a 20-note batch, 20 candidates came back accepted and
// only 7 edges existed, every miss being a note outside the vault root, which is
// every companion. A receipt more permissive than the resolver is worse than no
// receipt, because it is believed.
func linked(c nodes.LinkCandidate, targets map[string]bool) bool {
	if targets[c.ID] || targets[c.Title] {
		return true
	}
	if c.Path != nil {
		stem := strings.TrimSuffix(*c.Path, ".md")
		if stem != "" && targets[stem] {
			return true
		}
	}
	return false
}

// ShouldSuggest reports whether a write from the given channel should offer
// link suggestions. The user writes in the web UI and finding the connections
// himself is part of why he writes; the assistant writes through the CLI and
// needs the help. An unidentified client defaults to suggesting: an unwanted
// suggestion is a field in a JSON response nobody reads, while a missing one is
// a note born disconnected.
func ShouldSuggest(channel string) bool {
	return channel != "ui"
}
