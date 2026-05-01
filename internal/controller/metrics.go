package controller

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ReconcileTotal counts reconciliations by outcome.
//
// Labels:
//   - cluster_domain: Babylon cluster FQDN (static, from config)
//   - result: "success", "error", or "skipped"
var ReconcileTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "poolboy_scoring_reconcile_total",
		Help: "Total number of reconciliations by result",
	},
	[]string{"cluster_domain", "result"},
)

// ScorePatchesTotal counts score patches applied to ResourceHandles.
//
// Labels:
//   - cluster_domain: Babylon cluster FQDN (static, from config)
//   - cluster: target ocpvXX cluster name (bounded, ~7 values)
var ScorePatchesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "poolboy_scoring_score_patches_total",
		Help: "Total number of preferenceScore patches applied",
	},
	[]string{"cluster_domain", "cluster"},
)

// SchedulerDuration measures time spent calling the cluster-scheduler
// /evaluate endpoint. Uses default Prometheus buckets which cover the
// range from 5ms to 10s — appropriate for HTTP calls with a 5s timeout.
//
// Labels:
//   - cluster_domain: Babylon cluster FQDN (static, from config)
var SchedulerDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "poolboy_scoring_scheduler_duration_seconds",
		Help:    "Time spent calling cluster-scheduler /evaluate",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"cluster_domain"},
)

// HandlesTracked reports how many ResourceHandles have a tracked score
// in the reconciler's LastWrittenScores map. This represents handles that
// have been scored at least once since the controller started.
//
// Labels:
//   - cluster_domain: Babylon cluster FQDN (static, from config)
var HandlesTracked = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "poolboy_scoring_handles_tracked",
		Help: "Number of ResourceHandles with a tracked score",
	},
	[]string{"cluster_domain"},
)

func init() {
	metrics.Registry.MustRegister(
		ReconcileTotal,
		ScorePatchesTotal,
		SchedulerDuration,
		HandlesTracked,
	)
}

// ObserveSchedulerDuration records the duration of a scheduler /evaluate call.
func ObserveSchedulerDuration(start time.Time, clusterDomain string) {
	SchedulerDuration.WithLabelValues(clusterDomain).Observe(time.Since(start).Seconds())
}
