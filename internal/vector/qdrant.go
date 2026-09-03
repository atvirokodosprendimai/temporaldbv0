package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/uuid"
)

// QdrantClient is a hand-rolled HTTP client for Qdrant vector database.
// It handles upsert and search operations against Qdrant's documented REST API.
type QdrantClient struct {
	BaseURL string
	client  *http.Client
}

// PointID derives a deterministic UUID from a key.
// This ensures re-indexing the same key updates the existing point rather than creating a duplicate.
func PointID(key string) string {
	// Use SHA1 with NameSpaceOID to generate a deterministic UUID from the key
	u := uuid.NewSHA1(uuid.NameSpaceOID, []byte(key))
	return u.String()
}

// qdrantUpsertRequest is the documented request format for Qdrant's upsert endpoint.
type qdrantUpsertRequest struct {
	Points []qdrantPoint `json:"points"`
}

// qdrantPoint represents a single point to upsert in Qdrant.
type qdrantPoint struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

// qdrantUpsertResponse is the documented response format from Qdrant's upsert endpoint.
type qdrantUpsertResponse struct {
	Status string `json:"status"`
}

// Upsert sends a point to Qdrant's upsert endpoint.
// collection: the Qdrant collection name
// pointID: the UUID for this point (deterministic)
// key: the document key to store in the payload
// vector: the embedding vector
func (c *QdrantClient) Upsert(ctx context.Context, collection string, pointID string, key string, vector []float32) error {
	req := qdrantUpsertRequest{
		Points: []qdrantPoint{
			{
				ID:     pointID,
				Vector: vector,
				Payload: map[string]interface{}{
					"key": key,
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal upsert request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/collections/%s/points/upsert?wait=true", c.BaseURL, url.PathEscape(collection))
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create upsert request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("upsert request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upsert request returned status %d", resp.StatusCode)
	}

	var result qdrantUpsertResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode upsert response: %w", err)
	}

	return nil
}

// qdrantSearchRequest is the documented request format for Qdrant's search endpoint.
type qdrantSearchRequest struct {
	Vector  []float32 `json:"vector"`
	Limit   int       `json:"limit"`
	WithID  bool      `json:"with_id,omitempty"`
	WithPay bool      `json:"with_payload,omitempty"`
	WithVec bool      `json:"with_vectors,omitempty"`
}

// qdrantSearchResponse is the documented response format from Qdrant's search endpoint.
type qdrantSearchResponse struct {
	Result []struct {
		ID      string                 `json:"id"`
		Score   float32                `json:"score"`
		Payload map[string]interface{} `json:"payload"`
	} `json:"result"`
}

// Search queries Qdrant for the nearest neighbors to a vector.
// collection: the Qdrant collection name
// vector: the query embedding vector
// k: the number of results to return
func (c *QdrantClient) Search(ctx context.Context, collection string, vector []float32, k int) ([]Hit, error) {
	req := qdrantSearchRequest{
		Vector:  vector,
		Limit:   k,
		WithID:  true,
		WithPay: true,
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal search request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/collections/%s/points/search", c.BaseURL, url.PathEscape(collection))
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create search request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search request returned status %d", resp.StatusCode)
	}

	var result qdrantSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	// Convert Qdrant results to Hit format
	hits := make([]Hit, len(result.Result))
	for i, r := range result.Result {
		key := ""
		if keyVal, ok := r.Payload["key"]; ok {
			if keyStr, ok := keyVal.(string); ok {
				key = keyStr
			}
		}
		hits[i] = Hit{
			Key:   key,
			Score: r.Score,
		}
	}

	return hits, nil
}
