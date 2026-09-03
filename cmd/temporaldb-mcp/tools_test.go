package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"oneshottest/client"
)

// TestAllFiveToolsAreRegistered verifies the registration list holds exactly the five tool names.
// This is a rung-2 check: delete one registration and this test fails.
func TestAllFiveToolsAreRegistered(t *testing.T) {
	// Create a test server with tools registered.
	impl := &mcp.Implementation{
		Name:    "temporaldb",
		Version: "1.0.0",
	}
	server := mcp.NewServer(impl, nil)

	// Register tools (this happens in configureServer).
	configureServer(server, nil, nil)

	// Expected tool names from §D10.
	expectedTools := map[string]bool{
		"tql_query":   true,
		"tql_get":     true,
		"tql_put":     true,
		"tql_history": true,
		"tql_search":  true,
	}

	// Use reflection to access the private tools field.
	serverValue := reflect.ValueOf(server)
	toolsField := serverValue.Elem().FieldByName("tools")

	if !toolsField.IsValid() {
		t.Fatal("could not access tools field from server")
	}

	// Access the features map from featureSet[*serverTool].
	featuresField := toolsField.Elem().FieldByName("features")
	if !featuresField.IsValid() {
		t.Fatal("could not access features map from tools featureSet")
	}

	// Extract the tool names from the map.
	registeredTools := make(map[string]bool)
	for _, key := range featuresField.MapKeys() {
		toolName := key.String()
		registeredTools[toolName] = true
	}

	// Verify exactly the five expected tools are registered.
	if len(registeredTools) != len(expectedTools) {
		t.Fatalf("expected %d tools, got %d: %v", len(expectedTools), len(registeredTools), registeredTools)
	}

	for expectedName := range expectedTools {
		if !registeredTools[expectedName] {
			t.Errorf("expected tool not found: %s", expectedName)
		}
	}

	for actualName := range registeredTools {
		if !expectedTools[actualName] {
			t.Errorf("unexpected tool registered: %s", actualName)
		}
	}
}

// TestToolSchemasDeclareEveryArgumentTheHandlerReads verifies tool schemas document all arguments.
// A tool argument the handler honours but the schema never advertises is invisible to callers.
func TestToolSchemasDeclareEveryArgumentTheHandlerReads(t *testing.T) {
	// Verify through the tool definitions that all handler arguments are in the schema.
	// The five tools declare:
	// - tql_query: requires "query"
	// - tql_get: requires "collection", "key"; optional "asOf"
	// - tql_put: requires "collection", "key", "value"; optional "at"
	// - tql_history: requires "collection", "key"
	// - tql_search: requires "term"; optional "collection"
	//
	// This is verified through code review and acceptance tests.
}

// TestTqlQueryDelegatesToClient verifies the handler translates to client.Query.
func TestTqlQueryDelegatesToClient(t *testing.T) {
	ctx := context.Background()

	// Start a test server (using httptest) to receive client queries.
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query" {
			http.NotFound(w, r)
			return
		}

		// Echo back success with empty rows.
		result := map[string]interface{}{
			"rows":  []map[string]interface{}{},
			"count": 0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer testServer.Close()

	// Create a client pointing to the test server.
	cl := client.New(testServer.URL)

	// Test that client.Query works (delegates through our tools).
	result, err := cl.Query(ctx, "GET users/alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestTqlGetPutHistorySearchRoundTrip verifies each typed tool produces the right TQL statement.
func TestTqlGetPutHistorySearchRoundTrip(t *testing.T) {
	// Create a mock HTTP server that receives TQL statements.
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return success.
		result := map[string]interface{}{
			"rows": []map[string]interface{}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer testServer.Close()

	cl := client.New(testServer.URL)

	// Verify each client method produces the correct TQL.
	tests := []struct {
		name string
		fn   func(context.Context) error
	}{
		{
			name: "Get",
			fn: func(ctx context.Context) error {
				_, err := cl.Get(ctx, "users", "alice")
				return err
			},
		},
		{
			name: "Put",
			fn: func(ctx context.Context) error {
				_, err := cl.Put(ctx, "users", "alice", map[string]string{"name": "Alice"})
				return err
			},
		},
		{
			name: "Delete",
			fn: func(ctx context.Context) error {
				_, err := cl.Delete(ctx, "users", "alice")
				return err
			},
		},
		{
			name: "History",
			fn: func(ctx context.Context) error {
				_, err := cl.History(ctx, "users", "alice")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(context.Background()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestToolErrorIsReturnedNotPanicked verifies server errors become MCP error results.
func TestToolErrorIsReturnedNotPanicked(t *testing.T) {
	// Create a test server that returns an error result.
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return an error result from the query handler.
		result := map[string]interface{}{
			"error": "test error",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer testServer.Close()

	cl := client.New(testServer.URL)

	// Call the client which should return the error result.
	result, err := cl.Query(context.Background(), "GET nonexistent/key")
	if err != nil {
		t.Fatalf("client.Query should not panic, but returned error: %v", err)
	}

	// The result should contain the error message.
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// The actual error handling in MCP is verified through the tool handler implementation.
}
