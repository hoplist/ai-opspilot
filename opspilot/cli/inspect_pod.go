package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

func runInspectPod(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("inspect pod", flag.ExitOnError)
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	pod := fs.String("pod", "", "pod")
	source := fs.String("source", "", "prometheus datasource")
	cluster := fs.String("cluster", "", "cluster name")
	tail := fs.Int("tail", 300, "tail lines")
	since := fs.Int("since", defaultPodLogSinceSeconds, "since seconds")
	_ = fs.Parse(args)
	if *pod == "" && fs.NArg() > 0 {
		*pod = fs.Arg(0)
	}
	if *namespace == "" || *pod == "" {
		return fmt.Errorf("inspect pod requires --namespace and --pod")
	}
	result, err := fetchInspectPod(opts.backendURL, *namespace, *pod, *source, firstNonEmptyString(*cluster, opts.cluster), *tail, *since)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Pod: %s/%s\n", result.Namespace, result.Pod)
		fmt.Fprintf(w, "Status: %s ready=%t restarts=%d node=%s\n", result.Status, result.Ready, result.RestartCount, result.Node)
		writeImageEvidenceHuman(w, result)
		fmt.Fprintf(w, "Usage: CPU %.3f cores, memory %.1f MiB\n", result.CPUCore, result.MemoryMiB)
		fmt.Fprintf(w, "Logs: Kubernetes %d bytes, ELK hits %d\n", result.KubernetesLogBytes, result.ElasticsearchLogHits)
		if len(result.EvidenceGaps) > 0 {
			fmt.Fprintf(w, "Evidence gaps: %s\n", strings.Join(result.EvidenceGaps, ", "))
		}
		if len(result.AvailableEvidence) > 0 {
			fmt.Fprintf(w, "Available evidence: %s\n", strings.Join(result.AvailableEvidence, "; "))
		}
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, "; "))
		}
		if len(result.Findings) > 0 {
			fmt.Fprintf(w, "Findings: %s\n", strings.Join(result.Findings, "; "))
		}
		writeSkillRecommendationsHuman(w, result.SkillRecommendations)
		if len(result.CapabilityWarnings) > 0 {
			fmt.Fprintf(w, "Capability warnings: %s\n", strings.Join(result.CapabilityWarnings, "; "))
		}
		return nil
	})
}

func fetchInspectPod(backendURL, namespace, pod, source, cluster string, tail, since int) (inspectPodResult, error) {
	result := inspectPodResult{Cluster: cluster, Namespace: namespace, Pod: pod, Raw: map[string]any{}}
	if capabilities, err := fetchCapabilities(backendURL, cluster); err == nil {
		result.AvailableEvidence = capabilities.AvailableEvidence
		result.MissingEvidence = capabilities.MissingEvidence
		result.CapabilityWarnings = capabilities.Warnings
		result.Raw["capabilities"] = capabilities.Raw
	} else {
		if strings.Contains(err.Error(), "does not have a Kubernetes datasource") {
			return result, err
		}
		result.CapabilityWarnings = append(result.CapabilityWarnings, "capabilities: "+err.Error())
	}
	contextBody, err := get(backendURL, "/api/context/pod", addCluster(url.Values{"namespace": {namespace}, "pod": {pod}, "source": {source}}, cluster))
	if err != nil {
		return result, err
	}
	var contextPayload map[string]any
	_ = json.Unmarshal(contextBody, &contextPayload)
	result.Raw["context"] = contextPayload
	if data := mapValue(contextPayload, "data"); data != nil {
		if summary := mapValue(data, "summary"); summary != nil {
			result.Node = stringValue(summary["node"])
			result.Status = stringValue(summary["status"])
			result.Ready = boolValue(summary["ready"])
			result.RestartCount = intValue(summary["restart_count"])
			applyPrimaryContainerEvidence(&result, summary)
		}
	}
	metricsBody, err := get(backendURL, "/api/metrics/pod", url.Values{"namespace": {namespace}, "pod": {pod}, "source": {source}})
	if err == nil {
		var metricsPayload map[string]any
		_ = json.Unmarshal(metricsBody, &metricsPayload)
		result.Raw["metrics"] = metricsPayload
		if data := mapValue(metricsPayload, "data"); data != nil {
			result.CPUCore = floatValue(data["cpu_cores"])
			result.MemoryMiB = round1(floatValue(data["memory_working_set_bytes"]) / (1024 * 1024))
			if result.RestartCount == 0 {
				result.RestartCount = intValue(data["restart_count"])
			}
		}
	}
	k8sLogAvailable := false
	elkLogAvailable := false
	logBody, err := get(backendURL, "/api/k8s/logs/pod", addCluster(url.Values{
		"namespace":     {namespace},
		"pod":           {pod},
		"tail_lines":    {strconv.Itoa(tail)},
		"since_seconds": {strconv.Itoa(since)},
	}, cluster))
	if err == nil {
		k8sLogAvailable = true
		var logPayload map[string]any
		_ = json.Unmarshal(logBody, &logPayload)
		result.Raw["kubernetes_logs"] = logPayload
		if data := mapValue(logPayload, "data"); data != nil {
			result.KubernetesLogBytes = len(stringValue(data["text"]))
		}
	} else {
		result.Raw["kubernetes_logs_error"] = err.Error()
		result.EvidenceGaps = append(result.EvidenceGaps, "kubernetes_logs_unavailable")
	}
	elkBody, err := get(backendURL, "/api/logs/search", url.Values{"namespace": {namespace}, "pod": {pod}, "limit": {"1"}})
	if err == nil {
		elkLogAvailable = true
		var elkPayload map[string]any
		_ = json.Unmarshal(elkBody, &elkPayload)
		result.Raw["elk_logs"] = elkPayload
		if data := mapValue(elkPayload, "data"); data != nil {
			result.ElasticsearchLogHits = intValue(data["total"])
			if result.ElasticsearchLogHits == 0 {
				result.ElasticsearchLogHits = intValue(data["item_count"])
			}
		}
	} else {
		result.Raw["elk_logs_error"] = err.Error()
		result.EvidenceGaps = append(result.EvidenceGaps, "elk_logs_unavailable")
	}
	if k8sLogAvailable && result.KubernetesLogBytes == 0 {
		result.EvidenceGaps = append(result.EvidenceGaps, "kubernetes_logs_empty")
	}
	if elkLogAvailable && result.ElasticsearchLogHits == 0 {
		result.EvidenceGaps = append(result.EvidenceGaps, "elk_logs_missing_or_empty")
	}
	if result.Ready {
		result.Findings = append(result.Findings, "Pod is currently ready.")
	}
	result.Findings = append(result.Findings, logEvidenceFindings(result, k8sLogAvailable, elkLogAvailable)...)
	if result.RestartCount > 0 {
		result.Findings = append(result.Findings, fmt.Sprintf("Pod has historical restarts: %d.", result.RestartCount))
	}
	recommendations, warning := fetchSkillRecommendations(backendURL, "pod", podEvidenceStatus(result),
		uniqueStrings(append(result.MissingEvidence, result.EvidenceGaps...)), result.Findings)
	result.SkillRecommendations = recommendations
	if warning != "" {
		result.CapabilityWarnings = append(result.CapabilityWarnings, warning)
	}
	return result, nil
}

func applyPrimaryContainerEvidence(result *inspectPodResult, summary map[string]any) {
	containers, _ := summary["containers"].([]any)
	if len(containers) == 0 {
		return
	}
	first, _ := containers[0].(map[string]any)
	if first == nil {
		return
	}
	result.Container = stringValue(first["name"])
	result.SpecImage = firstNonEmptyString(stringValue(first["spec_image"]), stringValue(first["image"]))
	result.StatusImage = stringValue(first["status_image"])
	result.ImageID = stringValue(first["image_id"])
}

func imageTagHint(pod inspectPodResult) string {
	image := firstNonEmptyString(pod.SpecImage, pod.StatusImage)
	if image == "" {
		return "-"
	}
	if idx := strings.LastIndex(image, ":"); idx >= 0 && idx < len(image)-1 {
		return image[idx+1:]
	}
	if idx := strings.LastIndex(image, "@"); idx >= 0 && idx < len(image)-1 {
		return image[idx+1:]
	}
	return image
}

func writeImageEvidenceHuman(w io.Writer, pod inspectPodResult) {
	if pod.SpecImage == "" && pod.StatusImage == "" && pod.ImageID == "" {
		return
	}
	if pod.Container != "" {
		fmt.Fprintf(w, "Container: %s\n", pod.Container)
	}
	if pod.SpecImage != "" {
		fmt.Fprintf(w, "Spec image: %s\n", pod.SpecImage)
	}
	if pod.StatusImage != "" {
		fmt.Fprintf(w, "Status image: %s\n", pod.StatusImage)
	}
	if pod.ImageID != "" {
		fmt.Fprintf(w, "Image ID: %s\n", pod.ImageID)
	}
	if pod.SpecImage != "" && pod.StatusImage != "" && pod.SpecImage != pod.StatusImage {
		fmt.Fprintln(w, "Image note: Kubernetes status may show an older tag when both tags point to the same image digest; use spec image and image ID for rollout evidence.")
	}
}

func logEvidenceFindings(result inspectPodResult, k8sLogAvailable, elkLogAvailable bool) []string {
	findings := []string{}
	switch {
	case k8sLogAvailable && result.KubernetesLogBytes > 0:
		findings = append(findings, "Kubernetes short-window logs are available.")
	case k8sLogAvailable:
		findings = append(findings, "Kubernetes short-window logs are empty; continue with Pod status, events, metrics, and release evidence.")
	default:
		findings = append(findings, "Kubernetes short-window logs could not be read; continue with Pod status, events, metrics, and release evidence.")
	}
	switch {
	case elkLogAvailable && result.ElasticsearchLogHits > 0:
		findings = append(findings, "ELK/OpenSearch log evidence is available.")
	case elkLogAvailable:
		findings = append(findings, "ELK/OpenSearch returned no matching logs for this Pod; this does not block Pod-level checks.")
	default:
		findings = append(findings, "ELK/OpenSearch is unavailable or not connected for this service; historical or rotated logs are missing, but Pod-level checks remain usable.")
	}
	return findings
}
