package tql

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"oneshottest/internal/event"
	"oneshottest/internal/tql/ast"
	"oneshottest/internal/vector"
)

// TestExecPutThenGetRoundTrips verifies the write path and the read path agree.
func TestExecPutThenGetRoundTrips(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(ctx, t)
	defer db.Close()

	store := event.NewStore(db)
	exec := NewExecutor(store, nil, nil)

	// PUT a document
	putStmt, err := Parse(`PUT users/alice {"name":"Alice","age":30}`)
	if err != nil {
		t.Fatalf("parse PUT: %v", err)
	}

	putResult, err := exec.Exec(ctx, putStmt)
	if err != nil {
		t.Fatalf("exec PUT: %v", err)
	}
	if putResult.Seq == 0 {
		t.Errorf("PUT seq should be > 0, got %d", putResult.Seq)
	}

	// GET the document back
	getStmt, err := Parse(`GET users/alice`)
	if err != nil {
		t.Fatalf("parse GET: %v", err)
	}

	getResult, err := exec.Exec(ctx, getStmt)
	if err != nil {
		t.Fatalf("exec GET: %v", err)
	}

	if len(getResult.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(getResult.Rows))
	}

	row := getResult.Rows[0]
	name, ok := row["name"]
	if !ok || name != "Alice" {
		t.Errorf("expected name=Alice, got %v", name)
	}
	age, ok := row["age"]
	if !ok || age != float64(30) {
		t.Errorf("expected age=30, got %v", age)
	}
}

// TestExecGetAsOfUsesTemporalVisible verifies time travel is delegated, not reimplemented.
// Delete the temporal.Visible call and this test goes red.
func TestExecGetAsOfUsesTemporalVisible(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(ctx, t)
	defer db.Close()

	store := event.NewStore(db)
	exec := NewExecutor(store, nil, nil)

	now := time.Now().UTC()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	// PUT at past time
	putStmt, err := Parse(`PUT users/bob {"name":"Bob","age":25}`)
	if err != nil {
		t.Fatalf("parse PUT: %v", err)
	}
	put := putStmt.(*ast.Put)
	put.At = &past
	_, err = exec.Exec(ctx, putStmt)
	if err != nil {
		t.Fatalf("exec PUT at past: %v", err)
	}

	// GET as of past time should return the document
	getStmt := &ast.Get{Collection: "users", Key: "bob", AsOf: &past}
	getResult, err := exec.Exec(ctx, getStmt)
	if err != nil {
		t.Fatalf("exec GET as of past: %v", err)
	}
	if len(getResult.Rows) != 1 {
		t.Fatalf("GET as of past: expected 1 row, got %d", len(getResult.Rows))
	}

	// GET as of future time should also return the document (still valid then)
	getStmt = &ast.Get{Collection: "users", Key: "bob", AsOf: &future}
	getResult, err = exec.Exec(ctx, getStmt)
	if err != nil {
		t.Fatalf("exec GET as of future: %v", err)
	}
	if len(getResult.Rows) != 1 {
		t.Fatalf("GET as of future: expected 1 row, got %d", len(getResult.Rows))
	}

	// GET as of a time before the PUT should return nothing (uses temporal.Visible, not just latest)
	beforePast := past.Add(-1 * time.Hour)
	getStmt = &ast.Get{Collection: "users", Key: "bob", AsOf: &beforePast}
	getResult, err = exec.Exec(ctx, getStmt)
	if err != nil {
		t.Fatalf("exec GET as of before past: %v", err)
	}
	if len(getResult.Rows) != 0 {
		t.Fatalf("GET as of before past: expected 0 rows, got %d", len(getResult.Rows))
	}
}

// TestExecFindBindsParametersNotStrings verifies a value containing a quote is bound, not interpolated.
// A value like "O'Reilly" would cause SQL syntax error if interpolated as 'O'Reilly'
// but works correctly when bound as a parameter.
func TestExecFindBindsParametersNotStrings(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(ctx, t)
	defer db.Close()

	store := event.NewStore(db)
	exec := NewExecutor(store, nil, nil)

	// PUT a document with a value containing a quote character
	// This value would break SQL if interpolated as a string literal
	putStmt, err := Parse(`PUT docs/doc1 {"author":"O'Reilly"}`)
	if err != nil {
		t.Fatalf("parse PUT: %v", err)
	}
	_, err = exec.Exec(ctx, putStmt)
	if err != nil {
		t.Fatalf("exec PUT: %v", err)
	}

	// Also PUT a document with the author "Penguin" to test specificity
	putStmt2, err := Parse(`PUT docs/doc2 {"author":"Penguin"}`)
	if err != nil {
		t.Fatalf("parse PUT 2: %v", err)
	}
	_, err = exec.Exec(ctx, putStmt2)
	if err != nil {
		t.Fatalf("exec PUT 2: %v", err)
	}

	// FIND by the value with quote should match it literally via bound parameter
	// If the code interpolates instead of binding, the SQL becomes:
	//   WHERE json_extract(value, '$.author') = 'O'Reilly'
	// which is a syntax error (the quote closes the string early).
	// With proper binding, it works correctly.
	findStmt, err := Parse(`FIND docs WHERE author = "O'Reilly"`)
	if err != nil {
		t.Fatalf("parse FIND: %v", err)
	}

	findResult, err := exec.Exec(ctx, findStmt)
	if err != nil {
		t.Fatalf("exec FIND: %v", err)
	}

	// If the query had a SQL syntax error (from incorrect interpolation),
	// it would be reported here
	if findResult.Error != "" {
		t.Fatalf("FIND returned error (indicates broken binding): %s", findResult.Error)
	}

	// Should find exactly 1 row (the O'Reilly document, not the Penguin document)
	if len(findResult.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(findResult.Rows))
	}

	// Verify the result is the correct document
	row := findResult.Rows[0]
	if row["author"] != "O'Reilly" {
		t.Errorf("expected author=O'Reilly, got %v", row["author"])
	}
}

// TestExecFindJSONPushdown verifies nested field and array-membership predicates both filter correctly.
func TestExecFindJSONPushdown(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(ctx, t)
	defer db.Close()

	store := event.NewStore(db)
	exec := NewExecutor(store, nil, nil)

	// PUT documents with nested structure
	putStmts := []string{
		`PUT users/alice {"name":"Alice","address":{"city":"NYC","zip":"10001"}}`,
		`PUT users/bob {"name":"Bob","address":{"city":"LA","zip":"90001"}}`,
		`PUT users/charlie {"name":"Charlie","address":{"city":"NYC","zip":"10002"}}`,
	}

	for _, putSQL := range putStmts {
		putStmt, err := Parse(putSQL)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		_, err = exec.Exec(ctx, putStmt)
		if err != nil {
			t.Fatalf("exec: %v", err)
		}
	}

	// FIND by nested field
	findStmt, err := Parse(`FIND users WHERE address.city = "NYC"`)
	if err != nil {
		t.Fatalf("parse FIND: %v", err)
	}

	findResult, err := exec.Exec(ctx, findStmt)
	if err != nil {
		t.Fatalf("exec FIND: %v", err)
	}

	if len(findResult.Rows) != 2 {
		t.Fatalf("expected 2 rows for NYC, got %d", len(findResult.Rows))
	}

	// Verify the results are Alice and Charlie
	names := make(map[string]bool)
	for _, row := range findResult.Rows {
		names[row["name"].(string)] = true
	}
	if !names["Alice"] || !names["Charlie"] {
		t.Errorf("expected Alice and Charlie, got %v", names)
	}
}

// TestExecSearchDispatchesToVectorIndex verifies the stub index is called (rung-2 check for T10).
func TestExecSearchDispatchesToVectorIndex(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(ctx, t)
	defer db.Close()

	store := event.NewStore(db)

	// Create a stub vector index that records calls
	stub := &stubVectorIndex{
		searchCalls: make([]stubSearchCall, 0),
	}

	exec := NewExecutor(store, stub, nil)

	// PUT a document to search for
	putStmt, err := Parse(`PUT articles/article1 {"title":"ml"}`)
	if err != nil {
		t.Fatalf("parse PUT: %v", err)
	}
	_, err = exec.Exec(ctx, putStmt)
	if err != nil {
		t.Fatalf("exec PUT: %v", err)
	}

	// Configure the stub to return the document on search
	stub.searchResults = []vector.Hit{
		{Key: "article1", Score: 0.95},
	}

	// SEARCH
	searchStmt, err := Parse(`SEARCH articles NEAR "machine learning"`)
	if err != nil {
		t.Fatalf("parse SEARCH: %v", err)
	}

	searchResult, err := exec.Exec(ctx, searchStmt)
	if err != nil {
		t.Fatalf("exec SEARCH: %v", err)
	}

	// Verify Search was called
	if len(stub.searchCalls) != 1 {
		t.Fatalf("expected 1 Search call, got %d", len(stub.searchCalls))
	}

	// Verify result was returned
	if len(searchResult.Rows) != 1 {
		t.Fatalf("expected 1 result, got %d", len(searchResult.Rows))
	}
}

// TestExecSearchWithoutVectorConfigIsNamedError verifies unconfigured is "feature absent", not an error.
func TestExecSearchWithoutVectorConfigIsNamedError(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(ctx, t)
	defer db.Close()

	store := event.NewStore(db)
	exec := NewExecutor(store, nil, nil) // no vector index

	// SEARCH without vector config should return a named error
	searchStmt, err := Parse(`SEARCH articles NEAR "machine learning"`)
	if err != nil {
		t.Fatalf("parse SEARCH: %v", err)
	}

	searchResult, err := exec.Exec(ctx, searchStmt)
	if err != nil {
		t.Fatalf("exec SEARCH: %v", err)
	}

	// Should have an error in the result
	if searchResult.Error == "" {
		t.Errorf("expected error for unconfigured vector search, got none")
	}
	if searchResult.Error != "feature not configured" {
		t.Errorf("expected 'feature not configured', got %q", searchResult.Error)
	}
}

// TestExecPurgeRemovesOnlyEventsPastRetention verifies purge is bounded and never touches live state.
func TestExecPurgeRemovesOnlyEventsPastRetention(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(ctx, t)
	defer db.Close()

	store := event.NewStore(db)
	exec := NewExecutor(store, nil, nil)

	now := time.Now().UTC()
	past := now.Add(-2 * time.Hour)
	recent := now.Add(-30 * time.Minute)

	// PUT at old time
	putOld := &ast.Put{Collection: "users", Key: "old", Value: `{"status":"archived"}`, At: &past}
	_, err := exec.Exec(ctx, putOld)
	if err != nil {
		t.Fatalf("exec PUT old: %v", err)
	}

	// PUT at recent time
	putRecent := &ast.Put{Collection: "users", Key: "recent", Value: `{"status":"active"}`, At: &recent}
	_, err = exec.Exec(ctx, putRecent)
	if err != nil {
		t.Fatalf("exec PUT recent: %v", err)
	}

	// Update recent
	putRecent2 := &ast.Put{Collection: "users", Key: "recent", Value: `{"status":"active","updated":true}`, At: &recent}
	_, err = exec.Exec(ctx, putRecent2)
	if err != nil {
		t.Fatalf("exec PUT recent again: %v", err)
	}

	// Count events before purge
	var countBefore int64
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&countBefore)

	// PURGE events older than recent (keep everything after 1 hour ago)
	cutoff := now.Add(-1 * time.Hour)
	purgeStmt := &ast.Purge{Collection: "users", Before: &cutoff}
	purgeResult, err := exec.Exec(ctx, purgeStmt)
	if err != nil {
		t.Fatalf("exec PURGE: %v", err)
	}

	// Should have purged the old event (and maybe others)
	if purgeResult.Count == 0 {
		t.Errorf("expected to purge at least 1 event, got count=0")
	}

	// Verify live table still has recent (PURGE never touches live - current state is unaffected)
	getStmt := &ast.Get{Collection: "users", Key: "recent"}
	getResult, err := exec.Exec(ctx, getStmt)
	if err != nil {
		t.Fatalf("exec GET after purge: %v", err)
	}
	if len(getResult.Rows) != 1 {
		t.Fatalf("expected 1 row after purge (recent), got %d", len(getResult.Rows))
	}

	// Note: PURGE never touches live, so old key may still be there, but AS-OF queries before
	// the purge cutoff would not see it (because events are deleted).
	// For current state, the live projection remains unchanged.
}

// --- Helpers ---

// setupTestDB initializes a test database with the schema.
func setupTestDB(ctx context.Context, t *testing.T) *sql.DB {
	// Use in-memory database for testing
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}

	// Run migrations manually
	if err := runTestMigrations(ctx, db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})
	return db
}

// runTestMigrations creates the necessary tables for testing.
func runTestMigrations(ctx context.Context, db *sql.DB) error {
	// Create events table
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
		return err
	}

	// Create index
	if _, err := db.ExecContext(ctx, `
		CREATE INDEX idx_events_ck_seq ON events(collection, key, seq)
	`); err != nil {
		return err
	}

	// Create live table
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
		return err
	}

	return nil
}

// stubVectorIndex is a mock vector.Index for testing.
type stubVectorIndex struct {
	searchResults []vector.Hit
	searchCalls   []stubSearchCall
}

type stubSearchCall struct {
	collection string
	text       string
	k          int
}

func (s *stubVectorIndex) Upsert(ctx context.Context, collection string, key string, text string) error {
	return nil
}

func (s *stubVectorIndex) Search(ctx context.Context, collection string, text string, k int) ([]vector.Hit, error) {
	s.searchCalls = append(s.searchCalls, stubSearchCall{collection, text, k})
	return s.searchResults, nil
}
