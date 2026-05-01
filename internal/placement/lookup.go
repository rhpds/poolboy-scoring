package placement

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PlacementLookup resolves a ResourceHandle's cluster placements by
// orchestrating the field extraction functions and the controller-runtime client.
type PlacementLookup struct {
	reader client.Reader
}

// NewLookup creates a PlacementLookup with the given client.Reader.
// In production, pass mgr.GetClient(). In tests, pass a fake client.
func NewLookup(reader client.Reader) *PlacementLookup {
	return &PlacementLookup{reader: reader}
}

// Lookup resolves all cluster placements for a ResourceHandle.
//
// Three-tier strategy (first match wins, but tier 3 collects ALL results):
//  1. Cached status.placements — written by the reconciler on previous runs
//  2. provision_data shortcut — some catalog items propagate placement here
//  3. AnarchySubject GET — fetch each ref and extract placement from job_vars
//
// Returns all successfully resolved placements. A handle with resources on
// multiple clusters returns multiple placements so each cluster gets scored.
func (p *PlacementLookup) Lookup(ctx context.Context, handle *unstructured.Unstructured) ([]Placement, error) {
	if placements, found := GetPlacementsFromStatus(handle); found {
		return placements, nil
	}

	if placement, found := GetPlacementFromProvisionData(handle); found {
		return []Placement{*placement}, nil
	}

	return p.resolveFromAnarchySubjects(ctx, handle)
}

// resolveFromAnarchySubjects fetches all referenced AnarchySubjects and
// extracts placement from each. Returns all successful extractions.
func (p *PlacementLookup) resolveFromAnarchySubjects(ctx context.Context, handle *unstructured.Unstructured) ([]Placement, error) {
	refs, err := GetAnarchySubjectRefs(handle)
	if err != nil {
		return nil, fmt.Errorf("handle %s/%s: %w", handle.GetNamespace(), handle.GetName(), err)
	}

	var placements []Placement
	var lastErr error

	for _, ref := range refs {
		var subject unstructured.Unstructured
		subject.SetGroupVersionKind(AnarchySubjectGVK)

		err := p.reader.Get(ctx, types.NamespacedName{
			Name:      ref.Name,
			Namespace: ref.Namespace,
		}, &subject)
		if err != nil {
			lastErr = fmt.Errorf("fetching AnarchySubject %s/%s for handle %s/%s: %w",
				ref.Namespace, ref.Name, handle.GetNamespace(), handle.GetName(), err)
			continue
		}

		placement, err := ExtractPlacement(&subject)
		if err != nil {
			continue
		}

		placements = append(placements, *placement)
	}

	if len(placements) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("no placements resolved for handle %s/%s: none of the %d AnarchySubject refs had sandbox_openshift_cluster",
			handle.GetNamespace(), handle.GetName(), len(refs))
	}

	return placements, nil
}
