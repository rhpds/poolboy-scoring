package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all operator configuration, loaded from environment variables.
//
// Each field has an `envconfig` struct tag that maps it to an env var name.
// The `required` tag makes Load() return an error if the var is missing.
// The `default` tag provides a fallback value when the var is not set.
//
// Example: CLUSTER_SCHEDULER_URL=http://scheduler:8080 maps to Config.ClusterSchedulerURL
type Config struct {
	// Cluster Scheduler connection
	ClusterSchedulerURL    string `envconfig:"CLUSTER_SCHEDULER_URL" required:"true"`
	ClusterSchedulerAPIKey string `envconfig:"CLUSTER_SCHEDULER_API_KEY" required:"true"`

	// Cluster identity (used as Prometheus metric label)
	ClusterDomain string `envconfig:"CLUSTER_DOMAIN" default:"babydev.dev.open.redhat.com"`

	// Timing
	ResyncInterval string `envconfig:"RESYNC_INTERVAL" default:"5m"`
	ScoreTimeout   string `envconfig:"SCORE_TIMEOUT" default:"5s"`

	// Leader election
	LeaderElection   bool   `envconfig:"LEADER_ELECTION" default:"true"`
	LeaderElectionID string `envconfig:"LEADER_ELECTION_ID" default:"poolboy-scoring"`

	// Metrics server
	MetricsBindAddress string `envconfig:"METRICS_BIND_ADDRESS" default:":8080"`
	MetricsUsername    string `envconfig:"METRICS_USERNAME" default:"metrics"`
	MetricsPassword    string `envconfig:"METRICS_PASSWORD" required:"true"`

	// Logging
	Debug bool `envconfig:"DEBUG" default:"false"`
}

// Load reads configuration from environment variables.
// Returns an error if a required field is missing or a type conversion fails.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return &cfg, nil
}

// ScoreTimeoutDuration parses the ScoreTimeout string (e.g. "5s") into a time.Duration.
func (c *Config) ScoreTimeoutDuration() time.Duration {
	d, err := time.ParseDuration(c.ScoreTimeout)
	if err != nil {
		return 5 * time.Second
	}
	return d
}

// ResyncIntervalDuration parses the ResyncInterval string (e.g. "5m") into a time.Duration.
func (c *Config) ResyncIntervalDuration() time.Duration {
	d, err := time.ParseDuration(c.ResyncInterval)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}
