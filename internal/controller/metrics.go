package controller

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ReconcileTotal counts pool reconciliations by outcome.
var ReconcileTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "poolboy_scoring_reconcile_total",
		Help: "Total number of reconciliations by result",
	},
	[]string{"cluster_domain", "result"},
)

// ScorePatchesTotal counts score patches applied to ResourceHandles.
var ScorePatchesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "poolboy_scoring_score_patches_total",
		Help: "Total number of preferenceScore patches applied",
	},
	[]string{"cluster_domain", "cluster"},
)

// SchedulerDuration measures time spent calling the cluster-scheduler
// /evaluate endpoint.
var SchedulerDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "poolboy_scoring_scheduler_duration_seconds",
		Help:    "Time spent calling cluster-scheduler /evaluate",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"cluster_domain"},
)

// HandlesScored reports how many ResourceHandles were scored in the
// most recent reconciliation cycle across all pools.
var HandlesScored = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "poolboy_scoring_handles_scored",
		Help: "Number of ResourceHandles scored in the last reconciliation",
	},
	[]string{"cluster_domain"},
)

func init() {
	metrics.Registry.MustRegister(
		ReconcileTotal,
		ScorePatchesTotal,
		SchedulerDuration,
		HandlesScored,
	)
}

// ObserveSchedulerDuration records the duration of a scheduler /evaluate call.
func ObserveSchedulerDuration(start time.Time, clusterDomain string) {
	SchedulerDuration.WithLabelValues(clusterDomain).Observe(time.Since(start).Seconds())
}
