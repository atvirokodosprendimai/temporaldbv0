// Package client provides a reference implementation of the TemporalDB wire protocol (D9).
// It implements Client.Query(ctx, tql string) and typed convenience wrappers.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"oneshottest/internal/tql"
)

// Client is a TemporalDB client that speaks the wire protocol (D8, D9).
type Client struct {
	addr   string
	client *http.Client
}

// New creates a new TemporalDB client pointing to a server address.
func New(addr string) *Client {
	return &Client{
		addr: addr,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Query executes a raw TQL query against the server (D8, D9).
func (c *Client) Query(ctx context.Context, tqlText string) (*tql.Result, error) {
	url := fmt.Sprintf("%s/query", c.addr)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte(tqlText)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result tql.Result
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Get retrieves a document by collection and key (D9).
func (c *Client) Get(ctx context.Context, collection, key string) (*tql.Result, error) {
	tqlText := fmt.Sprintf("GET %s/%s", collection, key)
	return c.Query(ctx, tqlText)
}

// Put inserts or updates a document (D9).
func (c *Client) Put(ctx context.Context, collection, key string, value interface{}) (*tql.Result, error) {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	tqlText := fmt.Sprintf("PUT %s/%s %s", collection, key, string(valueJSON))
	return c.Query(ctx, tqlText)
}

// Delete removes a document (D9).
func (c *Client) Delete(ctx context.Context, collection, key string) (*tql.Result, error) {
	tqlText := fmt.Sprintf("DELETE %s/%s", collection, key)
	return c.Query(ctx, tqlText)
}

// Find queries documents in a collection (D9).
// whereClause is optional (empty string means no WHERE clause).
func (c *Client) Find(ctx context.Context, collection, whereClause string) (*tql.Result, error) {
	tqlText := fmt.Sprintf("FIND %s", collection)
	if whereClause != "" {
		tqlText = fmt.Sprintf("%s WHERE %s", tqlText, whereClause)
	}
	return c.Query(ctx, tqlText)
}

// History retrieves the history of a document (D9).
func (c *Client) History(ctx context.Context, collection, key string) (*tql.Result, error) {
	tqlText := fmt.Sprintf("HISTORY %s/%s", collection, key)
	return c.Query(ctx, tqlText)
}
