package revisions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// lineDiffOp mirrors the nodes package struct for deserialization.
type lineDiffOp struct {
	Op   string `json:"op"`
	Line int    `json:"ln"`
	Text string `json:"t"`
}

// lineDiffPayload is a self-describing line-diff revision payload.
type lineDiffPayload struct {
	Ops []lineDiffOp `json:"ops"`
}

// snapshotPayload is a self-describing snapshot revision payload.
type snapshotPayload struct {
	Title string   `json:"title"`
	Attrs string   `json:"attrs"`
	Tags  []string `json:"tags"`
}

// ReconstructBody rebuilds the body string for a node at a specific revision
// by walking the parent chain backward from the target to find the nearest
// snapshot, then applying diffs forward to the target.
//
// If the revision does not exist it returns "", graph.ErrNotFound.
func ReconstructBody(ctx context.Context, tx *graph.ReadTx, nodeID, targetRevID string) (string, error) {
	// 1. Walk parent chain from target backward to find nearest snapshot.
	revs, err := List(ctx, tx, nodeID)
	if err != nil {
		return "", fmt.Errorf("ReconstructBody: list: %w", err)
	}

	// Build a map for O(1) lookups by ID.
	revMap := make(map[string]Revision, len(revs))
	for _, r := range revs {
		revMap[r.ID] = r
	}

	targetRev, ok := revMap[targetRevID]
	if !ok {
		return "", graph.ErrNotFound
	}

	// Walk backward collecting the chain of revisions from target to the
	// nearest snapshot (inclusive for snapshot, exclusive for target).
	var chain []Revision
	current := targetRev
	for {
		chain = append(chain, current)
		// Check if this is a snapshot revision.
		if isSnapshot(current) {
			break
		}
		if current.ParentID == nil {
			break
		}
		prev, ok := revMap[*current.ParentID]
		if !ok {
			break
		}
		current = prev
	}

	// chain is [target, ..., snapshot] — we need to reverse to [snapshot, ..., target].
	reverseRevisions(chain)

	// 2. Start from the snapshot and apply diffs forward.
	body, err := bodyFromSnapshot(chain[0])
	if err != nil {
		return "", fmt.Errorf("ReconstructBody: snapshot: %w", err)
	}

	// Apply each diff in the chain after the snapshot.
	for i := 1; i < len(chain); i++ {
		body, err = applyDiff(body, chain[i].Diff)
		if err != nil {
			return "", fmt.Errorf("ReconstructBody: apply diff at %s: %w", chain[i].ID, err)
		}
	}

	return body, nil
}

// isSnapshot returns true if the revision's diff contains a snapshot payload.
// Legacy payloads (no "ops" key) are treated as snapshots.
func isSnapshot(r Revision) bool {
	var checker struct {
		Ops []json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(r.Diff, &checker); err != nil {
		return true // unparseable → treat as snapshot
	}
	return len(checker.Ops) == 0
}

// bodyFromSnapshot extracts the body from a snapshot revision diff.
func bodyFromSnapshot(r Revision) (string, error) {
	var s snapshotPayload
	if err := json.Unmarshal(r.Diff, &s); err != nil {
		// Try legacy snapshot format (title/attrs/tags).
		var legacy struct {
			Title string `json:"title"`
			Attrs string `json:"attrs"`
			Tags  []string `json:"tags"`
		}
		if err2 := json.Unmarshal(r.Diff, &legacy); err2 != nil {
			return "", fmt.Errorf("bodyFromSnapshot: %w", err)
		}
		s = snapshotPayload(legacy)
	}

	// Parse body from attrs JSON.
	if s.Attrs == "" {
		return "", nil
	}
	var attrs struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(s.Attrs), &attrs); err != nil {
		return "", fmt.Errorf("bodyFromSnapshot: attrs: %w", err)
	}
	return attrs.Body, nil
}

// applyDiff applies a line-diff payload to a body, returning the result.
func applyDiff(body string, diff json.RawMessage) (string, error) {
	var payload lineDiffPayload
	if err := json.Unmarshal(diff, &payload); err != nil {
		// Not a line-diff — treat as snapshot.
		b, err := bodyFromSnapshot(Revision{Diff: diff})
		if err != nil {
			return "", fmt.Errorf("applyDiff: %w", err)
		}
		return b, nil
	}
	if len(payload.Ops) == 0 {
		return body, nil
	}

	lines := splitLines(body)
	for _, op := range payload.Ops {
		switch op.Op {
		case "+":
			if op.Line < 0 {
				continue
			}
			if op.Line > len(lines) {
				// Append beyond current length; pad with empty lines.
				for len(lines) < op.Line {
					lines = append(lines, "")
				}
				lines = append(lines, op.Text)
			} else {
				lines = append(lines[:op.Line], append([]string{op.Text}, lines[op.Line:]...)...)
			}
		case "-":
			if op.Line >= 0 && op.Line < len(lines) {
				lines = append(lines[:op.Line], lines[op.Line+1:]...)
			}
		}
	}
	return strings.Join(lines, "\n"), nil
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func reverseRevisions(revs []Revision) {
	for i, j := 0, len(revs)-1; i < j; i, j = i+1, j-1 {
		revs[i], revs[j] = revs[j], revs[i]
	}
}
