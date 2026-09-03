package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"oneshottest/client"
)

// toolTqlQuery handles the tql_query MCP tool.
// It's a raw TQL passthrough — the general-purpose escape hatch.
func toolTqlQuery(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, error) {
	queryObj, ok := args["query"]
	if !ok {
		return nil, fmt.Errorf("missing required argument: query")
	}

	query, ok := queryObj.(string)
	if !ok {
		return nil, fmt.Errorf("argument 'query' must be a string")
	}

	// Delegate to client.Query.
	result, err := globalClient.Query(ctx, query)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Error executing query: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Return the result as JSON.
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("Error marshaling result: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(resultJSON),
			},
		},
	}, nil
}

// toolTqlGet handles the tql_get MCP tool.
// Retrieves a document by collection and key.
func toolTqlGet(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, error) {
	collection, ok := args["collection"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid required argument: collection")
	}

	key, ok := args["key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid required argument: key")
	}

	// Optional asOf parameter for time-travel queries.
	var asOf string
	if asOfObj, ok := args["asOf"]; ok {
		asOf, _ = asOfObj.(string)
	}

	// Build TQL query.
	var query string
	if asOf != "" {
		query = fmt.Sprintf("GET %s/%s AS OF %s", collection, key, asOf)
	} else {
		query = fmt.Sprintf("GET %s/%s", collection, key)
	}

	result, err := globalClient.Query(ctx, query)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error executing GET: %v", err)},
			},
			IsError: true,
		}, nil
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error marshaling result: %v", err)},
			},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(resultJSON)},
		},
	}, nil
}

// toolTqlPut handles the tql_put MCP tool.
// Inserts or updates a document.
func toolTqlPut(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, error) {
	collection, ok := args["collection"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid required argument: collection")
	}

	key, ok := args["key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid required argument: key")
	}

	value, ok := args["value"]
	if !ok {
		return nil, fmt.Errorf("missing required argument: value")
	}

	// Marshal value to JSON.
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error marshaling value: %v", err)},
			},
			IsError: true,
		}, nil
	}

	// Optional 'at' parameter for backdating writes.
	var at string
	if atObj, ok := args["at"]; ok {
		at, _ = atObj.(string)
	}

	// Build TQL query.
	var query string
	if at != "" {
		query = fmt.Sprintf("PUT %s/%s %s AT %s", collection, key, string(valueJSON), at)
	} else {
		query = fmt.Sprintf("PUT %s/%s %s", collection, key, string(valueJSON))
	}

	result, err := globalClient.Query(ctx, query)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error executing PUT: %v", err)},
			},
			IsError: true,
		}, nil
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error marshaling result: %v", err)},
			},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(resultJSON)},
		},
	}, nil
}

// toolTqlHistory handles the tql_history MCP tool.
// Retrieves the history of a document.
func toolTqlHistory(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, error) {
	collection, ok := args["collection"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid required argument: collection")
	}

	key, ok := args["key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid required argument: key")
	}

	// Build TQL query.
	query := fmt.Sprintf("HISTORY %s/%s", collection, key)

	result, err := globalClient.Query(ctx, query)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error executing HISTORY: %v", err)},
			},
			IsError: true,
		}, nil
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error marshaling result: %v", err)},
			},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(resultJSON)},
		},
	}, nil
}

// toolTqlSearch handles the tql_search MCP tool.
// Searches documents using a search term.
func toolTqlSearch(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, error) {
	term, ok := args["term"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid required argument: term")
	}

	// Optional collection parameter to scope the search.
	var collection string
	if collObj, ok := args["collection"]; ok {
		collection, _ = collObj.(string)
	}

	// Build TQL query.
	var query string
	if collection != "" {
		query = fmt.Sprintf("SEARCH %s WHERE %s", collection, term)
	} else {
		query = fmt.Sprintf("SEARCH * WHERE %s", term)
	}

	result, err := globalClient.Query(ctx, query)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error executing SEARCH: %v", err)},
			},
			IsError: true,
		}, nil
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error marshaling result: %v", err)},
			},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(resultJSON)},
		},
	}, nil
}

// configureServer registers all five MCP tools with the server.
// This is the site where tool registration happens — an unregistered tool is invisible.
func configureServer(server *mcp.Server, c *client.Client, backup interface{}) {
	if c != nil {
		globalClient = c
	}

	// Register tql_query tool.
	server.AddTool(&mcp.Tool{
		Name:        "tql_query",
		Description: "Execute a raw TQL query against the temporal database.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The raw TQL query string to execute.",
				},
			},
			"required": []string{"query"},
		},
	}, mcp.ToolHandler(func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]interface{}
		if req.Params.Arguments != nil {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("Error parsing arguments: %v", err)},
					},
					IsError: true,
				}, nil
			}
		}
		return toolTqlQuery(ctx, req, args)
	}))

	// Register tql_get tool.
	server.AddTool(&mcp.Tool{
		Name:        "tql_get",
		Description: "Retrieve a document from the temporal database.",
		InputSchema: map[string]interface{}{
			"type": "object",
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
	}, mcp.ToolHandler(func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]interface{}
		if req.Params.Arguments != nil {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("Error parsing arguments: %v", err)},
					},
					IsError: true,
				}, nil
			}
		}
		return toolTqlGet(ctx, req, args)
	}))

	// Register tql_put tool.
	server.AddTool(&mcp.Tool{
		Name:        "tql_put",
		Description: "Insert or update a document in the temporal database.",
		InputSchema: map[string]interface{}{
			"type": "object",
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
	}, mcp.ToolHandler(func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]interface{}
		if req.Params.Arguments != nil {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("Error parsing arguments: %v", err)},
					},
					IsError: true,
				}, nil
			}
		}
		return toolTqlPut(ctx, req, args)
	}))

	// Register tql_history tool.
	server.AddTool(&mcp.Tool{
		Name:        "tql_history",
		Description: "Retrieve the complete history of changes to a document.",
		InputSchema: map[string]interface{}{
			"type": "object",
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
	}, mcp.ToolHandler(func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]interface{}
		if req.Params.Arguments != nil {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("Error parsing arguments: %v", err)},
					},
					IsError: true,
				}, nil
			}
		}
		return toolTqlHistory(ctx, req, args)
	}))

	// Register tql_search tool.
	server.AddTool(&mcp.Tool{
		Name:        "tql_search",
		Description: "Search for documents matching a query term.",
		InputSchema: map[string]interface{}{
			"type": "object",
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
	}, mcp.ToolHandler(func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]interface{}
		if req.Params.Arguments != nil {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("Error parsing arguments: %v", err)},
					},
					IsError: true,
				}, nil
			}
		}
		return toolTqlSearch(ctx, req, args)
	}))
}
