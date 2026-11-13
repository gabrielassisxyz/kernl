-- Every full-text hit is joined back to its node on fts_rowid. Without this index
-- SQLite resolves each hit by walking every node of the searched type, which made a
-- single search term cost ~0.4 s on a vault of 1,200 notes, and link suggestion runs
-- one search per term of the note being written.
CREATE INDEX IF NOT EXISTS idx_nodes_fts_rowid ON nodes(fts_rowid);
