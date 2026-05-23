package relate

import (
	"context"
	"fmt"
	"sort"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// RelatedTo returns the top limit structurally-related node IDs sorted by
// relevance score descending (tie-break by node id ascending). The score
// is raw and un-normalised using the four signals defined in the scope.
//
// Candidate set is the structural neighbourhood only:
// direct neighbours, common-neighbour reachable nodes, and provenance
// co-neighbours. Type affinity NEVER generates candidates.
func RelatedTo(ctx context.Context, tx *graph.ReadTx, nodeID string, limit int) ([]string, error) {
	if limit <= 0 {
		return []string{}, nil
	}

	candSet, err := discoverCandidates(ctx, tx, nodeID)
	if err != nil {
		return nil, err
	}
	if len(candSet) == 0 {
		return []string{}, nil
	}

	type scored struct {
		id    string
		score float64
	}
	scores := make([]scored, 0, len(candSet))
	for cid := range candSet {
		// skip self
		if cid == nodeID {
			continue
		}
		s, err := scoreBetween(tx, nodeID, cid)
		if err != nil {
			return nil, err
		}
		scores = append(scores, scored{id: cid, score: s})
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score == scores[j].score {
			return scores[i].id < scores[j].id
		}
		return scores[i].score > scores[j].score
	})

	if limit > len(scores) {
		limit = len(scores)
	}

	out := make([]string, limit)
	for i := range out {
		out[i] = scores[i].id
	}
	return out, nil
}

// discoverCandidates returns a deduplicated set of candidate node IDs using
// structural signals only. It unions three sources:
//   1. Direct neighbours (any edge direction, any label).
//   2. Nodes reachable within two undirected hops (common neighbourhood).
//   3. Provenance co-neighbours (nodes sharing a provenance source).
func discoverCandidates(ctx context.Context, tx *graph.ReadTx, origin string) (map[string]struct{}, error) {
	cands := make(map[string]struct{})

	// 1. Direct neighbours
	rows, err := tx.Query(`
		SELECT CASE WHEN src = ? THEN dst ELSE src END
		FROM edges WHERE src = ? OR dst = ?
	`, origin, origin, origin)
	if err != nil {
		return nil, fmt.Errorf("relate.discoverCandidates: direct: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("relate.discoverCandidates: direct scan: %w", err)
		}
		cands[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("relate.discoverCandidates: direct rows: %w", err)
	}

	// 2. Two-hop structural neighbourhood (common neighbours + distance-2)
	rows, err = tx.Query(`
		WITH RECURSIVE near(id, steps) AS (
			SELECT ?, 0
			UNION
			SELECT CASE WHEN e.src = r.id THEN e.dst ELSE e.src END, r.steps + 1
			FROM near r
			JOIN edges e ON (e.src = r.id OR e.dst = r.id)
			WHERE r.steps < 2
		)
		SELECT DISTINCT id FROM near WHERE id != ?
	`, origin, origin)
	if err != nil {
		return nil, fmt.Errorf("relate.discoverCandidates: near: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("relate.discoverCandidates: near scan: %w", err)
		}
		cands[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("relate.discoverCandidates: near rows: %w", err)
	}

	// 3. Provenance co-neighbours (shared provenance source)
	if len(provenanceLabels) > 0 {
		ph := placeholders(len(provenanceLabels))
		args := make([]any, 0, 2+2*len(provenanceLabels))
		args = append(args, origin, origin)
		for _, l := range provenanceLabels {
			args = append(args, l)
		}
		for _, l := range provenanceLabels {
			args = append(args, l)
		}

		query := `
			SELECT DISTINCT e2.dst FROM edges e1
			JOIN edges e2 ON e1.src = e2.src
			WHERE e1.dst = ? AND e2.dst != ?
			  AND e1.label IN (` + ph + `)
			  AND e2.label IN (` + ph + `)`

		rows, err = tx.Query(query, args...)
		if err != nil {
			return nil, fmt.Errorf("relate.discoverCandidates: provenance: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, fmt.Errorf("relate.discoverCandidates: provenance scan: %w", err)
			}
			cands[id] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("relate.discoverCandidates: provenance rows: %w", err)
		}
	}

	return cands, nil
}
