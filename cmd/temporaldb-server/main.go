// Package main runs the TemporalDB server (D8).
// Composition root: config → storage → executor → server.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"oneshottest/internal/backup"
	"oneshottest/internal/config"
	"oneshottest/internal/event"
	"oneshottest/internal/server"
	"oneshottest/internal/storage"
	"oneshottest/internal/tql"
)

func main() {
	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx := context.Background()

	// Open storage.
	db, err := storage.Open(ctx, storage.Config{DataDir: cfg.DataDir})
	if err != nil {
		log.Fatalf("failed to open storage: %v", err)
	}
	defer db.Close()

	// Create event store.
	store := event.NewStore(db)

	// Set up backup if configured (D7).
	var backupSink backup.Sink
	var shipper *backup.Shipper
	var snapshotTicker *time.Ticker

	if cfg.BackupDir != "" {
		// Create file-based sink.
		backupSink = backup.NewFileSink(cfg.BackupDir)

		// Parse backup interval, default to 5 minutes.
		shipInterval := 5 * time.Minute
		if cfg.BackupInterval != "" {
			if d, err := time.ParseDuration(cfg.BackupInterval); err == nil {
				shipInterval = d
			} else {
				log.Printf("warning: invalid TEMPORALDB_BACKUP_INTERVAL %q, using default", cfg.BackupInterval)
			}
		}

		// Create shipper.
		shipper = backup.NewShipper(store, backupSink, shipInterval)

		// Parse snapshot interval (reuse backup interval for snapshots per ADR).
		snapshotInterval := shipInterval
		if snapshotInterval < 1*time.Minute {
			snapshotInterval = 1 * time.Minute // minimum 1 minute
		}
		snapshotTicker = time.NewTicker(snapshotInterval)
	}

	// Create executor with optional searcher and purger.
	// The backup.Sink implements tql.Purger interface (same Purge method signature).
	var purger tql.Purger
	if backupSink != nil {
		purger = backupSink
	}
	executor := tql.NewExecutor(store, nil, purger)

	// Create server.
	srv := server.New(executor)

	// Start HTTP server (default :7777 per D12).
	addr := ":7777"
	if cfg.Addr != "" {
		addr = cfg.Addr
	}

	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv,
	}

	// Handle graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Start backup shipper if configured.
	if shipper != nil {
		go shipper.Run(ctx)
		log.Printf("backup shipper started (interval: %s)", cfg.BackupInterval)
	}

	// Start snapshot ticker if configured.
	if snapshotTicker != nil {
		go func() {
			for range snapshotTicker.C {
				snapshotPath := cfg.BackupDir + "/snapshot-" + time.Now().Format("20060102-150405") + ".db"
				seq, err := backup.Snapshot(context.Background(), db, snapshotPath)
				if err != nil {
					log.Printf("snapshot failed: %v", err)
					continue
				}

				// Record snapshot in sink.
				if backupSink != nil {
					if err := backupSink.WriteSnapshot(context.Background(), snapshotPath, seq, time.Now()); err != nil {
						log.Printf("write snapshot metadata failed: %v", err)
					}
				}

				log.Printf("snapshot created: %s (seq=%d)", snapshotPath, seq)
			}
		}()
		log.Printf("snapshot ticker started")
	}

	go func() {
		sig := <-sigChan
		log.Printf("received signal: %v, shutting down", sig)

		// Stop tickers.
		if snapshotTicker != nil {
			snapshotTicker.Stop()
		}

		if err := httpServer.Shutdown(context.Background()); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Printf("starting TemporalDB server on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
