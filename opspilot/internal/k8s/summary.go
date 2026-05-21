package k8s

import (
	"encoding/json"
	"fmt"
	"strings"
)

func BoundedLogRequest(req LogRequest) LogRequest {
	req.TailLines = clamp(defaultInt(req.TailLines, DefaultTailLines), 1, MaxTailLines)
	req.SinceSeconds = clamp(defaultInt(req.SinceSeconds, DefaultSinceSeconds), 1, MaxSinceSeconds)
	req.LimitBytes = clamp(defaultInt(req.LimitBytes, DefaultLimitBytes), 1, MaxLimitBytes)
	return req
}

func PodSummary(item map[string]any) map[string]any {
	meta := object(item, "metadata")
	status := object(item, "status")
	spec := object(item, "spec")
	conditions := map[string]string{}
	for _, cond := range array(status, "conditions") {
		c := asMap(cond)
		conditions[stringValue(c, "type")] = stringValue(c, "status")
	}
	containers := []any{}
	waitingReasons := []any{}
	restartCount := 0
	for _, raw := range array(status, "containerStatuses") {
		container := asMap(raw)
		state := object(container, "state")
		waiting := object(state, "waiting")
		reason := stringValue(waiting, "reason")
		if reason != "" {
			waitingReasons = append(waitingReasons, reason)
		}
		restarts := intValue(container, "restartCount")
		restartCount += restarts
		containers = append(containers, map[string]any{
			"name":           stringValue(container, "name"),
			"ready":          boolValue(container, "ready"),
			"restart_count":  restarts,
			"image":          stringValue(container, "image"),
			"image_id":       stringValue(container, "imageID"),
			"state":          firstKey(state),
			"waiting_reason": reason,
		})
	}
	ownerKind := ""
	ownerName := ""
	owners := array(meta, "ownerReferences")
	if len(owners) > 0 {
		owner := asMap(owners[0])
		ownerKind = stringValue(owner, "kind")
		ownerName = stringValue(owner, "name")
	}
	phase := stringValue(status, "phase")
	ready := conditions["Ready"] == "True"
	podStatus := phase
	if ready && phase == "Running" {
		podStatus = "Ready"
	}
	if podStatus == "" {
		podStatus = "Unknown"
	}
	return map[string]any{
		"namespace":       stringValue(meta, "namespace"),
		"name":            stringValue(meta, "name"),
		"phase":           phase,
		"ready":           ready,
		"status":          podStatus,
		"node":            stringValue(spec, "nodeName"),
		"pod_ip":          stringValue(status, "podIP"),
		"host_ip":         stringValue(status, "hostIP"),
		"restart_count":   restartCount,
		"waiting_reasons": waitingReasons,
		"owner_kind":      ownerKind,
		"owner_name":      ownerName,
		"labels":          object(meta, "labels"),
		"containers":      containers,
		"start_time":      stringValue(status, "startTime"),
	}
}

func EventSummary(item map[string]any) map[string]any {
	meta := object(item, "metadata")
	involved := object(item, "involvedObject")
	return map[string]any{
		"namespace":       stringValue(meta, "namespace"),
		"name":            stringValue(meta, "name"),
		"type":            stringValue(item, "type"),
		"reason":          stringValue(item, "reason"),
		"message":         stringValue(item, "message"),
		"involved_kind":   stringValue(involved, "kind"),
		"involved_name":   stringValue(involved, "name"),
		"count":           intValue(item, "count"),
		"first_timestamp": stringValue(item, "firstTimestamp"),
		"last_timestamp":  stringValue(item, "lastTimestamp"),
		"event_time":      stringValue(item, "eventTime"),
	}
}

func MatchesStatus(item map[string]any, status string) bool {
	wanted := strings.ToLower(status)
	phase := strings.ToLower(fmt.Sprint(item["phase"]))
	ready, _ := item["ready"].(bool)
	waiting := lowerStrings(item["waiting_reasons"])
	switch wanted {
	case "", "all", "*":
		return true
	case "running":
		return phase == "running"
	case "pending":
		return phase == "pending"
	case "failed":
		return phase == "failed"
	case "not_ready", "not-ready":
		return !ready
	case "abnormal":
		if phase == "succeeded" {
			return len(waiting) > 0
		}
		return phase != "running" || !ready || len(waiting) > 0
	case "crashloop":
		return contains(waiting, "crashloop")
	case "imagepull":
		return contains(waiting, "imagepull") || contains(waiting, "errimagepull")
	default:
		return strings.Contains(phase, wanted) || contains(waiting, wanted)
	}
}

func matchesQuery(item map[string]any, query string) bool {
	raw, _ := json.Marshal(item)
	haystack := strings.ToLower(string(raw))
	for _, token := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func firstContainerName(summary map[string]any) string {
	containers, _ := summary["containers"].([]any)
	if len(containers) == 0 {
		return ""
	}
	first, _ := containers[0].(map[string]any)
	return fmt.Sprint(first["name"])
}

func logModes(summary map[string]any) []bool {
	if restarts, ok := summary["restart_count"].(int); ok && restarts > 0 {
		return []bool{false, true}
	}
	return []bool{false}
}

func object(m map[string]any, key string) map[string]any {
	if value, ok := m[key].(map[string]any); ok {
		return value
	}
	return map[string]any{}
}

func array(m map[string]any, key string) []any {
	if value, ok := m[key].([]any); ok {
		return value
	}
	return []any{}
}

func asMap(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func stringValue(m map[string]any, key string) string {
	if value, ok := m[key]; ok {
		return fmt.Sprint(value)
	}
	return ""
}

func boolValue(m map[string]any, key string) bool {
	value, _ := m[key].(bool)
	return value
}

func intValue(m map[string]any, key string) int {
	switch value := m[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func firstKey(m map[string]any) string {
	for key := range m {
		return key
	}
	return ""
}

func lowerStrings(value any) []string {
	out := []string{}
	for _, item := range toSlice(value) {
		out = append(out, strings.ToLower(fmt.Sprint(item)))
	}
	return out
}

func toSlice(value any) []any {
	if slice, ok := value.([]any); ok {
		return slice
	}
	return []any{}
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func labelsMatch(podLabels, wanted map[string]any) bool {
	if len(wanted) == 0 {
		return false
	}
	for key, value := range wanted {
		if fmt.Sprint(podLabels[key]) != fmt.Sprint(value) {
			return false
		}
	}
	return true
}

func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
