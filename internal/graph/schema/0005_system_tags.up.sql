-- Move machine-authored tags into the reserved `sys/` namespace so they stop
-- competing with user subjects in the flat tag namespace. Renaming (rather than
-- re-tagging) keeps every node_tags row intact: the link points at the tag id,
-- not the name.
--
-- `telos` is deliberately NOT touched: it marks a note the user authors by hand
-- in the vault, so it is user content the system happens to read.
UPDATE tags SET name = 'sys/pending'       WHERE name = 'pending';
UPDATE tags SET name = 'sys/triaged'       WHERE name = 'triaged';
UPDATE tags SET name = 'sys/discarded'     WHERE name = 'discarded';
UPDATE tags SET name = 'sys/audit'         WHERE name = 'audit';
UPDATE tags SET name = 'sys/autonomous'    WHERE name = 'autonomous';
UPDATE tags SET name = 'sys/ingest-source' WHERE name = 'ingest-source';

-- `capture` and `converted` were written onto capture-derived NOTES. Notes are
-- file-backed: reconcile overwrites a note's tags from its YAML frontmatter on
-- every file change, so a `sys/` tag on a note cannot survive — the frontmatter
-- is the source of truth and the vault must never author `sys/` tags. They are
-- dropped instead of renamed, which costs nothing: no code reads them, and the
-- provenance they duplicated is already carried by the note's `origin: capture`
-- field and its `derived_from` edge.
DELETE FROM node_tags WHERE tag_id IN (SELECT id FROM tags WHERE name IN ('capture', 'converted'));
DELETE FROM tags WHERE name IN ('capture', 'converted');
