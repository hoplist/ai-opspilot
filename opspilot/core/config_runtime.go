package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/flow"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/inspection"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/profile"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
)

type runtimeState struct {
	mu                 sync.RWMutex
	config             configloader.Config
	k8sRegistry        *k8s.Registry
	promRegistry       *prom.Registry
	agentRegistry      *nodeagent.Registry
	flowRegistry       *flow.Registry
	inspectionRegistry *inspection.Registry
	profileRegistry    *profile.Registry
	logClient          *logsearch.Client
	releaseRegistry    *release.Registry
}

type runtimeSnapshot struct {
	config             configloader.Config
	k8sRegistry        *k8s.Registry
	promRegistry       *prom.Registry
	agentRegistry      *nodeagent.Registry
	flowRegistry       *flow.Registry
	inspectionRegistry *inspection.Registry
	profileRegistry    *profile.Registry
	logClient          *logsearch.Client
	releaseRegistry    *release.Registry
}

func loadRuntimeConfig() configloader.Config {
	cfg := configloader.Load(env("OPSPILOT_CONFIG_DIR", ""))
	for _, errText := range cfg.Errors {
		fmt.Fprintln(os.Stderr, "opspilot config error: "+errText)
	}
	for _, warning := range cfg.Warnings {
		fmt.Fprintln(os.Stderr, "opspilot config warning: "+warning)
	}
	return cfg
}

func newRuntimeState(cfg configloader.Config) *runtimeState {
	snap := buildRuntimeSnapshot(cfg)
	return &runtimeState{
		config:             snap.config,
		k8sRegistry:        snap.k8sRegistry,
		promRegistry:       snap.promRegistry,
		agentRegistry:      snap.agentRegistry,
		flowRegistry:       snap.flowRegistry,
		inspectionRegistry: snap.inspectionRegistry,
		profileRegistry:    snap.profileRegistry,
		logClient:          snap.logClient,
		releaseRegistry:    snap.releaseRegistry,
	}
}

func (s *runtimeState) snapshot() runtimeSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return runtimeSnapshot{
		config:             s.config,
		k8sRegistry:        s.k8sRegistry,
		promRegistry:       s.promRegistry,
		agentRegistry:      s.agentRegistry,
		flowRegistry:       s.flowRegistry,
		inspectionRegistry: s.inspectionRegistry,
		profileRegistry:    s.profileRegistry,
		logClient:          s.logClient,
		releaseRegistry:    s.releaseRegistry,
	}
}

func (s *runtimeState) reload(cfg configloader.Config) {
	if !cfg.Valid {
		return
	}
	snap := buildRuntimeSnapshot(cfg)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = snap.config
	s.k8sRegistry = snap.k8sRegistry
	s.promRegistry = snap.promRegistry
	s.agentRegistry = snap.agentRegistry
	s.flowRegistry = snap.flowRegistry
	s.inspectionRegistry = snap.inspectionRegistry
	s.profileRegistry = snap.profileRegistry
	s.logClient = snap.logClient
	s.releaseRegistry = snap.releaseRegistry
}

func buildRuntimeSnapshot(runtimeConfig configloader.Config) runtimeSnapshot {
	return runtimeSnapshot{
		config:             runtimeConfig,
		k8sRegistry:        buildK8sRegistry(runtimeConfig),
		promRegistry:       buildPromRegistry(runtimeConfig),
		agentRegistry:      buildAgentRegistry(runtimeConfig),
		flowRegistry:       flow.NewRegistry(runtimeConfig),
		inspectionRegistry: inspection.NewRegistry(runtimeConfig),
		profileRegistry:    profile.NewRegistry(runtimeConfig),
		logClient:          buildLogClient(runtimeConfig),
		releaseRegistry:    buildReleaseRegistry(runtimeConfig),
	}
}

func startConfigReloadLoop(state *runtimeState, interval time.Duration) {
	if state == nil || interval <= 0 || strings.TrimSpace(env("OPSPILOT_CONFIG_DIR", "")) == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			cfg := loadRuntimeConfig()
			if !cfg.Valid {
				continue
			}
			state.reload(cfg)
		}
	}()
}

func buildK8sRegistry(runtimeConfig configloader.Config) *k8s.Registry {
	clusterCatalogRaw := mergeConfigRaw(env("OPSPILOT_CLUSTER_CATALOG", ""), runtimeConfig.ClusterCatalogRaw(), ";")
	return k8s.NewRegistry(k8s.RegistryConfig{
		CatalogRaw:     clusterCatalogRaw,
		DefaultCluster: configValue(runtimeConfig.Settings.DefaultCluster, env("OPSPILOT_CLUSTER", "")),
		KubeconfigDir:  configValue(runtimeConfig.Settings.KubeconfigDir, env("OPSPILOT_CLUSTER_KUBECONFIG_DIR", "")),
	})
}

func buildPromRegistry(runtimeConfig configloader.Config) *prom.Registry {
	prometheusRaw := mergeConfigRaw(env("OPSPILOT_PROMETHEUS_DATASOURCES", ""), runtimeConfig.PrometheusDatasourcesRaw(), ",")
	return prom.NewRegistry(
		configValue(runtimeConfig.DefaultPrometheusSource(), env("OPSPILOT_PROMETHEUS_DEFAULT_SOURCE", "")),
		env("OPSPILOT_PROMETHEUS_URL", ""),
		prometheusRaw,
	)
}

func buildAgentRegistry(runtimeConfig configloader.Config) *nodeagent.Registry {
	agentsRaw := mergeConfigRaw(env("OPSPILOT_NODE_AGENTS", ""), runtimeConfig.NodeAgentsRaw(), ",")
	tokensRaw := mergeConfigRaw(env("OPSPILOT_NODE_AGENT_TOKENS", ""), runtimeConfig.NodeAgentTokensRaw(), ",")
	return nodeagent.NewRegistryWithTokens(
		configValue(runtimeConfig.DefaultNodeAgent(), env("OPSPILOT_NODE_AGENT_DEFAULT_HOST", "")),
		agentsRaw,
		tokensRaw,
	)
}

func buildLogClient(runtimeConfig configloader.Config) *logsearch.Client {
	logURL, logIndex, correlationConfig, logUser, logPassword := logSearchConfigFrom(runtimeConfig)
	return logsearch.NewClientWithConfigAndAuth(logURL, logIndex, correlationConfig, logUser, logPassword)
}

func buildReleaseRegistry(runtimeConfig configloader.Config) *release.Registry {
	serviceCatalogRaw := mergeConfigRaw(env("OPSPILOT_SERVICE_CATALOG", ""), runtimeConfig.ServiceCatalogRaw(), ";")
	return release.NewRegistryWithCatalog(env("OPSPILOT_RELEASE_SERVICES", ""), serviceCatalogRaw, release.Datasources{
		GitLabURL:     configValue(runtimeConfig.Settings.GitLabURL, env("OPSPILOT_GITLAB_URL", "")),
		GitLabToken:   env("OPSPILOT_GITLAB_TOKEN", ""),
		GitOpsProject: configValue(runtimeConfig.Settings.GitOpsProject, env("OPSPILOT_GITOPS_PROJECT", "")),
		GitOpsRef:     configValue(runtimeConfig.Settings.GitOpsRef, env("OPSPILOT_GITOPS_REF", "main")),
	})
}

func configValue(preferred, fallback string) string {
	if strings.TrimSpace(preferred) != "" {
		return preferred
	}
	return fallback
}

func mergeConfigRaw(legacy, fileRaw, sep string) string {
	legacy = strings.TrimSpace(legacy)
	fileRaw = strings.TrimSpace(fileRaw)
	switch {
	case legacy == "":
		return fileRaw
	case fileRaw == "":
		return legacy
	default:
		return legacy + sep + fileRaw
	}
}

func logSearchConfigFrom(runtime configloader.Config) (string, string, logsearch.CorrelationConfig, string, string) {
	defaults := runtime.LogSearchDefaults()
	routesRaw := mergeConfigRaw(env("OPSPILOT_LOG_CORRELATION_ROUTES", ""), runtime.CorrelationRoutesRaw(), ";")
	correlation := logsearch.CorrelationConfig{
		APISIXIndex:     configValue(defaults.APISIXIndex, env("OPSPILOT_APISIX_INDEX", "")),
		DisableAPISIX:   boolEnv("OPSPILOT_APISIX_DISABLED", false) || !boolEnv("OPSPILOT_APISIX_ENABLED", true),
		ServiceIndex:    configValue(defaults.ServiceIndex, env("OPSPILOT_SERVICE_LOG_INDEX", "")),
		ServiceURIField: configValue(defaults.ServiceURIField, env("OPSPILOT_SERVICE_LOG_URI_FIELD", "")),
		Routes:          logsearch.ParseCorrelationRoutes(routesRaw),
	}
	return configValue(defaults.URL, env("OPSPILOT_LOGSEARCH_URL", "")),
		configValue(defaults.Index, env("OPSPILOT_LOGSEARCH_INDEX", "")),
		correlation,
		defaults.Username,
		defaults.Password
}
