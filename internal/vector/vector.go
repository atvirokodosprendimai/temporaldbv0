// Package vector provides Qdrant and TEI clients for vector search.
// The feature is optional and gated by configuration — if QDRANT_URL or TEI_URL is unset,
// New returns false and vector search is unavailable.
package vector

import (
	"context"

	"oneshottest/internal/config"
)

// Hit represents a single search result from the vector index.
type Hit struct {
	Key   string  // the document key
	Score float32 // similarity score
}

// Index is the interface for vector storage and search operations.
// It is only available when both QDRANT_URL and TEI_URL are configured.
type Index interface {
	// Upsert embeds text via TEI and upserts the vector to Qdrant.
	// collection: the Qdrant collection name (same as TemporalDB collection)
	// key: the document key (deterministic UUID derived in PointID)
	// text: the document text to embed
	Upsert(ctx context.Context, collection string, key string, text string) error

	// Search queries Qdrant for nearest neighbors to the given text.
	// text is embedded via TEI, then used to search Qdrant.
	// collection: the Qdrant collection name
	// text: the query text to embed and search
	// k: the number of results to return
	Search(ctx context.Context, collection string, text string, k int) ([]Hit, error)
}

// index is the concrete implementation of Index.
type index struct {
	qdrant *QdrantClient
	tei    *TEIClient
}

// New creates a new vector Index if both QDRANT_URL and TEI_URL are configured, otherwise returns nil, false.
// This allows vector search to be entirely optional — the server and all other features work without it.
func New(cfg *config.Config) (Index, bool) {
	// Both URLs must be set for the feature to be available
	if cfg.QdrantURL == "" || cfg.TEIURL == "" {
		return nil, false
	}

	return &index{
		qdrant: &QdrantClient{
			BaseURL: cfg.QdrantURL,
			client:  defaultHTTPClient(),
		},
		tei: &TEIClient{
			BaseURL: cfg.TEIURL,
			client:  defaultHTTPClient(),
		},
	}, true
}

// Upsert implements Index.
func (idx *index) Upsert(ctx context.Context, collection string, key string, text string) error {
	// Embed text via TEI
	vector, err := idx.tei.Embed(ctx, text)
	if err != nil {
		return err
	}
	// Point ID is deterministic from the key so re-indexing updates rather than duplicates
	pointID := PointID(key)
	return idx.qdrant.Upsert(ctx, collection, pointID, key, vector)
}

// Search implements Index.
func (idx *index) Search(ctx context.Context, collection string, text string, k int) ([]Hit, error) {
	// Embed query text via TEI
	vector, err := idx.tei.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	return idx.qdrant.Search(ctx, collection, vector, k)
}
