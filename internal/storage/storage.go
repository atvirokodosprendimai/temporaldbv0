package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Config holds storage configuration.
type Config struct {
	DataDir string
}

// Open opens a SQLite database with WAL mode, applies pragmas, runs migrations, and returns a connection pool.
// It configures the pool for a single writer with many readers using WAL mode.
func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	// If DataDir is empty, use current directory
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "."
	}

	// Construct DSN without pragmas (will apply them after opening)
	dbPath := filepath.Join(dataDir, "temporaldb.db")
	dsn := fmt.Sprintf("file:%s", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool cap for single writer with readers
	db.SetMaxOpenConns(10)

	// Test the connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Run migrations before setting pragmas
	goose.SetBaseFS(migrationsFS)
	goose.SetDialect("sqlite")
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Apply pragmas after migrations
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to apply pragma %s: %w", pragma, err)
		}
	}

	return db, nil
}

// OpenMemory opens an in-memory SQLite database for testing.
// It applies the same pragmas and migrations as Open.
func OpenMemory() (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		return nil, fmt.Errorf("failed to open memory database: %w", err)
	}

	// Set connection pool cap
	db.SetMaxOpenConns(1)

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping memory database: %w", err)
	}

	// Run migrations
	goose.SetBaseFS(migrationsFS)
	goose.SetDialect("sqlite")
	if err := goose.Up(db, "migrations"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations on memory database: %w", err)
	}

	// Apply pragmas
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to apply pragma %s: %w", pragma, err)
		}
	}

	return db, nil
}
