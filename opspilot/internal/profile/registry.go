package profile

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
)

const Version = "v1"

type Datasource struct {
	Name        string `json:"name"`
	Environment string `json:"environment,omitempty"`
	Region      string `json:"region,omitempty"`
	URL         string `json:"url,omitempty"`
	URLSet      bool   `json:"url_set"`
	Ready       bool   `json:"ready"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	Source      string `json:"source,omitempty"`
}

type Health struct {
	Version         string       `json:"version"`
	Configured      bool         `json:"configured"`
	Ready           bool         `json:"ready"`
	DatasourceCount int          `json:"datasource_count"`
	Datasources     []Datasource `json:"datasources"`
	MissingEvidence []string     `json:"missing_evidence,omitempty"`
}

type LinkRequest struct {
	Source    string
	Service   string
	Namespace string
	Pod       string
	Since     string
}

type LinkResult struct {
	Version         string       `json:"version"`
	Configured      bool         `json:"configured"`
	Ready           bool         `json:"ready"`
	Source          string       `json:"source,omitempty"`
	Query           string       `json:"query,omitempty"`
	Since           string       `json:"since,omitempty"`
	URL             string       `json:"url,omitempty"`
	MissingEvidence []string     `json:"missing_evidence,omitempty"`
	Datasources     []Datasource `json:"datasources,omitempty"`
}

type Registry struct {
	client      *http.Client
	datasources []configloader.Datasource
}

func NewRegistry(cfg configloader.Config) *Registry {
	return &Registry{
		client:      &http.Client{Timeout: 5 * time.Second},
		datasources: parcaDatasources(cfg.Datasources),
	}
}

func (r *Registry) Configured() bool {
	return len(r.datasources) > 0
}

func (r *Registry) Health(ctx context.Context) Health {
	items := make([]Datasource, 0, len(r.datasources))
	ready := false
	for _, item := range r.datasources {
		ds := Datasource{
			Name:        item.Name,
			Environment: item.Environment,
			Region:      item.Region,
			URL:         strings.TrimRight(item.URL, "/"),
			URLSet:      strings.TrimSpace(item.URL) != "",
			Source:      item.Source,
		}
		if !ds.URLSet {
			ds.Status = "missing_url"
			ds.Error = "url is not configured"
			items = append(items, ds)
			continue
		}
		if err := r.check(ctx, ds.URL); err != nil {
			ds.Status = "not_ready"
			ds.Error = err.Error()
			items = append(items, ds)
			continue
		}
		ds.Ready = true
		ds.Status = "ready"
		ready = true
		items = append(items, ds)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	out := Health{
		Version:         Version,
		Configured:      len(items) > 0,
		Ready:           ready,
		DatasourceCount: len(items),
		Datasources:     items,
	}
	if !out.Configured {
		out.MissingEvidence = []string{"profile_evidence_missing: parca datasource not configured"}
	} else if !out.Ready {
		out.MissingEvidence = []string{"profile_evidence_not_ready: parca datasource is configured but unreachable"}
	}
	return out
}

func (r *Registry) Link(ctx context.Context, req LinkRequest) LinkResult {
	health := r.Health(ctx)
	result := LinkResult{
		Version:     Version,
		Configured:  health.Configured,
		Ready:       health.Ready,
		Since:       firstNonEmpty(req.Since, "10m"),
		Datasources: health.Datasources,
	}
	if !health.Configured || !health.Ready {
		result.MissingEvidence = health.MissingEvidence
		return result
	}
	ds := selectDatasource(health.Datasources, req.Source)
	if ds == nil {
		result.MissingEvidence = []string{"profile_datasource_not_found"}
		return result
	}
	query := buildQuery(req)
	if query == "" {
		result.MissingEvidence = []string{"profile_target_missing: provide service, namespace, or pod"}
		return result
	}
	result.Source = ds.Name
	result.Query = query
	result.URL = parcaUIURL(ds.URL, query, result.Since)
	return result
}

func (r *Registry) check(ctx context.Context, baseURL string) error {
	checks := []string{"/-/healthy", "/-/ready", "/"}
	var lastErr error
	for _, suffix := range checks {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+suffix, nil)
		if err != nil {
			return err
		}
		resp, err := r.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return nil
		}
		lastErr = fmt.Errorf("%s returned %s", suffix, resp.Status)
	}
	return lastErr
}

func parcaDatasources(items []configloader.Datasource) []configloader.Datasource {
	out := []configloader.Datasource{}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Kind), "parca") {
			out = append(out, item)
		}
	}
	return out
}

func selectDatasource(items []Datasource, source string) *Datasource {
	source = strings.TrimSpace(source)
	for idx := range items {
		if source == "" || items[idx].Name == source {
			if items[idx].Ready {
				return &items[idx]
			}
		}
	}
	return nil
}

func buildQuery(req LinkRequest) string {
	matchers := []string{}
	if req.Namespace != "" {
		matchers = append(matchers, `namespace="`+escapeMatcher(req.Namespace)+`"`)
	}
	if req.Pod != "" {
		matchers = append(matchers, `pod="`+escapeMatcher(req.Pod)+`"`)
	}
	if req.Service != "" {
		matchers = append(matchers, `service="`+escapeMatcher(req.Service)+`"`)
	}
	if len(matchers) == 0 {
		return ""
	}
	return "{" + strings.Join(matchers, ",") + "}"
}

func parcaUIURL(baseURL, query, since string) string {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return ""
	}
	u.Path = path.Join(u.Path, "/")
	values := u.Query()
	values.Set("query", query)
	values.Set("from", "now-"+firstNonEmpty(since, "10m"))
	values.Set("to", "now")
	u.RawQuery = values.Encode()
	return u.String()
}

func escapeMatcher(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), `"`, `\"`)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
