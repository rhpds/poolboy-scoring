package controller

import (
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// newHandle builds a minimal ResourceHandle for predicate testing.
// If bound is true, the handle has a spec.resourceClaim (already claimed).
func newHandle(name string, bound bool, score *float64) *unstructured.Unstructured {
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

	if bound {
		obj.Object["spec"].(map[string]interface{})["resourceClaim"] = map[string]interface{}{
			"name":      "claim-1",
			"namespace": "user-ns",
		}
	}

	if score != nil {
		obj.Object["spec"].(map[string]interface{})["preferenceScore"] = *score
	}

	return obj
}

func float64Ptr(v float64) *float64 {
	return &v
}

// --- BoundHandlePredicate tests ---

func TestBoundPredicate_Create_Unbound(t *testing.T) {
	p := NewBoundHandlePredicate()
	e := event.CreateEvent{Object: newHandle("h1", false, nil)}

	if !p.Create(e) {
		t.Error("Create() = false for unbound handle, want true")
	}
}

func TestBoundPredicate_Create_Bound(t *testing.T) {
	p := NewBoundHandlePredicate()
	e := event.CreateEvent{Object: newHandle("h1", true, nil)}

	if p.Create(e) {
		t.Error("Create() = true for bound handle, want false")
	}
}

func TestBoundPredicate_Update_Unbound(t *testing.T) {
	p := NewBoundHandlePredicate()
	e := event.UpdateEvent{
		ObjectOld: newHandle("h1", false, nil),
		ObjectNew: newHandle("h1", false, nil),
	}

	if !p.Update(e) {
		t.Error("Update() = false for unbound handle, want true")
	}
}

func TestBoundPredicate_Update_Bound(t *testing.T) {
	p := NewBoundHandlePredicate()
	e := event.UpdateEvent{
		ObjectOld: newHandle("h1", false, nil),
		ObjectNew: newHandle("h1", true, nil),
	}

	if p.Update(e) {
		t.Error("Update() = true for bound handle, want false")
	}
}

func TestBoundPredicate_Delete(t *testing.T) {
	p := NewBoundHandlePredicate()
	e := event.DeleteEvent{Object: newHandle("h1", false, nil)}

	if p.Delete(e) {
		t.Error("Delete() = true, want false (nothing to do for deleted handles)")
	}
}

// --- SelfUpdatePredicate tests ---

func TestSelfUpdate_NoStoredScore(t *testing.T) {
	scores := &sync.Map{}
	p := NewSelfUpdatePredicate(scores)

	e := event.UpdateEvent{
		ObjectOld: newHandle("h1", false, nil),
		ObjectNew: newHandle("h1", false, float64Ptr(82.5)),
	}

	if !p.Update(e) {
		t.Error("Update() = false with no stored score, want true (first time seeing this handle)")
	}
}

func TestSelfUpdate_MatchingScore(t *testing.T) {
	scores := &sync.Map{}
	scores.Store("poolboy/h1", 82.5)
	p := NewSelfUpdatePredicate(scores)

	e := event.UpdateEvent{
		ObjectOld: newHandle("h1", false, float64Ptr(65.0)),
		ObjectNew: newHandle("h1", false, float64Ptr(82.5)),
	}

	if p.Update(e) {
		t.Error("Update() = true when score matches stored value, want false (self-update)")
	}
}

func TestSelfUpdate_DifferentScore(t *testing.T) {
	scores := &sync.Map{}
	scores.Store("poolboy/h1", 82.5)
	p := NewSelfUpdatePredicate(scores)

	e := event.UpdateEvent{
		ObjectOld: newHandle("h1", false, float64Ptr(82.5)),
		ObjectNew: newHandle("h1", false, float64Ptr(10.0)),
	}

	if !p.Update(e) {
		t.Error("Update() = false when score differs from stored, want true (external change)")
	}
}

func TestSelfUpdate_Delete(t *testing.T) {
	scores := &sync.Map{}
	p := NewSelfUpdatePredicate(scores)

	e := event.DeleteEvent{Object: newHandle("h1", false, nil)}

	if p.Delete(e) {
		t.Error("Delete() = true, want false")
	}
}

func TestSelfUpdate_ScoreNotFound(t *testing.T) {
	scores := &sync.Map{}
	scores.Store("poolboy/h1", 82.5)
	p := NewSelfUpdatePredicate(scores)

	e := event.UpdateEvent{
		ObjectOld: newHandle("h1", false, float64Ptr(82.5)),
		ObjectNew: newHandle("h1", false, nil),
	}

	if !p.Update(e) {
		t.Error("Update() = false when score field absent, want true (score was removed externally)")
	}
}

func TestBoundPredicate_Create_NonUnstructured(t *testing.T) {
	p := NewBoundHandlePredicate()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}}
	e := event.CreateEvent{Object: pod}

	if !p.Create(e) {
		t.Error("Create() = false for non-Unstructured object, want true (safe fallback)")
	}
}

func TestBoundPredicate_Update_NonUnstructured(t *testing.T) {
	p := NewBoundHandlePredicate()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}}
	e := event.UpdateEvent{
		ObjectOld: pod,
		ObjectNew: pod,
	}

	if !p.Update(e) {
		t.Error("Update() = false for non-Unstructured object, want true (safe fallback)")
	}
}

func TestSelfUpdate_Update_NonUnstructured(t *testing.T) {
	scores := &sync.Map{}
	p := NewSelfUpdatePredicate(scores)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}}
	e := event.UpdateEvent{
		ObjectOld: pod,
		ObjectNew: pod,
	}

	if !p.Update(e) {
		t.Error("Update() = false for non-Unstructured object, want true (safe fallback)")
	}
}
