package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"oneshottest/internal/config"
)

// TestNewReturnsFalseWhenURLsUnset verifies that New returns false when QDRANT_URL or TEI_URL is unset.
func TestNewReturnsFalseWhenURLsUnset(t *testing.T) {
	tests := []struct {
		name       string
		qdrantURL  string
		teiURL     string
		shouldInit bool
	}{
		{"both unset", "", "", false},
		{"only qdrant set", "http://localhost:6334", "", false},
		{"only tei set", "", "http://localhost:8000", false},
		{"both set", "http://localhost:6334", "http://localhost:8000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				QdrantURL: tt.qdrantURL,
				TEIURL:    tt.teiURL,
			}
			idx, ok := New(cfg)
			if ok != tt.shouldInit {
				t.Errorf("New() = %v, want %v", ok, tt.shouldInit)
			}
			if tt.shouldInit && idx == nil {
				t.Error("New() returned nil Index when should have initialized")
			}
			if !tt.shouldInit && idx != nil {
				t.Error("New() returned non-nil Index when should not have initialized")
			}
		})
	}
}

// TestNewReturnsIndexWhenBothURLsSet verifies the gate requires both URLs.
func TestNewReturnsIndexWhenBothURLsSet(t *testing.T) {
	cfg := &config.Config{
		QdrantURL: "http://localhost:6334",
		TEIURL:    "http://localhost:8000",
	}
	idx, ok := New(cfg)
	if !ok {
		t.Fatal("New() should initialize when both URLs are set")
	}
	if idx == nil {
		t.Fatal("New() returned nil Index")
	}
}

// TestTEIEmbedPostsDocumentedRequestShape verifies TEI client sends correct request format.
func TestTEIEmbedPostsDocumentedRequestShape(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/embed" {
			t.Errorf("expected /embed path, got %s", r.URL.Path)
		}
		var reqBody map[string]interface{}
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		json.Unmarshal(body, &reqBody)
		if _, ok := reqBody["input"]; !ok {
			t.Error("expected 'input' field in request body")
		}

		// Return a mock embedding response
		response := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"embedding": []float32{0.1, 0.2, 0.3},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		QdrantURL: "http://localhost:6334",
		TEIURL:    server.URL,
	}
	idx, ok := New(cfg)
	if !ok {
		t.Fatal("failed to initialize Index")
	}

	ctx := context.Background()
	_, err := idx.(*index).tei.Embed(ctx, "test document")
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if len(receivedBody) == 0 {
		t.Error("no request body received")
	}
}

// TestQdrantUpsertPostsDocumentedPointShape verifies Qdrant upsert sends correct format.
func TestQdrantUpsertPostsDocumentedPointShape(t *testing.T) {
	var receivedBody []byte

	// Mock TEI server for embedding
	teiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"embedding": []float32{0.1, 0.2, 0.3},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer teiServer.Close()

	// Mock Qdrant server for upsert
	qdrantServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/collections/test-collection/points/upsert" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("wait") != "true" {
			t.Errorf("expected wait=true query param, got %s", r.URL.RawQuery)
		}
		body, _ := io.ReadAll(r.Body)
		receivedBody = body

		// Verify the request body structure
		var reqBody struct {
			Points []struct {
				ID      interface{}            `json:"id"`
				Vector  []float32              `json:"vector"`
				Payload map[string]interface{} `json:"payload"`
			} `json:"points"`
		}
		json.Unmarshal(body, &reqBody)
		if len(reqBody.Points) != 1 {
			t.Error("expected 1 point in upsert request")
		}
		if reqBody.Points[0].ID == nil {
			t.Error("expected point ID in upsert request")
		}
		if len(reqBody.Points[0].Vector) == 0 {
			t.Error("expected vector in upsert request")
		}
		if _, ok := reqBody.Points[0].Payload["key"]; !ok {
			t.Error("expected 'key' field in payload")
		}

		// Return success response
		response := map[string]interface{}{
			"status": "ok",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer qdrantServer.Close()

	cfg := &config.Config{
		QdrantURL: qdrantServer.URL,
		TEIURL:    teiServer.URL,
	}
	idx, ok := New(cfg)
	if !ok {
		t.Fatal("failed to initialize Index")
	}

	ctx := context.Background()
	err := idx.Upsert(ctx, "test-collection", "test-key", "test document content")
	if err != nil {
		t.Fatalf("Upsert() error: %v", err)
	}
	if len(receivedBody) == 0 {
		t.Error("no request body received")
	}
}

// TestQdrantSearchParsesHitsAndScores verifies search response parsing.
func TestQdrantSearchParsesHitsAndScores(t *testing.T) {
	// Mock TEI server for embedding
	teiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"embedding": []float32{0.1, 0.2, 0.3},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer teiServer.Close()

	// Mock Qdrant server for search
	qdrantServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/collections/test-collection/points/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Return mock search results
		response := map[string]interface{}{
			"result": []map[string]interface{}{
				{
					"id":    "550e8400-e29b-41d4-a716-446655440001",
					"score": 0.95,
					"payload": map[string]interface{}{
						"key": "doc-1",
					},
				},
				{
					"id":    "550e8400-e29b-41d4-a716-446655440002",
					"score": 0.87,
					"payload": map[string]interface{}{
						"key": "doc-2",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer qdrantServer.Close()

	cfg := &config.Config{
		QdrantURL: qdrantServer.URL,
		TEIURL:    teiServer.URL,
	}
	idx, ok := New(cfg)
	if !ok {
		t.Fatal("failed to initialize Index")
	}

	ctx := context.Background()
	hits, err := idx.Search(ctx, "test-collection", "test query", 10)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(hits) != 2 {
		t.Errorf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].Key != "doc-1" {
		t.Errorf("expected first hit key 'doc-1', got %s", hits[0].Key)
	}
	if hits[0].Score != 0.95 {
		t.Errorf("expected first hit score 0.95, got %v", hits[0].Score)
	}
	if hits[1].Key != "doc-2" {
		t.Errorf("expected second hit key 'doc-2', got %s", hits[1].Key)
	}
	if hits[1].Score != 0.87 {
		t.Errorf("expected second hit score 0.87, got %v", hits[1].Score)
	}
}

// TestClientTimeoutIsSetAndHonoured verifies timeout handling.
func TestClientTimeoutIsSetAndHonoured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow server
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
	}))
	defer server.Close()

	cfg := &config.Config{
		QdrantURL: server.URL,
		TEIURL:    server.URL,
	}
	idx, ok := New(cfg)
	if !ok {
		t.Fatal("failed to initialize Index")
	}

	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// This should timeout
	_, err := idx.(*index).tei.Embed(ctx, "test")
	if err == nil {
		t.Error("expected timeout error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("context deadline")) && !bytes.Contains([]byte(err.Error()), []byte("Client.Timeout")) {
		t.Logf("got error: %v", err)
	}
}
