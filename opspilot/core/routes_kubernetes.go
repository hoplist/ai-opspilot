package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
)

func registerKubernetesRoutes(mux *http.ServeMux, k8sRegistry *k8s.Registry, promRegistry *prom.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, errorCollector *errorevidence.Collector) {
	mux.HandleFunc("/api/errors/recent", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		client, warnings, err := k8sClientForRequest(r, k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		return errorCollector.Recent(ctx, client, releaseRegistry, promRegistry, logClient, errorevidence.Request{
			Source:    q.Get("source"),
			Service:   q.Get("service"),
			Namespace: q.Get("namespace"),
			Limit:     intQuery(r, "limit", 20),
		})
	}))
	mux.HandleFunc("/api/inventory/overview", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		client, warnings, err := k8sClientForRequest(r, k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		result := client.InventoryOverview(ctx, intQuery(r, "limit", 10))
		result["cluster"] = client.ClusterName()
		return result, warnings, nil
	}))

	mux.HandleFunc("/api/context/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		namespace := required(q.Get("namespace"), "namespace")
		pod := required(q.Get("pod"), "pod")
		client, warnings, err := k8sClientForRequest(r, k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		podContext, err := client.PodContext(ctx, namespace, pod)
		if err == nil {
			addPodMetrics(ctx, promRegistry, sourceQuery(r), podContext, namespace, pod)
		}
		return podContext, warnings, err
	}))
	mux.HandleFunc("/api/diagnose/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		namespace := required(q.Get("namespace"), "namespace")
		pod := required(q.Get("pod"), "pod")
		client, warnings, err := k8sClientForRequest(r, k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		diagnosis, err := client.DiagnosePod(ctx, namespace, pod)
		if err == nil {
			addPodMetrics(ctx, promRegistry, sourceQuery(r), diagnosis, namespace, pod)
		}
		return diagnosis, warnings, err
	}))
	mux.HandleFunc("/api/k8s/pods", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		client, warnings, err := k8sClientForRequest(r, k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		result, err := client.ListPods(ctx, q.Get("namespace"), q.Get("status"), q.Get("q"), intQuery(r, "limit", 100))
		return result, warnings, err
	}))
	mux.HandleFunc("/api/k8s/logs/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		client, warnings, err := k8sClientForRequest(r, k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		req := k8s.LogRequest{
			Namespace:    required(q.Get("namespace"), "namespace"),
			Pod:          required(q.Get("pod"), "pod"),
			Container:    q.Get("container"),
			TailLines:    intQueryAliases(r, []string{"tail_lines", "tail"}, 300),
			SinceSeconds: intQueryAliases(r, []string{"since_seconds", "since"}, k8s.DefaultSinceSeconds),
			LimitBytes:   intQuery(r, "limit_bytes", 1024*1024),
			Previous:     boolQuery(r, "previous"),
			Timestamps:   boolQuery(r, "timestamps"),
		}
		log, err := client.ReadPodLog(ctx, req)
		return log, warnings, err
	}))
}
