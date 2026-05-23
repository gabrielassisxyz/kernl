ALTER TABLE nodes ADD COLUMN deleted_at TEXT;

CREATE TABLE note_paths (
    uuid TEXT PRIMARY KEY,
    path TEXT NOT NULL UNIQUE,
    content_hash TEXT,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE TABLE dangling_links (
    id TEXT PRIMARY KEY,
    src_node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    target_key TEXT NOT NULL,
    target_kind TEXT NOT NULL CHECK (target_kind IN ('stem', 'title', 'uuid')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE INDEX idx_note_paths_path ON note_paths(path);
CREATE INDEX idx_dangling_links_target_key ON dangling_links(target_key);
