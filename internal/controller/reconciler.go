package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/rhpds/poolboy-scoring/internal/config"
	"github.com/rhpds/poolboy-scoring/internal/placement"
	"github.com/rhpds/poolboy-scoring/internal/scheduler"
)

// PlacementResolver resolves cluster placements for a ResourceHandle.
type PlacementResolver interface {
	Lookup(ctx context.Context, handle *unstructured.Unstructured) ([]placement.Placement, error)
}

// handleWithCluster pairs a resolved handle with its cluster name.
type handleWithCluster struct {
	handle      *unstructured.Unstructured
	clusterName string
}

// ResourcePoolReconciler watches ResourcePool objects, collects unbound
// healthy handles, resolves their cluster placements, sends one batch
// /evaluate call per pool, and patches each handle's spec.preferenceScore.
type ResourcePoolReconciler struct {
	client.Client
	Scorer            scheduler.Scorer
	Resolver          PlacementResolver
	LastWrittenScores sync.Map
	Config            *config.Config
}

// Reconcile processes a single ResourcePool.
func (r *ResourcePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	domain := r.Config.ClusterDomain
	result := "success"
	defer func() { ReconcileTotal.WithLabelValues(domain, result).Inc() }()

	var pool unstructured.Unstructured
	pool.SetGroupVersionKind(placement.ResourcePoolGVK)
	if err := r.Get(ctx, req.NamespacedName, &pool); err != nil {
		if client.IgnoreNotFound(err) == nil {
			result = "skipped"
			return ctrl.Result{}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}

	poolStatus, err := placement.ParsePoolStatus(&pool)
	if err != nil {
		result = "error"
		return ctrl.Result{}, fmt.Errorf("parsing pool status: %w", err)
	}

	if poolStatus != nil && poolStatus.ResourceHandleCount != nil &&
		poolStatus.ResourceHandleCount.Available == 0 {
		result = "skipped"
		log.V(1).Info("No available handles, skipping",
			"pool", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, nil
	}

	if poolStatus == nil || len(poolStatus.ResourceHandles) == 0 {
		result = "skipped"
		log.V(1).Info("No handle entries in pool status, skipping",
			"pool", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, nil
	}

	var resolved []handleWithCluster
	var placementFailed int

	for _, entry := range poolStatus.ResourceHandles {
		if entry.Healthy != nil && !*entry.Healthy {
			log.Info("Skipping unhealthy handle",
				"pool", req.Name, "handle", entry.Name)
			continue
		}

		var handle unstructured.Unstructured
		handle.SetGroupVersionKind(placement.ResourceHandleGVK)
		if err := r.Get(ctx, types.NamespacedName{
			Name:      entry.Name,
			Namespace: req.Namespace,
		}, &handle); err != nil {
			if client.IgnoreNotFound(err) == nil {
				log.V(1).Info("Handle not found, skipping",
					"pool", req.Name, "handle", entry.Name)
				continue
			}
			result = "error"
			return ctrl.Result{}, fmt.Errorf("getting handle %s: %w", entry.Name, err)
		}

		handleSpec, err := placement.ParseHandleSpec(&handle)
		if err != nil {
			result = "error"
			return ctrl.Result{}, fmt.Errorf("parsing handle spec %s: %w", entry.Name, err)
		}
		if handleSpec.ResourceClaim != nil {
			continue
		}

		placements, err := r.Resolver.Lookup(ctx, &handle)
		if err != nil {
			log.Info("Failed to resolve placement, will retry",
				"pool", req.Name, "handle", entry.Name, "error", err.Error())
			placementFailed++
			continue
		}
		if len(placements) == 0 {
			log.V(1).Info("No placements resolved, skipping handle",
				"pool", req.Name, "handle", entry.Name)
			continue
		}

		resolved = append(resolved, handleWithCluster{
			handle:      handle.DeepCopy(),
			clusterName: placements[0].ClusterName,
		})
	}

	if len(resolved) == 0 {
		result = "skipped"
		if placementFailed > 0 {
			return ctrl.Result{RequeueAfter: r.Config.RetryIntervalDuration()}, nil
		}
		return ctrl.Result{}, nil
	}

	uniqueClusters := deduplicateClusters(resolved)
	candidates := make([]scheduler.Candidate, len(uniqueClusters))
	for i, c := range uniqueClusters {
		candidates[i] = scheduler.Candidate{ClusterName: c}
	}

	log.V(1).Info("Calling /evaluate",
		"pool", req.Name, "namespace", req.Namespace,
		"clusters", uniqueClusters)

	evalStart := time.Now()
	resp, err := r.Scorer.Evaluate(ctx, candidates)
	ObserveSchedulerDuration(evalStart, domain)
	if err != nil {
		result = "error"
		log.Info("Scheduler evaluation failed, keeping existing scores",
			"pool", req.Name, "namespace", req.Namespace, "error", err.Error())
		return ctrl.Result{RequeueAfter: r.Config.RetryIntervalDuration()}, nil
	}

	respJSON, _ := json.Marshal(resp)
	log.V(1).Info("Evaluate response",
		"pool", req.Name, "namespace", req.Namespace,
		"candidates", len(candidates), "ranked", len(resp.Ranked),
		"excluded", len(resp.Excluded), "strategy", resp.Strategy,
		"response", json.RawMessage(respJSON),
	)

	scoreMap := buildScoreMap(resp)
	var scored int

	for _, hwc := range resolved {
		newScore := scoreMap[hwc.clusterName]

		handleSpec, err := placement.ParseHandleSpec(hwc.handle)
		if err != nil {
			result = "error"
			return ctrl.Result{}, fmt.Errorf("parsing handle spec %s: %w", hwc.handle.GetName(), err)
		}
		currentScore := float64(0)
		if handleSpec.PreferenceScore != nil {
			currentScore = *handleSpec.PreferenceScore
		}
		if newScore == currentScore {
			continue
		}

		if r.Config.DryRun {
			log.Info("[DRY-RUN] Would update preference score",
				"pool", req.Name, "handle", hwc.handle.GetName(),
				"cluster", hwc.clusterName,
				"oldPreferenceScore", currentScore,
				"newPreferenceScore", newScore,
			)
			scored++
			continue
		}

		handleStatus, _ := placement.ParseHandleStatus(hwc.handle)
		if handleStatus == nil || len(handleStatus.Placements) == 0 {
			p := []placement.Placement{{ClusterName: hwc.clusterName}}
			if err := r.patchStatusPlacements(ctx, hwc.handle, p); err != nil {
				if apierrors.IsConflict(err) {
					log.Info("Conflict caching placements, will retry",
						"pool", req.Name, "handle", hwc.handle.GetName())
					return ctrl.Result{Requeue: true}, nil
				}
				result = "error"
				return ctrl.Result{}, fmt.Errorf("caching placements for %s: %w", hwc.handle.GetName(), err)
			}
		}

		if err := r.patchPreferenceScore(ctx, hwc.handle, newScore); err != nil {
			if apierrors.IsConflict(err) {
				log.Info("Conflict patching score, will retry",
					"pool", req.Name, "handle", hwc.handle.GetName())
				return ctrl.Result{Requeue: true}, nil
			}
			result = "error"
			return ctrl.Result{}, fmt.Errorf("patching score for %s: %w", hwc.handle.GetName(), err)
		}

		ScorePatchesTotal.WithLabelValues(domain, hwc.clusterName).Inc()
		key := hwc.handle.GetNamespace() + "/" + hwc.handle.GetName()
		r.LastWrittenScores.Store(key, newScore)
		scored++

		log.Info("Updated preference score",
			"pool", req.Name, "handle", hwc.handle.GetName(),
			"cluster", hwc.clusterName,
			"oldPreferenceScore", currentScore,
			"newPreferenceScore", newScore,
		)
	}

	HandlesScored.WithLabelValues(domain).Set(float64(scored))

	log.Info("Pool reconciliation complete",
		"pool", req.Name, "namespace", req.Namespace,
		"handlesScored", scored,
		"handlesResolved", len(resolved),
		"placementFailed", placementFailed,
		"uniqueClusters", len(uniqueClusters),
	)

	if placementFailed > 0 {
		return ctrl.Result{RequeueAfter: r.Config.RetryIntervalDuration()}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler to watch ResourcePool objects.
func (r *ResourcePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(placement.ResourcePoolGVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(obj).
		Named("resourcepool-scoring").
		Complete(r)
}

// patchPreferenceScore applies a JSON merge patch to spec.preferenceScore.
func (r *ResourcePoolReconciler) patchPreferenceScore(ctx context.Context, handle *unstructured.Unstructured, score float64) error {
	scoreStr := strconv.FormatFloat(score, 'f', -1, 64)
	patch := []byte(fmt.Sprintf(`{"spec":{"preferenceScore":%s}}`, scoreStr))
	return r.Patch(ctx, handle, client.RawPatch(types.MergePatchType, patch))
}

// patchStatusPlacements caches resolved placements in status.placements.
func (r *ResourcePoolReconciler) patchStatusPlacements(ctx context.Context, handle *unstructured.Unstructured, placements []placement.Placement) error {
	placementsJSON, err := json.Marshal(placements)
	if err != nil {
		return fmt.Errorf("marshaling placements: %w", err)
	}

	patch := []byte(fmt.Sprintf(`{"status":{"placements":%s}}`, placementsJSON))
	return r.Status().Patch(ctx, handle, client.RawPatch(types.MergePatchType, patch))
}

// deduplicateClusters returns unique cluster names from the resolved handles,
// preserving the order of first appearance.
func deduplicateClusters(handles []handleWithCluster) []string {
	seen := make(map[string]bool)
	var clusters []string
	for _, hwc := range handles {
		if !seen[hwc.clusterName] {
			seen[hwc.clusterName] = true
			clusters = append(clusters, hwc.clusterName)
		}
	}
	return clusters
}

// buildScoreMap creates a cluster→score lookup from the evaluate response.
func buildScoreMap(resp *scheduler.EvaluateResponse) map[string]float64 {
	m := make(map[string]float64, len(resp.Ranked))
	for _, sc := range resp.Ranked {
		m[sc.ClusterName] = sc.Score
	}
	return m
}
