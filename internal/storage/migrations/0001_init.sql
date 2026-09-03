-- +goose Up
CREATE TABLE events (
    seq         INTEGER PRIMARY KEY AUTOINCREMENT,
    collection  TEXT NOT NULL,
    key         TEXT NOT NULL,
    op          TEXT NOT NULL CHECK (op IN ('put','delete')),
    value       BLOB,
    valid_from  TEXT NOT NULL,
    tx_time     TEXT NOT NULL,
    meta        TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE live (
    collection  TEXT NOT NULL,
    key         TEXT NOT NULL,
    value       BLOB,
    valid_from  TEXT NOT NULL,
    tx_time     TEXT NOT NULL,
    deleted     INTEGER NOT NULL DEFAULT 0,
    seq         INTEGER NOT NULL,
    PRIMARY KEY (collection, key)
);

CREATE INDEX idx_events_ck_seq ON events(collection, key, seq);

-- +goose Down
DROP INDEX IF EXISTS idx_events_ck_seq;
DROP TABLE IF EXISTS live;
DROP TABLE IF EXISTS events;
