package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// TEIClient is a hand-rolled HTTP client for Text Embeddings Inference (TEI).
// It sends text to TEI for embedding and returns vectors.
type TEIClient struct {
	BaseURL string
	client  *http.Client
}

// teiEmbedRequest is the documented request format for TEI's /embed endpoint.
type teiEmbedRequest struct {
	Input []string `json:"input"`
}

// teiEmbedResponse is the documented response format from TEI's /embed endpoint.
type teiEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed sends text to TEI and returns the embedding vector.
// text is the document or query text to embed.
func (c *TEIClient) Embed(ctx context.Context, text string) ([]float32, error) {
	req := teiEmbedRequest{
		Input: []string{text},
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/embed", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("embed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed request returned status %d", resp.StatusCode)
	}

	var result teiEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embed response has no data")
	}

	return result.Data[0].Embedding, nil
}

// defaultHTTPClient creates an HTTP client with a reasonable timeout.
func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}
