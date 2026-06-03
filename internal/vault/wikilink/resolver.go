package wikilink

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/google/uuid"
)

// Resolver resolves wikilinks against a graph instance.
type Resolver struct{}

// ResolveOutcome describes what happened for a single link resolution.
type ResolveOutcome struct {
	Link       Link
	Resolved   bool
	EdgeID     string // set when resolved
	ResolvedBy string // "uuid", "stem", "title", or "" if dangling
}

// Resolve parses and resolves all wikilinks in body for the given source node.
// Resolved links become edges (label "links_to"); unresolved links are stored
// as dangling_links rows.
// Returns the outcomes for each link found.
func (r *Resolver) Resolve(ctx context.Context, g *graph.Graph, srcNodeID, body string) ([]ResolveOutcome, error) {
	var outcomes []ResolveOutcome
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		outcomes, err = resolveInTx(ctx, tx, srcNodeID, body)
		return err
	})
	if err != nil {
		return nil, err
	}
	return outcomes, nil
}

// ResolveInTx performs wikilink resolution inside a caller-supplied write tx.
// Callers that need single-transaction-per-event discipline should use this
// variant and supply the tx from their own g.DoWrite block.
func (r *Resolver) ResolveInTx(ctx context.Context, tx *graph.WriteTx, srcNodeID, body string) ([]ResolveOutcome, error) {
	return resolveInTx(ctx, tx, srcNodeID, body)
}

// resolveInTx is the inner implementation shared by Resolve and ResolveInTx.
func resolveInTx(ctx context.Context, tx *graph.WriteTx, srcNodeID, body string) ([]ResolveOutcome, error) {
	links := Parse(body)

	// Re-indexing rebuilds this source's complete outgoing link state, so clear
	// its stale rows first — both resolved links_to edges and unresolved dangling
	// links. This runs before the empty-body early return so that removing every
	// wikilink from a note also clears its old edges/dangling rows, and it makes
	// re-indexing idempotent (a repeated change does not duplicate edges).
	if _, err := tx.Exec(`DELETE FROM edges WHERE src = ? AND label = ?`, srcNodeID, string(edges.EdgeTypeLinksTo)); err != nil {
		return nil, fmt.Errorf("resolver: clear old edges: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM dangling_links WHERE src_node_id = ?`, srcNodeID); err != nil {
		return nil, fmt.Errorf("resolver: clear old dangling: %w", err)
	}

	if len(links) == 0 {
		return nil, nil
	}

	outcomes := make([]ResolveOutcome, 0, len(links))

	for _, link := range links {
		kind := ClassifyTarget(link.Target)

		var dstNodeID string
		var resolvedBy string

		switch kind {
		case KindUUID:
			dstNodeID = link.Target
			resolvedBy = "uuid"
		case KindStem:
			// Look up by stem in note_paths
			var foundID string
			err := tx.QueryRow(
				`SELECT uuid FROM note_paths WHERE path = ? LIMIT 1`,
				link.Target+".md",
			).Scan(&foundID)
			if err == nil {
				dstNodeID = foundID
				resolvedBy = "stem"
			} else {
				// Fall back to title lookup
				err2 := tx.QueryRow(
					`SELECT id FROM nodes WHERE type = 'note' AND title = ? AND deleted_at IS NULL ORDER BY id ASC LIMIT 1`,
					link.Target,
				).Scan(&foundID)
				if err2 == nil {
					dstNodeID = foundID
					resolvedBy = "title"
				}
			}
		case KindTitle:
			var foundID string
			err := tx.QueryRow(
				`SELECT id FROM nodes WHERE type = 'note' AND title = ? AND deleted_at IS NULL ORDER BY id ASC LIMIT 1`,
				link.Target,
			).Scan(&foundID)
			if err == nil {
				dstNodeID = foundID
				resolvedBy = "title"
			}
		}

		if dstNodeID != "" && dstNodeID != srcNodeID {
			// Resolved — create edge
			attrsJSON := buildLinkAttrs(link.Target, resolvedBy, false)
			edgeID, err := edges.Create(ctx, tx, edges.Edge{
				Src:   srcNodeID,
				Dst:   dstNodeID,
				Label: string(edges.EdgeTypeLinksTo),
				Attrs: attrsJSON,
			}, nodes.Author{Name: "agent:wikilink-resolver"})
			if err != nil {
				return nil, fmt.Errorf("resolver: create edge %s -> %s: %w", srcNodeID, dstNodeID, err)
			}
			outcomes = append(outcomes, ResolveOutcome{
				Link:       link,
				Resolved:   true,
				EdgeID:     edgeID,
				ResolvedBy: resolvedBy,
			})
		} else if dstNodeID == srcNodeID {
			// Self-link — resolved but no edge created
			outcomes = append(outcomes, ResolveOutcome{
				Link:       link,
				Resolved:   true,
				ResolvedBy: resolvedBy,
			})
		} else {
			// Unresolved — store as dangling
			targetKind := string(kind)
			if targetKind == "" {
				targetKind = "stem"
			}
			u, _ := uuid.NewV7()
			id := "dl-" + u.String()
			if _, err := tx.Exec(
				`INSERT INTO dangling_links (id, src_node_id, target_key, target_kind) VALUES (?, ?, ?, ?)`,
				id, srcNodeID, link.Target, targetKind,
			); err != nil {
				return nil, fmt.Errorf("resolver: insert dangling: %w", err)
			}
			outcomes = append(outcomes, ResolveOutcome{
				Link:     link,
				Resolved: false,
			})
		}
	}
	return outcomes, nil
}

// PromoteKey represents a key that can be matched against dangling_links
// when a new note appears.
type PromoteKey struct {
	Key  string
	Kind string // "stem" or "title"
}

// PromoteDangling scans dangling_links for rows matching the given keys and
// promotes each match to a real edge, then deletes the dangling row.
// The keys should be derived from the new note's filename stem and title.
// Returns the count of promoted edges. Idempotent — calling twice produces
// no duplicate edges.
func PromoteDangling(ctx context.Context, g *graph.Graph, noteID string, keys ...PromoteKey) (int, error) {
	if len(keys) == 0 {
		return 0, nil
	}

	var promoted int
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		promoted, err = promoteDanglingInTx(ctx, tx, noteID, keys...)
		return err
	})
	if err != nil {
		return promoted, err
	}
	return promoted, nil
}

// PromoteDanglingInTx promotes dangling links inside a caller-supplied write tx.
// Callers that need single-transaction-per-event discipline should use this
// variant and supply the tx from their own g.DoWrite block.
func PromoteDanglingInTx(ctx context.Context, tx *graph.WriteTx, noteID string, keys ...PromoteKey) (int, error) {
	return promoteDanglingInTx(ctx, tx, noteID, keys...)
}

// promoteDanglingInTx is the inner implementation shared by PromoteDangling and PromoteDanglingInTx.
func promoteDanglingInTx(ctx context.Context, tx *graph.WriteTx, noteID string, keys ...PromoteKey) (int, error) {
	if len(keys) == 0 {
		return 0, nil
	}

	var promoted int
	for _, key := range keys {
		rows, err := tx.Query(
			`SELECT id, src_node_id, target_key, target_kind FROM dangling_links WHERE target_key = ? AND target_kind = ?`,
			key.Key, key.Kind,
		)
		if err != nil {
			return promoted, fmt.Errorf("PromoteDangling: query: %w", err)
		}

		type danglingRow struct {
			id, srcNodeID, targetKey, targetKind string
		}
		var matches []danglingRow
		for rows.Next() {
			var dr danglingRow
			if err := rows.Scan(&dr.id, &dr.srcNodeID, &dr.targetKey, &dr.targetKind); err != nil {
				rows.Close()
				return promoted, fmt.Errorf("PromoteDangling: scan: %w", err)
			}
			matches = append(matches, dr)
		}
		rows.Close()

		for _, dr := range matches {
			if dr.srcNodeID == noteID {
				// Self-link — just delete the dangling row, no edge
				if _, err := tx.Exec(`DELETE FROM dangling_links WHERE id = ?`, dr.id); err != nil {
					return promoted, fmt.Errorf("PromoteDangling: delete self-dangling: %w", err)
				}
				continue
			}

			attrsJSON := buildLinkAttrs(dr.targetKey, dr.targetKind, true)
			_, err := edges.Create(ctx, tx, edges.Edge{
				Src:   dr.srcNodeID,
				Dst:   noteID,
				Label: string(edges.EdgeTypeLinksTo),
				Attrs: attrsJSON,
			}, nodes.Author{Name: "agent:wikilink-resolver"})
			if err != nil {
				// If edge creation fails (e.g. dst node doesn't exist or
				// duplicate), still delete the dangling row to avoid
				// accumulating stale entries.
				if !strings.Contains(err.Error(), "edges.Create: dst node") {
					return promoted, fmt.Errorf("PromoteDangling: create edge %s -> %s: %w", dr.srcNodeID, noteID, err)
				}
			}
			// Delete the dangling row
			if _, err := tx.Exec(`DELETE FROM dangling_links WHERE id = ?`, dr.id); err != nil {
				return promoted, fmt.Errorf("PromoteDangling: delete: %w", err)
			}
			promoted++
		}
	}
	return promoted, nil
}

// buildLinkAttrs marshals provenance attributes for a wikilink edge.
func buildLinkAttrs(targetText, resolvedBy string, promoted bool) json.RawMessage {
	m := map[string]interface{}{
		"target_text": targetText,
		"resolved_by": resolvedBy,
	}
	if promoted {
		m["promoted"] = true
	}
	b, _ := json.Marshal(m)
	return json.RawMessage(b)
}
