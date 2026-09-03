// Package event implements the append-only event log and derived live projection.
// It provides the write path for TemporalDB: append to events and upsert live in one transaction,
// guarded by a single-writer mutex to enforce D13 (single writer, many readers).
package event

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// Op is the operation type for an event.
type Op string

const (
	// OpPut indicates a value was inserted or updated.
	OpPut Op = "put"
	// OpDelete indicates a value was deleted.
	OpDelete Op = "delete"
)

// Event represents a single event in the append-only log.
type Event struct {
	// Seq is the sequence number (auto-assigned by database, read-only).
	Seq int64
	// Collection is the logical collection the key belongs to.
	Collection string
	// Key is the document key within the collection.
	Key string
	// Op is the operation: put or delete.
	Op Op
	// Value is the JSON document (nil for delete).
	Value []byte
	// ValidFrom is the business time when this value becomes valid (RFC3339Nano).
	ValidFrom time.Time
	// TxTime is the wall-clock commit time (RFC3339Nano).
	TxTime time.Time
	// Meta is JSON-encoded op-specific metadata (edges, relations, etc.).
	Meta []byte
}

// Store is the single writer for the event log. It guards all writes with a mutex.
// Reads go directly to the database and are not serialized.
type Store struct {
	db    *sql.DB
	mu    sync.Mutex
	stmts *stmts
}

// stmts holds prepared statements for the hot path.
type stmts struct {
	insertEvent *sql.Stmt
	upsertLive  *sql.Stmt
	selectAfter *sql.Stmt
	selectAll   *sql.Stmt
}

// NewStore creates a new event Store backed by db.
func NewStore(db *sql.DB) *Store {
	return &Store{
		db: db,
	}
}

// Append appends an event to the log and upserts the live projection in one transaction.
// Returns the sequence number of the inserted event.
// It is the only method that writes; all writes are serialized by a single mutex.
func (s *Store) Append(ctx context.Context, ev Event) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Begin immediate transaction (serializes writers at SQLite level too).
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert into events table.
	result, err := tx.ExecContext(ctx,
		`INSERT INTO events (collection, key, op, value, valid_from, tx_time, meta)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ev.Collection, ev.Key, ev.Op, ev.Value,
		ev.ValidFrom.Format(time.RFC3339Nano), ev.TxTime.Format(time.RFC3339Nano),
		ev.Meta)
	if err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}

	seq, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get sequence: %w", err)
	}

	// Upsert into live table. Use ON CONFLICT with explicit column names for partial-index safety.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO live (collection, key, value, valid_from, tx_time, deleted, seq)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (collection, key) DO UPDATE SET
		   value = excluded.value,
		   valid_from = excluded.valid_from,
		   tx_time = excluded.tx_time,
		   deleted = excluded.deleted,
		   seq = excluded.seq`,
		ev.Collection, ev.Key, ev.Value,
		ev.ValidFrom.Format(time.RFC3339Nano), ev.TxTime.Format(time.RFC3339Nano),
		boolToInt(ev.Op == OpDelete), seq)
	if err != nil {
		return 0, fmt.Errorf("upsert live: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return seq, nil
}

// History returns all versions of a key in sequence order.
func (s *Store) History(ctx context.Context, collection string, key string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, collection, key, op, value, valid_from, tx_time, meta
		 FROM events
		 WHERE collection = ? AND key = ?
		 ORDER BY seq ASC`,
		collection, key)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var ev Event
		var validFromStr, txTimeStr string
		err := rows.Scan(&ev.Seq, &ev.Collection, &ev.Key, &ev.Op,
			&ev.Value, &validFromStr, &txTimeStr, &ev.Meta)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		ev.ValidFrom, err = time.Parse(time.RFC3339Nano, validFromStr)
		if err != nil {
			return nil, fmt.Errorf("parse valid_from: %w", err)
		}

		ev.TxTime, err = time.Parse(time.RFC3339Nano, txTimeStr)
		if err != nil {
			return nil, fmt.Errorf("parse tx_time: %w", err)
		}

		events = append(events, ev)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return events, nil
}

// After returns all events with seq > cursor, in sequence order.
// This is the cursor interface for the streaming backup (T7).
// The cursor is exclusive: After(5) returns events with seq > 5, not seq >= 5.
func (s *Store) After(ctx context.Context, cursor int64) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, collection, key, op, value, valid_from, tx_time, meta
		 FROM events
		 WHERE seq > ?
		 ORDER BY seq ASC`,
		cursor)
	if err != nil {
		return nil, fmt.Errorf("query after: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var ev Event
		var validFromStr, txTimeStr string
		err := rows.Scan(&ev.Seq, &ev.Collection, &ev.Key, &ev.Op,
			&ev.Value, &validFromStr, &txTimeStr, &ev.Meta)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		ev.ValidFrom, err = time.Parse(time.RFC3339Nano, validFromStr)
		if err != nil {
			return nil, fmt.Errorf("parse valid_from: %w", err)
		}

		ev.TxTime, err = time.Parse(time.RFC3339Nano, txTimeStr)
		if err != nil {
			return nil, fmt.Errorf("parse tx_time: %w", err)
		}

		events = append(events, ev)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return events, nil
}

// ReplayAsOf returns the current state of a collection as it was at validAt time,
// by replaying the event log up to that point and returning the last write for each key.
// This is used by the temporal query engine (T3/T5).
func (s *Store) ReplayAsOf(ctx context.Context, collection string, validAt time.Time) (map[string]Event, error) {
	validAtStr := validAt.Format(time.RFC3339Nano)

	rows, err := s.db.QueryContext(ctx,
		`WITH ranked AS (
			SELECT seq, collection, key, op, value, valid_from, tx_time, meta,
				   ROW_NUMBER() OVER (PARTITION BY key ORDER BY seq DESC) as rn
			FROM events
			WHERE collection = ? AND valid_from <= ?
		)
		SELECT seq, collection, key, op, value, valid_from, tx_time, meta
		FROM ranked
		WHERE rn = 1 AND op = 'put'`,
		collection, validAtStr)
	if err != nil {
		return nil, fmt.Errorf("query replay as of: %w", err)
	}
	defer rows.Close()

	state := make(map[string]Event)
	for rows.Next() {
		var ev Event
		var validFromStr, txTimeStr string
		err := rows.Scan(&ev.Seq, &ev.Collection, &ev.Key, &ev.Op,
			&ev.Value, &validFromStr, &txTimeStr, &ev.Meta)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		ev.ValidFrom, err = time.Parse(time.RFC3339Nano, validFromStr)
		if err != nil {
			return nil, fmt.Errorf("parse valid_from: %w", err)
		}

		ev.TxTime, err = time.Parse(time.RFC3339Nano, txTimeStr)
		if err != nil {
			return nil, fmt.Errorf("parse tx_time: %w", err)
		}

		state[ev.Key] = ev
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return state, nil
}

// QueryContext executes a query against the database.
// Used by the executor to query the live projection and other tables.
func (s *Store) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that returns at most one row.
// Used by the executor to fetch single rows.
func (s *Store) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

// ExecContext executes a statement that does not return rows.
// Used by the executor for DELETE, PURGE, and other mutations.
func (s *Store) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

// boolToInt converts a boolean to 0 or 1 for SQLite.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
