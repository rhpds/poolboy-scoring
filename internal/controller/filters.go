package controller

import (
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/rhpds/poolboy-scoring/internal/placement"
)

// BoundHandlePredicate skips events for ResourceHandles that are already
// bound to a ResourceClaim. Bound handles are no longer candidates for
// scoring — Poolboy has already selected them.
//
// Embedding predicate.Funcs provides default "return true" behavior for
// all event types. We override only the ones that need filtering.
type BoundHandlePredicate struct {
	predicate.Funcs
}

func NewBoundHandlePredicate() BoundHandlePredicate {
	return BoundHandlePredicate{}
}

func (BoundHandlePredicate) Create(e event.CreateEvent) bool {
	obj, ok := e.Object.(*unstructured.Unstructured)
	if !ok {
		return true
	}
	if placement.IsHandleBound(obj) {
		ctrl.Log.V(2).Info("Skipping bound handle on create",
			"name", obj.GetName(), "namespace", obj.GetNamespace())
		return false
	}
	return true
}

func (BoundHandlePredicate) Update(e event.UpdateEvent) bool {
	obj, ok := e.ObjectNew.(*unstructured.Unstructured)
	if !ok {
		return true
	}
	if placement.IsHandleBound(obj) {
		ctrl.Log.V(2).Info("Skipping bound handle on update",
			"name", obj.GetName(), "namespace", obj.GetNamespace())
		return false
	}
	return true
}

func (BoundHandlePredicate) Delete(_ event.DeleteEvent) bool {
	return false
}

// SelfUpdatePredicate skips update events caused by our own score patches.
// When the reconciler patches spec.preferenceScore, the API server fires
// a MODIFIED event. Without this predicate, that event would trigger
// another reconcile → patch → event → infinite loop.
//
// Detection: the reconciler stores each score it writes in a sync.Map.
// On update, this predicate compares the new score against the stored
// value. If they match, the event is our own patch — skip it.
type SelfUpdatePredicate struct {
	predicate.Funcs
	lastWrittenScores *sync.Map
}

func NewSelfUpdatePredicate(scores *sync.Map) SelfUpdatePredicate {
	return SelfUpdatePredicate{lastWrittenScores: scores}
}

func (p SelfUpdatePredicate) Update(e event.UpdateEvent) bool {
	obj, ok := e.ObjectNew.(*unstructured.Unstructured)
	if !ok {
		return true
	}

	key := obj.GetNamespace() + "/" + obj.GetName()
	stored, ok := p.lastWrittenScores.Load(key)
	if !ok {
		return true
	}

	currentScore, found := placement.GetCurrentScore(obj)
	if !found {
		return true
	}

	if currentScore == stored.(float64) {
		ctrl.Log.V(2).Info("Skipping self-update",
			"name", obj.GetName(), "namespace", obj.GetNamespace(),
			"score", currentScore)
		return false
	}
	return true
}

func (SelfUpdatePredicate) Delete(_ event.DeleteEvent) bool {
	return false
}
