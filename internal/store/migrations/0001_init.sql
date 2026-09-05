CREATE TABLE folders (
    path TEXT PRIMARY KEY,
    added_at INTEGER NOT NULL
) WITHOUT ROWID;

CREATE TABLE images (
    id INTEGER PRIMARY KEY,
    hash INTEGER NOT NULL UNIQUE,
    taken_at INTEGER NOT NULL,
    type TEXT,
    created_at INTEGER NOT NULL,
    orphaned_at INTEGER
);

CREATE INDEX images_taken_at ON images (taken_at);
CREATE INDEX images_orphaned_at ON images (orphaned_at) WHERE orphaned_at IS NOT NULL;

CREATE TABLE paths (
    path TEXT PRIMARY KEY,
    root TEXT NOT NULL,
    dir TEXT NOT NULL,
    image_id INTEGER NOT NULL REFERENCES images (id),
    size INTEGER NOT NULL,
    mtime INTEGER NOT NULL
) WITHOUT ROWID;

CREATE INDEX paths_image_id ON paths (image_id);
CREATE INDEX paths_root ON paths (root);

CREATE VIRTUAL TABLE image_text USING fts5 (text, tokenize = 'trigram remove_diacritics 1');

CREATE TABLE jobs (
    id INTEGER PRIMARY KEY,
    image_id INTEGER NOT NULL REFERENCES images (id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    lane INTEGER NOT NULL,
    priority INTEGER NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    not_before INTEGER NOT NULL DEFAULT 0,
    lease_until INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    UNIQUE (image_id, kind)
);

CREATE INDEX jobs_ready ON jobs (kind, failed, lane, priority DESC);

CREATE TABLE enrichments (
    image_id INTEGER NOT NULL REFERENCES images (id) ON DELETE CASCADE,
    enricher TEXT NOT NULL,
    version TEXT NOT NULL,
    done_at INTEGER NOT NULL,
    PRIMARY KEY (image_id, enricher)
) WITHOUT ROWID;
