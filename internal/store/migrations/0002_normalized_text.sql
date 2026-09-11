CREATE VIRTUAL TABLE image_text_v2 USING fts5 (text, folded, compact, tokenize = 'trigram remove_diacritics 1');

INSERT INTO image_text_v2 (rowid, text, folded, compact) SELECT rowid, text, '', '' FROM image_text;

DROP TABLE image_text;

ALTER TABLE image_text_v2 RENAME TO image_text;
