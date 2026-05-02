package main

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/rhpds/poolboy-scoring/internal/config"
	"github.com/rhpds/poolboy-scoring/internal/controller"
	appmetrics "github.com/rhpds/poolboy-scoring/internal/metrics"
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

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		ctrl.Log.WithName("setup").Error(err, "Failed to load kubeconfig")
		os.Exit(1)
	}

	if err := run(ctrl.SetupSignalHandler(), cfg, restCfg); err != nil {
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
		"dryRun", cfg.DryRun,
		"debug", cfg.Debug,
	)

	resyncInterval := cfg.ResyncIntervalDuration()
	scheme := runtime.NewScheme()
	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: cfg.HealthProbeBindAddress,
		LeaderElection:         cfg.LeaderElection,
		LeaderElectionID:       cfg.LeaderElectionID,
		Cache: cache.Options{
			SyncPeriod: &resyncInterval,
		},
		Metrics: metricsserver.Options{
			BindAddress:    cfg.MetricsBindAddress,
			FilterProvider: appmetrics.BasicAuthFilterProvider(cfg.MetricsUsername, cfg.MetricsPassword),
		},
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

	reconciler := &controller.ResourcePoolReconciler{
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
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager exited: %w", err)
	}

	log.Info("Manager stopped cleanly")
	return nil
}
