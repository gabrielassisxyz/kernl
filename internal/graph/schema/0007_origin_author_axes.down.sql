-- Back to origin "da". The author values are not removed: they are correct under both
-- spellings, and dropping them would lose the one field that says who wrote a note.
UPDATE nodes
   SET attrs = json_set(attrs, '$.origin', 'da')
 WHERE type = 'note' AND json_extract(attrs, '$.origin') = 'prep';
