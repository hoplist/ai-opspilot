package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
)

func registerLogAndNodeRoutes(mux *http.ServeMux, state *runtimeState) {
	handleAPI(mux, "/api/node-agents", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return state.snapshot().agentRegistry.Health(ctx), nil, nil
	}))
	handleAPI(mux, "/api/docker/containers", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		result, warnings, err := state.snapshot().agentRegistry.Containers(ctx, hostQuery(r))
		return result, warnings, err
	}))
	handleAPI(mux, "/api/docker/inspect", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, err := state.snapshot().agentRegistry.Inspect(ctx, hostQuery(r), required(q.Get("container"), "container"))
		return result, nil, err
	}))
	handleAPI(mux, "/api/docker/logs", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		req := nodeagent.LogRequest{
			Host:         hostQuery(r),
			Container:    required(q.Get("container"), "container"),
			TailLines:    intQueryAliases(r, []string{"tail_lines", "tail"}, 300),
			SinceSeconds: intQueryAliases(r, []string{"since_seconds", "since"}, 1800),
			LimitBytes:   intQuery(r, "limit_bytes", 1024*1024),
			Timestamps:   boolQuery(r, "timestamps"),
		}
		log, err := state.snapshot().agentRegistry.Logs(ctx, req)
		return log, nil, err
	}))
	handleAPI(mux, "/api/docker/stats", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, err := state.snapshot().agentRegistry.Stats(ctx, hostQuery(r), required(q.Get("container"), "container"))
		return result, nil, err
	}))
	handleAPI(mux, "/api/host/disk", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		result, warnings, err := state.snapshot().agentRegistry.HostDisk(ctx, nodeagent.HostDiskRequest{
			Host:  hostQuery(r),
			Limit: intQuery(r, "limit", nodeagent.DefaultDiskTopLimit),
			Depth: intQuery(r, "depth", nodeagent.DefaultDiskMaxDepth),
		})
		return result, warnings, err
	}))
	handleAPI(mux, "/api/host/network", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		result, warnings, err := state.snapshot().agentRegistry.HostNetwork(ctx, nodeagent.HostNetworkRequest{
			Host:            hostQuery(r),
			Limit:           intQuery(r, "limit", nodeagent.DefaultNetworkTopLimit),
			DurationSeconds: intQuery(r, "duration", nodeagent.DefaultNetworkDurationSeconds),
		})
		return result, warnings, err
	}))
	handleAPI(mux, "/api/logs/search", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, err := state.snapshot().logClient.Search(ctx, logsearch.SearchRequest{
			Namespace:    q.Get("namespace"),
			Pod:          q.Get("pod"),
			Container:    q.Get("container"),
			Query:        q.Get("q"),
			Limit:        intQuery(r, "limit", 20),
			SinceSeconds: intQueryAliases(r, []string{"since_seconds", "since"}, logsearch.DefaultSearchSinceSeconds),
		})
		return result, nil, err
	}))
	handleAPI(mux, "/api/evidence/request", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, err := state.snapshot().logClient.CorrelateRequest(ctx, logsearch.CorrelateRequest{
			Host:            q.Get("host"),
			URI:             q.Get("uri"),
			Status:          q.Get("status"),
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
	handleAPI(mux, "/api/diagnose/docker", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		req := nodeagent.LogRequest{
			Host:         hostQuery(r),
			Container:    required(q.Get("container"), "container"),
			TailLines:    intQueryAliases(r, []string{"tail_lines", "tail"}, 300),
			SinceSeconds: intQueryAliases(r, []string{"since_seconds", "since"}, 1800),
			LimitBytes:   intQuery(r, "limit_bytes", 1024*1024),
			Timestamps:   boolQuery(r, "timestamps"),
		}
		return state.snapshot().agentRegistry.Diagnose(ctx, req)
	}))
}
