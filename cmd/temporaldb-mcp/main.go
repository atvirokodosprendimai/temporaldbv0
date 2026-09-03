// Package main provides the temporaldb MCP server.
// It exposes the TQL API via the official modelcontextprotocol/go-sdk.
package main

import (
	"context"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"oneshottest/client"
)

// globalClient holds the reference to the TemporalDB client.
// It is set during server configuration.
var globalClient *client.Client

func main() {
	// Get the server address from environment; default to localhost:8080.
	addr := os.Getenv("TEMPORALDB_ADDR")
	if addr == "" {
		addr = "http://localhost:8080"
	}

	// Create a client pointing to the TemporalDB server.
	cl := client.New(addr)

	// Create an MCP server implementation.
	impl := &mcp.Implementation{
		Name:    "temporaldb-mcp",
		Version: "1.0.0",
	}

	// Create the MCP server.
	server := mcp.NewServer(impl, nil)

	// Configure the server with tools.
	configureServer(server, cl, nil)

	// Run the server over stdio (the MCP standard transport).
	ctx := context.Background()
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server run failed: %v", err)
	}
}
