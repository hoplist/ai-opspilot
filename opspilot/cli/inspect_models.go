package main

import (
	"encoding/json"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
)

type apiEnvelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Warnings []string        `json:"warnings"`
}

type metricItem struct {
	Metric map[string]string `json:"metric"`
	Source string            `json:"source"`
	Value  float64           `json:"value"`
}

type metricItemsData struct {
	Items []metricItem `json:"items"`
}

type filesystemRow struct {
	Source   string  `json:"source"`
	Node     string  `json:"node"`
	Mount    string  `json:"mount"`
	Device   string  `json:"device"`
	FSType   string  `json:"fstype"`
	FreeGiB  float64 `json:"free_gib"`
	TotalGiB float64 `json:"total_gib"`
	UsedPct  float64 `json:"used_percent"`
}

type filesystemsResult struct {
	Items []filesystemRow `json:"items"`
	Count int             `json:"item_count"`
}

type fixPlanResult struct {
	TargetType           string                         `json:"target_type"`
	Target               string                         `json:"target"`
	Namespace            string                         `json:"namespace,omitempty"`
	DryRun               bool                           `json:"dry_run"`
	Status               string                         `json:"status"`
	Summary              string                         `json:"summary"`
	Evidence             []evidenceItem                 `json:"evidence"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	LikelyCauses         []likelyCause                  `json:"likely_causes,omitempty"`
	RecommendedActions   []recommendedAction            `json:"recommended_actions"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Warnings             []string                       `json:"warnings,omitempty"`
	Raw                  any                            `json:"raw,omitempty"`
}

type inspectPodResult struct {
	Cluster              string                         `json:"cluster,omitempty"`
	Namespace            string                         `json:"namespace"`
	Pod                  string                         `json:"pod"`
	Node                 string                         `json:"node,omitempty"`
	Status               string                         `json:"status,omitempty"`
	Ready                bool                           `json:"ready"`
	RestartCount         int                            `json:"restart_count"`
	Container            string                         `json:"container,omitempty"`
	SpecImage            string                         `json:"spec_image,omitempty"`
	StatusImage          string                         `json:"status_image,omitempty"`
	ImageID              string                         `json:"image_id,omitempty"`
	CPUCore              float64                        `json:"cpu_cores"`
	MemoryMiB            float64                        `json:"memory_mib"`
	KubernetesLogBytes   int                            `json:"kubernetes_log_bytes"`
	ElasticsearchLogHits int                            `json:"elasticsearch_log_hits"`
	EvidenceGaps         []string                       `json:"evidence_gaps"`
	AvailableEvidence    []string                       `json:"available_evidence,omitempty"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	CapabilityWarnings   []string                       `json:"capability_warnings,omitempty"`
	Findings             []string                       `json:"findings"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Raw                  map[string]any                 `json:"raw,omitempty"`
}

type inspectServiceResult struct {
	Cluster              string                         `json:"cluster,omitempty"`
	Service              string                         `json:"service"`
	Environment          string                         `json:"environment,omitempty"`
	Namespace            string                         `json:"namespace,omitempty"`
	Deployment           string                         `json:"deployment,omitempty"`
	Status               string                         `json:"status,omitempty"`
	Stage                string                         `json:"stage,omitempty"`
	Image                string                         `json:"image,omitempty"`
	PodCount             int                            `json:"pod_count"`
	TotalCPUCore         float64                        `json:"total_cpu_cores"`
	TotalMemoryMiB       float64                        `json:"total_memory_mib"`
	RestartCount         int                            `json:"restart_count"`
	Pods                 []inspectPodResult             `json:"pods,omitempty"`
	ReleaseGaps          []string                       `json:"release_gaps,omitempty"`
	EvidenceGaps         []string                       `json:"evidence_gaps,omitempty"`
	AvailableEvidence    []string                       `json:"available_evidence,omitempty"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	CapabilityWarnings   []string                       `json:"capability_warnings,omitempty"`
	Findings             []string                       `json:"findings"`
	Next                 []string                       `json:"next,omitempty"`
	Warnings             []string                       `json:"warnings,omitempty"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Raw                  map[string]any                 `json:"raw,omitempty"`
}

type inspectClusterResult struct {
	Cluster              string                         `json:"cluster,omitempty"`
	AbnormalPods         map[string]any                 `json:"abnormal_pods"`
	Nodes                []map[string]any               `json:"nodes"`
	TopCPU               []map[string]any               `json:"top_cpu_pods"`
	TopMemory            []map[string]any               `json:"top_memory_pods"`
	Restarts24h          []metricItem                   `json:"restarts_24h"`
	Filesystems          []filesystemRow                `json:"filesystems"`
	AvailableEvidence    []string                       `json:"available_evidence,omitempty"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	CapabilityWarnings   []string                       `json:"capability_warnings,omitempty"`
	Findings             []string                       `json:"findings"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Raw                  map[string]any                 `json:"raw,omitempty"`
}
