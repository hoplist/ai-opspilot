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
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/response"
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
	mux := http.NewServeMux()
	registerRoutes(mux, client, promRegistry, agentRegistry, logClient, releaseRegistry)
	addr := *host + ":" + *port
	fmt.Printf("opspilot-core %s listening on http://%s\n", version.Version, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func registerRoutes(mux *http.ServeMux, client *k8s.Client, promRegistry *prom.Registry, agentRegistry *nodeagent.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry) {
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
		}, nil, nil
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
			SinceSeconds: intQueryAliases(r, []string{"since_seconds", "since"}, 1800),
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
		return releaseRegistry.Status(ctx, required(q.Get("service"), "service"), client, promRegistry, logClient)
	}))
	mux.HandleFunc("/api/release/jobs", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return releaseRegistry.Jobs(ctx, required(q.Get("service"), "service"))
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
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, response.Error("METHOD_NOT_ALLOWED", "only GET is supported"))
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

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
