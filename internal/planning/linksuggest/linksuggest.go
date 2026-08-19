// Package linksuggest turns a note being written into link candidates drawn
// from the vault, so a writer (the CLI assistant) can connect a fresh note to
// what it is about instead of leaving it disconnected.
package linksuggest

import (
	"context"
	"path"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/planning"
	"github.com/gabrielassisxyz/kernl/internal/vault/wikilink"
)

// Suggest returns the notes most relevant to seed as link candidates. It is
// BuildContext minus the memory claims: a claim is not a note and has no file,
// so it can never be a link target.
//
// excludeID, when non-empty, drops the note being written from the candidates:
// from the second write of a note onward it is already in the graph, matches
// its own body, and would otherwise be offered as a link to itself. An empty
// excludeID excludes nothing, so the first write of a note (which has no id
// yet) is unaffected.
func Suggest(ctx context.Context, g *graph.Graph, seed string, limit int, excludeID string) ([]nodes.LinkCandidate, error) {
	// One slot over when there is something to exclude, then trimmed back. The
	// point of dropping the self-candidate is that it was costing a real one;
	// filtering a list already closed at limit removes the bad link and keeps
	// the loss, which is half the fix.
	fetch := limit
	if excludeID != "" && limit > 0 {
		fetch = limit + 1
	}
	notes, err := planning.BuildContext(ctx, g, seed, fetch)
	if err != nil {
		return nil, err
	}
	out := make([]nodes.LinkCandidate, 0, len(notes))
	for _, n := range notes {
		// A link candidate must be a note. A claim (memory_claim) has no file,
		// and a task or project is an entity, not a note - none of them can be
		// a wikilink target. BuildContext now returns tasks and projects
		// alongside notes, so this filter must drop them too or every note
		// written through the CLI starts being offered links to task nodes.
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
// the body carries a wikilink whose target matches its id, title, or filename
// stem; everything else is rejected. Pure: it reads only its arguments.
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

// linked reports whether a candidate is referenced by the body's wikilink
// targets, matching on id, title, or the filename stem of its path.
func linked(c nodes.LinkCandidate, targets map[string]bool) bool {
	if targets[c.ID] || targets[c.Title] {
		return true
	}
	if c.Path != nil {
		stem := strings.TrimSuffix(path.Base(*c.Path), ".md")
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
