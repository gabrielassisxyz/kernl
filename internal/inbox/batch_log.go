package inbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// BatchLogRecord persists the original paste, deterministic raw segments,
// final capture candidates, and created capture IDs for a single batch.
// It is stored outside the public graph node namespace so the UI can later
// inspect the raw thread even after LLM grouping discards or merges support
// messages.
type BatchLogRecord struct {
	ID                    string
	Source                string
	Separator             string
	ContextTitle          string
	RawText               string
	RawSegmentsJSON       string
	FinalSegmentsJSON     string
	CreatedCaptureIDsJSON string
	CreatedAt             time.Time
}

// BatchLogStore writes and reads batch_logs records.
type BatchLogStore struct {
	g *graph.Graph
}

// NewBatchLogStore creates a store bound to a graph database.
func NewBatchLogStore(g *graph.Graph) *BatchLogStore {
	return &BatchLogStore{g: g}
}

// Put inserts or replaces a batch log record inside a write transaction.
func (s *BatchLogStore) Put(ctx context.Context, tx *graph.WriteTx, r BatchLogRecord) error {
	createdAt := r.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err := tx.Exec(`
		INSERT INTO batch_logs (
			id, source, separator, context_title, raw_text,
			raw_segments_json, final_segments_json, created_capture_ids_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			source = excluded.source,
			separator = excluded.separator,
			context_title = excluded.context_title,
			raw_text = excluded.raw_text,
			raw_segments_json = excluded.raw_segments_json,
			final_segments_json = excluded.final_segments_json,
			created_capture_ids_json = excluded.created_capture_ids_json,
			created_at = excluded.created_at
	`, r.ID, r.Source, r.Separator, r.ContextTitle, r.RawText,
		r.RawSegmentsJSON, r.FinalSegmentsJSON, r.CreatedCaptureIDsJSON, createdAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("batch log put: %w", err)
	}
	return nil
}

// Get fetches a batch log by id, or returns sql.ErrNoRows if missing.
func (s *BatchLogStore) Get(ctx context.Context, id string) (*BatchLogRecord, error) {
	var r BatchLogRecord
	var createdAt sql.NullString
	err := s.g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`
			SELECT id, source, separator, context_title, raw_text,
				raw_segments_json, final_segments_json, created_capture_ids_json, created_at
			FROM batch_logs WHERE id = ?
		`, id).Scan(
			&r.ID, &r.Source, &r.Separator, &r.ContextTitle, &r.RawText,
			&r.RawSegmentsJSON, &r.FinalSegmentsJSON, &r.CreatedCaptureIDsJSON, &createdAt,
		)
	})
	if err != nil {
		return nil, err
	}
	if createdAt.Valid {
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}
	return &r, nil
}

// BatchLogEntry is a compact, UI-friendly view of one raw final segment.
type BatchLogEntry struct {
	Sequence  int    `json:"sequence"`
	Body      string `json:"body"`
	Timestamp string `json:"timestamp,omitempty"`
}

// BatchLogResponse is returned by the batch-log API.
type BatchLogResponse struct {
	BatchID           string          `json:"batchId"`
	Source            string          `json:"source"`
	Separator         string          `json:"separator"`
	ContextTitle      string          `json:"contextTitle"`
	RawText           string          `json:"rawText,omitempty"`
	RawEntries        []BatchLogEntry `json:"rawEntries"`
	FinalEntries      []BatchLogEntry `json:"finalEntries"`
	CreatedCaptureIDs []string        `json:"createdCaptureIds"`
	Captures          []any           `json:"captures,omitempty"`
}
