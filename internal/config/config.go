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
	RetryInterval  string `envconfig:"RETRY_INTERVAL" default:"30s"`

	// Leader election
	LeaderElection   bool   `envconfig:"LEADER_ELECTION" default:"true"`
	LeaderElectionID string `envconfig:"LEADER_ELECTION_ID" default:"poolboy-scoring"`

	// Server bind addresses
	HealthProbeBindAddress string `envconfig:"HEALTH_PROBE_BIND_ADDRESS" default:":8081"`
	MetricsBindAddress     string `envconfig:"METRICS_BIND_ADDRESS" default:":9090"`
	MetricsUsername        string `envconfig:"METRICS_USERNAME" default:"metrics"`
	MetricsPassword        string `envconfig:"METRICS_PASSWORD" required:"true"`

	// Operation mode
	DryRun bool `envconfig:"DRY_RUN" default:"false"`

	// Logging
	Debug bool `envconfig:"DEBUG" default:"false"`

	// Parsed durations (populated by Load, not from env vars)
	resyncInterval time.Duration
	scoreTimeout   time.Duration
	retryInterval  time.Duration
}

// Load reads configuration from environment variables.
// Returns an error if a required field is missing, a type conversion fails,
// or a duration field has an invalid format.
func Load() (*Config, error) {
	var cfg Config
	var err error
	if err = envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	cfg.resyncInterval, err = time.ParseDuration(cfg.ResyncInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid RESYNC_INTERVAL %q: %w", cfg.ResyncInterval, err)
	}
	cfg.scoreTimeout, err = time.ParseDuration(cfg.ScoreTimeout)
	if err != nil {
		return nil, fmt.Errorf("invalid SCORE_TIMEOUT %q: %w", cfg.ScoreTimeout, err)
	}
	cfg.retryInterval, err = time.ParseDuration(cfg.RetryInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid RETRY_INTERVAL %q: %w", cfg.RetryInterval, err)
	}
	return &cfg, nil
}

// NewForTest creates a Config with the given retryInterval string, suitable for
// use in tests outside the config package. Panics on invalid duration.
func NewForTest(retryInterval string) (*Config, error) {
	d, err := time.ParseDuration(retryInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid retry interval %q: %w", retryInterval, err)
	}
	return &Config{
		RetryInterval: retryInterval,
		retryInterval: d,
	}, nil
}

// ScoreTimeoutDuration returns the parsed ScoreTimeout duration.
func (c *Config) ScoreTimeoutDuration() time.Duration { return c.scoreTimeout }

// ResyncIntervalDuration returns the parsed ResyncInterval duration.
func (c *Config) ResyncIntervalDuration() time.Duration { return c.resyncInterval }

// RetryIntervalDuration returns the parsed RetryInterval duration.
func (c *Config) RetryIntervalDuration() time.Duration { return c.retryInterval }
