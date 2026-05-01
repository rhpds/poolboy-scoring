package main

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/rhpds/poolboy-scoring/internal/config"
	"github.com/rhpds/poolboy-scoring/internal/controller"
	"github.com/rhpds/poolboy-scoring/internal/placement"
	"github.com/rhpds/poolboy-scoring/internal/scheduler"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	opts := zap.Options{Development: cfg.Debug}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if err := run(ctrl.SetupSignalHandler(), cfg, ctrl.GetConfigOrDie()); err != nil {
		ctrl.Log.WithName("setup").Error(err, "Controller exited with error")
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg *config.Config, restCfg *rest.Config) error {
	log := ctrl.Log.WithName("setup")

	log.Info("Starting poolboy-scoring",
		"clusterDomain", cfg.ClusterDomain,
		"schedulerURL", cfg.ClusterSchedulerURL,
		"resyncInterval", cfg.ResyncInterval,
		"scoreTimeout", cfg.ScoreTimeout,
		"retryInterval", cfg.RetryInterval,
		"leaderElection", cfg.LeaderElection,
		"metricsBindAddress", cfg.MetricsBindAddress,
		"debug", cfg.Debug,
	)

	scheme := runtime.NewScheme()
	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: cfg.HealthProbeBindAddress,
		LeaderElection:         cfg.LeaderElection,
		LeaderElectionID:       cfg.LeaderElectionID,
	})
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	scorer := scheduler.NewClient(
		cfg.ClusterSchedulerURL,
		cfg.ClusterSchedulerAPIKey,
		cfg.ScoreTimeoutDuration(),
	)

	resolver := placement.NewLookup(mgr.GetClient())

	reconciler := &controller.Reconciler{
		Client:   mgr.GetClient(),
		Scorer:   scorer,
		Resolver: resolver,
		Config:   cfg,
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("setting up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("setting up ready check: %w", err)
	}

	log.Info("Starting manager")
	return mgr.Start(ctx)
}
