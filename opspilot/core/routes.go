package main

import (
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
)

func registerRoutes(mux *http.ServeMux, k8sRegistry *k8s.Registry, promRegistry *prom.Registry, agentRegistry *nodeagent.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, errorCollector *errorevidence.Collector, qualitySettings release.QualitySettings) {
	registerSystemRoutes(mux, k8sRegistry, promRegistry, agentRegistry, logClient, releaseRegistry, qualitySettings)
	registerCatalogRoutes(mux, releaseRegistry)
	registerKubernetesRoutes(mux, k8sRegistry, promRegistry, logClient, releaseRegistry, errorCollector)
	registerMetricsRoutes(mux, promRegistry)
	registerLogAndNodeRoutes(mux, agentRegistry, logClient)
	registerReleaseRoutes(mux, k8sRegistry, promRegistry, logClient, releaseRegistry, qualitySettings)
}
