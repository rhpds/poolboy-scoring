package config

import (
	"os"
	"testing"
	"time"
)

// setRequiredEnv sets the minimum environment variables needed for Load() to succeed.
// Returns a cleanup function that restores the original environment.
//
// In Go, t.Setenv() automatically restores the variable when the test ends,
// but we need a helper because multiple tests share the same required set.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CLUSTER_SCHEDULER_URL", "http://scheduler:8080")
	t.Setenv("CLUSTER_SCHEDULER_API_KEY", "test-api-key")
	t.Setenv("METRICS_PASSWORD", "test-password")
}

func TestLoad_AllDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	// Verify required fields
	if cfg.ClusterSchedulerURL != "http://scheduler:8080" {
		t.Errorf("ClusterSchedulerURL = %q, want %q", cfg.ClusterSchedulerURL, "http://scheduler:8080")
	}
	if cfg.ClusterSchedulerAPIKey != "test-api-key" {
		t.Errorf("ClusterSchedulerAPIKey = %q, want %q", cfg.ClusterSchedulerAPIKey, "test-api-key")
	}
	if cfg.MetricsPassword != "test-password" {
		t.Errorf("MetricsPassword = %q, want %q", cfg.MetricsPassword, "test-password")
	}

	// Verify defaults
	if cfg.ClusterDomain != "babydev.dev.open.redhat.com" {
		t.Errorf("ClusterDomain = %q, want %q", cfg.ClusterDomain, "babydev.dev.open.redhat.com")
	}
	if cfg.ResyncInterval != "5m" {
		t.Errorf("ResyncInterval = %q, want %q", cfg.ResyncInterval, "5m")
	}
	if cfg.ScoreTimeout != "5s" {
		t.Errorf("ScoreTimeout = %q, want %q", cfg.ScoreTimeout, "5s")
	}
	if cfg.RetryInterval != "30s" {
		t.Errorf("RetryInterval = %q, want %q", cfg.RetryInterval, "30s")
	}
	if cfg.LeaderElection != true {
		t.Errorf("LeaderElection = %v, want true", cfg.LeaderElection)
	}
	if cfg.LeaderElectionID != "poolboy-scoring" {
		t.Errorf("LeaderElectionID = %q, want %q", cfg.LeaderElectionID, "poolboy-scoring")
	}
	if cfg.MetricsBindAddress != ":8080" {
		t.Errorf("MetricsBindAddress = %q, want %q", cfg.MetricsBindAddress, ":8080")
	}
	if cfg.MetricsUsername != "metrics" {
		t.Errorf("MetricsUsername = %q, want %q", cfg.MetricsUsername, "metrics")
	}
	if cfg.Debug != false {
		t.Errorf("Debug = %v, want false", cfg.Debug)
	}
}

// TestLoad_MissingRequired uses a table-driven pattern.
// Each test case removes one required variable and expects Load() to fail.
//
// Table-driven tests are a Go convention: define a slice of test cases,
// then loop over them calling t.Run() for each. This gives clear names
// in test output and makes it easy to add new cases.
func TestLoad_MissingRequired(t *testing.T) {
	requiredVars := []struct {
		name   string
		envVar string
	}{
		{"missing CLUSTER_SCHEDULER_URL", "CLUSTER_SCHEDULER_URL"},
		{"missing CLUSTER_SCHEDULER_API_KEY", "CLUSTER_SCHEDULER_API_KEY"},
		{"missing METRICS_PASSWORD", "METRICS_PASSWORD"},
	}

	for _, tc := range requiredVars {
		t.Run(tc.name, func(t *testing.T) {
			// Set all required vars, then unset the one we're testing
			setRequiredEnv(t)
			os.Unsetenv(tc.envVar)

			_, err := Load()
			if err == nil {
				t.Errorf("Load() should return error when %s is missing", tc.envVar)
			}
		})
	}
}

func TestLoad_CustomValues(t *testing.T) {
	setRequiredEnv(t)

	// Override all defaults
	t.Setenv("CLUSTER_DOMAIN", "prod.example.com")
	t.Setenv("RESYNC_INTERVAL", "10m")
	t.Setenv("SCORE_TIMEOUT", "3s")
	t.Setenv("RETRY_INTERVAL", "15s")
	t.Setenv("LEADER_ELECTION", "false")
	t.Setenv("LEADER_ELECTION_ID", "custom-id")
	t.Setenv("METRICS_BIND_ADDRESS", ":9090")
	t.Setenv("METRICS_USERNAME", "admin")
	t.Setenv("DEBUG", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if cfg.ClusterDomain != "prod.example.com" {
		t.Errorf("ClusterDomain = %q, want %q", cfg.ClusterDomain, "prod.example.com")
	}
	if cfg.ResyncInterval != "10m" {
		t.Errorf("ResyncInterval = %q, want %q", cfg.ResyncInterval, "10m")
	}
	if cfg.ScoreTimeout != "3s" {
		t.Errorf("ScoreTimeout = %q, want %q", cfg.ScoreTimeout, "3s")
	}
	if cfg.RetryInterval != "15s" {
		t.Errorf("RetryInterval = %q, want %q", cfg.RetryInterval, "15s")
	}
	if cfg.LeaderElection != false {
		t.Errorf("LeaderElection = %v, want false", cfg.LeaderElection)
	}
	if cfg.LeaderElectionID != "custom-id" {
		t.Errorf("LeaderElectionID = %q, want %q", cfg.LeaderElectionID, "custom-id")
	}
	if cfg.MetricsBindAddress != ":9090" {
		t.Errorf("MetricsBindAddress = %q, want %q", cfg.MetricsBindAddress, ":9090")
	}
	if cfg.MetricsUsername != "admin" {
		t.Errorf("MetricsUsername = %q, want %q", cfg.MetricsUsername, "admin")
	}
	if cfg.Debug != true {
		t.Errorf("Debug = %v, want true", cfg.Debug)
	}
}

func TestScoreTimeoutDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"valid 5s", "5s", 5 * time.Second},
		{"valid 10s", "10s", 10 * time.Second},
		{"valid 500ms", "500ms", 500 * time.Millisecond},
		{"invalid falls back to 5s", "not-a-duration", 5 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{ScoreTimeout: tc.input}
			got := cfg.ScoreTimeoutDuration()
			if got != tc.expected {
				t.Errorf("ScoreTimeoutDuration() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestRetryIntervalDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"valid 30s", "30s", 30 * time.Second},
		{"valid 15s", "15s", 15 * time.Second},
		{"valid 1m", "1m", 1 * time.Minute},
		{"invalid falls back to 30s", "not-a-duration", 30 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{RetryInterval: tc.input}
			got := cfg.RetryIntervalDuration()
			if got != tc.expected {
				t.Errorf("RetryIntervalDuration() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestResyncIntervalDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"valid 5m", "5m", 5 * time.Minute},
		{"valid 10m", "10m", 10 * time.Minute},
		{"valid 30s", "30s", 30 * time.Second},
		{"invalid falls back to 5m", "not-a-duration", 5 * time.Minute},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{ResyncInterval: tc.input}
			got := cfg.ResyncIntervalDuration()
			if got != tc.expected {
				t.Errorf("ResyncIntervalDuration() = %v, want %v", got, tc.expected)
			}
		})
	}
}
