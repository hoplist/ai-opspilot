package inspection

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
)

const Version = "v1"

type Catalog struct {
	Version string `json:"version"`
	Source  string `json:"source,omitempty"`
	Count   int    `json:"count"`
	Items   []Item `json:"items"`
}

type Item struct {
	Name        string   `json:"name"`
	Cluster     string   `json:"cluster,omitempty"`
	Environment string   `json:"environment,omitempty"`
	Region      string   `json:"region,omitempty"`
	Schedule    string   `json:"schedule,omitempty"`
	Namespaces  []string `json:"namespaces,omitempty"`
	Services    []string `json:"services,omitempty"`
	Flows       []string `json:"flows,omitempty"`
	CheckCount  int      `json:"check_count"`
	Source      string   `json:"source,omitempty"`
}

type RunRequest struct {
	Name    string
	Cluster string
}

type RunResult struct {
	Version         string        `json:"version"`
	Configured      bool          `json:"configured"`
	Ready           bool          `json:"ready"`
	Name            string        `json:"name,omitempty"`
	Cluster         string        `json:"cluster,omitempty"`
	Environment     string        `json:"environment,omitempty"`
	Region          string        `json:"region,omitempty"`
	Schedule        string        `json:"schedule,omitempty"`
	Scope           ScopeResult   `json:"scope,omitempty"`
	Checks          []CheckResult `json:"checks,omitempty"`
	MissingEvidence []string      `json:"missing_evidence,omitempty"`
	Warnings        []string      `json:"warnings,omitempty"`
}

type ScopeResult struct {
	Namespaces []string `json:"namespaces,omitempty"`
	Services   []string `json:"services,omitempty"`
	Flows      []string `json:"flows,omitempty"`
}

type CheckResult struct {
	Name            string         `json:"name"`
	Type            string         `json:"type,omitempty"`
	Enabled         bool           `json:"enabled"`
	Datasource      string         `json:"datasource,omitempty"`
	Flows           []string       `json:"flows,omitempty"`
	Thresholds      map[string]any `json:"thresholds,omitempty"`
	Status          string         `json:"status"`
	MissingEvidence []string       `json:"missing_evidence,omitempty"`
}

type GenerateRequest struct {
	Cluster string
	Service string
}

type GenerateResult struct {
	Version         string                  `json:"version"`
	Ready           bool                    `json:"ready"`
	Cluster         string                  `json:"cluster,omitempty"`
	Service         string                  `json:"service,omitempty"`
	Draft           configloader.Inspection `json:"draft"`
	YAML            string                  `json:"yaml"`
	MissingEvidence []string                `json:"missing_evidence,omitempty"`
	Warnings        []string                `json:"warnings,omitempty"`
}

type Registry struct {
	config      configloader.Config
	inspections []configloader.Inspection
}

func NewRegistry(cfg configloader.Config) *Registry {
	return &Registry{config: cfg, inspections: cfg.Inspections}
}

func (r *Registry) Catalog() Catalog {
	items := make([]Item, 0, len(r.inspections))
	source := "empty"
	for _, item := range r.inspections {
		if source == "empty" && item.Source != "" {
			source = "file"
		}
		items = append(items, Item{
			Name:        item.Name,
			Cluster:     item.Cluster,
			Environment: item.Environment,
			Region:      item.Region,
			Schedule:    item.Schedule,
			Namespaces:  item.Scope.Namespaces,
			Services:    item.Scope.Services,
			Flows:       item.Scope.Flows,
			CheckCount:  len(item.Checks),
			Source:      item.Source,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return Catalog{Version: Version, Source: source, Count: len(items), Items: items}
}

func (r *Registry) Run(req RunRequest) RunResult {
	target := r.find(req.Name, req.Cluster)
	if target == nil {
		return RunResult{
			Version:         Version,
			Configured:      len(r.inspections) > 0,
			Ready:           false,
			Name:            strings.TrimSpace(req.Name),
			Cluster:         strings.TrimSpace(req.Cluster),
			MissingEvidence: []string{"inspection_not_found"},
		}
	}
	out := RunResult{
		Version:     Version,
		Configured:  true,
		Name:        target.Name,
		Cluster:     target.Cluster,
		Environment: target.Environment,
		Region:      target.Region,
		Schedule:    target.Schedule,
		Scope: ScopeResult{
			Namespaces: target.Scope.Namespaces,
			Services:   target.Scope.Services,
			Flows:      target.Scope.Flows,
		},
	}
	for _, check := range target.Checks {
		result := r.inspectCheck(check)
		out.Checks = append(out.Checks, result)
		out.MissingEvidence = append(out.MissingEvidence, result.MissingEvidence...)
	}
	if len(target.Checks) == 0 {
		out.MissingEvidence = append(out.MissingEvidence, "inspection_checks_missing")
	}
	out.MissingEvidence = dedupeStrings(out.MissingEvidence)
	out.Ready = len(out.MissingEvidence) == 0
	if !out.Ready {
		out.Warnings = append(out.Warnings, "inspection run is read-only; missing adapters and evidence gaps are reported without blocking other checks")
	}
	return out
}

func (r *Registry) Generate(req GenerateRequest) GenerateResult {
	cluster := strings.TrimSpace(req.Cluster)
	serviceName := strings.TrimSpace(req.Service)
	if cluster == "" {
		cluster = r.config.Settings.DefaultCluster
	}
	if cluster == "" && len(r.config.Clusters) == 1 {
		cluster = r.config.Clusters[0].Name
	}
	name := "generated-inspection"
	if cluster != "" {
		name = cluster + "-inspection"
	}
	draft := configloader.Inspection{
		Name:     name,
		Cluster:  cluster,
		Schedule: "manual",
		Scope: configloader.InspectionScope{
			Services: compactStrings([]string{serviceName}),
		},
		Checks: []configloader.InspectionCheck{
			{Name: "abnormal-pods", Type: "kubernetes_pods", Enabled: boolPtr(true)},
			{Name: "node-resources", Type: "node_resources", Enabled: boolPtr(true), Thresholds: map[string]any{"cpu_usage_percent": 85, "memory_usage_percent": 85}},
			{Name: "filesystems", Type: "filesystems", Enabled: boolPtr(true), Thresholds: map[string]any{"usage_percent": 85, "free_gib": 20}},
			{Name: "pod-restarts", Type: "pod_restarts", Enabled: boolPtr(true), Thresholds: map[string]any{"restart_count": 3}},
		},
	}
	for _, flow := range r.config.Flows {
		if cluster != "" && flow.Cluster != "" && flow.Cluster != cluster {
			continue
		}
		draft.Scope.Flows = append(draft.Scope.Flows, flow.Name)
	}
	if len(draft.Scope.Flows) > 0 {
		draft.Checks = append(draft.Checks, configloader.InspectionCheck{
			Name:    "flow-health",
			Type:    "flow",
			Enabled: boolPtr(true),
			Flows:   draft.Scope.Flows,
		})
	}
	if ds := firstDatasourceByKind(r.config.Datasources, cluster, "kafka_exporter"); ds != "" {
		draft.Checks = append(draft.Checks, configloader.InspectionCheck{
			Name:       "kafka-lag",
			Type:       "kafka_lag",
			Enabled:    boolPtr(false),
			Datasource: ds,
		})
	}
	out := GenerateResult{
		Version: Version,
		Ready:   cluster != "",
		Cluster: cluster,
		Service: serviceName,
		Draft:   draft,
		YAML:    renderDraftYAML(draft),
	}
	if cluster == "" {
		out.MissingEvidence = append(out.MissingEvidence, "cluster_missing")
		out.Warnings = append(out.Warnings, "set --cluster or configure settings.default_cluster before using this draft")
	}
	return out
}

func (r *Registry) inspectCheck(check configloader.InspectionCheck) CheckResult {
	enabled := check.Enabled == nil || *check.Enabled
	out := CheckResult{
		Name:       check.Name,
		Type:       check.Type,
		Enabled:    enabled,
		Datasource: check.Datasource,
		Flows:      check.Flows,
		Thresholds: check.Thresholds,
		Status:     "not_executed",
	}
	if !enabled {
		out.Status = "disabled"
		return out
	}
	if check.Name == "" {
		out.MissingEvidence = append(out.MissingEvidence, "inspection_check_name_missing")
	}
	if check.Type == "" {
		out.MissingEvidence = append(out.MissingEvidence, "inspection_check_type_missing:"+check.Name)
	}
	switch check.Type {
	case "flow":
		out.Status = "configured"
		flows := check.Flows
		if len(flows) == 0 {
			flows = r.findAllFlowNames()
		}
		for _, flowName := range flows {
			if r.findFlow(flowName) == nil {
				out.MissingEvidence = append(out.MissingEvidence, "flow_not_found:"+flowName)
			}
		}
	case "kafka_lag":
		if check.Datasource == "" {
			out.MissingEvidence = append(out.MissingEvidence, "datasource_missing:"+check.Name)
		} else if r.findDatasource(check.Datasource) == nil {
			out.MissingEvidence = append(out.MissingEvidence, "datasource_not_found:"+check.Datasource)
		}
		out.MissingEvidence = append(out.MissingEvidence, "inspection_check_adapter_not_configured:"+check.Name)
	default:
		out.MissingEvidence = append(out.MissingEvidence, "inspection_check_adapter_not_configured:"+check.Name)
	}
	if len(out.MissingEvidence) > 0 {
		out.Status = "missing_evidence"
	}
	return out
}

func (r *Registry) find(name, cluster string) *configloader.Inspection {
	name = strings.TrimSpace(name)
	cluster = strings.TrimSpace(cluster)
	if name == "" && cluster == "" && len(r.inspections) == 1 {
		return &r.inspections[0]
	}
	for idx := range r.inspections {
		item := &r.inspections[idx]
		if name != "" && item.Name != name {
			continue
		}
		if cluster != "" && item.Cluster != cluster {
			continue
		}
		return item
	}
	return nil
}

func (r *Registry) findFlow(name string) *configloader.Flow {
	for idx := range r.config.Flows {
		if r.config.Flows[idx].Name == name {
			return &r.config.Flows[idx]
		}
	}
	return nil
}

func (r *Registry) findAllFlowNames() []string {
	out := make([]string, 0, len(r.config.Flows))
	for _, item := range r.config.Flows {
		out = append(out, item.Name)
	}
	sort.Strings(out)
	return out
}

func (r *Registry) findDatasource(name string) *configloader.Datasource {
	for idx := range r.config.Datasources {
		if r.config.Datasources[idx].Name == name {
			return &r.config.Datasources[idx]
		}
	}
	return nil
}

func firstDatasourceByKind(items []configloader.Datasource, cluster, kind string) string {
	for _, item := range items {
		if item.Kind != kind {
			continue
		}
		if cluster != "" && item.Cluster != "" && item.Cluster != cluster {
			continue
		}
		return item.Name
	}
	return ""
}

func renderDraftYAML(item configloader.Inspection) string {
	var b strings.Builder
	b.WriteString("apiVersion: opspilot.io/v1\n")
	b.WriteString("kind: Inspection\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + item.Name + "\n")
	b.WriteString("spec:\n")
	b.WriteString("  cluster: " + item.Cluster + "\n")
	b.WriteString("  schedule: " + item.Schedule + "\n")
	if len(item.Scope.Namespaces) > 0 || len(item.Scope.Services) > 0 || len(item.Scope.Flows) > 0 {
		b.WriteString("  scope:\n")
		writeStringList(&b, "    namespaces", item.Scope.Namespaces)
		writeStringList(&b, "    services", item.Scope.Services)
		writeStringList(&b, "    flows", item.Scope.Flows)
	}
	b.WriteString("  checks:\n")
	for _, check := range item.Checks {
		b.WriteString("    - name: " + check.Name + "\n")
		b.WriteString("      type: " + check.Type + "\n")
		b.WriteString("      enabled: " + boolString(check.Enabled == nil || *check.Enabled) + "\n")
		if check.Datasource != "" {
			b.WriteString("      datasource: " + check.Datasource + "\n")
		}
		writeScalarMap(&b, "      thresholds", check.Thresholds)
		writeStringList(&b, "      flows", check.Flows)
	}
	return b.String()
}

func writeStringList(b *strings.Builder, key string, items []string) {
	items = compactStrings(items)
	if len(items) == 0 {
		return
	}
	b.WriteString(key + ":\n")
	itemIndent := strings.Repeat(" ", leadingSpaces(key)+2)
	for _, item := range items {
		b.WriteString(itemIndent + "- " + item + "\n")
	}
}

func writeScalarMap(b *strings.Builder, key string, values map[string]any) {
	if len(values) == 0 {
		return
	}
	keys := make([]string, 0, len(values))
	for item := range values {
		if strings.TrimSpace(item) != "" {
			keys = append(keys, item)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return
	}
	b.WriteString(key + ":\n")
	itemIndent := strings.Repeat(" ", leadingSpaces(key)+2)
	for _, item := range keys {
		b.WriteString(itemIndent + item + ": " + scalarString(values[item]) + "\n")
	}
}

func scalarString(value any) string {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v)
	case fmt.Stringer:
		return strconv.Quote(v.String())
	default:
		return fmt.Sprint(v)
	}
}

func leadingSpaces(value string) int {
	count := 0
	for _, ch := range value {
		if ch != ' ' {
			break
		}
		count++
	}
	return count
}

func dedupeStrings(items []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func compactStrings(items []string) []string {
	out := []string{}
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			out = append(out, strings.TrimSpace(item))
		}
	}
	return out
}

func boolPtr(value bool) *bool {
	return &value
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
