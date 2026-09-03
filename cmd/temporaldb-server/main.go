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

	// Create executor (with nil searcher and purger for now; T7/T10 will wire these).
	executor := tql.NewExecutor(store, nil, nil)

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

	go func() {
		sig := <-sigChan
		log.Printf("received signal: %v, shutting down", sig)
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
