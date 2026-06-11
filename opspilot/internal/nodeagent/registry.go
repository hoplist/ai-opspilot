package nodeagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const AllHosts = "all"

type DataSource struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"-"`
}

type Registry struct {
	clients     map[string]*Client
	order       []string
	defaultHost string
}

func NewRegistry(defaultHost, rawAgents string) *Registry {
	return NewRegistryWithTokens(defaultHost, rawAgents, "")
}

func NewRegistryWithTokens(defaultHost, rawAgents, rawTokens string) *Registry {
	sources := ParseDataSources(rawAgents)
	tokens := ParseTokenMap(rawTokens)
	clients := map[string]*Client{}
	order := []string{}
	for _, source := range sources {
		if source.Name == "" || source.URL == "" {
			continue
		}
		if _, exists := clients[source.Name]; exists {
			continue
		}
		clients[source.Name] = NewClientWithToken(source.URL, firstNonEmpty(tokens[source.Name], source.Token))
		order = append(order, source.Name)
	}
	if defaultHost == "" && len(order) > 0 {
		defaultHost = order[0]
	}
	if _, ok := clients[defaultHost]; !ok && len(order) > 0 {
		defaultHost = order[0]
	}
	return &Registry{clients: clients, order: order, defaultHost: defaultHost}
}

func ParseDataSources(raw string) []DataSource {
	sources := []DataSource{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, endpoint, ok := strings.Cut(part, "=")
		if !ok {
			name = fmt.Sprintf("host-%d", len(sources)+1)
			endpoint = part
		}
		sources = append(sources, DataSource{
			Name: strings.TrimSpace(name),
			URL:  strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		})
	}
	return sources
}

func ParseTokenMap(raw string) map[string]string {
	tokens := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, token, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		token = strings.TrimSpace(token)
		if name != "" && token != "" {
			tokens[name] = token
		}
	}
	return tokens
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (r *Registry) Configured() bool {
	return r != nil && len(r.clients) > 0
}

func (r *Registry) DefaultHost() string {
	if r == nil {
		return ""
	}
	return r.defaultHost
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
		"configured":   r.Configured(),
		"default_host": r.DefaultHost(),
		"agent_count":  0,
		"agents":       []any{},
	}
	if !r.Configured() {
		return out
	}
	agents := []any{}
	defaultReady := false
	for _, name := range r.order {
		client := r.clients[name]
		health := client.Health(ctx)
		health["name"] = name
		agents = append(agents, health)
		if name == r.defaultHost {
			ready, _ := health["ready"].(bool)
			defaultReady = ready
			out["url"] = health["url"]
			out["status_code"] = health["status_code"]
		}
	}
	out["ready"] = defaultReady
	out["agent_count"] = len(agents)
	out["agents"] = agents
	return out
}

func (r *Registry) Client(host string) (*Client, string, error) {
	if !r.Configured() {
		return nil, "", fmt.Errorf("node agent is not configured")
	}
	if host == "" {
		host = r.defaultHost
	}
	client, ok := r.clients[host]
	if !ok {
		return nil, "", fmt.Errorf("unknown node agent host: %s", host)
	}
	return client, host, nil
}

func (r *Registry) Containers(ctx context.Context, host string) (map[string]any, []string, error) {
	if host == AllHosts {
		items := []any{}
		warnings := []string{}
		for _, name := range r.order {
			result, err := r.clients[name].Containers(ctx)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			for _, item := range anySlice(result["items"]) {
				if mapped, ok := item.(map[string]any); ok {
					mapped["host"] = name
				}
				items = append(items, item)
			}
		}
		return map[string]any{"host": AllHosts, "hosts": r.Names(), "items": items, "item_count": len(items)}, warnings, nil
	}
	client, resolved, err := r.Client(host)
	if err != nil {
		return nil, nil, err
	}
	result, err := client.Containers(ctx)
	if err != nil {
		return nil, nil, err
	}
	result["host"] = resolved
	return result, nil, nil
}

func (r *Registry) Inspect(ctx context.Context, host, container string) (map[string]any, error) {
	client, resolved, err := r.Client(host)
	if err != nil {
		return nil, err
	}
	result, err := client.Inspect(ctx, container)
	if err != nil {
		return nil, err
	}
	result["host"] = resolved
	return result, nil
}

func (r *Registry) Logs(ctx context.Context, req LogRequest) (ContainerLog, error) {
	req = BoundedLogRequest(req)
	client, resolved, err := r.Client(req.Host)
	if err != nil {
		return ContainerLog{}, err
	}
	log, err := client.Logs(ctx, req)
	if err != nil {
		return ContainerLog{}, err
	}
	log.Host = resolved
	return log, nil
}

func (r *Registry) Stats(ctx context.Context, host, container string) (map[string]any, error) {
	client, resolved, err := r.Client(host)
	if err != nil {
		return nil, err
	}
	result, err := client.Stats(ctx, container)
	if err != nil {
		return nil, err
	}
	result["host"] = resolved
	return result, nil
}

func (r *Registry) HostDisk(ctx context.Context, req HostDiskRequest) (map[string]any, []string, error) {
	req = BoundedHostDiskRequest(req)
	if req.Host == AllHosts {
		items := []any{}
		warnings := []string{}
		for _, name := range r.order {
			result, err := r.clients[name].HostDisk(ctx, req)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			result["host"] = name
			items = append(items, result)
		}
		return map[string]any{"host": AllHosts, "hosts": r.Names(), "items": items, "item_count": len(items)}, warnings, nil
	}
	client, resolved, err := r.Client(req.Host)
	if err != nil {
		return nil, nil, err
	}
	result, err := client.HostDisk(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	result["host"] = resolved
	return result, nil, nil
}

func (r *Registry) Diagnose(ctx context.Context, req LogRequest) (map[string]any, []string, error) {
	req = BoundedLogRequest(req)
	inspect, err := r.Inspect(ctx, req.Host, req.Container)
	if err != nil {
		return nil, nil, err
	}
	stats, statsErr := r.Stats(ctx, req.Host, req.Container)
	log, logErr := r.Logs(ctx, req)
	warnings := []string{}
	if statsErr != nil {
		warnings = append(warnings, "stats: "+statsErr.Error())
	}
	if logErr != nil {
		warnings = append(warnings, "logs: "+logErr.Error())
	}
	findings := dockerFindings(inspect, log.Text)
	return map[string]any{
		"target": map[string]any{
			"type":      "docker-container",
			"host":      inspect["host"],
			"container": req.Container,
		},
		"findings": findings,
		"evidence": map[string]any{
			"inspect": inspect,
			"stats":   stats,
			"logs":    log,
		},
	}, warnings, nil
}

func dockerFindings(inspect map[string]any, text string) []string {
	findings := []string{}
	state, _ := inspect["state"].(map[string]any)
	if running, _ := state["running"].(bool); !running {
		findings = append(findings, "Container is not running")
	}
	if exitCode, ok := state["exit_code"].(float64); ok && exitCode != 0 {
		findings = append(findings, fmt.Sprintf("Container exited with code %.0f", exitCode))
	}
	lower := strings.ToLower(text)
	for _, token := range []string{"panic", "fatal", "exception", "error", "oom", "out of memory", "timeout"} {
		if strings.Contains(lower, token) {
			findings = append(findings, "Recent logs contain keyword: "+token)
		}
	}
	if len(findings) == 0 {
		findings = append(findings, "No obvious container failure signal found in first-pass evidence")
	}
	return findings
}

func anySlice(value any) []any {
	if slice, ok := value.([]any); ok {
		return slice
	}
	return []any{}
}
