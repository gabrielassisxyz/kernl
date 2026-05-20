CREATE TABLE nodes (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    fts_rowid INTEGER,
    attrs TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(attrs)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE TABLE edges (
    id TEXT PRIMARY KEY,
    src TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    dst TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    owner_id TEXT,
    visibility TEXT,
    attrs TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(attrs)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE TABLE revisions (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES nodes(id),
    parent_id TEXT,
    diff TEXT NOT NULL,
    attrs TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(attrs)),
    author TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE TABLE tags (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
) STRICT;

CREATE TABLE node_tags (
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (node_id, tag_id)
) STRICT;

CREATE VIRTUAL TABLE nodes_fts USING fts5 (
    title,
    attrs,
    content=nodes,
    content_rowid=fts_rowid
);
