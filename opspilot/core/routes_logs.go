package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
)

func registerLogAndNodeRoutes(mux *http.ServeMux, agentRegistry *nodeagent.Registry, logClient *logsearch.Client) {
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
}
