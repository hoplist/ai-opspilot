package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/version"
)

func registerSystemRoutes(mux *http.ServeMux, k8sRegistry *k8s.Registry, promRegistry *prom.Registry, agentRegistry *nodeagent.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, qualitySettings release.QualitySettings) {
	defaultClient := k8sRegistry.DefaultClient()
	handleAPI(mux, "/api/live", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return map[string]any{
			"version": version.Version,
			"ready":   true,
		}, nil, nil
	}))
	handleAPI(mux, "/api/health", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		k8sHealth := defaultClient.Health()
		k8sHealth["registry"] = k8sRegistry.Health()
		return map[string]any{
			"version":    version.Version,
			"kubernetes": k8sHealth,
			"prometheus": promRegistry.Health(ctx),
			"node_agent": agentRegistry.Health(ctx),
			"logsearch":  logClient.Health(ctx),
			"release": map[string]any{
				"configured": releaseRegistry.Configured(),
				"services":   releaseRegistry.Services(),
			},
			"quality": map[string]any{
				"enabled":           qualitySettings.Enabled,
				"runner_image":      qualitySettings.RunnerImage,
				"image_pull_secret": qualitySettings.ImagePullSecret,
			},
		}, nil, nil
	}))
	handleAPI(mux, "/api/capabilities", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		client, clusterWarnings, err := k8sClientForRequest(r, k8sRegistry)
		if err != nil {
			return nil, clusterWarnings, err
		}
		data, warnings, err := buildCapabilities(ctx, client, promRegistry, agentRegistry, logClient, releaseRegistry, qualitySettings)
		if data != nil {
			data["cluster"] = client.ClusterName()
		}
		return data, append(clusterWarnings, warnings...), err
	}))
}
