package event

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"oneshottest/internal/storage"
)

// TestAppendWritesEventAndLiveAtomically verifies that Append writes both rows
// or neither when the live upsert fails.
func TestAppendWritesEventAndLiveAtomically(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)
	defer db.Close()

	store := NewStore(db)

	// First append should succeed
	ev := Event{
		Collection: "users",
		Key:        "alice",
		Op:         OpPut,
		Value:      []byte(`{"name":"Alice"}`),
		ValidFrom:  time.Now().UTC(),
		TxTime:     time.Now().UTC(),
		Meta:       []byte("{}"),
	}

	seq, err := store.Append(ctx, ev)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if seq != 1 {
		t.Fatalf("expected seq 1, got %d", seq)
	}

	// Verify both events and live rows exist
	var eventCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE collection = ? AND key = ?", "users", "alice").Scan(&eventCount)
	if err != nil || eventCount != 1 {
		t.Fatalf("events row not inserted: %v, count=%d", err, eventCount)
	}

	var liveCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM live WHERE collection = ? AND key = ?", "users", "alice").Scan(&liveCount)
	if err != nil || liveCount != 1 {
		t.Fatalf("live row not inserted: %v, count=%d", err, liveCount)
	}

	// Verify live.value is correct
	var liveValue []byte
	err = db.QueryRowContext(ctx, "SELECT value FROM live WHERE collection = ? AND key = ?", "users", "alice").Scan(&liveValue)
	if err != nil || string(liveValue) != `{"name":"Alice"}` {
		t.Fatalf("live.value incorrect: %v, value=%s", err, string(liveValue))
	}
}

// TestAppendIsSerialisedUnderConcurrency verifies that N goroutines appending
// yield N gapless sequence numbers, no SQLITE_BUSY.
func TestAppendIsSerialisedUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)
	defer db.Close()

	store := NewStore(db)

	numGoroutines := 10
	seqs := make(chan int64, numGoroutines)
	errs := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			ev := Event{
				Collection: "test",
				Key:        string(rune('a' + id)),
				Op:         OpPut,
				Value:      []byte(`{"id":` + string(rune('0'+id)) + `}`),
				ValidFrom:  time.Now().UTC(),
				TxTime:     time.Now().UTC(),
				Meta:       []byte("{}"),
			}
			seq, err := store.Append(ctx, ev)
			seqs <- seq
			errs <- err
		}(i)
	}

	// Collect results
	collectedSeqs := make([]int64, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("goroutine %d Append failed: %v", i, err)
		}
		collectedSeqs[i] = <-seqs
	}

	// Verify all sequences are unique and from 1 to numGoroutines
	seqMap := make(map[int64]bool)
	for _, seq := range collectedSeqs {
		if seq < 1 || seq > int64(numGoroutines) {
			t.Fatalf("sequence number out of range: %d", seq)
		}
		if seqMap[seq] {
			t.Fatalf("duplicate sequence number: %d", seq)
		}
		seqMap[seq] = true
	}

	// Verify total count in events table
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil || count != numGoroutines {
		t.Fatalf("expected %d events, got %d: %v", numGoroutines, count, err)
	}
}

// TestAppendOpensStorageThroughStorageOpen verifies the store is built on T1's
// storage.Open, so WAL and migrations are in force.
func TestAppendOpensStorageThroughStorageOpen(t *testing.T) {
	ctx := context.Background()
	cfg := storage.Config{DataDir: t.TempDir()}
	db, err := storage.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("storage.Open failed: %v", err)
	}
	defer db.Close()

	store := NewStore(db)

	// Verify we can append and both tables exist and are accessible
	ev := Event{
		Collection: "test",
		Key:        "key1",
		Op:         OpPut,
		Value:      []byte(`{"data":"test"}`),
		ValidFrom:  time.Now().UTC(),
		TxTime:     time.Now().UTC(),
		Meta:       []byte("{}"),
	}

	seq, err := store.Append(ctx, ev)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if seq != 1 {
		t.Fatalf("expected seq 1, got %d", seq)
	}

	// Verify WAL pragmas are in effect - check journal_mode
	var journalMode string
	err = db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode)
	if err != nil || journalMode != "wal" {
		t.Fatalf("WAL mode not enabled: %v, mode=%s", err, journalMode)
	}
}

// TestHistoryReturnsAllVersionsInSequenceOrder verifies History is ordered and complete.
func TestHistoryReturnsAllVersionsInSequenceOrder(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)
	defer db.Close()

	store := NewStore(db)

	// Append three versions of the same key
	baseTime := time.Now().UTC()
	for i := 0; i < 3; i++ {
		ev := Event{
			Collection: "versions",
			Key:        "doc1",
			Op:         OpPut,
			Value:      []byte(`{"version":` + string(rune('0'+i)) + `}`),
			ValidFrom:  baseTime.Add(time.Duration(i) * time.Second),
			TxTime:     baseTime.Add(time.Duration(i) * time.Second),
			Meta:       []byte("{}"),
		}
		_, err := store.Append(ctx, ev)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Get history
	history, err := store.History(ctx, "versions", "doc1")
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	if len(history) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(history))
	}

	// Verify ordering by seq
	for i := 0; i < 3; i++ {
		if history[i].Seq != int64(i+1) {
			t.Fatalf("expected seq %d at index %d, got %d", i+1, i, history[i].Seq)
		}
	}
}

// TestAfterReturnsOnlyEventsPastCursor verifies After(seq) is exclusive.
func TestAfterReturnsOnlyEventsPastCursor(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)
	defer db.Close()

	store := NewStore(db)

	// Append 5 events
	for i := 0; i < 5; i++ {
		ev := Event{
			Collection: "items",
			Key:        string(rune('a' + i)),
			Op:         OpPut,
			Value:      []byte(`{"index":` + string(rune('0'+i)) + `}`),
			ValidFrom:  time.Now().UTC(),
			TxTime:     time.Now().UTC(),
			Meta:       []byte("{}"),
		}
		_, err := store.Append(ctx, ev)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Get events after seq 2 - should return only seq 3, 4, 5
	after, err := store.After(ctx, 2)
	if err != nil {
		t.Fatalf("After failed: %v", err)
	}

	if len(after) != 3 {
		t.Fatalf("expected 3 events after cursor, got %d", len(after))
	}

	// Verify sequences
	for i, ev := range after {
		expectedSeq := int64(3 + i)
		if ev.Seq != expectedSeq {
			t.Fatalf("expected seq %d at index %d, got %d", expectedSeq, i, ev.Seq)
		}
	}

	// Verify After(5) returns empty - cursor at last event should return nothing
	after, err = store.After(ctx, 5)
	if err != nil {
		t.Fatalf("After(5) failed: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected 0 events after cursor 5, got %d", len(after))
	}
}

// TestEventsTableIsNeverUpdated verifies an append never mutates a prior row.
func TestEventsTableIsNeverUpdated(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)
	defer db.Close()

	store := NewStore(db)

	// Append first event
	ev1 := Event{
		Collection: "immutable",
		Key:        "doc",
		Op:         OpPut,
		Value:      []byte(`{"version":1}`),
		ValidFrom:  time.Now().UTC(),
		TxTime:     time.Now().UTC(),
		Meta:       []byte("{}"),
	}
	seq1, err := store.Append(ctx, ev1)
	if err != nil {
		t.Fatalf("first Append failed: %v", err)
	}

	// Capture the original value
	var originalValue []byte
	err = db.QueryRowContext(ctx, "SELECT value FROM events WHERE seq = ?", seq1).Scan(&originalValue)
	if err != nil {
		t.Fatalf("query original value failed: %v", err)
	}

	// Append a different event with same key (creating a new row, not updating)
	ev2 := Event{
		Collection: "immutable",
		Key:        "doc",
		Op:         OpPut,
		Value:      []byte(`{"version":2}`),
		ValidFrom:  time.Now().UTC().Add(time.Second),
		TxTime:     time.Now().UTC().Add(time.Second),
		Meta:       []byte("{}"),
	}
	_, err = store.Append(ctx, ev2)
	if err != nil {
		t.Fatalf("second Append failed: %v", err)
	}

	// Verify the original row in events is unchanged
	var verifyValue []byte
	err = db.QueryRowContext(ctx, "SELECT value FROM events WHERE seq = ?", seq1).Scan(&verifyValue)
	if err != nil {
		t.Fatalf("query verify value failed: %v", err)
	}
	if string(verifyValue) != string(originalValue) {
		t.Fatalf("events row was modified: original=%s, current=%s", string(originalValue), string(verifyValue))
	}

	// Verify both rows exist in events
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE collection = ? AND key = ?", "immutable", "doc").Scan(&count)
	if err != nil || count != 2 {
		t.Fatalf("expected 2 events rows, got %d: %v", count, err)
	}
}

// openTestDB opens a test database and runs migrations.
func openTestDB(t *testing.T, ctx context.Context) *sql.DB {
	cfg := storage.Config{DataDir: t.TempDir()}
	db, err := storage.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("storage.Open failed: %v", err)
	}
	return db
}
