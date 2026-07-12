-- Restore the flat machine-tag names. The `capture` / `converted` tags dropped
-- by the up migration are NOT restored: their node_tags links are gone, so
-- there is nothing left to name.
UPDATE tags SET name = 'pending'       WHERE name = 'sys/pending';
UPDATE tags SET name = 'triaged'       WHERE name = 'sys/triaged';
UPDATE tags SET name = 'discarded'     WHERE name = 'sys/discarded';
UPDATE tags SET name = 'audit'         WHERE name = 'sys/audit';
UPDATE tags SET name = 'autonomous'    WHERE name = 'sys/autonomous';
UPDATE tags SET name = 'ingest-source' WHERE name = 'sys/ingest-source';
