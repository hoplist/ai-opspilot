package main

import (
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/audit"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/evidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
)

func registerRoutes(mux *http.ServeMux, state *runtimeState, errorCollector *errorevidence.Collector, qualitySettings release.QualitySettings, auditRecorder *audit.Recorder, evidenceStore *evidence.Store) {
	setAuditRecorder(auditRecorder)
	registerSystemRoutes(mux, state, qualitySettings)
	registerCatalogRoutes(mux, state)
	registerAuditRoutes(mux, auditRecorder)
	registerAssetRoutes(mux, state)
	registerFlowRoutes(mux, state)
	registerInspectionRoutes(mux, state)
	registerRepoRoutes(mux, state)
	registerKubernetesRoutes(mux, state, errorCollector)
	registerEvidencePackRoutes(mux, state, errorCollector, qualitySettings, evidenceStore)
	registerProbeRoutes(mux, state, evidenceStore)
	registerMetricsRoutes(mux, state)
	registerProfileRoutes(mux, state)
	registerLogAndNodeRoutes(mux, state)
	registerReleaseRoutes(mux, state, qualitySettings)
}
