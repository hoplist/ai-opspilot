package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/audit"
)

func registerAuditRoutes(mux *http.ServeMux, recorder *audit.Recorder) {
	if recorder == nil {
		recorder = audit.NewRecorder("")
	}
	handleAPI(mux, "/api/audit/recent", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		result, err := recorder.Recent(audit.Query{
			Limit:   intQuery(r, "limit", 50),
			Actor:   r.URL.Query().Get("actor"),
			Action:  r.URL.Query().Get("action"),
			Risk:    r.URL.Query().Get("risk"),
			Outcome: r.URL.Query().Get("outcome"),
		})
		return result, nil, err
	}))
	handleAPI(mux, "/api/audit/policy", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return audit.Policy(), nil, nil
	}))
}
