DROP INDEX IF EXISTS idx_dangling_links_target_key;
DROP INDEX IF EXISTS idx_note_paths_path;
DROP TABLE IF EXISTS dangling_links;
DROP TABLE IF EXISTS note_paths;
ALTER TABLE nodes DROP COLUMN deleted_at;
