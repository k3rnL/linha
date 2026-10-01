package engine

import (
	"context"
	"errors"
	"linha/server/internal/domain"
	"linha/server/internal/kube"
)

func executorTemplateName(contextID string) string { return "linha-" + contextID + "-executor" }
func (k *Kubernetes) ensureExecutorTemplate(ctx context.Context, contextID string, template PodTemplate) error {
	pod := cloneTemplate(template)
	pod["apiVersion"] = "v1"
	pod["kind"] = "Pod"
	content := string(domain.JSON(pod))
	labels := map[string]string{"app.kubernetes.io/managed-by": "linha", "linha.io/context": contextID}
	body := map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": executorTemplateName(contextID), "labels": labels}, "immutable": true, "data": map[string]string{"executor.json": content}}
	err := k.Client.Do(ctx, "POST", k.Client.Path("configmaps", ""), body, nil)
	var api *kube.APIError
	if !errors.As(err, &api) || api.Status != 409 {
		return err
	}
	var existing struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Immutable bool              `json:"immutable"`
		Data      map[string]string `json:"data"`
	}
	if err = k.Client.Do(ctx, "GET", k.Client.Path("configmaps", executorTemplateName(contextID)), nil, &existing); err != nil {
		return err
	}
	if !existing.Immutable || existing.Data["executor.json"] != content || existing.Metadata.Labels["linha.io/context"] != contextID || existing.Metadata.Labels["app.kubernetes.io/managed-by"] != "linha" {
		return domain.Conflict("executor template ConfigMap has different ownership or content")
	}
	return nil
}
