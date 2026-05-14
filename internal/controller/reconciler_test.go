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

type mockResolver struct {
	placements map[string][]placement.Placement
	errors     map[string]error
	called     bool
}

func newMockResolver() *mockResolver {
	return &mockResolver{
		placements: make(map[string][]placement.Placement),
		errors:     make(map[string]error),
	}
}

func (m *mockResolver) Lookup(_ context.Context, handle *unstructured.Unstructured) ([]placement.Placement, error) {
	m.called = true
	name := handle.GetName()
	if err, ok := m.errors[name]; ok {
		return nil, err
	}
	if p, ok := m.placements[name]; ok {
		return p, nil
	}
	return nil, nil
}

// --- Test helpers ---

func testConfig() *config.Config {
	cfg, err := config.NewForTest("30s")
	if err != nil {
		panic(fmt.Sprintf("testConfig: %v", err))
	}
	return cfg
}

func newTestPool(name string, available int64, handles []map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "poolboy.gpte.redhat.com/v1",
			"kind":       "ResourcePool",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "poolboy",
			},
			"status": map[string]interface{}{
				"resourceHandleCount": map[string]interface{}{
					"available": available,
				},
				"resourceHandles": toInterfaceSlice(handles),
			},
		},
	}
	return obj
}

func toInterfaceSlice(entries []map[string]interface{}) []interface{} {
	result := make([]interface{}, len(entries))
	for i, e := range entries {
		result[i] = e
	}
	return result
}

func handleEntry(name string, healthy bool) map[string]interface{} {
	return map[string]interface{}{
		"name":    name,
		"healthy": healthy,
		"ready":   true,
	}
}

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
		spec := obj.Object["spec"].(map[string]interface{})
		spec["resourceClaim"] = map[string]interface{}{
			"name":      "claim-1",
			"namespace": "user-ns",
		}
	}
}

func withScore(score float64) testHandleOption {
	return func(obj *unstructured.Unstructured) {
		spec := obj.Object["spec"].(map[string]interface{})
		spec["preferenceScore"] = score
	}
}

func withCachedPlacements(cluster string) testHandleOption {
	return func(obj *unstructured.Unstructured) {
		if obj.Object["status"] == nil {
			obj.Object["status"] = map[string]interface{}{}
		}
		status := obj.Object["status"].(map[string]interface{})
		status["placements"] = []interface{}{
			map[string]interface{}{"clusterName": cluster},
		}
	}
}

func defaultResponse(clusters ...string) *scheduler.EvaluateResponse {
	ranked := make([]scheduler.ScoredCandidate, len(clusters))
	for i, c := range clusters {
		ranked[i] = scheduler.ScoredCandidate{
			Name:     c,
			Score:    80 - float64(i)*10,
			Eligible: true,
		}
	}
	return &scheduler.EvaluateResponse{
		Ranked:      ranked,
		Strategy:    "most_capacity",
		GeneratedAt: time.Now(),
	}
}

func newReconciler(c client.Client, scorer *mockScorer, resolver *mockResolver) *ResourcePoolReconciler {
	return &ResourcePoolReconciler{
		Client:   c,
		Scorer:   scorer,
		Resolver: resolver,
		Config:   testConfig(),
	}
}

func poolRequest(name string) ctrl.Request {
	return ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{
			Name:      name,
			Namespace: "poolboy",
		},
	}
}

// --- Tests ---

func TestReconcile_PoolNotFound(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	scorer := &mockScorer{}
	resolver := newMockResolver()
	r := newReconciler(c, scorer, resolver)

	res, err := r.Reconcile(context.Background(), poolRequest("nonexistent"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue {
		t.Error("expected no requeue")
	}
	if scorer.called {
		t.Error("scorer should not be called for missing pool")
	}
}

func TestReconcile_NoAvailableHandles(t *testing.T) {
	pool := newTestPool("test-pool", 0, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	c := fake.NewClientBuilder().WithObjects(pool).Build()
	scorer := &mockScorer{}
	resolver := newMockResolver()
	r := newReconciler(c, scorer, resolver)

	res, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue {
		t.Error("expected no requeue")
	}
	if scorer.called {
		t.Error("scorer should not be called when available=0")
	}
}

func TestReconcile_NoHandlesInStatus(t *testing.T) {
	pool := newTestPool("test-pool", 2, nil)
	// Override to empty resourceHandles
	pool.Object["status"].(map[string]interface{})["resourceHandles"] = []interface{}{}

	c := fake.NewClientBuilder().WithObjects(pool).Build()
	scorer := &mockScorer{}
	resolver := newMockResolver()
	r := newReconciler(c, scorer, resolver)

	res, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue {
		t.Error("expected no requeue")
	}
	if scorer.called {
		t.Error("scorer should not be called for empty handle list")
	}
}

func TestReconcile_UnhealthyHandlesSkipped(t *testing.T) {
	pool := newTestPool("test-pool", 2, []map[string]interface{}{
		handleEntry("handle-unhealthy", false),
		handleEntry("handle-healthy", true),
	})

	handleHealthy := newTestHandle("handle-healthy", withCachedPlacements("ocpv06"))
	resolver := newMockResolver()
	resolver.placements["handle-healthy"] = []placement.Placement{
		{ClusterName: "ocpv06"},
	}

	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	c := fake.NewClientBuilder().WithObjects(pool, handleHealthy).Build()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !scorer.called {
		t.Error("scorer should be called for the healthy handle")
	}
	if len(scorer.received) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(scorer.received))
	}
}

func TestReconcile_AllBoundHandles(t *testing.T) {
	pool := newTestPool("test-pool", 0, []map[string]interface{}{
		handleEntry("handle-1", true),
		handleEntry("handle-2", true),
	})
	// available is 0 so it should short-circuit before even checking handles
	pool.Object["status"].(map[string]interface{})["resourceHandleCount"] = map[string]interface{}{
		"available": int64(0),
	}

	handle1 := newTestHandle("handle-1", withBound())
	handle2 := newTestHandle("handle-2", withBound())

	c := fake.NewClientBuilder().WithObjects(pool, handle1, handle2).Build()
	scorer := &mockScorer{}
	resolver := newMockResolver()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scorer.called {
		t.Error("scorer should not be called when all handles are bound")
	}
}

func TestReconcile_SingleUnboundHandle(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1", withCachedPlacements("ocpv06"))

	resolver := newMockResolver()
	resolver.placements["handle-1"] = []placement.Placement{
		{ClusterName: "ocpv06"},
	}
	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	c := fake.NewClientBuilder().WithObjects(pool, handle).Build()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !scorer.called {
		t.Fatal("scorer should be called")
	}
	if len(scorer.received) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(scorer.received))
	}
	if scorer.received[0].Name != "ocpv06" {
		t.Errorf("expected cluster ocpv06, got %s", scorer.received[0].Name)
	}

	// Verify score was patched
	var updated unstructured.Unstructured
	updated.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "handle-1", Namespace: "poolboy"}, &updated); err != nil {
		t.Fatalf("failed to get updated handle: %v", err)
	}
	spec, err := placement.ParseHandleSpec(&updated)
	if err != nil {
		t.Fatalf("failed to parse handle spec: %v", err)
	}
	if spec.PreferenceScore == nil {
		t.Fatal("expected score to be set")
	}
	if *spec.PreferenceScore != 80 {
		t.Errorf("expected score 80, got %v", *spec.PreferenceScore)
	}
}

func TestReconcile_MultiHandleMultiCluster(t *testing.T) {
	pool := newTestPool("test-pool", 5, []map[string]interface{}{
		handleEntry("h-a", true),
		handleEntry("h-b", true),
		handleEntry("h-c", true),
		handleEntry("h-d", true),
		handleEntry("h-e", true),
	})

	ha := newTestHandle("h-a", withCachedPlacements("ocpv06"))
	hb := newTestHandle("h-b", withCachedPlacements("ocpv06"))
	hc := newTestHandle("h-c", withCachedPlacements("ocpv05"))
	hd := newTestHandle("h-d", withCachedPlacements("ocpv10"))
	he := newTestHandle("h-e", withCachedPlacements("ocpv10"))

	resolver := newMockResolver()
	resolver.placements["h-a"] = []placement.Placement{{ClusterName: "ocpv06"}}
	resolver.placements["h-b"] = []placement.Placement{{ClusterName: "ocpv06"}}
	resolver.placements["h-c"] = []placement.Placement{{ClusterName: "ocpv05"}}
	resolver.placements["h-d"] = []placement.Placement{{ClusterName: "ocpv10"}}
	resolver.placements["h-e"] = []placement.Placement{{ClusterName: "ocpv10"}}

	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{
				{Name: "ocpv06", Score: 73.69, Eligible: true},
				{Name: "ocpv05", Score: 65.58, Eligible: true},
				{Name: "ocpv10", Score: 34.44, Eligible: true},
			},
			Strategy:    "most_capacity",
			GeneratedAt: time.Now(),
		},
	}

	c := fake.NewClientBuilder().WithObjects(pool, ha, hb, hc, hd, he).Build()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(scorer.received) != 3 {
		t.Fatalf("expected 3 deduplicated candidates, got %d", len(scorer.received))
	}

	// Verify differentiated scores
	expected := map[string]float64{
		"h-a": 73.69, "h-b": 73.69,
		"h-c": 65.58,
		"h-d": 34.44, "h-e": 34.44,
	}
	for name, wantScore := range expected {
		var h unstructured.Unstructured
		h.SetGroupVersionKind(placement.ResourceHandleGVK)
		if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: name, Namespace: "poolboy"}, &h); err != nil {
			t.Fatalf("get %s: %v", name, err)
		}
		spec, err := placement.ParseHandleSpec(&h)
		if err != nil {
			t.Fatalf("%s: parse spec: %v", name, err)
		}
		if spec.PreferenceScore == nil {
			t.Errorf("%s: score not set", name)
			continue
		}
		if *spec.PreferenceScore != wantScore {
			t.Errorf("%s: score = %v, want %v", name, *spec.PreferenceScore, wantScore)
		}
	}
}

func TestReconcile_SameClusterDeduplication(t *testing.T) {
	pool := newTestPool("test-pool", 3, []map[string]interface{}{
		handleEntry("h-1", true),
		handleEntry("h-2", true),
		handleEntry("h-3", true),
	})

	h1 := newTestHandle("h-1", withCachedPlacements("ocpv06"))
	h2 := newTestHandle("h-2", withCachedPlacements("ocpv06"))
	h3 := newTestHandle("h-3", withCachedPlacements("ocpv06"))

	resolver := newMockResolver()
	resolver.placements["h-1"] = []placement.Placement{{ClusterName: "ocpv06"}}
	resolver.placements["h-2"] = []placement.Placement{{ClusterName: "ocpv06"}}
	resolver.placements["h-3"] = []placement.Placement{{ClusterName: "ocpv06"}}

	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	c := fake.NewClientBuilder().WithObjects(pool, h1, h2, h3).Build()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scorer.received) != 1 {
		t.Errorf("expected 1 deduplicated candidate, got %d", len(scorer.received))
	}
}

func TestReconcile_PartialPlacementFailure(t *testing.T) {
	pool := newTestPool("test-pool", 3, []map[string]interface{}{
		handleEntry("h-ok", true),
		handleEntry("h-fail1", true),
		handleEntry("h-fail2", true),
	})

	hOk := newTestHandle("h-ok", withCachedPlacements("ocpv06"))
	hFail1 := newTestHandle("h-fail1")
	hFail2 := newTestHandle("h-fail2")

	resolver := newMockResolver()
	resolver.placements["h-ok"] = []placement.Placement{{ClusterName: "ocpv06"}}
	resolver.errors["h-fail1"] = fmt.Errorf("subject not found")
	resolver.errors["h-fail2"] = fmt.Errorf("subject not found")

	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	c := fake.NewClientBuilder().WithObjects(pool, hOk, hFail1, hFail2).Build()
	r := newReconciler(c, scorer, resolver)

	res, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !scorer.called {
		t.Error("scorer should be called for the successful handle")
	}
	if res.RequeueAfter == 0 {
		t.Error("expected requeue due to placement failures")
	}
}

func TestReconcile_ScoreUnchanged(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1", withScore(80))

	resolver := newMockResolver()
	resolver.placements["handle-1"] = []placement.Placement{{ClusterName: "ocpv06"}}
	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	patchCalled := false
	c := fake.NewClientBuilder().
		WithObjects(pool, handle).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, client client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if obj.GetObjectKind().GroupVersionKind() == placement.ResourceHandleGVK {
					patchCalled = true
				}
				return client.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patchCalled {
		t.Error("should not patch when score is unchanged")
	}
}

func TestReconcile_DryRun(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1")

	resolver := newMockResolver()
	resolver.placements["handle-1"] = []placement.Placement{{ClusterName: "ocpv06"}}
	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	patchCalled := false
	c := fake.NewClientBuilder().
		WithObjects(pool, handle).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, client client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if obj.GetObjectKind().GroupVersionKind() == placement.ResourceHandleGVK {
					patchCalled = true
				}
				return client.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	r := newReconciler(c, scorer, resolver)
	r.Config.DryRun = true

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !scorer.called {
		t.Error("scorer should be called even in dry-run mode")
	}
	if patchCalled {
		t.Error("should not patch in dry-run mode")
	}
}

func TestReconcile_ConflictOnPatch(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1", withCachedPlacements("ocpv06"))

	resolver := newMockResolver()
	resolver.placements["handle-1"] = []placement.Placement{{ClusterName: "ocpv06"}}
	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	c := fake.NewClientBuilder().
		WithObjects(pool, handle).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, client client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if obj.GetObjectKind().GroupVersionKind() == placement.ResourceHandleGVK {
					return apierrors.NewConflict(
						schema.GroupResource{Group: "poolboy.gpte.redhat.com", Resource: "resourcehandles"},
						"handle-1",
						fmt.Errorf("the object has been modified"),
					)
				}
				return client.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	r := newReconciler(c, scorer, resolver)

	res, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("expected no error on conflict, got: %v", err)
	}
	if !res.Requeue {
		t.Error("expected requeue on conflict")
	}
}

func TestReconcile_SchedulerError(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1")

	resolver := newMockResolver()
	resolver.placements["handle-1"] = []placement.Placement{{ClusterName: "ocpv06"}}
	scorer := &mockScorer{err: fmt.Errorf("connection refused")}

	c := fake.NewClientBuilder().WithObjects(pool, handle).Build()
	r := newReconciler(c, scorer, resolver)

	res, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("expected no error on scheduler failure, got: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Error("expected requeue after scheduler error")
	}
}

func TestReconcile_SchedulerCircuitBreaker(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1")

	resolver := newMockResolver()
	resolver.placements["handle-1"] = []placement.Placement{{ClusterName: "ocpv06"}}
	scorer := &mockScorer{err: fmt.Errorf("connection refused")}

	c := fake.NewClientBuilder().WithObjects(pool, handle).Build()
	r := newReconciler(c, scorer, resolver)

	// First N-1 failures should requeue aggressively
	for i := 0; i < maxConsecutiveSchedulerFailures-1; i++ {
		res, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if res.RequeueAfter == 0 {
			t.Fatalf("iteration %d: expected RequeueAfter, got none", i)
		}
	}

	// Nth failure should stop requeueing (circuit open)
	res, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("circuit breaker: unexpected error: %v", err)
	}
	if res.RequeueAfter != 0 || res.Requeue {
		t.Error("circuit breaker: expected no requeue after max failures")
	}
}

func TestReconcile_SchedulerRecovery(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1", withCachedPlacements("ocpv06"))

	resolver := newMockResolver()
	resolver.placements["handle-1"] = []placement.Placement{{ClusterName: "ocpv06"}}

	scorer := &mockScorer{err: fmt.Errorf("connection refused")}
	c := fake.NewClientBuilder().WithObjects(pool, handle).Build()
	r := newReconciler(c, scorer, resolver)

	// Accumulate some failures
	for i := 0; i < 3; i++ {
		r.Reconcile(context.Background(), poolRequest("test-pool"))
	}
	if r.schedulerFailures.Load() != 3 {
		t.Fatalf("expected 3 failures, got %d", r.schedulerFailures.Load())
	}

	// Scheduler recovers
	scorer.err = nil
	scorer.response = defaultResponse("ocpv06")
	r.Reconcile(context.Background(), poolRequest("test-pool"))

	if r.schedulerFailures.Load() != 0 {
		t.Errorf("expected failure counter reset to 0, got %d", r.schedulerFailures.Load())
	}
}

func TestReconcile_HandleDeletedMidReconcile(t *testing.T) {
	pool := newTestPool("test-pool", 2, []map[string]interface{}{
		handleEntry("handle-exists", true),
		handleEntry("handle-deleted", true),
	})
	handleExists := newTestHandle("handle-exists", withCachedPlacements("ocpv06"))
	// handle-deleted is NOT created — simulates deletion between pool status and reconcile

	resolver := newMockResolver()
	resolver.placements["handle-exists"] = []placement.Placement{{ClusterName: "ocpv06"}}
	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	c := fake.NewClientBuilder().WithObjects(pool, handleExists).Build()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !scorer.called {
		t.Error("scorer should be called for the existing handle")
	}
	if len(scorer.received) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(scorer.received))
	}
}

func TestReconcile_StatusPlacementCaching(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1") // no cached placements

	resolver := newMockResolver()
	resolver.placements["handle-1"] = []placement.Placement{{ClusterName: "ocpv06"}}
	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	statusObj := &unstructured.Unstructured{}
	statusObj.SetGroupVersionKind(placement.ResourceHandleGVK)

	c := fake.NewClientBuilder().
		WithStatusSubresource(statusObj).
		WithObjects(pool, handle).
		Build()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify placements were cached in status
	var updated unstructured.Unstructured
	updated.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "handle-1", Namespace: "poolboy"}, &updated); err != nil {
		t.Fatalf("failed to get updated handle: %v", err)
	}
	handleStatus, err := placement.ParseHandleStatus(&updated)
	if err != nil {
		t.Fatalf("failed to parse handle status: %v", err)
	}
	if handleStatus == nil || len(handleStatus.Placements) == 0 {
		t.Fatal("expected placements to be cached in status")
	}
	if handleStatus.Placements[0].ClusterName != "ocpv06" {
		t.Errorf("cached cluster = %s, want ocpv06", handleStatus.Placements[0].ClusterName)
	}
}

func TestSetupWithManager_GVK(t *testing.T) {
	gvk := placement.ResourcePoolGVK
	expected := schema.GroupVersionKind{
		Group:   "poolboy.gpte.redhat.com",
		Version: "v1",
		Kind:    "ResourcePool",
	}
	if gvk != expected {
		t.Errorf("ResourcePoolGVK = %v, want %v", gvk, expected)
	}
}

func TestReconcile_PoolGetError(t *testing.T) {
	c := fake.NewClientBuilder().
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key k8stypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				if obj.GetObjectKind().GroupVersionKind() == placement.ResourcePoolGVK {
					return fmt.Errorf("api server unavailable")
				}
				return client.Get(ctx, key, obj, opts...)
			},
		}).
		Build()
	scorer := &mockScorer{}
	resolver := newMockResolver()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err == nil {
		t.Fatal("expected error from pool GET failure")
	}
}

func TestReconcile_HandleGetError(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1")

	c := fake.NewClientBuilder().
		WithObjects(pool, handle).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key k8stypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				if obj.GetObjectKind().GroupVersionKind() == placement.ResourceHandleGVK {
					return fmt.Errorf("connection reset")
				}
				return client.Get(ctx, key, obj, opts...)
			},
		}).
		Build()
	scorer := &mockScorer{}
	resolver := newMockResolver()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err == nil {
		t.Fatal("expected error from handle GET failure")
	}
}

func TestReconcile_BoundHandleSkippedDuringIteration(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-bound", true),
		handleEntry("handle-unbound", true),
	})
	handleBound := newTestHandle("handle-bound", withBound())
	handleUnbound := newTestHandle("handle-unbound", withCachedPlacements("ocpv06"))

	resolver := newMockResolver()
	resolver.placements["handle-unbound"] = []placement.Placement{{ClusterName: "ocpv06"}}
	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	c := fake.NewClientBuilder().WithObjects(pool, handleBound, handleUnbound).Build()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scorer.received) != 1 {
		t.Errorf("expected 1 candidate (bound skipped), got %d", len(scorer.received))
	}
}

func TestReconcile_NoPlacementsResolved(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1")

	resolver := newMockResolver()
	// resolver returns nil placements for handle-1 (no error, just empty)
	scorer := &mockScorer{}

	c := fake.NewClientBuilder().WithObjects(pool, handle).Build()
	r := newReconciler(c, scorer, resolver)

	res, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue || res.RequeueAfter > 0 {
		t.Error("expected no requeue when placements are empty (not errors)")
	}
	if scorer.called {
		t.Error("scorer should not be called when no placements resolved")
	}
}

func TestReconcile_AllPlacementsFailed(t *testing.T) {
	pool := newTestPool("test-pool", 2, []map[string]interface{}{
		handleEntry("h-1", true),
		handleEntry("h-2", true),
	})
	h1 := newTestHandle("h-1")
	h2 := newTestHandle("h-2")

	resolver := newMockResolver()
	resolver.errors["h-1"] = fmt.Errorf("subject not found")
	resolver.errors["h-2"] = fmt.Errorf("subject not found")
	scorer := &mockScorer{}

	c := fake.NewClientBuilder().WithObjects(pool, h1, h2).Build()
	r := newReconciler(c, scorer, resolver)

	res, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Error("expected requeue when all placements failed")
	}
	if scorer.called {
		t.Error("scorer should not be called when no handles resolved")
	}
}

func TestReconcile_ConflictOnStatusPatch(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1") // no cached placements — will trigger status patch

	resolver := newMockResolver()
	resolver.placements["handle-1"] = []placement.Placement{{ClusterName: "ocpv06"}}
	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	statusObj := &unstructured.Unstructured{}
	statusObj.SetGroupVersionKind(placement.ResourceHandleGVK)

	c := fake.NewClientBuilder().
		WithStatusSubresource(statusObj).
		WithObjects(pool, handle).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, client client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				return apierrors.NewConflict(
					schema.GroupResource{Group: "poolboy.gpte.redhat.com", Resource: "resourcehandles"},
					"handle-1",
					fmt.Errorf("the object has been modified"),
				)
			},
		}).
		Build()
	r := newReconciler(c, scorer, resolver)

	res, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("expected no error on status patch conflict, got: %v", err)
	}
	if !res.Requeue {
		t.Error("expected requeue on status patch conflict")
	}
}

func TestReconcile_StatusPatchNonConflictError(t *testing.T) {
	pool := newTestPool("test-pool", 1, []map[string]interface{}{
		handleEntry("handle-1", true),
	})
	handle := newTestHandle("handle-1") // no cached placements

	resolver := newMockResolver()
	resolver.placements["handle-1"] = []placement.Placement{{ClusterName: "ocpv06"}}
	scorer := &mockScorer{response: defaultResponse("ocpv06")}

	statusObj := &unstructured.Unstructured{}
	statusObj.SetGroupVersionKind(placement.ResourceHandleGVK)

	c := fake.NewClientBuilder().
		WithStatusSubresource(statusObj).
		WithObjects(pool, handle).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, client client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				return fmt.Errorf("etcd timeout")
			},
		}).
		Build()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err == nil {
		t.Fatal("expected error from status patch failure")
	}
}

func TestDeduplicateClusters(t *testing.T) {
	handles := []handleWithCluster{
		{clusterName: "ocpv06"},
		{clusterName: "ocpv05"},
		{clusterName: "ocpv06"},
		{clusterName: "ocpv10"},
		{clusterName: "ocpv05"},
	}
	got := deduplicateClusters(handles)
	want := []string{"ocpv06", "ocpv05", "ocpv10"}

	if len(got) != len(want) {
		t.Fatalf("deduplicateClusters() returned %d clusters, want %d", len(got), len(want))
	}
	for i, c := range got {
		if c != want[i] {
			t.Errorf("deduplicateClusters()[%d] = %s, want %s", i, c, want[i])
		}
	}
}

func TestBuildScoreMap(t *testing.T) {
	resp := &scheduler.EvaluateResponse{
		Ranked: []scheduler.ScoredCandidate{
			{Name: "ocpv06", Score: 73.69},
			{Name: "ocpv05", Score: 65.58},
			{Name: "ocpv10", Score: 34.44},
		},
	}
	m := buildScoreMap(resp)

	expected := map[string]float64{
		"ocpv06": 73.69,
		"ocpv05": 65.58,
		"ocpv10": 34.44,
	}
	for k, want := range expected {
		if got, ok := m[k]; !ok {
			t.Errorf("missing key %s", k)
		} else if got != want {
			t.Errorf("scoreMap[%s] = %v, want %v", k, got, want)
		}
	}
}

func TestBuildScoreMap_Empty(t *testing.T) {
	resp := &scheduler.EvaluateResponse{}
	m := buildScoreMap(resp)
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestBuildExcludedSet(t *testing.T) {
	reason := "cluster in maintenance"
	resp := &scheduler.EvaluateResponse{
		Excluded: []scheduler.ScoredCandidate{
			{Name: "ocpv10", Eligible: false, IneligibilityReason: &reason},
		},
	}
	m := buildExcludedSet(resp)
	if !m["ocpv10"] {
		t.Error("expected ocpv10 in excluded set")
	}
	if m["ocpv06"] {
		t.Error("ocpv06 should not be in excluded set")
	}
}

func TestBuildExcludedSet_Empty(t *testing.T) {
	resp := &scheduler.EvaluateResponse{}
	m := buildExcludedSet(resp)
	if len(m) != 0 {
		t.Errorf("expected empty set, got %v", m)
	}
}

func TestReconcile_ExcludedClusterGetsScoreZero(t *testing.T) {
	pool := newTestPool("test-pool", 2, []map[string]interface{}{
		handleEntry("h-ranked", true),
		handleEntry("h-excluded", true),
	})

	hRanked := newTestHandle("h-ranked", withScore(50), withCachedPlacements("ocpv06"))
	hExcluded := newTestHandle("h-excluded", withScore(70), withCachedPlacements("ocpv10"))

	resolver := newMockResolver()
	resolver.placements["h-ranked"] = []placement.Placement{{ClusterName: "ocpv06"}}
	resolver.placements["h-excluded"] = []placement.Placement{{ClusterName: "ocpv10"}}

	reason := "cluster in maintenance"
	scorer := &mockScorer{
		response: &scheduler.EvaluateResponse{
			Ranked: []scheduler.ScoredCandidate{
				{Name: "ocpv06", Score: 80, Eligible: true},
			},
			Excluded: []scheduler.ScoredCandidate{
				{Name: "ocpv10", Score: 0, Eligible: false, IneligibilityReason: &reason},
			},
			Strategy:    "most_capacity",
			GeneratedAt: time.Now(),
		},
	}

	c := fake.NewClientBuilder().WithObjects(pool, hRanked, hExcluded).Build()
	r := newReconciler(c, scorer, resolver)

	_, err := r.Reconcile(context.Background(), poolRequest("test-pool"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ranked cluster should get new score
	var updatedRanked unstructured.Unstructured
	updatedRanked.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "h-ranked", Namespace: "poolboy"}, &updatedRanked); err != nil {
		t.Fatalf("get h-ranked: %v", err)
	}
	specRanked, _ := placement.ParseHandleSpec(&updatedRanked)
	if specRanked.PreferenceScore == nil || *specRanked.PreferenceScore != 80 {
		t.Errorf("h-ranked score = %v, want 80", specRanked.PreferenceScore)
	}

	// Excluded cluster should get score 0
	var updatedExcluded unstructured.Unstructured
	updatedExcluded.SetGroupVersionKind(placement.ResourceHandleGVK)
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "h-excluded", Namespace: "poolboy"}, &updatedExcluded); err != nil {
		t.Fatalf("get h-excluded: %v", err)
	}
	specExcluded, _ := placement.ParseHandleSpec(&updatedExcluded)
	if specExcluded.PreferenceScore == nil || *specExcluded.PreferenceScore != 0 {
		t.Errorf("h-excluded score = %v, want 0 (cluster excluded)", specExcluded.PreferenceScore)
	}
}
