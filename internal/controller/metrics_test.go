package controller

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestReconcileTotalRegistered(t *testing.T) {
	ReconcileTotal.WithLabelValues("test.local", "success").Inc()

	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	if !findMetricFamily(families, "poolboy_scoring_reconcile_total") {
		t.Error("poolboy_scoring_reconcile_total not found in registry")
	}
}

func TestReconcileTotalIncrement(t *testing.T) {
	counter, err := ReconcileTotal.GetMetricWithLabelValues("test.local", "error")
	if err != nil {
		t.Fatalf("failed to get metric: %v", err)
	}

	before := readCounter(t, counter)

	ReconcileTotal.WithLabelValues("test.local", "error").Inc()

	after := readCounter(t, counter)
	if after != before+1 {
		t.Errorf("counter = %v, want %v", after, before+1)
	}
}

func TestScorePatchesTotalRegistered(t *testing.T) {
	ScorePatchesTotal.WithLabelValues("test.local", "ocpv05").Inc()

	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	if !findMetricFamily(families, "poolboy_scoring_score_patches_total") {
		t.Error("poolboy_scoring_score_patches_total not found in registry")
	}
}

func TestSchedulerDurationRegistered(t *testing.T) {
	SchedulerDuration.WithLabelValues("test.local").Observe(0.1)

	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	if !findMetricFamily(families, "poolboy_scoring_scheduler_duration_seconds") {
		t.Error("poolboy_scoring_scheduler_duration_seconds not found in registry")
	}
}

func TestSchedulerDurationObserve(t *testing.T) {
	ObserveSchedulerDuration(time.Now().Add(-100*time.Millisecond), "observe-test.local")

	observer, err := SchedulerDuration.GetMetricWithLabelValues("observe-test.local")
	if err != nil {
		t.Fatalf("failed to get metric: %v", err)
	}

	metric := &dto.Metric{}
	if err := observer.(prometheus.Metric).Write(metric); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}

	if metric.Histogram == nil {
		t.Fatal("expected histogram metric")
	}
	if metric.Histogram.GetSampleCount() == 0 {
		t.Error("expected at least one observation")
	}
	if metric.Histogram.GetSampleSum() <= 0 {
		t.Error("expected positive sample sum")
	}
}

func TestSchedulerConsecutiveFailuresRegistered(t *testing.T) {
	SchedulerConsecutiveFailures.WithLabelValues("test.local").Set(3)

	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	if !findMetricFamily(families, "poolboy_scoring_scheduler_consecutive_failures") {
		t.Error("poolboy_scoring_scheduler_consecutive_failures not found in registry")
	}
}

func TestHandlesScoredRegistered(t *testing.T) {
	HandlesScored.WithLabelValues("test.local", "test-pool").Set(42)

	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	if !findMetricFamily(families, "poolboy_scoring_handles_scored") {
		t.Error("poolboy_scoring_handles_scored not found in registry")
	}

	gauge, err := HandlesScored.GetMetricWithLabelValues("test.local", "test-pool")
	if err != nil {
		t.Fatalf("failed to get metric: %v", err)
	}

	metric := &dto.Metric{}
	if err := gauge.Write(metric); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	if metric.Gauge == nil {
		t.Fatal("expected gauge metric")
	}
	if metric.Gauge.GetValue() != 42 {
		t.Errorf("gauge = %v, want 42", metric.Gauge.GetValue())
	}
}

// --- helpers ---

func findMetricFamily(families []*dto.MetricFamily, name string) bool {
	for _, f := range families {
		if f.GetName() == name {
			return true
		}
	}
	return false
}

func readCounter(t *testing.T, counter prometheus.Counter) float64 {
	t.Helper()
	metric := &dto.Metric{}
	if err := counter.Write(metric); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	return metric.Counter.GetValue()
}
