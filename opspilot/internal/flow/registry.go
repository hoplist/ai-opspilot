package flow

import (
	"sort"
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
	Type        string   `json:"type,omitempty"`
	Cluster     string   `json:"cluster,omitempty"`
	Environment string   `json:"environment,omitempty"`
	Region      string   `json:"region,omitempty"`
	Service     string   `json:"service,omitempty"`
	StageCount  int      `json:"stage_count"`
	MatchKeys   []string `json:"match_keys,omitempty"`
	Source      string   `json:"source,omitempty"`
}

type InspectRequest struct {
	Name   string
	Stage  string
	Window string
}

type InspectResult struct {
	Version         string        `json:"version"`
	Configured      bool          `json:"configured"`
	Ready           bool          `json:"ready"`
	Name            string        `json:"name,omitempty"`
	Type            string        `json:"type,omitempty"`
	Cluster         string        `json:"cluster,omitempty"`
	Environment     string        `json:"environment,omitempty"`
	Region          string        `json:"region,omitempty"`
	Service         string        `json:"service,omitempty"`
	Window          string        `json:"window,omitempty"`
	MatchKeys       []string      `json:"match_keys,omitempty"`
	Stages          []StageResult `json:"stages,omitempty"`
	MissingEvidence []string      `json:"missing_evidence,omitempty"`
	Warnings        []string      `json:"warnings,omitempty"`
}

type StageResult struct {
	Name             string              `json:"name"`
	Type             string              `json:"type,omitempty"`
	Service          string              `json:"service,omitempty"`
	Namespace        string              `json:"namespace,omitempty"`
	Workload         string              `json:"workload,omitempty"`
	DefaultContainer string              `json:"default_container,omitempty"`
	Containers       []FlowContainerView `json:"containers,omitempty"`
	Datasource       string              `json:"datasource,omitempty"`
	Topic            string              `json:"topic,omitempty"`
	ConsumerGroup    string              `json:"consumer_group,omitempty"`
	Database         string              `json:"database,omitempty"`
	Table            string              `json:"table,omitempty"`
	Endpoint         string              `json:"endpoint,omitempty"`
	EvidenceSources  []string            `json:"evidence_sources,omitempty"`
	Status           string              `json:"status"`
	MissingEvidence  []string            `json:"missing_evidence,omitempty"`
}

type FlowContainerView struct {
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

type Registry struct {
	flows []configloader.Flow
}

func NewRegistry(cfg configloader.Config) *Registry {
	return &Registry{flows: cfg.Flows}
}

func (r *Registry) Catalog() Catalog {
	items := make([]Item, 0, len(r.flows))
	source := "empty"
	for _, item := range r.flows {
		if source == "empty" && item.Source != "" {
			source = "file"
		}
		items = append(items, Item{
			Name:        item.Name,
			Type:        item.Type,
			Cluster:     item.Cluster,
			Environment: item.Environment,
			Region:      item.Region,
			Service:     item.Service,
			StageCount:  len(item.Stages),
			MatchKeys:   item.MatchKeys,
			Source:      item.Source,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return Catalog{Version: Version, Source: source, Count: len(items), Items: items}
}

func (r *Registry) Inspect(req InspectRequest) InspectResult {
	target := r.find(req.Name)
	if target == nil {
		return InspectResult{
			Version:         Version,
			Configured:      len(r.flows) > 0,
			Ready:           false,
			Name:            strings.TrimSpace(req.Name),
			MissingEvidence: []string{"flow_not_found"},
		}
	}
	window := firstNonEmpty(strings.TrimSpace(req.Window), target.Window.Default, "10m")
	out := InspectResult{
		Version:     Version,
		Configured:  true,
		Name:        target.Name,
		Type:        target.Type,
		Cluster:     target.Cluster,
		Environment: target.Environment,
		Region:      target.Region,
		Service:     target.Service,
		Window:      window,
		MatchKeys:   target.MatchKeys,
	}
	stageFilter := strings.TrimSpace(req.Stage)
	for _, stage := range target.Stages {
		if stageFilter != "" && stage.Name != stageFilter {
			continue
		}
		stageResult := inspectStage(stage)
		out.Stages = append(out.Stages, stageResult)
		out.MissingEvidence = append(out.MissingEvidence, stageResult.MissingEvidence...)
	}
	if len(target.Stages) == 0 {
		out.MissingEvidence = append(out.MissingEvidence, "flow_stages_missing")
	}
	if stageFilter != "" && len(out.Stages) == 0 {
		out.MissingEvidence = append(out.MissingEvidence, "flow_stage_not_found:"+stageFilter)
	}
	out.Ready = len(out.MissingEvidence) == 0
	if !out.Ready {
		out.Warnings = append(out.Warnings, "flow evidence is configuration-only in this phase; unavailable sources are reported as gaps")
	}
	return out
}

func (r *Registry) find(name string) *configloader.Flow {
	name = strings.TrimSpace(name)
	if name == "" && len(r.flows) == 1 {
		return &r.flows[0]
	}
	for idx := range r.flows {
		if r.flows[idx].Name == name {
			return &r.flows[idx]
		}
	}
	return nil
}

func inspectStage(stage configloader.FlowStage) StageResult {
	evidenceSources := evidenceSourceNames(stage.Evidence)
	out := StageResult{
		Name:             stage.Name,
		Type:             stage.Type,
		Service:          stage.Service,
		Namespace:        stage.Namespace,
		Workload:         stage.Workload,
		DefaultContainer: stage.DefaultContainer,
		Datasource:       stage.Datasource,
		Topic:            stage.Topic,
		ConsumerGroup:    stage.ConsumerGroup,
		Database:         stage.Database,
		Table:            stage.Table,
		Endpoint:         stage.Endpoint,
		EvidenceSources:  evidenceSources,
		Status:           "configured",
	}
	for _, item := range stage.Containers {
		out.Containers = append(out.Containers, FlowContainerView{Name: item.Name, Role: item.Role})
	}
	if stage.Name == "" {
		out.MissingEvidence = append(out.MissingEvidence, "stage_name_missing")
	}
	if stage.Type == "" {
		out.MissingEvidence = append(out.MissingEvidence, "stage_type_missing:"+stage.Name)
	}
	if len(evidenceSources) == 0 {
		out.MissingEvidence = append(out.MissingEvidence, "stage_evidence_missing:"+stage.Name)
	}
	if len(stage.Containers) > 1 && stage.DefaultContainer == "" {
		out.MissingEvidence = append(out.MissingEvidence, "stage_default_container_missing:"+stage.Name)
	}
	if len(out.MissingEvidence) > 0 {
		out.Status = "missing_evidence"
	}
	return out
}

func evidenceSourceNames(evidence map[string]any) []string {
	out := make([]string, 0, len(evidence))
	for key := range evidence {
		if strings.TrimSpace(key) != "" {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
