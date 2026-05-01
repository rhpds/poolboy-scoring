package main

import (
	"context"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/rhpds/poolboy-scoring/internal/config"
)

func init() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
}

func testConfig() *config.Config {
	return &config.Config{
		ClusterSchedulerURL:    "http://localhost:9999",
		ClusterSchedulerAPIKey: "test-key",
		ClusterDomain:          "test.example.com",
		ResyncInterval:         "5m",
		ScoreTimeout:           "5s",
		RetryInterval:          "30s",
		LeaderElection:         false,
		LeaderElectionID:       "test-poolboy-scoring",
		HealthProbeBindAddress: "0",
		MetricsBindAddress:     "0",
		MetricsUsername:         "metrics",
		MetricsPassword:        "test",
		Debug:                  true,
	}
}

func TestRun_SetupAndShutdown(t *testing.T) {
	srv := httptest.NewServer(nil)
	defer srv.Close()

	restCfg := &rest.Config{Host: srv.URL}
	cfg := testConfig()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := run(ctx, cfg, restCfg)
	if err != nil {
		t.Fatalf("run() returned unexpected error: %v", err)
	}
}

func TestRun_NilRestConfig(t *testing.T) {
	cfg := testConfig()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := run(ctx, cfg, nil)
	if err == nil {
		t.Fatal("run() should return error with nil rest config")
	}
}
