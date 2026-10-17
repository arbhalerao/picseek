CREATE TABLE text_vectors (
    image_id INTEGER NOT NULL REFERENCES images (id) ON DELETE CASCADE,
    chunk INTEGER NOT NULL,
    embedding BLOB NOT NULL,
    PRIMARY KEY (image_id, chunk)
) WITHOUT ROWID;

CREATE TABLE image_vectors (
    image_id INTEGER PRIMARY KEY REFERENCES images (id) ON DELETE CASCADE,
    embedding BLOB NOT NULL
);

ALTER TABLE images ADD COLUMN type_score REAL;
