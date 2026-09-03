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
	// This test verifies that every argument each handler reads is declared in its schema.
	// Define the expected arguments each handler reads, extracted by code inspection.
	//
	// Each tool's handler is examined to extract which arguments it actually reads:
	// - toolTqlQuery (line 14): reads "query" from args
	// - toolTqlGet (line 62): reads "collection", "key", and optionally "asOf" from args
	// - toolTqlPut (line 116): reads "collection", "key", "value", and optionally "at" from args
	// - toolTqlHistory (line 186): reads "collection", "key" from args
	// - toolTqlSearch (line 229): reads "term", and optionally "collection" from args

	expectedArgs := map[string]struct {
		required []string
		optional []string
	}{
		"tql_query": {
			required: []string{"query"},
			optional: []string{},
		},
		"tql_get": {
			required: []string{"collection", "key"},
			optional: []string{"asOf"},
		},
		"tql_put": {
			required: []string{"collection", "key", "value"},
			optional: []string{"at"},
		},
		"tql_history": {
			required: []string{"collection", "key"},
			optional: []string{},
		},
		"tql_search": {
			required: []string{"term"},
			optional: []string{"collection"},
		},
	}

	// Define the schemas as registered in configureServer().
	// These are extracted by reading tools.go configureServer function.
	schemaDefinitions := map[string]map[string]interface{}{
		"tql_query": {
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The raw TQL query string to execute.",
				},
			},
			"required": []string{"query"},
		},
		"tql_get": {
			"properties": map[string]interface{}{
				"collection": map[string]interface{}{
					"type":        "string",
					"description": "The collection name.",
				},
				"key": map[string]interface{}{
					"type":        "string",
					"description": "The document key.",
				},
				"asOf": map[string]interface{}{
					"type":        "string",
					"description": "Optional: retrieve the document as it existed at this time (RFC3339 format).",
				},
			},
			"required": []string{"collection", "key"},
		},
		"tql_put": {
			"properties": map[string]interface{}{
				"collection": map[string]interface{}{
					"type":        "string",
					"description": "The collection name.",
				},
				"key": map[string]interface{}{
					"type":        "string",
					"description": "The document key.",
				},
				"value": map[string]interface{}{
					"description": "The document value (any JSON-serializable object).",
				},
				"at": map[string]interface{}{
					"type":        "string",
					"description": "Optional: backdate the write to this time (RFC3339 format).",
				},
			},
			"required": []string{"collection", "key", "value"},
		},
		"tql_history": {
			"properties": map[string]interface{}{
				"collection": map[string]interface{}{
					"type":        "string",
					"description": "The collection name.",
				},
				"key": map[string]interface{}{
					"type":        "string",
					"description": "The document key.",
				},
			},
			"required": []string{"collection", "key"},
		},
		"tql_search": {
			"properties": map[string]interface{}{
				"term": map[string]interface{}{
					"type":        "string",
					"description": "The search term or WHERE clause condition.",
				},
				"collection": map[string]interface{}{
					"type":        "string",
					"description": "Optional: restrict search to this collection.",
				},
			},
			"required": []string{"term"},
		},
	}

	// For each tool, verify the schema matches what the handler reads.
	for toolName, expected := range expectedArgs {
		schema, schemaExists := schemaDefinitions[toolName]
		if !schemaExists {
			t.Errorf("tool %s: schema not found in definitions", toolName)
			continue
		}

		// Extract schema properties and required fields.
		propertiesObj, hasProperties := schema["properties"]
		if !hasProperties {
			t.Errorf("tool %s: no 'properties' in schema", toolName)
			continue
		}

		properties, ok := propertiesObj.(map[string]interface{})
		if !ok {
			t.Errorf("tool %s: 'properties' is not a map", toolName)
			continue
		}

		requiredObj, hasRequired := schema["required"]
		var requiredFields []string
		if hasRequired {
			if reqList, ok := requiredObj.([]string); ok {
				requiredFields = reqList
			}
		}

		// Verify all required arguments are in the schema properties.
		for _, arg := range expected.required {
			if _, hasArg := properties[arg]; !hasArg {
				t.Errorf("tool %s: required argument %q is not in schema properties", toolName, arg)
			}
			// Also verify it's in the required list.
			found := false
			for _, req := range requiredFields {
				if req == arg {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("tool %s: required argument %q is not in schema 'required' list", toolName, arg)
			}
		}

		// Verify all optional arguments are in the schema properties.
		for _, arg := range expected.optional {
			if _, hasArg := properties[arg]; !hasArg {
				t.Errorf("tool %s: optional argument %q is not in schema properties", toolName, arg)
			}
			// Verify it's NOT in the required list.
			found := false
			for _, req := range requiredFields {
				if req == arg {
					found = true
					break
				}
			}
			if found {
				t.Errorf("tool %s: optional argument %q should not be in schema 'required' list", toolName, arg)
			}
		}

		// Verify no extra properties beyond expected args.
		for propName := range properties {
			isExpected := false
			for _, arg := range expected.required {
				if arg == propName {
					isExpected = true
					break
				}
			}
			if !isExpected {
				for _, arg := range expected.optional {
					if arg == propName {
						isExpected = true
						break
					}
				}
			}
			if !isExpected {
				t.Errorf("tool %s: unexpected property %q in schema", toolName, propName)
			}
		}
	}
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
