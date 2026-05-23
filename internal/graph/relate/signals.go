package relate

import (
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// scoreBetween returns the raw weighted score between a and b.
func scoreBetween(tx *graph.ReadTx, a, b string) (float64, error) {
	score := 0.0

	dl, err := directLink(tx, a, b)
	if err != nil {
		return 0, err
	}
	score += WeightDirectLink * dl

	so, err := sourceOverlap(tx, a, b)
	if err != nil {
		return 0, err
	}
	score += WeightSourceOverlap * float64(so)

	aa, err := adamicAdar(tx, a, b)
	if err != nil {
		return 0, err
	}
	score += WeightAdamicAdar * aa

	score += WeightTypeAffinity * typeAffinity(tx, a, b)

	return score, nil
}

// directLink returns 1 if any edge exists between a and b (either direction,
// any label), otherwise 0.
func directLink(tx *graph.ReadTx, a, b string) (float64, error) {
	var n int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM edges
		WHERE (src = ? AND dst = ?) OR (src = ? AND dst = ?)
	`, a, b, b, a).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("relate.directLink: %w", err)
	}
	if n > 0 {
		return 1.0, nil
	}
	return 0.0, nil
}

// sourceOverlap counts the number of distinct source nodes S that have a
// provenance edge to both a and b.
func sourceOverlap(tx *graph.ReadTx, a, b string) (int, error) {
	if len(provenanceLabels) == 0 {
		return 0, nil
	}
	phSQL := placeholders(len(provenanceLabels))
	if phSQL == "" {
		return 0, nil
	}
	args := make([]any, 0, 2+2*len(provenanceLabels))
	args = append(args, a, b)
	for _, l := range provenanceLabels {
		args = append(args, l)
	}
	for _, l := range provenanceLabels {
		args = append(args, l)
	}
	var count int
	err := tx.QueryRow(`
		SELECT COUNT(DISTINCT s1.src)
		FROM edges s1
		JOIN edges s2 ON s1.src = s2.src
		WHERE s1.dst = ? AND s2.dst = ?
		  AND s1.label IN (`+phSQL+`)
		  AND s2.label IN (`+phSQL+`)
	`, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("relate.sourceOverlap: %w", err)
	}
	return count, nil
}

// adamicAdar returns the Adamic-Adar similarity using natural log.
func adamicAdar(tx *graph.ReadTx, a, b string) (float64, error) {
	// Common undirected neighbors
	rows, err := tx.Query(`
		SELECT n.id FROM nodes n
		WHERE n.id != ? AND n.id != ?
		  AND n.id IN (
			SELECT CASE WHEN e.src = ? THEN e.dst ELSE e.src END
			FROM edges e WHERE e.src = ? OR e.dst = ?
		  )
		  AND n.id IN (
			SELECT CASE WHEN e.src = ? THEN e.dst ELSE e.src END
			FROM edges e WHERE e.src = ? OR e.dst = ?
		  )
	`, a, b, a, a, a, b, b, b)
	if err != nil {
		return 0, fmt.Errorf("relate.adamicAdar: query: %w", err)
	}
	defer rows.Close()

	var sum float64
	for rows.Next() {
		var nid string
		if err := rows.Scan(&nid); err != nil {
			return 0, fmt.Errorf("relate.adamicAdar: scan: %w", err)
		}

		deg, err := undirectedDegree(tx, nid)
		if err != nil {
			return 0, err
		}
		if deg <= 1 {
			continue // guard against ln(1)=0 or ln(<=0) invalid
		}
		sum += 1.0 / math.Log(float64(deg))
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("relate.adamicAdar: rows: %w", err)
	}
	return sum, nil
}

// undirectedDegree counts the number of edges touching nid (either src or dst).
func undirectedDegree(tx *graph.ReadTx, nid string) (int, error) {
	var n int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM edges WHERE src = ? OR dst = ?
	`, nid, nid).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("relate.undirectedDegree: %w", err)
	}
	return n, nil
}

// typeAffinity returns 1.0 if a and b share a type, else CrossTypeAffinity.
func typeAffinity(tx *graph.ReadTx, a, b string) float64 {
	var ta, tb string
	if err := tx.QueryRow(`SELECT type FROM nodes WHERE id = ?`, a).Scan(&ta); err != nil {
		if err == sql.ErrNoRows {
			return CrossTypeAffinity
		}
		return CrossTypeAffinity
	}
	if err := tx.QueryRow(`SELECT type FROM nodes WHERE id = ?`, b).Scan(&tb); err != nil {
		if err == sql.ErrNoRows {
			return CrossTypeAffinity
		}
		return CrossTypeAffinity
	}
	if ta == tb {
		return 1.0
	}
	return CrossTypeAffinity
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]string, n)
	for i := range out {
		out[i] = "?"
	}
	return strings.Join(out, ",")
}
