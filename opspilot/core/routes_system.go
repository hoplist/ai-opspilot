package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/version"
)

func registerSystemRoutes(mux *http.ServeMux, state *runtimeState, qualitySettings release.QualitySettings) {
	handleAPI(mux, "/api/live", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return map[string]any{
			"version": version.Version,
			"ready":   true,
		}, nil, nil
	}))
	handleAPI(mux, "/api/health", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		snap := state.snapshot()
		defaultClient := snap.k8sRegistry.DefaultClient()
		k8sHealth := defaultClient.Health()
		k8sHealth["registry"] = snap.k8sRegistry.Health()
		return map[string]any{
			"version":    version.Version,
			"kubernetes": k8sHealth,
			"prometheus": snap.promRegistry.Health(ctx),
			"node_agent": snap.agentRegistry.Health(ctx),
			"logsearch":  snap.logClient.Health(ctx),
			"release": map[string]any{
				"configured": snap.releaseRegistry.Configured(),
				"services":   snap.releaseRegistry.Services(),
			},
			"quality": map[string]any{
				"enabled":           qualitySettings.Enabled,
				"runner_image":      qualitySettings.RunnerImage,
				"image_pull_secret": qualitySettings.ImagePullSecret,
			},
		}, nil, nil
	}))
	handleAPI(mux, "/api/config/status", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		cfg := state.snapshot().config
		return cfg.Summary(), cfg.Warnings, nil
	}))
	handleAPI(mux, "/api/capabilities", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		snap := state.snapshot()
		client, clusterWarnings, err := k8sClientForRequest(r, snap.k8sRegistry)
		if err != nil {
			return nil, clusterWarnings, err
		}
		data, warnings, err := buildCapabilities(ctx, client, snap.promRegistry, snap.agentRegistry, snap.logClient, snap.releaseRegistry, qualitySettings)
		if data != nil {
			data["cluster"] = client.ClusterName()
		}
		return data, append(clusterWarnings, warnings...), err
	}))
}
