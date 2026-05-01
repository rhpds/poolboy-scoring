package main

import (
	"fmt"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/rhpds/poolboy-scoring/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize structured logging via controller-runtime's zap integration.
	// Development mode (cfg.Debug=true) gives human-readable output with caller info.
	// Production mode (cfg.Debug=false) gives JSON output optimized for log aggregators.
	opts := zap.Options{Development: cfg.Debug}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	log := ctrl.Log.WithName("setup")

	log.Info("Starting poolboy-scoring",
		"clusterDomain", cfg.ClusterDomain,
		"schedulerURL", cfg.ClusterSchedulerURL,
		"resyncInterval", cfg.ResyncInterval,
		"scoreTimeout", cfg.ScoreTimeout,
		"leaderElection", cfg.LeaderElection,
		"metricsBindAddress", cfg.MetricsBindAddress,
		"debug", cfg.Debug,
	)

	// Phase 3B will replace this with the controller-runtime Manager.
	log.Info("Controller not yet implemented — exiting")
}
