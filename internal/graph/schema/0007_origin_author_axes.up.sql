-- Origin and author become separate axes, and origin stops being spelled like an author.
--
-- The value "da" said who, not where from, so the two fields read as one field written
-- twice. It becomes "prep", after the pipeline that produces those notes. The rename is
-- data and code at once on purpose: the retrieval filter that keeps the DA's own briefings
-- out of what it retrieves compares against this exact value, so renaming the constant
-- without rewriting the rows would switch that filter off in silence, and twenty briefings
-- would quietly return as knowledge about the user.
UPDATE nodes
   SET attrs = json_set(attrs, '$.origin', 'prep')
 WHERE type = 'note' AND json_extract(attrs, '$.origin') = 'da';

-- Every note that came out of a DA pipeline is also written by the DA, so it carries the
-- author too. Capture is the exception that defines the axis: the body is the user's own
-- words, carried through untouched, and the DA only filed it - so a capture-origin note
-- gets no author at all.
UPDATE nodes
   SET attrs = json_set(attrs, '$.author', 'da')
 WHERE type = 'note'
   AND json_extract(attrs, '$.origin') IS NOT NULL
   AND json_extract(attrs, '$.origin') <> ''
   AND json_extract(attrs, '$.origin') <> 'capture'
   AND (json_extract(attrs, '$.author') IS NULL OR json_extract(attrs, '$.author') = '');

-- author "user" was written on two notes out of eight hundred. Two occurrences are not a
-- convention, and an enum whose second value is almost never written cannot discriminate.
-- The rule that survives is the negative one: an author other than "da" - absent included -
-- means the user wrote it, which makes the field optional by construction and leaves the
-- DA filter depending on one thing, the presence of the value "da".
UPDATE nodes
   SET attrs = json_remove(attrs, '$.author')
 WHERE type = 'note' AND json_extract(attrs, '$.author') = 'user';
