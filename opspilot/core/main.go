package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/catalog"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/intent"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/response"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/version"
)

func main() {
	host := flag.String("host", env("OPSPILOT_HOST", "0.0.0.0"), "listen host")
	port := flag.String("port", env("OPSPILOT_PORT", "18080"), "listen port")
	flag.Parse()

	client := k8s.NewClient()
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
	registerRoutes(mux, client, promRegistry, agentRegistry, logClient, releaseRegistry, errorCollector, qualitySettings)
	addr := *host + ":" + *port
	fmt.Printf("opspilot-core %s listening on http://%s\n", version.Version, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func registerRoutes(mux *http.ServeMux, client *k8s.Client, promRegistry *prom.Registry, agentRegistry *nodeagent.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, errorCollector *errorevidence.Collector, qualitySettings release.QualitySettings) {
	mux.HandleFunc("/api/live", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return map[string]any{
			"version": version.Version,
			"ready":   true,
		}, nil, nil
	}))
	mux.HandleFunc("/api/health", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return map[string]any{
			"version":    version.Version,
			"kubernetes": client.Health(),
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
	mux.HandleFunc("/api/capabilities", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return buildCapabilities(ctx, client, promRegistry, agentRegistry, logClient, releaseRegistry, qualitySettings)
	}))
	mux.HandleFunc("/api/skills/registry", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		catalog, warnings := skillregistry.RegistryFromEnv(q.Get("category"), boolQuery(r, "integrated_only"))
		return catalog, warnings, nil
	}))
	mux.HandleFunc("/api/intent/parse", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return intent.Interpret(intent.Request{
			Query:           required(q.Get("query"), "query"),
			ServiceOverride: q.Get("service"),
			Services:        releaseRegistry.Services(),
		}), nil, nil
	}))
	mux.HandleFunc("/api/credentials/catalog", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		catalog, warnings := catalog.CredentialsFromEnv(env("OPSPILOT_CREDENTIAL_CATALOG", ""))
		return catalog, warnings, nil
	}))
	mux.HandleFunc("/api/clusters/catalog", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		catalog, warnings := catalog.ClustersFromEnv(env("OPSPILOT_CLUSTER_CATALOG", ""))
		return catalog, warnings, nil
	}))
	mux.HandleFunc("/api/errors/recent", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return errorCollector.Recent(ctx, client, releaseRegistry, promRegistry, logClient, errorevidence.Request{
			Source:    q.Get("source"),
			Service:   q.Get("service"),
			Namespace: q.Get("namespace"),
			Limit:     intQuery(r, "limit", 20),
		})
	}))
	mux.HandleFunc("/api/inventory/overview", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return client.InventoryOverview(ctx, intQuery(r, "limit", 10)), nil, nil
	}))
	mux.HandleFunc("/api/metrics/health", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return promRegistry.Health(ctx), nil, nil
	}))
	mux.HandleFunc("/api/metrics/datasources", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return promRegistry.Health(ctx), nil, nil
	}))
	mux.HandleFunc("/api/metrics/query", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := required(r.URL.Query().Get("query"), "query")
		result, warnings, err := promRegistry.QueryRaw(ctx, sourceQuery(r), q)
		return result, warnings, err
	}))
	mux.HandleFunc("/api/metrics/nodes", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		result, warnings, err := promRegistry.NodeMetrics(ctx, sourceQuery(r), intQuery(r, "limit", 20))
		return result, warnings, err
	}))
	mux.HandleFunc("/api/metrics/pods", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, warnings, err := promRegistry.PodMetrics(ctx, sourceQuery(r), q.Get("namespace"), q.Get("sort"), intQuery(r, "limit", 20))
		return result, warnings, err
	}))
	mux.HandleFunc("/api/metrics/containers", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, warnings, err := promRegistry.ContainerMetrics(ctx, sourceQuery(r), q.Get("sort"), intQuery(r, "limit", 20))
		return result, warnings, err
	}))
	mux.HandleFunc("/api/metrics/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, warnings, err := promRegistry.SinglePodMetrics(ctx, sourceQuery(r), required(q.Get("namespace"), "namespace"), required(q.Get("pod"), "pod"))
		return result, warnings, err
	}))
	mux.HandleFunc("/api/k8s/pods", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, err := client.ListPods(ctx, q.Get("namespace"), q.Get("status"), q.Get("q"), intQuery(r, "limit", 100))
		return result, nil, err
	}))
	mux.HandleFunc("/api/k8s/logs/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
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
		return log, nil, err
	}))
	mux.HandleFunc("/api/node-agents", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return agentRegistry.Health(ctx), nil, nil
	}))
	mux.HandleFunc("/api/docker/containers", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		result, warnings, err := agentRegistry.Containers(ctx, hostQuery(r))
		return result, warnings, err
	}))
	mux.HandleFunc("/api/docker/inspect", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, err := agentRegistry.Inspect(ctx, hostQuery(r), required(q.Get("container"), "container"))
		return result, nil, err
	}))
	mux.HandleFunc("/api/docker/logs", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		req := nodeagent.LogRequest{
			Host:         hostQuery(r),
			Container:    required(q.Get("container"), "container"),
			TailLines:    intQueryAliases(r, []string{"tail_lines", "tail"}, 300),
			SinceSeconds: intQueryAliases(r, []string{"since_seconds", "since"}, 1800),
			LimitBytes:   intQuery(r, "limit_bytes", 1024*1024),
			Timestamps:   boolQuery(r, "timestamps"),
		}
		log, err := agentRegistry.Logs(ctx, req)
		return log, nil, err
	}))
	mux.HandleFunc("/api/docker/stats", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, err := agentRegistry.Stats(ctx, hostQuery(r), required(q.Get("container"), "container"))
		return result, nil, err
	}))
	mux.HandleFunc("/api/logs/search", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, err := logClient.Search(ctx, logsearch.SearchRequest{
			Namespace: q.Get("namespace"),
			Pod:       q.Get("pod"),
			Container: q.Get("container"),
			Query:     q.Get("q"),
			Limit:     intQuery(r, "limit", 20),
		})
		return result, nil, err
	}))
	mux.HandleFunc("/api/evidence/request", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, err := logClient.CorrelateRequest(ctx, logsearch.CorrelateRequest{
			Host:            q.Get("host"),
			URI:             required(q.Get("uri"), "uri"),
			At:              q.Get("at"),
			SinceSeconds:    intQueryAliases(r, []string{"since_seconds", "since"}, 900),
			WindowSeconds:   intQueryAliases(r, []string{"window_seconds", "window"}, 300),
			Limit:           intQuery(r, "limit", 20),
			IncludeOptions:  boolQuery(r, "include_options"),
			SkipAPISIX:      boolQuery(r, "skip_apisix") || boolQuery(r, "service_only"),
			APISIXIndex:     q.Get("apisix_index"),
			ServiceIndex:    q.Get("service_index"),
			ServiceURIField: q.Get("service_uri_field"),
		})
		return result, nil, err
	}))
	mux.HandleFunc("/api/release/status", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return releaseRegistry.Status(ctx, required(q.Get("service"), "service"), client, promRegistry, logClient, qualitySettings)
	}))
	mux.HandleFunc("/api/quality/status", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return releaseRegistry.QualityStatus(ctx, required(q.Get("service"), "service"), client, qualitySettings)
	}))
	mux.HandleFunc("/api/quality/run", wrapPost(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		if err := r.ParseForm(); err != nil {
			return nil, nil, requestError{message: "form body is invalid"}
		}
		return releaseRegistry.RunQuality(ctx, required(r.Form.Get("service"), "service"), r.Form.Get("base_url"), client, qualitySettings)
	}))
	mux.HandleFunc("/api/release/jobs", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return releaseRegistry.Jobs(ctx, required(q.Get("service"), "service"))
	}))
	mux.HandleFunc("/api/release/trigger", wrapPost(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		if err := r.ParseForm(); err != nil {
			return nil, nil, requestError{message: "form body is invalid"}
		}
		return releaseRegistry.Trigger(
			ctx,
			required(r.Form.Get("service"), "service"),
			r.Form.Get("ref"),
			releaseVariablesFromForm(r),
		)
	}))
	mux.HandleFunc("/api/release/logs", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return releaseRegistry.JobTrace(
			ctx,
			required(q.Get("service"), "service"),
			int64Query(r, "job_id", 0),
			q.Get("job"),
			intQuery(r, "limit_bytes", 128*1024),
			intQueryAliases(r, []string{"tail_lines", "tail"}, 200),
		)
	}))
	mux.HandleFunc("/api/release/history", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return releaseRegistry.History(ctx, required(q.Get("service"), "service"), intQuery(r, "limit", 10))
	}))
	mux.HandleFunc("/api/release/rollback", wrapPost(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		if err := r.ParseForm(); err != nil {
			return nil, nil, requestError{message: "form body is invalid"}
		}
		return releaseRegistry.Rollback(
			ctx,
			required(r.Form.Get("service"), "service"),
			required(r.Form.Get("to"), "to"),
			boolForm(r, "confirm"),
		)
	}))
	mux.HandleFunc("/api/diagnose/docker", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		req := nodeagent.LogRequest{
			Host:         hostQuery(r),
			Container:    required(q.Get("container"), "container"),
			TailLines:    intQueryAliases(r, []string{"tail_lines", "tail"}, 300),
			SinceSeconds: intQueryAliases(r, []string{"since_seconds", "since"}, 1800),
			LimitBytes:   intQuery(r, "limit_bytes", 1024*1024),
			Timestamps:   boolQuery(r, "timestamps"),
		}
		return agentRegistry.Diagnose(ctx, req)
	}))
	mux.HandleFunc("/api/context/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		namespace := required(q.Get("namespace"), "namespace")
		pod := required(q.Get("pod"), "pod")
		podContext, err := client.PodContext(ctx, namespace, pod)
		if err == nil {
			addPodMetrics(ctx, promRegistry, sourceQuery(r), podContext, namespace, pod)
		}
		return podContext, nil, err
	}))
	mux.HandleFunc("/api/diagnose/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		namespace := required(q.Get("namespace"), "namespace")
		pod := required(q.Get("pod"), "pod")
		diagnosis, err := client.DiagnosePod(ctx, namespace, pod)
		if err == nil {
			addPodMetrics(ctx, promRegistry, sourceQuery(r), diagnosis, namespace, pod)
		}
		return diagnosis, nil, err
	}))
}

type handlerFunc func(context.Context, *http.Request) (any, []string, error)

func wrap(fn handlerFunc) http.HandlerFunc {
	return wrapMethod(http.MethodGet, fn)
}

func wrapPost(fn handlerFunc) http.HandlerFunc {
	return wrapMethod(http.MethodPost, fn)
}

func wrapMethod(method string, fn handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			err, ok := rec.(error)
			if !ok {
				err = fmt.Errorf("%v", rec)
			}
			status := http.StatusInternalServerError
			code := "INTERNAL_ERROR"
			if errors.Is(err, errBadRequest) {
				status = http.StatusBadRequest
				code = "BAD_REQUEST"
			}
			writeJSON(w, status, response.Error(code, err.Error()))
		}()
		if r.Method != method {
			writeJSON(w, http.StatusMethodNotAllowed, response.Error("METHOD_NOT_ALLOWED", "only "+method+" is supported"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		data, warnings, err := fn(ctx, r)
		if err != nil {
			status := http.StatusInternalServerError
			code := "INTERNAL_ERROR"
			if errors.Is(err, errBadRequest) {
				status = http.StatusBadRequest
				code = "BAD_REQUEST"
			}
			writeJSON(w, status, response.Error(code, err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, response.OK(data, warnings))
	}
}

func writeJSON(w http.ResponseWriter, status int, body response.Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

var errBadRequest = errors.New("bad request")

type requestError struct {
	message string
}

func (e requestError) Error() string {
	return e.message
}

func (e requestError) Is(target error) bool {
	return target == errBadRequest
}

func required(value, name string) string {
	if value == "" {
		panic(requestError{message: name + " is required"})
	}
	return value
}

func intQuery(r *http.Request, name string, fallback int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		panic(requestError{message: name + " must be an integer"})
	}
	return value
}

func int64Query(r *http.Request, name string, fallback int64) int64 {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		panic(requestError{message: name + " must be an integer"})
	}
	return value
}

func intQueryAliases(r *http.Request, names []string, fallback int) int {
	for _, name := range names {
		if r.URL.Query().Get(name) != "" {
			return intQuery(r, name, fallback)
		}
	}
	return fallback
}

func boolQuery(r *http.Request, name string) bool {
	raw := r.URL.Query().Get(name)
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func boolForm(r *http.Request, name string) bool {
	raw := r.Form.Get(name)
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func releaseVariablesFromForm(r *http.Request) map[string]string {
	variables := map[string]string{}
	for key, values := range r.Form {
		if !strings.HasPrefix(key, "var.") || len(values) == 0 {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(key, "var."))
		if name == "" {
			continue
		}
		variables[name] = values[len(values)-1]
	}
	return variables
}

func buildCapabilities(ctx context.Context, client *k8s.Client, promRegistry *prom.Registry, agentRegistry *nodeagent.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, qualitySettings release.QualitySettings) (map[string]any, []string, error) {
	warnings := []string{}
	capabilities := []map[string]any{}
	availableEvidence := []string{}
	missingEvidence := []string{}

	add := func(item map[string]any) {
		capabilities = append(capabilities, item)
		if boolMapValue(item, "available") {
			availableEvidence = append(availableEvidence, stringSliceValue(item["available_evidence"])...)
			return
		}
		missingEvidence = append(missingEvidence, stringSliceValue(item["missing_evidence"])...)
	}

	k8sReady := false
	k8sDetails := client.Health()
	if _, err := client.ListPods(ctx, "", "", "", 1); err != nil {
		warnings = append(warnings, "kubernetes: "+err.Error())
		k8sDetails["error"] = err.Error()
	} else {
		k8sReady = true
	}
	add(capabilityItem("kubernetes_api", "Kubernetes API", "core", true, k8sReady,
		[]string{"Pod 状态", "Pod 事件", "Deployment/ReplicaSet/Node 基础信息"},
		[]string{"Kubernetes API 未接入或 RBAC 不足，无法读取 Pod 状态、事件和工作负载。"},
		"Kubernetes API is the minimum Pod troubleshooting source.", k8sDetails))
	add(capabilityItem("pod_logs", "Kubernetes Pod Logs", "core", true, k8sReady,
		[]string{"当前容器日志", "短窗口 kubernetes logs"},
		[]string{"pods/log 权限不可用或 Kubernetes API 不可用，无法读取当前容器日志。"},
		"Pod logs are read on demand through the Kubernetes pods/log API.", map[string]any{"depends_on": "kubernetes_api"}))

	promHealth := promRegistry.Health(ctx)
	promReady := boolMapValue(promHealth, "configured") && boolMapValue(promHealth, "ready")
	if boolMapValue(promHealth, "configured") && !promReady {
		warnings = append(warnings, "prometheus: configured but not ready")
	}
	add(capabilityItem("prometheus_metrics", "Prometheus Metrics", "metrics", boolMapValue(promHealth, "configured"), promReady,
		[]string{"CPU/Memory 当前值和趋势", "Top Pod", "节点资源", "磁盘挂载点"},
		[]string{"Prometheus 未接入或不可达，无法判断 CPU/Memory 趋势、Top Pod、节点资源和磁盘挂载点。"},
		"Prometheus enriches Pod and cluster checks; Kubernetes-only inspection can still continue.", promHealth))

	logHealth := logClient.Health(ctx)
	logReady := boolMapValue(logHealth, "configured") && boolMapValue(logHealth, "ready")
	if boolMapValue(logHealth, "configured") && !logReady {
		warnings = append(warnings, "logsearch: configured but not ready")
	}
	add(capabilityItem("elk_logs", "ELK/OpenSearch Logs", "logs", boolMapValue(logHealth, "configured"), logReady,
		[]string{"历史日志", "已轮转日志", "跨 Pod 日志搜索"},
		[]string{"ELK/OpenSearch 未接入或不可达，无法查询历史日志或已轮转日志。"},
		"Log search is optional; Kubernetes current logs remain available when pod_logs is ready.", logHealth))

	add(capabilityItem("service_logs", "Service Log Correlation", "logs", logClient.ServiceLogsConfigured(), logReady && logClient.ServiceLogsConfigured(),
		[]string{"按接口 URI 查询服务日志", "service-only 初步排查"},
		[]string{"服务日志索引未配置，无法按接口 URI 做服务侧日志关联。"},
		"Service log correlation can work without APISIX when service logs are indexed.", logHealth))
	add(capabilityItem("apisix_logs", "APISIX Access Logs", "gateway", logClient.APISIXConfigured(), logReady && logClient.APISIXConfigured(),
		[]string{"按域名/接口查询网关 access 日志", "网关到服务的链路证据"},
		[]string{"APISIX 日志未配置，无法通过域名或接口做网关链路关联；可继续做 service-only 排查。"},
		"APISIX evidence is optional and should not block Pod-first investigation.", logHealth))

	agentHealth := agentRegistry.Health(ctx)
	agentReady := boolMapValue(agentHealth, "configured") && boolMapValue(agentHealth, "ready")
	if boolMapValue(agentHealth, "configured") && !agentReady {
		warnings = append(warnings, "node agent: configured but not ready")
	}
	add(capabilityItem("docker_agent", "Docker Node Agent", "host", boolMapValue(agentHealth, "configured"), agentReady,
		[]string{"node206 Docker 容器状态", "Docker inspect/stats/logs"},
		[]string{"Docker node-agent 未接入或不可达，无法查询宿主机 Docker 容器。"},
		"Docker agent is only needed for non-Kubernetes containers or host-side checks.", agentHealth))

	releaseHealth := releaseRegistry.Health()
	add(capabilityItem("release_mapping", "Release Service Mapping", "release", boolMapValue(releaseHealth, "configured"), boolMapValue(releaseHealth, "configured"),
		[]string{"服务到 namespace/deployment/image 的映射", "发布状态入口"},
		[]string{"发布服务映射未配置，无法按服务名查询发布链路；仍可按 namespace/pod 排查。"},
		"Release mapping is required for service-level release evidence, not for raw Pod inspection.", releaseHealth))
	add(capabilityItem("gitlab_release", "GitLab/Registry/GitOps Evidence", "release", boolMapValue(releaseHealth, "gitlab_configured"), boolMapValue(releaseHealth, "gitlab_configured"),
		[]string{"GitLab pipeline", "BuildKit job", "Registry tag", "GitOps desired image"},
		[]string{"GitLab token/GitOps 数据源未配置，无法查询 pipeline、BuildKit、Registry 或 GitOps 证据。"},
		"GitLab evidence is required for release and rollback evidence chains.", releaseHealth))
	qualityReady := qualitySettings.Enabled && qualitySettings.RunnerImage != ""
	add(capabilityItem("quality_checks", "Optional API Quality Checks", "release", qualitySettings.Enabled, qualityReady,
		[]string{"Post-deploy API smoke checks", "response-time evidence", "quality Job logs"},
		[]string{"Optional quality runner is not configured; release and Kubernetes inspection continue without API quality evidence."},
		"Quality checks are optional release evidence and do not block core troubleshooting.", map[string]any{
			"enabled":      qualitySettings.Enabled,
			"runner_image": qualitySettings.RunnerImage,
			"pull_secret":  qualitySettings.ImagePullSecret,
			"job_ttl":      qualitySettings.TTLSeconds,
			"deadline":     qualitySettings.DeadlineSeconds,
		}))

	argoDetails := map[string]any{"namespace": "argocd"}
	argoReady := false
	if _, err := client.ListArgoApplications(ctx, "argocd", 1); err != nil {
		warnings = append(warnings, "argocd: "+err.Error())
		argoDetails["error"] = err.Error()
	} else {
		argoReady = true
	}
	add(capabilityItem("argocd", "Argo CD Applications", "release", true, argoReady,
		[]string{"Argo CD sync/health 状态", "GitOps reconciliation 证据"},
		[]string{"Argo CD Application CRD/RBAC 不可用，无法读取 sync/health；Kubernetes Pod 排查仍可继续。"},
		"Argo CD is optional unless release evidence is requested.", argoDetails))

	skillCatalog, skillWarnings := skillregistry.RegistryFromEnv("", true)
	warnings = append(warnings, skillWarnings...)
	add(capabilityItem("skills_registry", "OpsPilot Skills Registry", "ai", true, skillCatalog.IntegratedCount > 0,
		[]string{"AI skill routing metadata", "integrated skill-to-command mapping", "safe follow-up instructions"},
		[]string{"Skills registry is empty, so AI cannot route evidence to domain-specific troubleshooting rules."},
		"Skills registry maps Codex skills into deterministic OpsPilot evidence and command surfaces.", skillregistry.Summary(skillCatalog)))

	return map[string]any{
		"ready":              k8sReady,
		"capabilities":       capabilities,
		"available_evidence": uniqueStringsCore(availableEvidence),
		"missing_evidence":   uniqueStringsCore(missingEvidence),
		"skills":             skillregistry.Summary(skillCatalog),
		"summary": map[string]any{
			"core_ready":       k8sReady,
			"capability_count": len(capabilities),
			"available_count":  countAvailable(capabilities),
			"missing_count":    len(capabilities) - countAvailable(capabilities),
		},
	}, uniqueStringsCore(warnings), nil
}

func capabilityItem(name, label, category string, configured, ready bool, availableEvidence, missingEvidence []string, message string, details map[string]any) map[string]any {
	available := configured && ready
	status := "missing"
	switch {
	case available:
		status = "ready"
	case configured:
		status = "not_ready"
	}
	return map[string]any{
		"name":               name,
		"label":              label,
		"category":           category,
		"configured":         configured,
		"ready":              ready,
		"available":          available,
		"status":             status,
		"available_evidence": availableEvidence,
		"missing_evidence":   missingEvidence,
		"message":            message,
		"details":            details,
	}
}

func sourceQuery(r *http.Request) string {
	return r.URL.Query().Get("source")
}

func hostQuery(r *http.Request) string {
	return r.URL.Query().Get("host")
}

func addPodMetrics(ctx context.Context, promRegistry *prom.Registry, source string, target map[string]any, namespace, pod string) {
	if !promRegistry.Configured() {
		appendWarning(target, "prometheus: not configured")
		return
	}
	metrics, _, err := promRegistry.SinglePodMetrics(ctx, source, namespace, pod)
	if err != nil {
		appendWarning(target, "prometheus: "+err.Error())
		return
	}
	evidence, ok := target["evidence"].(map[string]any)
	if !ok {
		evidence = map[string]any{}
		target["evidence"] = evidence
	}
	evidence["metrics"] = metrics
}

func appendWarning(target map[string]any, warning string) {
	warnings := []string{}
	for _, raw := range toSlice(target["warnings"]) {
		warnings = append(warnings, fmt.Sprint(raw))
	}
	warnings = append(warnings, warning)
	target["warnings"] = warnings
}

func toSlice(value any) []any {
	if slice, ok := value.([]any); ok {
		return slice
	}
	return []any{}
}

func boolMapValue(values map[string]any, key string) bool {
	value, ok := values[key].(bool)
	return ok && value
}

func stringSliceValue(value any) []string {
	out := []string{}
	switch typed := value.(type) {
	case []string:
		out = append(out, typed...)
	case []any:
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				out = append(out, text)
			}
		}
	}
	return out
}

func uniqueStringsCore(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func countAvailable(items []map[string]any) int {
	count := 0
	for _, item := range items {
		if boolMapValue(item, "available") {
			count++
		}
	}
	return count
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
