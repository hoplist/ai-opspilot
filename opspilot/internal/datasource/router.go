package datasource

import (
	"sort"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
)

type RouteRequest struct {
	Service string
	Host    string
	Cluster string
	Region  string
	Global  bool
}

type RouteResult struct {
	Status      string           `json:"status"`
	Input       RouteInput       `json:"input"`
	Service     *RouteService    `json:"service,omitempty"`
	Selected    *RouteCandidate  `json:"selected,omitempty"`
	Candidates  []RouteCandidate `json:"candidates"`
	UI          []RouteCandidate `json:"ui_candidates,omitempty"`
	Gaps        []string         `json:"gaps,omitempty"`
	Strategy    []string         `json:"strategy"`
	QueryLimits QueryLimits      `json:"query_limits"`
}

type RouteInput struct {
	Service string `json:"service,omitempty"`
	Host    string `json:"host,omitempty"`
	Cluster string `json:"cluster,omitempty"`
	Region  string `json:"region,omitempty"`
	Global  bool   `json:"global"`
}

type RouteService struct {
	Name       string   `json:"name"`
	Cluster    string   `json:"cluster,omitempty"`
	Namespace  string   `json:"namespace,omitempty"`
	Deployment string   `json:"deployment,omitempty"`
	Domains    []string `json:"domains,omitempty"`
}

type RouteCandidate struct {
	Name             string   `json:"name"`
	Kind             string   `json:"kind"`
	Region           string   `json:"region,omitempty"`
	URLConfigured    bool     `json:"url_configured"`
	Queryable        bool     `json:"queryable"`
	Reason           string   `json:"reason"`
	Confidence       string   `json:"confidence"`
	APISIXIndex      string   `json:"apisix_index,omitempty"`
	ServiceIndexes   []string `json:"service_indexes,omitempty"`
	ServiceURIFields []string `json:"service_uri_fields,omitempty"`
}

type QueryLimits struct {
	DefaultSinceSeconds int    `json:"default_since_seconds"`
	MaxSinceSeconds     int    `json:"max_since_seconds"`
	MaxLimit            int    `json:"max_limit"`
	SearchTimeout       string `json:"search_timeout"`
	GlobalSearch        string `json:"global_search"`
}

func Resolve(cfg configloader.Config, req RouteRequest) RouteResult {
	req = normalizeRequest(req)
	result := RouteResult{
		Input: RouteInput{
			Service: req.Service,
			Host:    req.Host,
			Cluster: req.Cluster,
			Region:  req.Region,
			Global:  req.Global,
		},
		Strategy: []string{
			"service/domain exact match",
			"cluster default log datasource",
			"same-region datasource",
			"neighbor-region datasource",
			"global search only when explicitly requested",
		},
		QueryLimits: QueryLimits{
			DefaultSinceSeconds: logsearch.DefaultSearchSinceSeconds,
			MaxSinceSeconds:     logsearch.MaxSearchSinceSeconds,
			MaxLimit:            logsearch.MaxSearchLimit,
			SearchTimeout:       "5s",
			GlobalSearch:        "explicit",
		},
	}
	service := matchService(cfg.Services, req)
	if service != nil {
		result.Service = &RouteService{
			Name:       service.Name,
			Cluster:    service.Runtime.Cluster,
			Namespace:  service.Runtime.Namespace,
			Deployment: service.Runtime.Deployment,
			Domains:    cleanList(service.Domains),
		}
		if req.Cluster == "" {
			req.Cluster = service.Runtime.Cluster
		}
	}
	cluster := matchCluster(cfg.Clusters, req.Cluster)
	if cluster != nil && req.Region == "" {
		req.Region = datasourceRegion(cfg.Datasources, cluster.Logs)
		result.Input.Region = req.Region
	}
	if req.Region == "" && service != nil {
		req.Region = datasourceRegion(cfg.Datasources, service.Gateway.Datasource)
		result.Input.Region = req.Region
	}

	added := map[string]bool{}
	addByName := func(name, reason, confidence string) {
		ds := datasourceByName(cfg.Datasources, name)
		if ds == nil {
			if name != "" {
				result.Gaps = append(result.Gaps, "datasource_not_found:"+name)
			}
			return
		}
		addCandidate(&result, *ds, service, reason, confidence, added)
	}
	if service != nil && service.Gateway.Datasource != "" {
		addByName(service.Gateway.Datasource, "service gateway datasource", "high")
	}
	if cluster != nil && cluster.Logs != "" {
		addByName(cluster.Logs, "cluster default logs datasource", "high")
	}
	if req.Region != "" {
		addByRegion(&result, cfg.Datasources, service, req.Region, "same region", "medium", added)
		for _, neighbor := range neighborsOf(cfg.Topology, req.Region) {
			addByRegion(&result, cfg.Datasources, service, neighbor, "neighbor region:"+neighbor, "low", added)
		}
	}
	if req.Global {
		addByRegion(&result, cfg.Datasources, service, "", "explicit global search", "low", added)
	}

	result.Candidates, result.UI = splitCandidates(result.Candidates)
	if len(result.Candidates) > 0 {
		result.Status = "ready"
		selected := result.Candidates[0]
		result.Selected = &selected
	} else {
		result.Status = "missing"
		result.Gaps = append(result.Gaps, "log_datasource_route_missing")
	}
	if service == nil && (req.Service != "" || req.Host != "") {
		result.Gaps = append(result.Gaps, "service_catalog_match_missing")
	}
	result.Gaps = dedupe(result.Gaps)
	return result
}

func normalizeRequest(req RouteRequest) RouteRequest {
	req.Service = strings.TrimSpace(req.Service)
	req.Host = strings.ToLower(strings.TrimSpace(req.Host))
	req.Cluster = strings.TrimSpace(req.Cluster)
	req.Region = strings.TrimSpace(req.Region)
	return req
}

func matchService(services []configloader.Service, req RouteRequest) *configloader.Service {
	for idx := range services {
		if req.Service != "" && strings.EqualFold(services[idx].Name, req.Service) {
			return &services[idx]
		}
	}
	if req.Host == "" {
		return nil
	}
	for idx := range services {
		for _, domain := range services[idx].Domains {
			if strings.EqualFold(strings.TrimSpace(domain), req.Host) {
				return &services[idx]
			}
		}
	}
	return nil
}

func matchCluster(clusters []configloader.Cluster, name string) *configloader.Cluster {
	if name == "" {
		return nil
	}
	for idx := range clusters {
		if strings.EqualFold(clusters[idx].Name, name) {
			return &clusters[idx]
		}
	}
	return nil
}

func datasourceByName(items []configloader.Datasource, name string) *configloader.Datasource {
	for idx := range items {
		if strings.EqualFold(items[idx].Name, name) {
			return &items[idx]
		}
	}
	return nil
}

func datasourceRegion(items []configloader.Datasource, name string) string {
	if ds := datasourceByName(items, name); ds != nil {
		return ds.Region
	}
	return ""
}

func addByRegion(result *RouteResult, items []configloader.Datasource, service *configloader.Service, region, reason, confidence string, added map[string]bool) {
	for _, ds := range items {
		if region != "" && !strings.EqualFold(ds.Region, region) {
			continue
		}
		addCandidate(result, ds, service, reason, confidence, added)
	}
}

func addCandidate(result *RouteResult, ds configloader.Datasource, service *configloader.Service, reason, confidence string, added map[string]bool) {
	if ds.Name == "" || added[strings.ToLower(ds.Name)] {
		return
	}
	added[strings.ToLower(ds.Name)] = true
	kind := strings.ToLower(strings.TrimSpace(ds.Kind))
	queryable := kind == "elasticsearch" || kind == "opensearch" || kind == "elk"
	if !queryable && kind != "kibana" {
		return
	}
	candidate := RouteCandidate{
		Name:             ds.Name,
		Kind:             kind,
		Region:           ds.Region,
		URLConfigured:    strings.TrimSpace(ds.URL) != "",
		Queryable:        queryable,
		Reason:           reason,
		Confidence:       confidence,
		APISIXIndex:      ds.Indexes.APISIX,
		ServiceIndexes:   cleanList(append(append([]string{}, ds.Indexes.AppDefault...), ds.Indexes.App...)),
		ServiceURIFields: fieldCandidates(ds.Fields),
	}
	if service != nil {
		if service.Gateway.APISIXIndex != "" {
			candidate.APISIXIndex = service.Gateway.APISIXIndex
		}
		if len(service.Logs.AppIndexes) > 0 {
			candidate.ServiceIndexes = cleanList(service.Logs.AppIndexes)
		}
		if len(service.Logs.MessageFields) > 0 {
			candidate.ServiceURIFields = cleanList(service.Logs.MessageFields)
		}
	}
	if !candidate.URLConfigured {
		result.Gaps = append(result.Gaps, "datasource_url_missing:"+ds.Name)
	}
	if !queryable && kind == "kibana" {
		candidate.Reason = reason + " (ui metadata only)"
	}
	result.Candidates = append(result.Candidates, candidate)
}

func splitCandidates(items []RouteCandidate) ([]RouteCandidate, []RouteCandidate) {
	query := []RouteCandidate{}
	ui := []RouteCandidate{}
	for _, item := range items {
		if item.Queryable {
			query = append(query, item)
			continue
		}
		ui = append(ui, item)
	}
	return query, ui
}

func fieldCandidates(fields map[string]string) []string {
	values := []string{}
	for _, key := range []string{"service_uri", "message", "log", "uri"} {
		if fields != nil && strings.TrimSpace(fields[key]) != "" {
			values = append(values, strings.TrimSpace(fields[key]))
		}
	}
	return cleanList(values)
}

func neighborsOf(items []configloader.Region, region string) []string {
	for _, item := range items {
		if strings.EqualFold(item.Name, region) {
			return cleanList(item.Neighbors)
		}
	}
	return nil
}

func cleanList(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return dedupe(out)
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
