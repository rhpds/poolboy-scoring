package placement

import (
	"context"
	"errors"
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
	status, err := ParseHandleStatus(handle)
	if err != nil {
		return nil, err
	}

	if status != nil {
		// Tier 1: cached placements
		if valid := validPlacements(status.Placements); len(valid) > 0 {
			return valid, nil
		}

		// Tier 2: provision_data shortcut
		if status.Summary != nil {
			if pl, found := PlacementFromProvisionData(status.Summary); found {
				return []Placement{*pl}, nil
			}
		}
	}

	// Tier 3: AnarchySubject GET
	return p.resolveFromAnarchySubjects(ctx, handle, status)
}

// resolveFromAnarchySubjects fetches all referenced AnarchySubjects and
// extracts placement from each. Returns all successful extractions.
func (p *PlacementLookup) resolveFromAnarchySubjects(ctx context.Context, handle *unstructured.Unstructured, status *ResourceHandleStatus) ([]Placement, error) {
	var resources []HandleResource
	if status != nil {
		resources = status.Resources
	}

	refs, err := AnarchySubjectRefsFromResources(resources)
	if err != nil {
		return nil, fmt.Errorf("handle %s/%s: %w", handle.GetNamespace(), handle.GetName(), err)
	}

	var placements []Placement
	var errs []error

	for _, ref := range refs {
		var subject unstructured.Unstructured
		subject.SetGroupVersionKind(AnarchySubjectGVK)

		err := p.reader.Get(ctx, types.NamespacedName{
			Name:      ref.Name,
			Namespace: ref.Namespace,
		}, &subject)
		if err != nil {
			errs = append(errs, fmt.Errorf("fetching AnarchySubject %s/%s for handle %s/%s: %w",
				ref.Namespace, ref.Name, handle.GetNamespace(), handle.GetName(), err))
			continue
		}

		spec, err := ParseAnarchySubjectSpec(&subject)
		if err != nil {
			errs = append(errs, fmt.Errorf("parsing AnarchySubject %s/%s spec: %w",
				ref.Namespace, ref.Name, err))
			continue
		}

		placement, err := ExtractPlacement(spec, subject.GetName())
		if err != nil {
			errs = append(errs, fmt.Errorf("extracting placement from AnarchySubject %s/%s: %w",
				ref.Namespace, ref.Name, err))
			continue
		}

		placements = append(placements, *placement)
	}

	if len(placements) == 0 {
		if len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
		return nil, nil
	}

	return placements, nil
}

// validPlacements filters placements with non-empty ClusterName.
func validPlacements(placements []Placement) []Placement {
	var valid []Placement
	for _, p := range placements {
		if p.ClusterName != "" {
			valid = append(valid, p)
		}
	}
	return valid
}
