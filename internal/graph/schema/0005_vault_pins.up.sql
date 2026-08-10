-- Pinning in the vault index: a pinned note or tag is lifted into its own
-- section above the rest of the list.
--
-- Both pins live on the vault-index tables rather than on `nodes.attrs`, and
-- that is not a style choice. A note's attrs are REGENERATED from the file on
-- every reconcile - `Note.NodeAttrs()` returns a fresh map and `updateNode`
-- writes it over the old one - so a pin stored there survives exactly until the
-- next time the note is edited, then vanishes with no error anywhere. Project
-- pinning gets away with `attrs.pinned` because nothing regenerates a project's
-- attrs from a file.
--
-- `note_paths` is the right home for the note pin for the same reason it is
-- safe: the reconciler's upsert sets only path, content_hash and updated_at, so
-- the column is untouched by indexing, and a note deleted from disk drops its
-- row and its pin together. A tag is not a node at all and has nowhere else to
-- go.
ALTER TABLE note_paths ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tags ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
