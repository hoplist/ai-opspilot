package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/version"
)

func main() {
	host := flag.String("host", env("OPSPILOT_HOST", "0.0.0.0"), "listen host")
	port := flag.String("port", env("OPSPILOT_PORT", "18080"), "listen port")
	flag.Parse()

	k8sRegistry := k8s.NewRegistry(k8s.RegistryConfig{
		CatalogRaw:     env("OPSPILOT_CLUSTER_CATALOG", ""),
		DefaultCluster: env("OPSPILOT_CLUSTER", ""),
		KubeconfigDir:  env("OPSPILOT_CLUSTER_KUBECONFIG_DIR", ""),
	})
	promRegistry := prom.NewRegistry(
		env("OPSPILOT_PROMETHEUS_DEFAULT_SOURCE", ""),
		env("OPSPILOT_PROMETHEUS_URL", ""),
		env("OPSPILOT_PROMETHEUS_DATASOURCES", ""),
	)
	agentRegistry := nodeagent.NewRegistry(
		env("OPSPILOT_NODE_AGENT_DEFAULT_HOST", ""),
		env("OPSPILOT_NODE_AGENTS", ""),
	)
	logClient := logsearch.NewClientWithConfig(
		env("OPSPILOT_LOGSEARCH_URL", ""),
		env("OPSPILOT_LOGSEARCH_INDEX", ""),
		logsearch.CorrelationConfig{
			APISIXIndex:     env("OPSPILOT_APISIX_INDEX", ""),
			DisableAPISIX:   boolEnv("OPSPILOT_APISIX_DISABLED", false) || !boolEnv("OPSPILOT_APISIX_ENABLED", true),
			ServiceIndex:    env("OPSPILOT_SERVICE_LOG_INDEX", ""),
			ServiceURIField: env("OPSPILOT_SERVICE_LOG_URI_FIELD", ""),
			Routes:          logsearch.ParseCorrelationRoutes(env("OPSPILOT_LOG_CORRELATION_ROUTES", "")),
		},
	)
	releaseRegistry := release.NewRegistryWithDatasources(env("OPSPILOT_RELEASE_SERVICES", ""), release.Datasources{
		GitLabURL:     env("OPSPILOT_GITLAB_URL", ""),
		GitLabToken:   env("OPSPILOT_GITLAB_TOKEN", ""),
		GitOpsProject: env("OPSPILOT_GITOPS_PROJECT", ""),
		GitOpsRef:     env("OPSPILOT_GITOPS_REF", "main"),
	})
	qualitySettings := release.QualitySettings{
		Enabled:         boolEnv("OPSPILOT_QUALITY_ENABLED", true),
		RunnerImage:     env("OPSPILOT_QUALITY_RUNNER_IMAGE", ""),
		ImagePullSecret: env("OPSPILOT_QUALITY_IMAGE_PULL_SECRET", ""),
		Ref:             env("OPSPILOT_QUALITY_REF", ""),
		TTLSeconds:      intEnv("OPSPILOT_QUALITY_JOB_TTL_SECONDS", 3600),
		DeadlineSeconds: intEnv("OPSPILOT_QUALITY_DEADLINE_SECONDS", 120),
	}
	errorCollector := errorevidence.NewCollector(env("OPSPILOT_ERROR_EVENT_DIR", "/var/lib/opspilot/error-events"))
	mux := http.NewServeMux()
	registerRoutes(mux, k8sRegistry, promRegistry, agentRegistry, logClient, releaseRegistry, errorCollector, qualitySettings)
	addr := *host + ":" + *port
	fmt.Printf("opspilot-core %s listening on http://%s\n", version.Version, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
