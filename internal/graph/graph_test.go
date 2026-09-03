package graph

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"oneshottest/internal/event"
)

// TestRelateIsStoredAsAnEvent verifies an edge appears in events, so D5's "one storage mechanism" holds.
func TestRelateIsStoredAsAnEvent(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(ctx, t)
	defer db.Close()

	store := event.NewStore(db)

	// Use the graph package to relate two documents
	from := "users/alice"
	to := "users/bob"
	edgeType := "knows"

	seq, err := Relate(ctx, store, from, to, edgeType, nil)
	if err != nil {
		t.Fatalf("Relate: %v", err)
	}

	// Verify the edge is stored as an event in the events table
	var count int64
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE collection = '__edges__'`).Scan(&count)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 edge event, got %d", count)
	}

	// Verify the seq matches
	var storedSeq int64
	err = db.QueryRowContext(ctx,
		`SELECT seq FROM events WHERE collection = '__edges__'`).Scan(&storedSeq)
	if err != nil {
		t.Fatalf("query seq: %v", err)
	}

	if storedSeq != seq {
		t.Errorf("expected seq=%d, got %d", seq, storedSeq)
	}

	// Verify the edge can be found in live
	var count2 int64
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM live WHERE collection = '__edges__'`).Scan(&count2)
	if err != nil {
		t.Fatalf("query live: %v", err)
	}

	if count2 != 1 {
		t.Fatalf("expected 1 edge in live, got %d", count2)
	}
}

// TestTraverseAsOfSeesHistoricalEdges verifies relations time-travel because they are events.
func TestTraverseAsOfSeesHistoricalEdges(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(ctx, t)
	defer db.Close()

	store := event.NewStore(db)

	now := time.Now().UTC()
	past := now.Add(-1 * time.Hour)

	// Create a relate event at a past time
	from := "users/alice"
	to := "users/bob"
	edgeType := "knows"

	// Relate in the past
	_, err := RelateAsOf(ctx, store, from, to, edgeType, nil, past)
	if err != nil {
		t.Fatalf("RelateAsOf: %v", err)
	}

	// Query as of the past time should see the edge
	related, err := RelatedAsOf(ctx, store, from, &edgeType, past, 100)
	if err != nil {
		t.Fatalf("RelatedAsOf at past: %v", err)
	}

	if len(related) != 1 {
		t.Fatalf("expected 1 related edge at past, got %d", len(related))
	}

	// Verify we can go back further and not see it
	beforePast := past.Add(-1 * time.Hour)
	related, err = RelatedAsOf(ctx, store, from, &edgeType, beforePast, 100)
	if err != nil {
		t.Fatalf("RelatedAsOf before past: %v", err)
	}

	if len(related) != 0 {
		t.Fatalf("expected 0 related edges before past, got %d", len(related))
	}
}

// --- Helpers ---

func setupTestDB(ctx context.Context, t *testing.T) *sql.DB {
	// Use in-memory database for testing
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}

	// Create tables
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE events (
			seq         INTEGER PRIMARY KEY AUTOINCREMENT,
			collection  TEXT    NOT NULL,
			key         TEXT    NOT NULL,
			op          TEXT    NOT NULL CHECK (op IN ('put','delete')),
			value       BLOB,
			valid_from  TEXT    NOT NULL,
			tx_time     TEXT    NOT NULL,
			meta        TEXT    NOT NULL DEFAULT '{}'
		)
	`); err != nil {
		t.Fatalf("create events table: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		CREATE INDEX idx_events_ck_seq ON events(collection, key, seq)
	`); err != nil {
		t.Fatalf("create index: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE live (
			collection  TEXT NOT NULL,
			key         TEXT NOT NULL,
			value       BLOB,
			valid_from  TEXT NOT NULL,
			tx_time     TEXT NOT NULL,
			deleted     INTEGER NOT NULL DEFAULT 0,
			seq         INTEGER NOT NULL,
			PRIMARY KEY (collection, key)
		)
	`); err != nil {
		t.Fatalf("create live table: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})
	return db
}
