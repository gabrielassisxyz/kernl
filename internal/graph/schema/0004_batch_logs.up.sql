CREATE TABLE batch_logs (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL DEFAULT '',
    separator TEXT NOT NULL DEFAULT '',
    context_title TEXT NOT NULL DEFAULT '',
    raw_text TEXT NOT NULL DEFAULT '',
    raw_segments_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(raw_segments_json)),
    final_segments_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(final_segments_json)),
    created_capture_ids_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(created_capture_ids_json)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE INDEX idx_batch_logs_created_at ON batch_logs(created_at);
