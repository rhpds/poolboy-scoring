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
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/rhpds/poolboy-scoring/internal/config"
	"github.com/rhpds/poolboy-scoring/internal/placement"
	"github.com/rhpds/poolboy-scoring/internal/scheduler"
)

// PlacementResolver resolves cluster placements for a ResourceHandle.
// Defined here (at the consumer) rather than in the placement package —
// this is the Go idiom of "accept interfaces, return structs."
// placement.PlacementLookup satisfies this interface implicitly.
type PlacementResolver interface {
	Lookup(ctx context.Context, handle *unstructured.Unstructured) ([]placement.Placement, error)
}

// Reconciler watches unbound ResourceHandles, resolves their cluster
// placement, calls the cluster-scheduler /evaluate endpoint, and patches
// spec.preferenceScore when the score changes.
type Reconciler struct {
	client.Client
	Scorer            scheduler.Scorer
	Resolver          PlacementResolver
	LastWrittenScores sync.Map
	Config            *config.Config
}

// Reconcile processes a single ResourceHandle.
//
// Flow: get → skip if bound → resolve placement → cache status.placements →
// evaluate → compare score → patch if changed → track in sync.Map.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	domain := r.Config.ClusterDomain
	result := "success"
	defer func() { ReconcileTotal.WithLabelValues(domain, result).Inc() }()

	var handle unstructured.Unstructured
	handle.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := r.Get(ctx, req.NamespacedName, &handle); err != nil {
		if client.IgnoreNotFound(err) == nil {
			result = "skipped"
			return ctrl.Result{}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}

	if placement.IsHandleBound(&handle) {
		result = "skipped"
		return ctrl.Result{}, nil
	}

	placements, err := r.Resolver.Lookup(ctx, &handle)
	if err != nil {
		result = "error"
		log.Info("Failed to resolve placement, will retry",
			"handle", req.Name, "namespace", req.Namespace, "error", err.Error())
		return ctrl.Result{RequeueAfter: r.Config.RetryIntervalDuration()}, nil
	}

	if len(placements) == 0 {
		result = "skipped"
		log.V(1).Info("No placements resolved, skipping",
			"handle", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, nil
	}

	if cached, _ := placement.GetPlacementsFromStatus(&handle); len(cached) == 0 {
		if err := r.patchStatusPlacements(ctx, &handle, placements); err != nil {
			result = "error"
			if apierrors.IsConflict(err) {
				log.Info("Conflict caching placements, will retry",
					"handle", req.Name, "namespace", req.Namespace)
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, fmt.Errorf("caching placements for %s/%s: %w", req.Namespace, req.Name, err)
		}
	}

	candidates := buildCandidates(placements)

	evalStart := time.Now()
	resp, err := r.Scorer.Evaluate(ctx, candidates)
	ObserveSchedulerDuration(evalStart, domain)
	if err != nil {
		result = "error"
		log.Info("Scheduler evaluation failed, keeping existing score",
			"handle", req.Name, "namespace", req.Namespace, "error", err.Error())
		return ctrl.Result{RequeueAfter: r.Config.RetryIntervalDuration()}, nil
	}

	// Use the highest ranked score. Handles with multiple placements on
	// different clusters get the best cluster's score, because Poolboy's
	// sort only has one preferenceScore field per handle.
	newScore := bestScore(resp)

	currentScore, _ := placement.GetCurrentScore(&handle)
	if newScore == currentScore {
		return ctrl.Result{}, nil
	}

	if err := r.patchPreferenceScore(ctx, &handle, newScore); err != nil {
		result = "error"
		if apierrors.IsConflict(err) {
			log.Info("Conflict patching score, will retry",
				"handle", req.Name, "namespace", req.Namespace)
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("patching score for %s/%s: %w", req.Namespace, req.Name, err)
	}

	if len(resp.Ranked) > 0 {
		ScorePatchesTotal.WithLabelValues(domain, resp.Ranked[0].ClusterName).Inc()
	}

	r.LastWrittenScores.Store(req.NamespacedName.String(), newScore)
	r.updateHandlesTracked()

	log.Info("Updated preference score",
		"handle", req.Name, "namespace", req.Namespace,
		"oldScore", currentScore,
		"newScore", newScore,
	)

	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler to watch unstructured
// ResourceHandles, filtered by the bound-handle and self-update predicates.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(placement.ResourceHandleGVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(obj, builder.WithPredicates(
			NewBoundHandlePredicate(),
			NewSelfUpdatePredicate(&r.LastWrittenScores),
		)).
		Named("resourcehandle-scoring").
		Complete(r)
}

// patchPreferenceScore applies a JSON merge patch to spec.preferenceScore.
func (r *Reconciler) patchPreferenceScore(ctx context.Context, handle *unstructured.Unstructured, score float64) error {
	scoreStr := strconv.FormatFloat(score, 'f', -1, 64)
	patch := []byte(fmt.Sprintf(`{"spec":{"preferenceScore":%s}}`, scoreStr))
	return r.Patch(ctx, handle, client.RawPatch(types.MergePatchType, patch))
}

// patchStatusPlacements caches resolved placements in status.placements
// via the /status subresource. This avoids re-fetching AnarchySubjects
// on every reconcile — the placement is resolved once and read from
// the informer cache on subsequent passes.
func (r *Reconciler) patchStatusPlacements(ctx context.Context, handle *unstructured.Unstructured, placements []placement.Placement) error {
	type placementEntry struct {
		ClusterName string `json:"clusterName"`
		Name        string `json:"name,omitempty"`
		Namespace   string `json:"namespace,omitempty"`
	}

	entries := make([]placementEntry, len(placements))
	for i, p := range placements {
		entries[i] = placementEntry{
			ClusterName: p.ClusterName,
			Name:        p.Name,
			Namespace:   p.Namespace,
		}
	}

	placementsJSON, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshaling placements: %w", err)
	}

	patch := []byte(fmt.Sprintf(`{"status":{"placements":%s}}`, placementsJSON))
	return r.Status().Patch(ctx, handle, client.RawPatch(types.MergePatchType, patch))
}

// updateHandlesTracked counts entries in LastWrittenScores and sets the
// handles-tracked gauge. Called after each successful score patch.
func (r *Reconciler) updateHandlesTracked() {
	var count int
	r.LastWrittenScores.Range(func(_, _ any) bool { count++; return true })
	HandlesTracked.WithLabelValues(r.Config.ClusterDomain).Set(float64(count))
}

// buildCandidates converts placements to scheduler candidates.
func buildCandidates(placements []placement.Placement) []scheduler.Candidate {
	candidates := make([]scheduler.Candidate, len(placements))
	for i, p := range placements {
		c := scheduler.Candidate{ClusterName: p.ClusterName}
		if p.Name != "" {
			name := p.Name
			c.HandleName = &name
		}
		if p.Namespace != "" {
			ns := p.Namespace
			c.HandleNamespace = &ns
		}
		candidates[i] = c
	}
	return candidates
}

// bestScore returns the highest score from the evaluate response.
// Ranked is sorted by score descending, so the first entry is the best.
// Returns 0 if all candidates were excluded or the response is empty.
func bestScore(resp *scheduler.EvaluateResponse) float64 {
	if len(resp.Ranked) > 0 {
		return resp.Ranked[0].Score
	}
	return 0
}
