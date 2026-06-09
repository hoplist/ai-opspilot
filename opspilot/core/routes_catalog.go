package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/catalog"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/intent"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
)

func registerCatalogRoutes(mux *http.ServeMux, releaseRegistry *release.Registry) {
	mux.HandleFunc("/api/skills/registry", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		catalog, warnings := skillregistry.RegistryFromEnv(q.Get("category"), boolQuery(r, "integrated_only"))
		return catalog, warnings, nil
	}))
	mux.HandleFunc("/api/skills/validate", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return skillregistry.ValidateDirectory(env("OPSPILOT_SKILLS_DIR", "")), nil, nil
	}))
	mux.HandleFunc("/api/skills/sources", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return skillregistry.MirrorFromEnv(), nil, nil
	}))
	mux.HandleFunc("/api/skills/candidates", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		index := skillregistry.MirrorFromEnv()
		return map[string]any{
			"ready":             index.Ready,
			"root":              index.Root,
			"candidate_count":   index.CandidateCount,
			"unsupported_count": index.UnsupportedCount,
			"candidates":        index.Candidates,
			"unsupported":       index.Unsupported,
			"warnings":          index.Warnings,
		}, nil, nil
	}))
	mux.HandleFunc("/api/skills/import-plan", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return skillregistry.ImportPlanFromEnv(required(q.Get("name"), "name")), nil, nil
	}))
	mux.HandleFunc("/api/skills/discover", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return skillregistry.ReviewPipelineFromEnv("", boolQuery(r, "include_unsupported")), nil, nil
	}))
	mux.HandleFunc("/api/skills/review", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return skillregistry.ReviewPipelineFromEnv(required(q.Get("name"), "name"), true), nil, nil
	}))
	mux.HandleFunc("/api/skills/recommend", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		catalog, warnings := skillregistry.RegistryFromEnv("", true)
		return map[string]any{
			"target_type": q.Get("target_type"),
			"status":      q.Get("status"),
			"items": skillregistry.RecommendFromCatalog(
				catalog,
				q.Get("target_type"),
				q.Get("status"),
				queryList(r, "missing_evidence"),
				queryList(r, "finding"),
			),
			"skills": skillregistry.Summary(catalog),
		}, warnings, nil
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
		credentialCatalog, warnings := catalog.CredentialsFromEnv(env("OPSPILOT_CREDENTIAL_CATALOG", ""))
		return credentialCatalog, warnings, nil
	}))
	mux.HandleFunc("/api/credentials/plan", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return catalog.CredentialRegistrationPlan(catalog.RegistrationPlanRequest{
			Type:        "credential",
			Kind:        required(q.Get("kind"), "kind"),
			Name:        q.Get("name"),
			Service:     q.Get("service"),
			Cluster:     q.Get("cluster"),
			Environment: q.Get("environment"),
			Scope:       q.Get("scope"),
		}), nil, nil
	}))
	mux.HandleFunc("/api/clusters/catalog", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		clusterCatalog, warnings := catalog.ClustersFromEnv(env("OPSPILOT_CLUSTER_CATALOG", ""))
		return clusterCatalog, warnings, nil
	}))
	mux.HandleFunc("/api/datasources/plan", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return catalog.DatasourceRegistrationPlan(catalog.RegistrationPlanRequest{
			Type:        "datasource",
			Kind:        required(q.Get("kind"), "kind"),
			Name:        q.Get("name"),
			Service:     q.Get("service"),
			Cluster:     q.Get("cluster"),
			Environment: q.Get("environment"),
			Scope:       q.Get("scope"),
		}), nil, nil
	}))
}
