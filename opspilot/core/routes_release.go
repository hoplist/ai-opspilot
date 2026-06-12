package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
)

func registerReleaseRoutes(mux *http.ServeMux, state *runtimeState, qualitySettings release.QualitySettings) {
	handleAPI(mux, "/api/release/status", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		snap := state.snapshot()
		if !snap.releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		client, warnings, err := k8sClientForRequest(r, snap.k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		data, moreWarnings, err := snap.releaseRegistry.Status(ctx, required(q.Get("service"), "service"), client, snap.promRegistry, snap.logClient, qualitySettings)
		return data, append(warnings, moreWarnings...), err
	}))
	handleAPI(mux, "/api/quality/status", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		snap := state.snapshot()
		if !snap.releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		client, warnings, err := k8sClientForRequest(r, snap.k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		data, moreWarnings, err := snap.releaseRegistry.QualityStatus(ctx, required(q.Get("service"), "service"), client, qualitySettings)
		return data, append(warnings, moreWarnings...), err
	}))
	handleAPI(mux, "/api/quality/run", wrapPost(func(ctx context.Context, r *http.Request) (any, []string, error) {
		snap := state.snapshot()
		if !snap.releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		if err := r.ParseForm(); err != nil {
			return nil, nil, requestError{message: "form body is invalid"}
		}
		client, warnings, err := k8sClientForRequest(r, snap.k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		data, moreWarnings, err := snap.releaseRegistry.RunQuality(ctx, required(r.Form.Get("service"), "service"), r.Form.Get("base_url"), client, qualitySettings)
		return data, append(warnings, moreWarnings...), err
	}))
	handleAPI(mux, "/api/release/jobs", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		snap := state.snapshot()
		if !snap.releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return snap.releaseRegistry.Jobs(ctx, required(q.Get("service"), "service"))
	}))
	handleAPI(mux, "/api/release/trigger", wrapPost(func(ctx context.Context, r *http.Request) (any, []string, error) {
		snap := state.snapshot()
		if !snap.releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		if err := r.ParseForm(); err != nil {
			return nil, nil, requestError{message: "form body is invalid"}
		}
		return snap.releaseRegistry.Trigger(
			ctx,
			required(r.Form.Get("service"), "service"),
			r.Form.Get("ref"),
			releaseVariablesFromForm(r),
		)
	}))
	handleAPI(mux, "/api/release/logs", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		snap := state.snapshot()
		if !snap.releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return snap.releaseRegistry.JobTrace(
			ctx,
			required(q.Get("service"), "service"),
			int64Query(r, "job_id", 0),
			q.Get("job"),
			intQuery(r, "limit_bytes", 128*1024),
			intQueryAliases(r, []string{"tail_lines", "tail"}, 200),
		)
	}))
	handleAPI(mux, "/api/release/history", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		snap := state.snapshot()
		if !snap.releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return snap.releaseRegistry.History(ctx, required(q.Get("service"), "service"), intQuery(r, "limit", 10))
	}))
	handleAPI(mux, "/api/release/rollback", wrapPost(func(ctx context.Context, r *http.Request) (any, []string, error) {
		snap := state.snapshot()
		if !snap.releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		if err := r.ParseForm(); err != nil {
			return nil, nil, requestError{message: "form body is invalid"}
		}
		return snap.releaseRegistry.Rollback(
			ctx,
			required(r.Form.Get("service"), "service"),
			required(r.Form.Get("to"), "to"),
			boolForm(r, "confirm"),
		)
	}))
}
