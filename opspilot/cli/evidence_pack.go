package main

import (
	"fmt"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
	"strings"
)

type evidenceSubject struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type evidenceItem struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

type likelyCause struct {
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type recommendedAction struct {
	Type        string `json:"type"`
	Target      string `json:"target,omitempty"`
	Instruction string `json:"instruction"`
}

type evidencePack struct {
	Subject              evidenceSubject                `json:"subject"`
	Status               string                         `json:"status"`
	Summary              string                         `json:"summary"`
	Evidence             []evidenceItem                 `json:"evidence"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	LikelyCauses         []likelyCause                  `json:"likely_causes,omitempty"`
	RecommendedActions   []recommendedAction            `json:"recommended_actions,omitempty"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Raw                  any                            `json:"raw,omitempty"`
}

func buildEvidencePack(payload any) evidencePack {
	switch v := payload.(type) {
	case doctorResult:
		return evidencePack{
			Subject:         evidenceSubject{Type: "opspilot", Name: v.BackendURL},
			Status:          statusFromBool(v.Ready),
			Summary:         strings.Join(v.Findings, "; "),
			Evidence:        evidenceItems("doctor", append([]string{fmt.Sprintf("backend_reachable=%t", v.BackendReachable)}, v.AvailableEvidence...)),
			MissingEvidence: v.MissingEvidence,
			LikelyCauses:    causesFromMissing(v.MissingEvidence),
			RecommendedActions: []recommendedAction{
				{Type: "next_check", Target: "cli", Instruction: strings.Join(v.Next, "; ")},
			},
			SkillRecommendations: skillregistry.Recommend("opspilot", statusFromBool(v.Ready), v.MissingEvidence, v.Findings),
		}
	case inspectPodResult:
		status := podEvidenceStatus(v)
		missing := uniqueStrings(append(v.MissingEvidence, v.EvidenceGaps...))
		return evidencePack{
			Subject:         evidenceSubject{Type: "pod", Name: v.Pod, Namespace: v.Namespace},
			Status:          status,
			Summary:         strings.Join(v.Findings, "; "),
			Evidence:        podEvidenceItems(v),
			MissingEvidence: missing,
			LikelyCauses:    podLikelyCauses(v),
			RecommendedActions: []recommendedAction{
				{Type: "next_check", Target: "pod", Instruction: "Review events, recent logs, resource usage, and missing evidence before changing code."},
			},
			SkillRecommendations: skillregistry.Recommend("pod", status, missing, v.Findings),
		}
	case inspectServiceResult:
		status := serviceEvidenceStatus(v)
		missing := uniqueStrings(append(append(v.MissingEvidence, v.EvidenceGaps...), v.ReleaseGaps...))
		actions := []recommendedAction{
			{Type: "code_or_config_review", Target: "repo", Instruction: "If logs or events point to application errors, inspect the service repository and generate a small fix."},
		}
		if next := strings.Join(v.Next, "; "); next != "" {
			actions = append([]recommendedAction{{Type: "next_check", Target: "service", Instruction: next}}, actions...)
		}
		return evidencePack{
			Subject:            evidenceSubject{Type: "service", Name: v.Service, Namespace: v.Namespace},
			Status:             status,
			Summary:            strings.Join(v.Findings, "; "),
			Evidence:           serviceEvidenceItems(v),
			MissingEvidence:    missing,
			LikelyCauses:       serviceLikelyCauses(v),
			RecommendedActions: actions,
			SkillRecommendations: skillregistry.Recommend("service", status, missing,
				append(v.Findings, v.Next...)),
		}
	case inspectClusterResult:
		status := clusterEvidenceStatus(v)
		return evidencePack{
			Subject:         evidenceSubject{Type: "cluster"},
			Status:          status,
			Summary:         strings.Join(v.Findings, "; "),
			Evidence:        clusterEvidenceItems(v),
			MissingEvidence: v.MissingEvidence,
			LikelyCauses:    causesFromMissing(v.MissingEvidence),
			RecommendedActions: []recommendedAction{
				{Type: "next_check", Target: "cluster", Instruction: "Inspect abnormal Pods, high restart containers, and high filesystem or memory usage first."},
			},
			SkillRecommendations: skillregistry.Recommend("cluster", status, v.MissingEvidence, v.Findings),
		}
	case fixPlanResult:
		return evidencePack{
			Subject:              evidenceSubject{Type: v.TargetType, Name: v.Target, Namespace: v.Namespace},
			Status:               v.Status,
			Summary:              v.Summary,
			Evidence:             v.Evidence,
			MissingEvidence:      v.MissingEvidence,
			LikelyCauses:         v.LikelyCauses,
			RecommendedActions:   v.RecommendedActions,
			SkillRecommendations: v.SkillRecommendations,
		}
	case map[string]any:
		if report := mapValue(v, "report"); report != nil || (boolValue(v["optional"]) && strings.HasPrefix(stringValue(v["reason"]), "quality_")) || strings.Contains(stringValue(v["job_name"]), "quality") || mapValue(v, "job") != nil {
			status := firstNonEmptyString(stringValue(v["status"]), stringValue(report["status"]), "unknown")
			summary := firstNonEmptyString(stringValue(report["summary"]), stringValue(v["reason"]), "Optional API quality evidence.")
			evidence := []evidenceItem{
				{Source: "quality", Message: fmt.Sprintf("status=%s optional=%t", status, boolValue(v["optional"]))},
			}
			if report != nil {
				evidence = append(evidence, evidenceItem{Source: "quality_report", Message: fmt.Sprintf("checks=%d passed=%d failed=%d duration=%dms",
					intValue(report["check_count"]), intValue(report["passed_count"]), intValue(report["failed_count"]), intValue(report["duration_ms"]))})
			}
			actions := []recommendedAction{
				{Type: "next_check", Target: "service", Instruction: "Use quality report together with release status, Pod logs, metrics, and events before changing code."},
			}
			if status == "failed" {
				actions = append(actions, recommendedAction{Type: "code_or_config_review", Target: "api", Instruction: "Inspect the failing endpoint, route, health check, service port, and application startup path."})
			}
			return evidencePack{
				Subject:            evidenceSubject{Type: "quality", Name: stringValue(v["service"]), Namespace: stringValue(v["namespace"])},
				Status:             status,
				Summary:            summary,
				Evidence:           evidence,
				RecommendedActions: actions,
				Raw:                v,
			}
		}
		return evidencePack{
			Subject: evidenceSubject{Type: "api_response", Name: firstNonEmptyString(stringValue(v["service"]), stringValue(v["name"]))},
			Status:  firstNonEmptyString(stringValue(v["status"]), "unknown"),
			Summary: firstNonEmptyString(stringValue(v["summary"]), "Raw API response evidence."),
			Evidence: []evidenceItem{
				{Source: "api", Message: "Raw response is available in raw."},
			},
			MissingEvidence: stringList(v["gaps"]),
			Raw:             v,
		}
	default:
		return evidencePack{
			Subject: evidenceSubject{Type: "unknown"},
			Status:  "unknown",
			Summary: "Raw payload evidence.",
			Evidence: []evidenceItem{
				{Source: "payload", Message: "Raw payload is available in raw."},
			},
			Raw: payload,
		}
	}
}

func statusFromBool(ok bool) string {
	if ok {
		return "healthy"
	}
	return "degraded"
}

func evidenceItems(source string, messages []string) []evidenceItem {
	out := []evidenceItem{}
	for _, message := range messages {
		if strings.TrimSpace(message) != "" {
			out = append(out, evidenceItem{Source: source, Message: message})
		}
	}
	return out
}

func evidenceItemMessages(items []evidenceItem) []string {
	messages := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Message) != "" {
			messages = append(messages, item.Message)
		}
	}
	return messages
}

func podEvidenceStatus(v inspectPodResult) string {
	if v.Ready && v.RestartCount == 0 {
		return "healthy"
	}
	if v.Ready {
		return "degraded"
	}
	return "unhealthy"
}

func serviceEvidenceStatus(v inspectServiceResult) string {
	if v.Status == "healthy" && v.RestartCount == 0 {
		return "healthy"
	}
	if v.Status == "" {
		return "unknown"
	}
	return v.Status
}

func clusterEvidenceStatus(v inspectClusterResult) string {
	if len(v.Findings) == 0 || (len(v.Findings) == 1 && strings.Contains(v.Findings[0], "No abnormal Pods")) {
		return "healthy"
	}
	return "degraded"
}

func podEvidenceItems(v inspectPodResult) []evidenceItem {
	items := []evidenceItem{
		{Source: "kubernetes_pod", Message: fmt.Sprintf("status=%s ready=%t restarts=%d node=%s", v.Status, v.Ready, v.RestartCount, v.Node)},
		{Source: "metrics", Message: fmt.Sprintf("cpu=%.3f cores memory=%.1f MiB", v.CPUCore, v.MemoryMiB)},
		{Source: "logs", Message: fmt.Sprintf("kubernetes_log_bytes=%d elk_hits=%d", v.KubernetesLogBytes, v.ElasticsearchLogHits)},
	}
	for _, finding := range v.Findings {
		items = append(items, evidenceItem{Source: "finding", Message: finding})
	}
	return items
}

func serviceEvidenceItems(v inspectServiceResult) []evidenceItem {
	items := []evidenceItem{
		{Source: "release", Message: fmt.Sprintf("status=%s stage=%s namespace=%s deployment=%s", v.Status, v.Stage, v.Namespace, v.Deployment)},
		{Source: "workload", Message: fmt.Sprintf("pods=%d restarts=%d cpu=%.3f cores memory=%.1f MiB", v.PodCount, v.RestartCount, v.TotalCPUCore, v.TotalMemoryMiB)},
	}
	if v.Image != "" {
		items = append(items, evidenceItem{Source: "image", Message: v.Image})
	}
	for _, finding := range v.Findings {
		items = append(items, evidenceItem{Source: "finding", Message: finding})
	}
	for _, pod := range v.Pods {
		items = append(items, evidenceItem{Source: "pod", Message: fmt.Sprintf("%s/%s status=%s ready=%t restarts=%d", pod.Namespace, pod.Pod, pod.Status, pod.Ready, pod.RestartCount)})
	}
	return items
}

func clusterEvidenceItems(v inspectClusterResult) []evidenceItem {
	items := []evidenceItem{
		{Source: "cluster", Message: fmt.Sprintf("nodes=%d top_cpu_pods=%d top_memory_pods=%d filesystems=%d", len(v.Nodes), len(v.TopCPU), len(v.TopMemory), len(v.Filesystems))},
	}
	for _, finding := range v.Findings {
		items = append(items, evidenceItem{Source: "finding", Message: finding})
	}
	return items
}

func podLikelyCauses(v inspectPodResult) []likelyCause {
	causes := []likelyCause{}
	if !v.Ready {
		causes = append(causes, likelyCause{Type: "runtime_or_configuration", Confidence: 0.7, Reason: "Pod is not ready."})
	}
	if v.RestartCount > 0 {
		causes = append(causes, likelyCause{Type: "application_crash_or_probe_failure", Confidence: 0.75, Reason: "Pod has restarts."})
	}
	return append(causes, causesFromMissing(v.EvidenceGaps)...)
}

func serviceLikelyCauses(v inspectServiceResult) []likelyCause {
	causes := []likelyCause{}
	if v.Status != "" && v.Status != "healthy" {
		causes = append(causes, likelyCause{Type: "release_or_rollout", Confidence: 0.75, Reason: "Release status is " + v.Status + "."})
	}
	if v.RestartCount > 0 {
		causes = append(causes, likelyCause{Type: "application_crash_or_probe_failure", Confidence: 0.75, Reason: "One or more Pods restarted."})
	}
	if v.PodCount == 0 {
		causes = append(causes, likelyCause{Type: "deployment_or_selector", Confidence: 0.65, Reason: "No Pods were found for the service."})
	}
	return append(causes, causesFromMissing(append(v.EvidenceGaps, v.ReleaseGaps...))...)
}

func causesFromMissing(missing []string) []likelyCause {
	if len(missing) == 0 {
		return nil
	}
	return []likelyCause{
		{Type: "missing_evidence", Confidence: 0.4, Reason: "Some integrations or evidence sources are missing: " + strings.Join(uniqueStrings(missing), ", ")},
	}
}

func availableCapabilityCount(items []capabilityItem) int {
	count := 0
	for _, item := range items {
		if item.Available {
			count++
		}
	}
	return count
}
