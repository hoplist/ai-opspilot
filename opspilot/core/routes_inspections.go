package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/inspection"
)

func registerInspectionRoutes(mux *http.ServeMux, state *runtimeState) {
	handleAPI(mux, "/api/inspections/catalog", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return state.snapshot().inspectionRegistry.Catalog(), nil, nil
	}))
	handleAPI(mux, "/api/inspections/run", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return state.snapshot().inspectionRegistry.Run(inspection.RunRequest{
			Name:    q.Get("name"),
			Cluster: q.Get("cluster"),
		}), nil, nil
	}))
	handleAPI(mux, "/api/inspections/generate", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return state.snapshot().inspectionRegistry.Generate(inspection.GenerateRequest{
			Cluster: q.Get("cluster"),
			Service: q.Get("service"),
		}), nil, nil
	}))
}
