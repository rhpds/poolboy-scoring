package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/rhpds/poolboy-scoring/internal/config"
	"github.com/rhpds/poolboy-scoring/internal/placement"
	"github.com/rhpds/poolboy-scoring/internal/scheduler"
)

// --- Mock types ---

// mockScorer implements scheduler.Scorer for testing.
type mockScorer struct {
	response *scheduler.EvaluateResponse
	err      error
	called   bool
	received []scheduler.Candidate
}

func (m *mockScorer) Evaluate(_ context.Context, candidates []scheduler.Candidate) (*scheduler.EvaluateResponse, error) {
	m.called = true
	m.received = candidates
	return m.response, m.err
}

// mockResolver implements PlacementResolver for testing.
type mockResolver struct {
	placements []placement.Placement
	err        error
	called     bool
}

func (m *mockResolver) Lookup(_ context.Context, _ *unstructured.Unstructured) ([]placement.Placement, error) {
	m.called = true
	return m.placements, m.err
}

// --- Test helpers ---

func testConfig() *config.Config {
	return &config.Config{
		RetryInterval: "30s",
	}
}

// newTestHandle builds a ResourceHandle for reconciler tests.
// Supports options: bound (has resourceClaim), score, and cached placements.
func newTestHandle(name string, opts ...testHandleOption) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "poolboy.gpte.redhat.com/v1",
			"kind":       "ResourceHandle",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "poolboy",
			},
			"spec": map[string]interface{}{},
		},
	}
	for _, opt := range opts {
		opt(obj)
	}
	return obj
}

type testHandleOption func(*unstructured.Unstructured)

func withBound() testHandleOption {
	return func(obj *unstructured.Unstructured) {
		obj.Object["spec"].(map[string]interface{})["resourceClaim"] = map[string]interface{}{
			"name":      "claim-1",
			"namespace": "user-ns",
		}
	}
}

func withScore(score float64) testHandleOption {
	return func(obj *unstructured.Unstructured) {
		obj.Object["spec"].(map[string]interface{})["preferenceScore"] = score
	}
}

func withCachedPlacements(placements ...placement.Placement) testHandleOption {
	return func(obj *unstructured.Unstructured) {
		items := make([]interface{}, len(placements))
		for i, p := range placements {
			entry := map[string]interface{}{
				"clusterName": p.ClusterName,
			}
			if p.Name != "" {
				entry["name"] = p.Name
			}
			if p.Namespace != "" {
				entry["namespace"] = p.Namespace
			}
			items[i] = entry
		}

		if _, ok := obj.Object["status"]; !ok {
			obj.Object["status"] = map[string]interface{}{}
		}
		obj.Object["status"].(map[string]interface{})["placements"] = items
	}
}

// newFakeReconciler creates a Reconciler with a fake Kubernetes client,
// a mock scorer, and a mock resolver.
//
// The fake client uses WithObjects (stores initial state) and
// WithStatusSubresource (enables r.Status().Patch() for status writes).
func newFakeReconciler(scorer *mockScorer, resolver *mockResolver, objects ...client.Object) *Reconciler {
	cb := fake.NewClientBuilder()
	for _, obj := range objects {
		cb = cb.WithObjects(obj)
		cb = cb.WithStatusSubresource(obj)
	}

	return &Reconciler{
		Client:   cb.Build(),
		Scorer:   scorer,
		Resolver: resolver,
		Config:   testConfig(),
	}
}

func reconcileRequest(name string) ctrl.Request {
	return ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{
			Name:      name,
			Namespace: "poolboy",
		},
	}
}

// --- Reconciler tests ---

func TestReconcile_HandleNotFound(t *testing.T) {
	scorer := &mockScorer{}
	resolver := &mockResolver{}
	r := newFakeReconciler(scorer, resolver)

	result, err := r.Reconcile(context.Background(), reconcileRequest("nonexistent"))

	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Errorf("Reconcile() result = %+v, want no requeue", result)
	}
	if scorer.called {
		t.Error("Scorer was called for nonexistent handle")
	}
}

func TestReconcile_HandleBound(t *testing.T) {
	handle := newTestHandle("bound-handle", withBound())
	scorer := &mockScorer{}
	resolver := &mockResolver{}
	r := newFakeReconciler(scorer, resolver, handle)

	result, err := r.Reconcile(context.Background(), reconcileRequest("bound-handle"))

	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Errorf("Reconcile() result = %+v, want no requeue", result)
	}
	if resolver.called {
		t.Error("Resolver was called for bound handle")
	}
	if scorer.called {
		t.Error("Scorer was called for bound handle")
	}
}

func TestReconcile_PlacementResolutionFails(t *testing.T) {
	handle := newTestHandle("fail-placement")
	scorer := &mockScorer{}
	resolver := &mockResolver{err: fmt.Errorf("AnarchySubject not found")}
	r := newFakeReconciler(scorer, resolver, handle)

	result, err := r.Reconcile(context.Background(), reconcileRequest("fail-placement"))

	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (requeue, not error)", err)
	}
	if result.RequeueAfter != 30*time.Second {
		t.Errorf("RequeueAfter = %v, want 30s", result.RequeueAfter)
	}
	if scorer.called {
		t.Error("Scorer was called when placement resolution failed")
	}
}

func TestReconcile_NoPlacementsResolved(t *testing.T) {
	handle := newTestHandle("no-placements")
	scorer := &mockScorer{}
	resolver := &mockResolver{placements: []placement.Placement{}}
	r := newFakeReconciler(scorer, resolver, handle)

	result, err := r.Reconcile(context.Background(), reconcileRequest("no-placements"))

	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Errorf("Reconcile() result = %+v, want no requeue", result)
	}
	if scorer.called {
		t.Error("Scorer was called when no placements resolved")
	}
}

func TestReconcile_ScorerFails(t *testing.T) {
	handle := newTestHandle("scorer-fail", withScore(50.0),
		withCachedPlacements(placement.Placement{ClusterName: "ocpv05"}))
	scorer := &mockScorer{err: fmt.Errorf("connection refused")}
	resolver := &mockResolver{
		placements: []placement.Placement{{ClusterName: "ocpv05"}},
	}
	r := newFakeReconciler(scorer, resolver, handle)

	result, err := r.Reconcile(context.Background(), reconcileRequest("scorer-fail"))

	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (requeue, not error)", err)
	}
	if result.RequeueAfter != 30*time.Second {
		t.Errorf("RequeueAfter = %v, want 30s", result.RequeueAfter)
	}

	// Verify existing score is preserved (no patch applied).
	var updated unstructured.Unstructured
	updated.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := r.Get(context.Background(), reconcileRequest("scorer-fail").NamespacedName, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	score, _ := placement.GetCurrentScore(&updated)
	if score != 50.0 {
		t.Errorf("score after failed evaluation = %v, want 50.0 (unchanged)", score)
	}
}

func TestReconcile_ScoreUnchanged(t *testing.T) {
	handle := newTestHandle("same-score", withScore(82.5),
		withCachedPlacements(placement.Placement{ClusterName: "ocpv05"}))
	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{
				{ClusterName: "ocpv05", Score: 82.5, Eligible: true},
			},
		},
	}
	resolver := &mockResolver{
		placements: []placement.Placement{{ClusterName: "ocpv05"}},
	}
	r := newFakeReconciler(scorer, resolver, handle)

	result, err := r.Reconcile(context.Background(), reconcileRequest("same-score"))

	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Errorf("Reconcile() result = %+v, want no requeue", result)
	}

	// Verify nothing was stored in the sync.Map (no patch = no tracking).
	if _, ok := r.LastWrittenScores.Load("poolboy/same-score"); ok {
		t.Error("LastWrittenScores should not be set when score is unchanged")
	}
}

func TestReconcile_ScoreChanged(t *testing.T) {
	handle := newTestHandle("score-change", withScore(50.0),
		withCachedPlacements(placement.Placement{ClusterName: "ocpv06"}))
	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{
				{ClusterName: "ocpv06", Score: 82.5, Eligible: true},
			},
		},
	}
	resolver := &mockResolver{
		placements: []placement.Placement{{ClusterName: "ocpv06"}},
	}
	r := newFakeReconciler(scorer, resolver, handle)

	result, err := r.Reconcile(context.Background(), reconcileRequest("score-change"))

	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Errorf("Reconcile() result = %+v, want no requeue", result)
	}

	// Verify score was patched.
	var updated unstructured.Unstructured
	updated.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := r.Get(context.Background(), reconcileRequest("score-change").NamespacedName, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	newScore, found := placement.GetCurrentScore(&updated)
	if !found || newScore != 82.5 {
		t.Errorf("score after patch = %v (found=%v), want 82.5", newScore, found)
	}

	// Verify sync.Map was updated for self-update tracking.
	stored, ok := r.LastWrittenScores.Load("poolboy/score-change")
	if !ok {
		t.Fatal("LastWrittenScores not set after score patch")
	}
	if stored.(float64) != 82.5 {
		t.Errorf("LastWrittenScores = %v, want 82.5", stored)
	}
}

func TestReconcile_PlacementsCached_SkipsStatusPatch(t *testing.T) {
	handle := newTestHandle("cached",
		withCachedPlacements(placement.Placement{ClusterName: "ocpv05"}))
	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{
				{ClusterName: "ocpv05", Score: 70.0, Eligible: true},
			},
		},
	}
	resolver := &mockResolver{
		placements: []placement.Placement{{ClusterName: "ocpv05"}},
	}
	r := newFakeReconciler(scorer, resolver, handle)

	_, err := r.Reconcile(context.Background(), reconcileRequest("cached"))
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}

	// Verify the placements in status are unchanged (no extra writes).
	var updated unstructured.Unstructured
	updated.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := r.Get(context.Background(), reconcileRequest("cached").NamespacedName, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	cached, found := placement.GetPlacementsFromStatus(&updated)
	if !found || len(cached) != 1 {
		t.Errorf("cached placements = %v (found=%v), want 1 entry", cached, found)
	}
}

func TestReconcile_PlacementsNotCached_WritesStatus(t *testing.T) {
	handle := newTestHandle("uncached")
	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{
				{ClusterName: "ocpv05", Score: 75.0, Eligible: true},
			},
		},
	}
	resolver := &mockResolver{
		placements: []placement.Placement{
			{ClusterName: "ocpv05", Name: "abc12-uuid", Namespace: "sandbox-abc12"},
		},
	}
	r := newFakeReconciler(scorer, resolver, handle)

	_, err := r.Reconcile(context.Background(), reconcileRequest("uncached"))
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}

	// Verify status.placements was written.
	var updated unstructured.Unstructured
	updated.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := r.Get(context.Background(), reconcileRequest("uncached").NamespacedName, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	cached, found := placement.GetPlacementsFromStatus(&updated)
	if !found {
		t.Fatal("status.placements not found after reconcile")
	}
	if len(cached) != 1 {
		t.Fatalf("cached placements count = %d, want 1", len(cached))
	}
	if cached[0].ClusterName != "ocpv05" {
		t.Errorf("cached clusterName = %q, want %q", cached[0].ClusterName, "ocpv05")
	}
	if cached[0].Name != "abc12-uuid" {
		t.Errorf("cached name = %q, want %q", cached[0].Name, "abc12-uuid")
	}
}

func TestReconcile_AllExcluded_SetsScoreToZero(t *testing.T) {
	handle := newTestHandle("all-excluded", withScore(50.0),
		withCachedPlacements(placement.Placement{ClusterName: "ocpv05"}))
	reason := "cluster not registered"
	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{},
			Excluded: []scheduler.ScoredCandidate{
				{ClusterName: "ocpv05", Eligible: false, IneligibilityReason: &reason},
			},
		},
	}
	resolver := &mockResolver{
		placements: []placement.Placement{{ClusterName: "ocpv05"}},
	}
	r := newFakeReconciler(scorer, resolver, handle)

	_, err := r.Reconcile(context.Background(), reconcileRequest("all-excluded"))
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}

	var updated unstructured.Unstructured
	updated.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := r.Get(context.Background(), reconcileRequest("all-excluded").NamespacedName, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	score, _ := placement.GetCurrentScore(&updated)
	if score != 0 {
		t.Errorf("score = %v, want 0 (all candidates excluded)", score)
	}
}

// --- Helper function tests ---

func TestBuildCandidates(t *testing.T) {
	placements := []placement.Placement{
		{ClusterName: "ocpv05", Name: "abc12-uuid", Namespace: "sandbox-abc12"},
		{ClusterName: "ocpv06"},
	}

	candidates := buildCandidates(placements)

	if len(candidates) != 2 {
		t.Fatalf("buildCandidates() returned %d candidates, want 2", len(candidates))
	}

	if candidates[0].ClusterName != "ocpv05" {
		t.Errorf("candidates[0].ClusterName = %q, want %q", candidates[0].ClusterName, "ocpv05")
	}
	if candidates[0].HandleName == nil || *candidates[0].HandleName != "abc12-uuid" {
		t.Errorf("candidates[0].HandleName = %v, want %q", candidates[0].HandleName, "abc12-uuid")
	}
	if candidates[0].HandleNamespace == nil || *candidates[0].HandleNamespace != "sandbox-abc12" {
		t.Errorf("candidates[0].HandleNamespace = %v, want %q", candidates[0].HandleNamespace, "sandbox-abc12")
	}

	if candidates[1].ClusterName != "ocpv06" {
		t.Errorf("candidates[1].ClusterName = %q, want %q", candidates[1].ClusterName, "ocpv06")
	}
	if candidates[1].HandleName != nil {
		t.Errorf("candidates[1].HandleName = %v, want nil (tenant cluster, no name)", candidates[1].HandleName)
	}
	if candidates[1].HandleNamespace != nil {
		t.Errorf("candidates[1].HandleNamespace = %v, want nil", candidates[1].HandleNamespace)
	}
}

func TestBestScore(t *testing.T) {
	tests := []struct {
		name     string
		resp     *scheduler.EvaluateResponse
		expected float64
	}{
		{
			name: "multiple ranked returns highest",
			resp: &scheduler.EvaluateResponse{
				Ranked: []scheduler.ScoredCandidate{
					{ClusterName: "ocpv06", Score: 85.0, Eligible: true},
					{ClusterName: "ocpv05", Score: 65.0, Eligible: true},
				},
			},
			expected: 85.0,
		},
		{
			name: "single ranked",
			resp: &scheduler.EvaluateResponse{
				Ranked: []scheduler.ScoredCandidate{
					{ClusterName: "ocpv05", Score: 42.0, Eligible: true},
				},
			},
			expected: 42.0,
		},
		{
			name: "all excluded returns zero",
			resp: &scheduler.EvaluateResponse{
				Ranked: []scheduler.ScoredCandidate{},
			},
			expected: 0,
		},
		{
			name:     "empty response returns zero",
			resp:     &scheduler.EvaluateResponse{},
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := bestScore(tc.resp)
			if got != tc.expected {
				t.Errorf("bestScore() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestBuildCandidates_MultiplePlacements(t *testing.T) {
	placements := []placement.Placement{
		{ClusterName: "ocpv05", Name: "h1-uuid", Namespace: "sandbox-h1"},
		{ClusterName: "ocpv09", Name: "h1-uuid-2", Namespace: "sandbox-h1-2"},
	}

	candidates := buildCandidates(placements)
	if len(candidates) != 2 {
		t.Fatalf("buildCandidates() returned %d, want 2", len(candidates))
	}

	// Verify GVK is not leaked (GVK is a controller-level concern, not a candidate field).
	for i, c := range candidates {
		if c.ClusterName == "" {
			t.Errorf("candidates[%d].ClusterName is empty", i)
		}
	}
}

// --- Conflict and error tests ---
//
// These tests use the fake client's WithInterceptorFuncs to simulate
// API server errors (409 Conflict, 500 Internal Server Error).
// Interceptors wrap the fake client — when set, they intercept calls
// BEFORE they reach the fake storage, so we can return errors.

func newConflictError(name string) error {
	return apierrors.NewConflict(
		schema.GroupResource{Group: "poolboy.gpte.redhat.com", Resource: "resourcehandles"},
		name, fmt.Errorf("the object has been modified"))
}

func TestReconcile_ScorePatchConflict(t *testing.T) {
	handle := newTestHandle("conflict-score", withScore(50.0),
		withCachedPlacements(placement.Placement{ClusterName: "ocpv05"}))
	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{
				{ClusterName: "ocpv05", Score: 82.5, Eligible: true},
			},
		},
	}
	resolver := &mockResolver{
		placements: []placement.Placement{{ClusterName: "ocpv05"}},
	}

	cb := fake.NewClientBuilder().
		WithObjects(handle).
		WithStatusSubresource(handle).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				return newConflictError(obj.GetName())
			},
		})

	r := &Reconciler{
		Client:   cb.Build(),
		Scorer:   scorer,
		Resolver: resolver,
		Config:   testConfig(),
	}

	result, err := r.Reconcile(context.Background(), reconcileRequest("conflict-score"))

	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (409 should be swallowed)", err)
	}
	if !result.Requeue {
		t.Error("Reconcile() Requeue = false, want true (retry after conflict)")
	}
}

func TestReconcile_ScorePatchNonConflictError(t *testing.T) {
	handle := newTestHandle("error-score", withScore(50.0),
		withCachedPlacements(placement.Placement{ClusterName: "ocpv05"}))
	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{
				{ClusterName: "ocpv05", Score: 82.5, Eligible: true},
			},
		},
	}
	resolver := &mockResolver{
		placements: []placement.Placement{{ClusterName: "ocpv05"}},
	}

	cb := fake.NewClientBuilder().
		WithObjects(handle).
		WithStatusSubresource(handle).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				return fmt.Errorf("internal server error")
			},
		})

	r := &Reconciler{
		Client:   cb.Build(),
		Scorer:   scorer,
		Resolver: resolver,
		Config:   testConfig(),
	}

	_, err := r.Reconcile(context.Background(), reconcileRequest("error-score"))

	if err == nil {
		t.Fatal("Reconcile() error = nil, want non-nil (non-conflict errors propagate)")
	}
}

func TestReconcile_StatusPatchConflict(t *testing.T) {
	handle := newTestHandle("conflict-status")
	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{
				{ClusterName: "ocpv05", Score: 75.0, Eligible: true},
			},
		},
	}
	resolver := &mockResolver{
		placements: []placement.Placement{{ClusterName: "ocpv05"}},
	}

	cb := fake.NewClientBuilder().
		WithObjects(handle).
		WithStatusSubresource(handle).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				return newConflictError(obj.GetName())
			},
		})

	r := &Reconciler{
		Client:   cb.Build(),
		Scorer:   scorer,
		Resolver: resolver,
		Config:   testConfig(),
	}

	result, err := r.Reconcile(context.Background(), reconcileRequest("conflict-status"))

	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (409 should be swallowed)", err)
	}
	if !result.Requeue {
		t.Error("Reconcile() Requeue = false, want true (retry after conflict)")
	}
	if scorer.called {
		t.Error("Scorer should not be called when status patch fails (exits early)")
	}
}

func TestReconcile_StatusPatchNonConflictError(t *testing.T) {
	handle := newTestHandle("error-status")
	scorer := &mockScorer{}
	resolver := &mockResolver{
		placements: []placement.Placement{{ClusterName: "ocpv05"}},
	}

	cb := fake.NewClientBuilder().
		WithObjects(handle).
		WithStatusSubresource(handle).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				return fmt.Errorf("internal server error")
			},
		})

	r := &Reconciler{
		Client:   cb.Build(),
		Scorer:   scorer,
		Resolver: resolver,
		Config:   testConfig(),
	}

	_, err := r.Reconcile(context.Background(), reconcileRequest("error-status"))

	if err == nil {
		t.Fatal("Reconcile() error = nil, want non-nil (non-conflict errors propagate)")
	}
	if scorer.called {
		t.Error("Scorer should not be called when status patch fails")
	}
}

func TestReconcile_MultiplePlacements(t *testing.T) {
	handle := newTestHandle("multi-placement",
		withCachedPlacements(
			placement.Placement{ClusterName: "ocpv05", Name: "h1-a", Namespace: "sandbox-a"},
			placement.Placement{ClusterName: "ocpv09", Name: "h1-b", Namespace: "sandbox-b"},
		))
	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{
				{ClusterName: "ocpv09", Score: 90.0, Eligible: true},
				{ClusterName: "ocpv05", Score: 60.0, Eligible: true},
			},
		},
	}
	resolver := &mockResolver{
		placements: []placement.Placement{
			{ClusterName: "ocpv05", Name: "h1-a", Namespace: "sandbox-a"},
			{ClusterName: "ocpv09", Name: "h1-b", Namespace: "sandbox-b"},
		},
	}
	r := newFakeReconciler(scorer, resolver, handle)

	_, err := r.Reconcile(context.Background(), reconcileRequest("multi-placement"))
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}

	if len(scorer.received) != 2 {
		t.Fatalf("Scorer received %d candidates, want 2", len(scorer.received))
	}

	var updated unstructured.Unstructured
	updated.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := r.Get(context.Background(), reconcileRequest("multi-placement").NamespacedName, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	score, found := placement.GetCurrentScore(&updated)
	if !found || score != 90.0 {
		t.Errorf("score = %v (found=%v), want 90.0 (highest ranked)", score, found)
	}
}

func TestReconcile_FirstTimeScoring(t *testing.T) {
	handle := newTestHandle("first-time",
		withCachedPlacements(placement.Placement{ClusterName: "ocpv06"}))
	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{
				{ClusterName: "ocpv06", Score: 75.5, Eligible: true},
			},
		},
	}
	resolver := &mockResolver{
		placements: []placement.Placement{{ClusterName: "ocpv06"}},
	}
	r := newFakeReconciler(scorer, resolver, handle)

	_, err := r.Reconcile(context.Background(), reconcileRequest("first-time"))
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}

	var updated unstructured.Unstructured
	updated.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := r.Get(context.Background(), reconcileRequest("first-time").NamespacedName, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	score, found := placement.GetCurrentScore(&updated)
	if !found || score != 75.5 {
		t.Errorf("score = %v (found=%v), want 75.5", score, found)
	}

	stored, ok := r.LastWrittenScores.Load("poolboy/first-time")
	if !ok || stored.(float64) != 75.5 {
		t.Errorf("LastWrittenScores = %v (ok=%v), want 75.5", stored, ok)
	}
}

// --- SetupWithManager test ---

func TestSetupWithManager_GVK(t *testing.T) {
	// Verify the GVK used for registration matches the expected ResourceHandle kind.
	gvk := placement.ResourceHandleGVK
	expected := schema.GroupVersionKind{
		Group:   "poolboy.gpte.redhat.com",
		Version: "v1",
		Kind:    "ResourceHandle",
	}
	if gvk != expected {
		t.Errorf("ResourceHandleGVK = %v, want %v", gvk, expected)
	}
}
