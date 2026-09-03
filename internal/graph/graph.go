// Package graph provides edge collection helpers for TemporalDB's knowledge graph.
// Relations (edges) are stored as ordinary events in the __edges__ collection (D5),
// so they are versioned, time-traveled, and backed up using the identical event-log machinery.
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"oneshottest/internal/event"
)

// Relate creates an edge from one document to another with optional properties.
// The edge is stored as an event in the __edges__ collection.
// Returns the sequence number of the edge event.
func Relate(ctx context.Context, store *event.Store, from, to, edgeType string, props interface{}) (int64, error) {
	now := time.Now().UTC()
	return RelateAsOf(ctx, store, from, to, edgeType, props, now)
}

// RelateAsOf creates an edge with an explicit valid-from time.
// This allows recording historical relationships.
func RelateAsOf(ctx context.Context, store *event.Store, from, to, edgeType string, props interface{}, validAt time.Time) (int64, error) {
	// Build the edge metadata
	meta := map[string]interface{}{
		"from": from,
		"type": edgeType,
		"to":   to,
	}
	if props != nil {
		meta["props"] = props
	}

	metaBytes, _ := json.Marshal(meta)

	// Compute the deterministic key for this edge
	key := edgeKey(from, edgeType, to)

	ev := event.Event{
		Collection: "__edges__",
		Key:        key,
		Op:         event.OpPut,
		Value:      []byte(`{}`),
		ValidFrom:  validAt,
		TxTime:     time.Now().UTC(),
		Meta:       metaBytes,
	}

	return store.Append(ctx, ev)
}

// Unrelate removes an edge.
func Unrelate(ctx context.Context, store *event.Store, from, to, edgeType string) (int64, error) {
	key := edgeKey(from, edgeType, to)

	ev := event.Event{
		Collection: "__edges__",
		Key:        key,
		Op:         event.OpDelete,
		Value:      nil,
		ValidFrom:  time.Now().UTC(),
		TxTime:     time.Now().UTC(),
		Meta:       []byte("{}"),
	}

	return store.Append(ctx, ev)
}

// Related returns all edges originating from the given node in the current state.
func Related(ctx context.Context, store *event.Store, from string, edgeType *string, limit int) ([]map[string]interface{}, error) {
	return RelatedAsOf(ctx, store, from, edgeType, time.Now().UTC(), limit)
}

// RelatedAsOf returns edges as they were at a specific point in time (time travel).
func RelatedAsOf(ctx context.Context, store *event.Store, from string, edgeType *string, validAt time.Time, limit int) ([]map[string]interface{}, error) {
	// Replay the state of edges as of validAt
	state, err := store.ReplayAsOf(ctx, "__edges__", validAt)
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for _, ev := range state {
		// Parse the edge metadata
		var meta map[string]interface{}
		if err := json.Unmarshal(ev.Meta, &meta); err != nil {
			continue
		}

		// Filter by from
		fromVal, ok := meta["from"].(string)
		if !ok || fromVal != from {
			continue
		}

		// Filter by edge type if specified
		if edgeType != nil {
			typeVal, ok := meta["type"].(string)
			if !ok || typeVal != *edgeType {
				continue
			}
		}

		results = append(results, meta)
	}

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// edgeKey computes a deterministic key for an edge using length-prefixed encoding.
// This prevents collisions when from/type/to contain the separator.
// Format: "<len(from)>:<from>:<len(type)>:<type>:<len(to)>:<to>"
func edgeKey(from, edgeType, to string) string {
	return fmt.Sprintf("%d:%s:%d:%s:%d:%s", len(from), from, len(edgeType), edgeType, len(to), to)
}
