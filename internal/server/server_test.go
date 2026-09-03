package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"oneshottest/internal/event"
	"oneshottest/internal/storage"
	"oneshottest/internal/tql"
)

// TestPostQueryExecutesTQL verifies that a real statement round-trips through the handler.
func TestPostQueryExecutesTQL(t *testing.T) {
	// Set up a minimal in-memory storage and executor.
	db, err := storage.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory failed: %v", err)
	}
	defer db.Close()

	store := event.NewStore(db)
	exec := tql.NewExecutor(store, nil, nil)
	server := New(exec)

	// Send a PUT command.
	body := []byte(`PUT test/key1 {"name":"alice"}`)
	req := httptest.NewRequest("POST", "/query", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result tql.Result
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode result failed: %v", err)
	}

	if result.Error != "" {
		t.Errorf("expected no error, got: %s", result.Error)
	}

	if result.Seq == 0 {
		t.Errorf("expected non-zero seq, got 0")
	}
}

// TestPostQueryParseErrorIs400 verifies that a client mistake is 400 with the parser's message.
func TestPostQueryParseErrorIs400(t *testing.T) {
	db, err := storage.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory failed: %v", err)
	}
	defer db.Close()

	store := event.NewStore(db)
	exec := tql.NewExecutor(store, nil, nil)
	server := New(exec)

	// Send an invalid TQL command.
	body := []byte(`INVALID command here`)
	req := httptest.NewRequest("POST", "/query", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for parse error, got %d", w.Code)
	}

	var result tql.Result
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode result failed: %v", err)
	}

	if result.Error == "" {
		t.Errorf("expected error message in result, got empty")
	}
}

// TestHealthzReports200 verifies that the health check returns 200.
func TestHealthzReports200(t *testing.T) {
	db, err := storage.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory failed: %v", err)
	}
	defer db.Close()

	store := event.NewStore(db)
	exec := tql.NewExecutor(store, nil, nil)
	server := New(exec)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestRouterMountsQueryAndHealthz verifies that both routes are registered.
// This test catches the defect of a handler that exists but is never mounted.
func TestRouterMountsQueryAndHealthz(t *testing.T) {
	db, err := storage.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory failed: %v", err)
	}
	defer db.Close()

	store := event.NewStore(db)
	exec := tql.NewExecutor(store, nil, nil)
	server := New(exec)

	// Verify POST /query is registered.
	req := httptest.NewRequest("POST", "/query", bytes.NewReader([]byte(`PUT x/y {"a":1}`)))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Errorf("POST /query is not registered (got 404)")
	}

	// Verify GET /healthz is registered.
	req = httptest.NewRequest("GET", "/healthz", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Errorf("GET /healthz is not registered (got 404)")
	}
}
