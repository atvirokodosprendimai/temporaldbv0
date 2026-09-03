package client

import (
	"context"
	"net/http/httptest"
	"testing"

	"oneshottest/internal/event"
	"oneshottest/internal/server"
	"oneshottest/internal/storage"
	"oneshottest/internal/tql"
)

// TestClientQueryRoundTripsAgainstServer verifies that the client speaks the server's actual contract over httptest.
func TestClientQueryRoundTripsAgainstServer(t *testing.T) {
	// Set up the server with a minimal in-memory database.
	db, err := storage.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory failed: %v", err)
	}
	defer db.Close()

	store := event.NewStore(db)
	exec := tql.NewExecutor(store, nil, nil)
	srv := server.New(exec)

	// Start an httptest server.
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Create a client pointing to the test server.
	client := New(ts.URL)

	// Execute a PUT command through the client.
	ctx := context.Background()
	result, err := client.Query(ctx, `PUT users/alice {"name":"Alice","age":30}`)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result.Error != "" {
		t.Errorf("expected no error, got: %s", result.Error)
	}

	if result.Seq == 0 {
		t.Errorf("expected non-zero seq, got 0")
	}

	// Execute a GET command to verify the write.
	result, err = client.Query(ctx, `GET users/alice`)
	if err != nil {
		t.Fatalf("GET Query failed: %v", err)
	}

	if result.Error != "" {
		t.Errorf("GET: expected no error, got: %s", result.Error)
	}

	if len(result.Rows) == 0 {
		t.Errorf("GET: expected rows, got none")
	}
}

// TestClientGet verifies the Get convenience wrapper.
func TestClientGet(t *testing.T) {
	db, err := storage.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory failed: %v", err)
	}
	defer db.Close()

	store := event.NewStore(db)
	exec := tql.NewExecutor(store, nil, nil)
	srv := server.New(exec)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := New(ts.URL)
	ctx := context.Background()

	// PUT a document first.
	_, err = client.Query(ctx, `PUT test/doc1 {"value":42}`)
	if err != nil {
		t.Fatalf("PUT failed: %v", err)
	}

	// Use the Get wrapper.
	result, err := client.Get(ctx, "test", "doc1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result.Error != "" {
		t.Errorf("expected no error, got: %s", result.Error)
	}

	if len(result.Rows) == 0 {
		t.Errorf("expected rows from Get, got none")
	}
}

// TestClientPut verifies the Put convenience wrapper.
func TestClientPut(t *testing.T) {
	db, err := storage.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory failed: %v", err)
	}
	defer db.Close()

	store := event.NewStore(db)
	exec := tql.NewExecutor(store, nil, nil)
	srv := server.New(exec)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := New(ts.URL)
	ctx := context.Background()

	// Use the Put wrapper.
	data := map[string]interface{}{"name": "Bob", "age": 25}
	result, err := client.Put(ctx, "users", "bob", data)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if result.Error != "" {
		t.Errorf("expected no error, got: %s", result.Error)
	}

	if result.Seq == 0 {
		t.Errorf("expected non-zero seq, got 0")
	}
}

// TestClientDelete verifies the Delete convenience wrapper.
func TestClientDelete(t *testing.T) {
	db, err := storage.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory failed: %v", err)
	}
	defer db.Close()

	store := event.NewStore(db)
	exec := tql.NewExecutor(store, nil, nil)
	srv := server.New(exec)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := New(ts.URL)
	ctx := context.Background()

	// PUT first.
	_, err = client.Query(ctx, `PUT test/doc1 {"value":42}`)
	if err != nil {
		t.Fatalf("PUT failed: %v", err)
	}

	// DELETE via wrapper.
	result, err := client.Delete(ctx, "test", "doc1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if result.Error != "" {
		t.Errorf("expected no error, got: %s", result.Error)
	}
}

// TestClientFind verifies the Find convenience wrapper.
func TestClientFind(t *testing.T) {
	db, err := storage.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory failed: %v", err)
	}
	defer db.Close()

	store := event.NewStore(db)
	exec := tql.NewExecutor(store, nil, nil)
	srv := server.New(exec)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := New(ts.URL)
	ctx := context.Background()

	// PUT some documents.
	_, err = client.Query(ctx, `PUT users/alice {"name":"Alice"}`)
	if err != nil {
		t.Fatalf("PUT alice failed: %v", err)
	}

	_, err = client.Query(ctx, `PUT users/bob {"name":"Bob"}`)
	if err != nil {
		t.Fatalf("PUT bob failed: %v", err)
	}

	// Use Find wrapper.
	result, err := client.Find(ctx, "users", "")
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if result.Error != "" {
		t.Errorf("expected no error, got: %s", result.Error)
	}

	if result.Count < 2 {
		t.Errorf("expected at least 2 results, got %d", result.Count)
	}
}

// TestClientHistory verifies the History convenience wrapper.
func TestClientHistory(t *testing.T) {
	db, err := storage.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory failed: %v", err)
	}
	defer db.Close()

	store := event.NewStore(db)
	exec := tql.NewExecutor(store, nil, nil)
	srv := server.New(exec)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := New(ts.URL)
	ctx := context.Background()

	// PUT a document.
	_, err = client.Query(ctx, `PUT test/doc1 {"version":1}`)
	if err != nil {
		t.Fatalf("PUT failed: %v", err)
	}

	// Use History wrapper.
	result, err := client.History(ctx, "test", "doc1")
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	if result.Error != "" {
		t.Errorf("expected no error, got: %s", result.Error)
	}

	if len(result.Rows) == 0 {
		t.Errorf("expected at least one history entry, got none")
	}
}
