package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEvaluate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(EvaluateResponse{
			Ranked: []ScoredCandidate{
				{ClusterName: "ocpv06", Score: 82.5, Eligible: true},
				{ClusterName: "ocpv05", Score: 65.3, Eligible: true},
			},
			Excluded:    []ScoredCandidate{},
			Strategy:    "most_capacity",
			GeneratedAt: time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", 5*time.Second)
	resp, err := client.Evaluate(context.Background(), []Candidate{
		{ClusterName: "ocpv05"},
		{ClusterName: "ocpv06"},
	})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if len(resp.Ranked) != 2 {
		t.Fatalf("len(Ranked) = %d, want 2", len(resp.Ranked))
	}
	if resp.Ranked[0].ClusterName != "ocpv06" {
		t.Errorf("Ranked[0].ClusterName = %q, want %q", resp.Ranked[0].ClusterName, "ocpv06")
	}
	if resp.Ranked[0].Score != 82.5 {
		t.Errorf("Ranked[0].Score = %v, want 82.5", resp.Ranked[0].Score)
	}
	if resp.Strategy != "most_capacity" {
		t.Errorf("Strategy = %q, want %q", resp.Strategy, "most_capacity")
	}
}

func TestEvaluate_EmptyRanked(t *testing.T) {
	reason := "cluster in maintenance"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(EvaluateResponse{
			Ranked: []ScoredCandidate{},
			Excluded: []ScoredCandidate{
				{ClusterName: "ocpv05", Score: 0, Eligible: false, IneligibilityReason: &reason},
			},
			Strategy:    "most_capacity",
			GeneratedAt: time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", 5*time.Second)
	resp, err := client.Evaluate(context.Background(), []Candidate{{ClusterName: "ocpv05"}})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if len(resp.Ranked) != 0 {
		t.Errorf("len(Ranked) = %d, want 0", len(resp.Ranked))
	}
	if len(resp.Excluded) != 1 {
		t.Fatalf("len(Excluded) = %d, want 1", len(resp.Excluded))
	}
	if resp.Excluded[0].IneligibilityReason == nil || *resp.Excluded[0].IneligibilityReason != reason {
		t.Error("Excluded[0].IneligibilityReason should be 'cluster in maintenance'")
	}
}

func TestEvaluate_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "bad-key", 5*time.Second)
	_, err := client.Evaluate(context.Background(), []Candidate{{ClusterName: "ocpv05"}})
	if err == nil {
		t.Fatal("Evaluate() should return error on 401")
	}
	if got := err.Error(); got != "cluster-scheduler returned 401 Unauthorized: check API key" {
		t.Errorf("error = %q, want 401 message", got)
	}
}

func TestEvaluate_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", 5*time.Second)
	_, err := client.Evaluate(context.Background(), []Candidate{{ClusterName: "ocpv05"}})
	if err == nil {
		t.Fatal("Evaluate() should return error on 500")
	}
}

func TestEvaluate_InvalidResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{not valid json`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", 5*time.Second)
	_, err := client.Evaluate(context.Background(), []Candidate{{ClusterName: "ocpv05"}})
	if err == nil {
		t.Fatal("Evaluate() should return error on invalid JSON")
	}
}

func TestEvaluate_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the context deadline
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", 5*time.Second)

	// Create a context with a short timeout — shorter than the server delay.
	// context.WithTimeout returns a new context and a cancel function.
	// The cancel function MUST be called to release resources (the timer).
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Evaluate(ctx, []Candidate{{ClusterName: "ocpv05"}})
	if err == nil {
		t.Fatal("Evaluate() should return error on timeout")
	}
}

func TestEvaluate_ConnectionRefused(t *testing.T) {
	// Point to a port that nothing listens on
	client := NewClient("http://127.0.0.1:1", "test-key", 1*time.Second)
	_, err := client.Evaluate(context.Background(), []Candidate{{ClusterName: "ocpv05"}})
	if err == nil {
		t.Fatal("Evaluate() should return error when server is unreachable")
	}
}

func TestEvaluate_InvalidURL(t *testing.T) {
	// A URL with a control character makes http.NewRequestWithContext fail.
	client := NewClient("http://\x00invalid", "test-key", 5*time.Second)
	_, err := client.Evaluate(context.Background(), []Candidate{{ClusterName: "ocpv05"}})
	if err == nil {
		t.Fatal("Evaluate() should return error on invalid URL")
	}
}

func TestEvaluate_TruncatedBody(t *testing.T) {
	// Server sends Content-Length header but closes the connection before
	// writing the full body. This triggers an io.ReadAll error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10000")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ranked":`))
		// Connection closes here — body is incomplete vs Content-Length
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", 5*time.Second)
	_, err := client.Evaluate(context.Background(), []Candidate{{ClusterName: "ocpv05"}})
	if err == nil {
		t.Fatal("Evaluate() should return error on truncated body")
	}
}

func TestEvaluate_APIKeyHeader(t *testing.T) {
	var receivedKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(EvaluateResponse{
			Ranked:      []ScoredCandidate{},
			Excluded:    []ScoredCandidate{},
			Strategy:    "most_capacity",
			GeneratedAt: time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "my-secret-key", 5*time.Second)
	_, err := client.Evaluate(context.Background(), []Candidate{{ClusterName: "ocpv05"}})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if receivedKey != "my-secret-key" {
		t.Errorf("X-API-Key = %q, want %q", receivedKey, "my-secret-key")
	}
}
