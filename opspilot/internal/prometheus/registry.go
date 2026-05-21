package prometheus

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const AllSources = "all"

type DataSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Registry struct {
	clients       map[string]*Client
	order         []string
	defaultSource string
}

func NewRegistry(defaultSource, singleURL, rawDataSources string) *Registry {
	sources := ParseDataSources(singleURL, rawDataSources)
	clients := map[string]*Client{}
	order := []string{}
	for _, source := range sources {
		if source.Name == "" || source.URL == "" {
			continue
		}
		if _, exists := clients[source.Name]; exists {
			continue
		}
		clients[source.Name] = NewClient(source.URL)
		order = append(order, source.Name)
	}
	if defaultSource == "" && len(order) > 0 {
		defaultSource = order[0]
	}
	if _, ok := clients[defaultSource]; !ok && len(order) > 0 {
		defaultSource = order[0]
	}
	return &Registry{clients: clients, order: order, defaultSource: defaultSource}
}

func ParseDataSources(singleURL, raw string) []DataSource {
	sources := []DataSource{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, endpoint, ok := strings.Cut(part, "=")
		if !ok {
			name = fmt.Sprintf("source-%d", len(sources)+1)
			endpoint = part
		}
		sources = append(sources, DataSource{
			Name: strings.TrimSpace(name),
			URL:  strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		})
	}
	if len(sources) == 0 && strings.TrimSpace(singleURL) != "" {
		sources = append(sources, DataSource{Name: "default", URL: strings.TrimRight(strings.TrimSpace(singleURL), "/")})
	}
	return sources
}

func (r *Registry) Configured() bool {
	return r != nil && len(r.clients) > 0
}

func (r *Registry) DefaultSource() string {
	if r == nil {
		return ""
	}
	return r.defaultSource
}

func (r *Registry) Names() []string {
	if r == nil {
		return []string{}
	}
	names := append([]string{}, r.order...)
	sort.Strings(names)
	return names
}

func (r *Registry) Health(ctx context.Context) map[string]any {
	out := map[string]any{
		"configured":       r.Configured(),
		"default_source":   r.DefaultSource(),
		"datasource_count": 0,
		"datasources":      []any{},
	}
	if !r.Configured() {
		return out
	}
	datasources := []any{}
	defaultReady := false
	for _, name := range r.order {
		client := r.clients[name]
		health := client.Health(ctx)
		health["name"] = name
		datasources = append(datasources, health)
		if name == r.defaultSource {
			ready, _ := health["ready"].(bool)
			defaultReady = ready
			out["url"] = health["url"]
			out["status_code"] = health["status_code"]
		}
	}
	out["ready"] = defaultReady
	out["datasource_count"] = len(datasources)
	out["datasources"] = datasources
	return out
}

func (r *Registry) Client(source string) (*Client, string, error) {
	if !r.Configured() {
		return nil, "", fmt.Errorf("prometheus is not configured")
	}
	if source == "" {
		source = r.defaultSource
	}
	client, ok := r.clients[source]
	if !ok {
		return nil, "", fmt.Errorf("unknown prometheus source: %s", source)
	}
	return client, source, nil
}

func (r *Registry) QueryRaw(ctx context.Context, source, query string) (map[string]any, []string, error) {
	if source == AllSources {
		items := []map[string]any{}
		warnings := []string{}
		for _, name := range r.order {
			client := r.clients[name]
			result, err := client.QueryRaw(ctx, query)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			for _, item := range asItems(result["items"]) {
				item["source"] = name
				items = append(items, item)
			}
		}
		return map[string]any{"source": AllSources, "query": query, "items": items, "item_count": len(items)}, warnings, nil
	}
	client, resolved, err := r.Client(source)
	if err != nil {
		return nil, nil, err
	}
	result, err := client.QueryRaw(ctx, query)
	if err != nil {
		return nil, nil, err
	}
	result["source"] = resolved
	return result, nil, nil
}

func (r *Registry) NodeMetrics(ctx context.Context, source string, limit int) (ListResult, []string, error) {
	if source == AllSources {
		result := r.allNodeMetrics(ctx, limit)
		return result, result.Warnings, nil
	}
	client, resolved, err := r.Client(source)
	if err != nil {
		return ListResult{}, nil, err
	}
	result, err := client.NodeMetrics(ctx, limit)
	if err != nil {
		return ListResult{}, nil, err
	}
	annotateItems(result.Items, resolved)
	result.Source = resolved
	return result, nil, nil
}

func (r *Registry) PodMetrics(ctx context.Context, source, namespace, sortBy string, limit int) (ListResult, []string, error) {
	if source == AllSources {
		result := r.allPodMetrics(ctx, namespace, sortBy, limit)
		return result, result.Warnings, nil
	}
	client, resolved, err := r.Client(source)
	if err != nil {
		return ListResult{}, nil, err
	}
	result, err := client.PodMetrics(ctx, namespace, sortBy, limit)
	if err != nil {
		return ListResult{}, nil, err
	}
	annotateItems(result.Items, resolved)
	result.Source = resolved
	return result, nil, nil
}

func (r *Registry) ContainerMetrics(ctx context.Context, source, sortBy string, limit int) (ListResult, []string, error) {
	if source == AllSources {
		result := r.allContainerMetrics(ctx, sortBy, limit)
		return result, result.Warnings, nil
	}
	client, resolved, err := r.Client(source)
	if err != nil {
		return ListResult{}, nil, err
	}
	result, err := client.ContainerMetrics(ctx, sortBy, limit)
	if err != nil {
		return ListResult{}, nil, err
	}
	annotateItems(result.Items, resolved)
	result.Source = resolved
	return result, nil, nil
}

func (r *Registry) SinglePodMetrics(ctx context.Context, source, namespace, pod string) (map[string]any, []string, error) {
	client, resolved, err := r.Client(source)
	if err != nil {
		return nil, nil, err
	}
	result, err := client.SinglePodMetrics(ctx, namespace, pod)
	if err != nil {
		return nil, nil, err
	}
	result["source"] = resolved
	return result, nil, nil
}

func (r *Registry) allNodeMetrics(ctx context.Context, limit int) ListResult {
	items := []map[string]any{}
	warnings := []string{}
	for _, name := range r.order {
		result, err := r.clients[name].NodeMetrics(ctx, 0)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		annotateItems(result.Items, name)
		items = append(items, result.Items...)
	}
	result := limited(items, limit)
	result.Source = AllSources
	result.Sources = r.Names()
	result.Warnings = warnings
	return result
}

func (r *Registry) allPodMetrics(ctx context.Context, namespace, sortBy string, limit int) ListResult {
	items := []map[string]any{}
	warnings := []string{}
	for _, name := range r.order {
		result, err := r.clients[name].PodMetrics(ctx, namespace, sortBy, 0)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		annotateItems(result.Items, name)
		items = append(items, result.Items...)
	}
	result := limited(items, limit)
	result.Source = AllSources
	result.Sources = r.Names()
	result.Warnings = warnings
	return result
}

func (r *Registry) allContainerMetrics(ctx context.Context, sortBy string, limit int) ListResult {
	items := []map[string]any{}
	warnings := []string{}
	for _, name := range r.order {
		result, err := r.clients[name].ContainerMetrics(ctx, sortBy, 0)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		annotateItems(result.Items, name)
		items = append(items, result.Items...)
	}
	result := limited(items, limit)
	result.Source = AllSources
	result.Sources = r.Names()
	result.Warnings = warnings
	return result
}

func annotateItems(items []map[string]any, source string) {
	for _, item := range items {
		item["source"] = source
	}
}

func asItems(value any) []map[string]any {
	raw, _ := value.([]map[string]any)
	if raw != nil {
		return raw
	}
	items := []map[string]any{}
	for _, item := range toAnySlice(value) {
		if mapped, ok := item.(map[string]any); ok {
			items = append(items, mapped)
		}
	}
	return items
}

func toAnySlice(value any) []any {
	if slice, ok := value.([]any); ok {
		return slice
	}
	return []any{}
}
