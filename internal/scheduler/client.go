package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Scorer evaluates a list of candidates and returns scored results.
// The reconciler depends on this interface, not on Client directly,
// so tests can substitute a mock without an HTTP server.
type Scorer interface {
	Evaluate(ctx context.Context, candidates []Candidate) (*EvaluateResponse, error)
}

// Client calls the cluster-scheduler's POST /api/v1/evaluate endpoint.
// It implements the Scorer interface.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a Client with the given base URL, API key, and timeout.
// The timeout applies to each HTTP request (connect + headers + body).
func NewClient(baseURL, apiKey string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Evaluate sends candidates to the cluster-scheduler and returns scored results.
// On any failure (network, timeout, non-200 status, bad JSON), it returns an error.
func (c *Client) Evaluate(ctx context.Context, candidates []Candidate) (*EvaluateResponse, error) {
	body, err := json.Marshal(EvaluateRequest{Candidates: candidates})
	if err != nil {
		return nil, fmt.Errorf("marshaling evaluate request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/evaluate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling cluster-scheduler: %w", err)
	}
	defer resp.Body.Close()

	const maxResponseSize = 1 << 20 // 1 MB
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("cluster-scheduler returned 401 Unauthorized: check API key")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cluster-scheduler returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result EvaluateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshaling evaluate response: %w", err)
	}

	return &result, nil
}
