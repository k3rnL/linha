package engine

import (
	"context"
	"errors"
	"fmt"
	"linha/server/internal/domain"
	"linha/server/internal/kube"
	"net/http"
	"net/url"
)

const managedResourceSelector = "app.kubernetes.io/managed-by=linha,linha.io/context,linha.io/instance"

// ListResources discovers existing roots, not names from historical intents.
// Driver/executor children of SparkApplications are removed by Kubernetes GC.
func (k *Kubernetes) ListResources(ctx context.Context) ([]ManagedResource, error) {
	var resources []ManagedResource
	for _, kind := range []string{"pods", "services", "sparkapplications"} {
		query := url.Values{"labelSelector": {managedResourceSelector}, "limit": {"100"}}
		seen := map[string]bool{}
		for {
			var page struct {
				Metadata struct {
					Continue string `json:"continue"`
				} `json:"metadata"`
				Items []struct {
					Metadata struct {
						Name, Namespace, UID string
						Labels               map[string]string
					} `json:"metadata"`
				} `json:"items"`
			}
			err := k.Client.Do(ctx, http.MethodGet, k.Client.Path(kind, "")+"?"+query.Encode(), nil, &page)
			var api *kube.APIError
			if kind == "sparkapplications" && errors.As(err, &api) && api.Status == http.StatusNotFound {
				break // Legacy-only clusters may not have the operator CRD.
			}
			if err != nil {
				return nil, fmt.Errorf("list managed %s: %w", kind, err)
			}
			for _, item := range page.Items {
				m := item.Metadata
				r := ManagedResource{InstanceID: m.Labels["linha.io/instance"], ContextID: m.Labels["linha.io/context"], InstanceKind: "pod", Kind: kind, UID: m.UID}
				if kind == "sparkapplications" {
					r.InstanceKind = "sparkapplication"
				}
				if m.Namespace != k.Client.Namespace || m.Labels["app.kubernetes.io/managed-by"] != "linha" || m.Name != "linha-"+r.InstanceID || !validManagedResource(r) {
					continue
				}
				resources = append(resources, r)
			}
			if page.Metadata.Continue == "" {
				break
			}
			if seen[page.Metadata.Continue] {
				return nil, fmt.Errorf("list managed %s: repeated continuation token", kind)
			}
			seen[page.Metadata.Continue] = true
			query.Set("continue", page.Metadata.Continue)
		}
	}
	return resources, nil
}

func validManagedResource(r ManagedResource) bool {
	// Linha intent IDs are immutable 128-bit hex values. Validate before using
	// labels as URL path components, even though discovery is namespaced.
	if len(r.InstanceID) != 32 || len(r.ContextID) != 32 || r.UID == "" {
		return false
	}
	for _, id := range []string{r.InstanceID, r.ContextID} {
		for _, c := range id {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false
			}
		}
	}
	return (r.Kind == "pods" || r.Kind == "services") && r.InstanceKind == "pod" || r.Kind == "sparkapplications" && r.InstanceKind == "sparkapplication"
}

// DeleteResource uses the UID from inventory, so it never needs another GET.
// A disappearance is success; a replacement (409) requires a fresh inventory.
func (k *Kubernetes) DeleteResource(ctx context.Context, r ManagedResource) error {
	if !validManagedResource(r) {
		return domain.Conflict("invalid managed resource identity")
	}
	body := map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "preconditions": map[string]string{"uid": r.UID}}
	if r.Kind == "sparkapplications" {
		body["propagationPolicy"] = "Foreground"
	}
	err := k.Client.Do(ctx, http.MethodDelete, k.Client.Path(r.Kind, "linha-"+r.InstanceID), body, nil)
	var api *kube.APIError
	if errors.As(err, &api) && api.Status == http.StatusNotFound {
		return nil
	}
	return err
}
