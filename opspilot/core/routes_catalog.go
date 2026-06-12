package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/catalog"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/intent"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
)

func registerCatalogRoutes(mux *http.ServeMux, state *runtimeState) {
	handleAPI(mux, "/api/skills/registry", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		catalog, warnings := skillregistry.RegistryFromEnv(q.Get("category"), boolQuery(r, "integrated_only"))
		return catalog, warnings, nil
	}))
	handleAPI(mux, "/api/skills/validate", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return skillregistry.ValidateRuntimeFromEnv(), nil, nil
	}))
	handleAPI(mux, "/api/skills/sources", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return skillregistry.MirrorFromEnv(), nil, nil
	}))
	handleAPI(mux, "/api/skills/candidates", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
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
	handleAPI(mux, "/api/skills/import-plan", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return skillregistry.ImportPlanFromEnv(required(q.Get("name"), "name")), nil, nil
	}))
	handleAPI(mux, "/api/skills/discover", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return skillregistry.ReviewPipelineFromEnv("", boolQuery(r, "include_unsupported")), nil, nil
	}))
	handleAPI(mux, "/api/skills/review", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return skillregistry.ReviewPipelineFromEnv(required(q.Get("name"), "name"), true), nil, nil
	}))
	handleAPI(mux, "/api/skills/recommend", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
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
	handleAPI(mux, "/api/intent/parse", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		snap := state.snapshot()
		return intent.Interpret(intent.Request{
			Query:           required(q.Get("query"), "query"),
			ServiceOverride: q.Get("service"),
			Services:        snap.releaseRegistry.Services(),
		}), nil, nil
	}))
	handleAPI(mux, "/api/credentials/catalog", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		raw := mergeConfigRaw(env("OPSPILOT_CREDENTIAL_CATALOG", ""), state.snapshot().config.CredentialCatalogRaw(), ";")
		credentialCatalog, warnings := catalog.CredentialsFromEnv(raw)
		return credentialCatalog, warnings, nil
	}))
	handleAPI(mux, "/api/credentials/plan", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return catalog.CredentialRegistrationPlan(catalog.RegistrationPlanRequest{
			Type:        "credential",
			Kind:        required(q.Get("kind"), "kind"),
			Name:        q.Get("name"),
			Service:     q.Get("service"),
			Cluster:     q.Get("cluster"),
			Environment: q.Get("environment"),
			Scope:       q.Get("scope"),
			Mode:        q.Get("mode"),
			TTL:         q.Get("ttl"),
		}), nil, nil
	}))
	handleAPI(mux, "/api/credentials/access", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return catalog.DebugAccessPlan(catalog.RegistrationPlanRequest{
			Type:        "credential_access",
			Kind:        required(q.Get("kind"), "kind"),
			Name:        q.Get("name"),
			Service:     q.Get("service"),
			Cluster:     q.Get("cluster"),
			Environment: q.Get("environment"),
			Scope:       q.Get("scope"),
			Mode:        q.Get("mode"),
			TTL:         q.Get("ttl"),
		}), nil, nil
	}))
	handleAPI(mux, "/api/credentials/revoke", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return catalog.CredentialRevokePlan(catalog.RegistrationPlanRequest{
			Type:        "credential_revoke",
			Kind:        q.Get("kind"),
			Name:        q.Get("name"),
			Service:     q.Get("service"),
			Cluster:     q.Get("cluster"),
			Environment: q.Get("environment"),
			Scope:       q.Get("scope"),
		}), nil, nil
	}))
	handleAPI(mux, "/api/credentials/rotate", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return catalog.CredentialRotatePlan(catalog.RegistrationPlanRequest{
			Type:        "credential_rotate",
			Kind:        q.Get("kind"),
			Name:        q.Get("name"),
			Service:     q.Get("service"),
			Cluster:     q.Get("cluster"),
			Environment: q.Get("environment"),
			Scope:       q.Get("scope"),
		}), nil, nil
	}))
	handleAPI(mux, "/api/clusters/catalog", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		raw := mergeConfigRaw(env("OPSPILOT_CLUSTER_CATALOG", ""), state.snapshot().config.ClusterCatalogRaw(), ";")
		clusterCatalog, warnings := catalog.ClustersFromEnv(raw)
		return clusterCatalog, warnings, nil
	}))
	handleAPI(mux, "/api/services/catalog", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		snap := state.snapshot()
		raw := mergeConfigRaw(env("OPSPILOT_SERVICE_CATALOG", ""), snap.config.ServiceCatalogRaw(), ";")
		serviceCatalog, warnings := catalog.ServicesFromEnv(raw, serviceSeedsFromRelease(snap.releaseRegistry))
		return serviceCatalog, warnings, nil
	}))
	handleAPI(mux, "/api/clusters/plan", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return catalog.ClusterRegistrationPlan(catalog.RegistrationPlanRequest{
			Type:        "cluster",
			Kind:        q.Get("kind"),
			Name:        q.Get("name"),
			Cluster:     q.Get("cluster"),
			Environment: q.Get("environment"),
			Scope:       q.Get("scope"),
			Mode:        q.Get("mode"),
		}), nil, nil
	}))
	handleAPI(mux, "/api/datasources/plan", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
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

func serviceSeedsFromRelease(registry *release.Registry) []catalog.ServiceSeed {
	seeds := []catalog.ServiceSeed{}
	if registry == nil {
		return seeds
	}
	for _, item := range registry.ServiceItems() {
		seeds = append(seeds, catalog.ServiceSeed{
			Name:       item.Name,
			Namespace:  item.Namespace,
			Deployment: item.Deployment,
			Container:  item.Container,
			Source:     item.Source,
			Image:      item.Image,
			GitLab:     item.GitLab,
			GitOps:     item.GitOps,
			ArgoCD:     item.ArgoCD,
		})
	}
	return seeds
}
