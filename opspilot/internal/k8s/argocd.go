package k8s

import (
	"context"
)

func (c *Client) ListArgoApplications(ctx context.Context, namespace string, limit int) (ListResult, error) {
	if namespace == "" {
		namespace = "argocd"
	}
	path := "/apis/argoproj.io/v1alpha1/namespaces/" + namespace + "/applications"
	payload, err := c.json(ctx, path, []string{"get", "applications", "-n", namespace, "-o", "json"})
	if err != nil {
		return ListResult{}, err
	}
	apps := []map[string]any{}
	for _, item := range items(payload) {
		apps = append(apps, ArgoApplicationSummary(item))
	}
	total := len(apps)
	if limit < 0 {
		limit = 0
	}
	if limit == 0 || limit > total {
		limit = total
	}
	return ListResult{Items: apps[:limit], ItemCount: limit, TotalCount: total, Truncated: total > limit}, nil
}

func ArgoApplicationSummary(item map[string]any) map[string]any {
	meta := object(item, "metadata")
	status := object(item, "status")
	spec := object(item, "spec")
	sync := object(status, "sync")
	health := object(status, "health")
	operation := object(status, "operationState")
	destination := object(spec, "destination")
	return map[string]any{
		"namespace":       stringValue(meta, "namespace"),
		"name":            stringValue(meta, "name"),
		"project":         stringValue(spec, "project"),
		"sync_status":     stringValue(sync, "status"),
		"health_status":   stringValue(health, "status"),
		"health_message":  stringValue(health, "message"),
		"revision":        stringValue(sync, "revision"),
		"operation_phase": stringValue(operation, "phase"),
		"message":         stringValue(operation, "message"),
		"dest_namespace":  stringValue(destination, "namespace"),
	}
}
