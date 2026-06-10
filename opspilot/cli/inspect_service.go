package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

func runInspectService(opts globalOptions, args []string, out io.Writer) error {
	positionalService := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("inspect service", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	envName := fs.String("env", "test", "target environment")
	source := fs.String("source", "", "prometheus datasource")
	cluster := fs.String("cluster", "", "cluster name")
	tail := fs.Int("tail", 300, "tail lines")
	since := fs.Int("since", defaultPodLogSinceSeconds, "since seconds")
	_ = fs.Parse(args)
	if *service == "" {
		*service = positionalService
	}
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("inspect service requires --service")
	}
	result, err := fetchInspectService(opts.backendURL, *service, *envName, *source, firstNonEmptyString(*cluster, opts.cluster), *tail, *since)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Service: %s env=%s\n", result.Service, result.Environment)
		fmt.Fprintf(w, "Status: %s stage=%s namespace=%s deployment=%s\n", result.Status, result.Stage, result.Namespace, result.Deployment)
		if result.Image != "" {
			fmt.Fprintf(w, "Image: %s\n", result.Image)
		}
		fmt.Fprintf(w, "Usage: pods=%d restarts=%d CPU %.3f cores memory %.1f MiB\n",
			result.PodCount, result.RestartCount, result.TotalCPUCore, result.TotalMemoryMiB)
		if len(result.Findings) > 0 {
			fmt.Fprintf(w, "Findings: %s\n", strings.Join(result.Findings, "; "))
		}
		if len(result.ReleaseGaps) > 0 {
			fmt.Fprintf(w, "Release gaps: %s\n", strings.Join(result.ReleaseGaps, ", "))
		}
		if len(result.EvidenceGaps) > 0 {
			fmt.Fprintf(w, "Evidence gaps: %s\n", strings.Join(result.EvidenceGaps, ", "))
		}
		if len(result.AvailableEvidence) > 0 {
			fmt.Fprintf(w, "Available evidence: %s\n", strings.Join(result.AvailableEvidence, "; "))
		}
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, "; "))
		}
		writeSkillRecommendationsHuman(w, result.SkillRecommendations)
		if len(result.Pods) > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "POD\tSTATUS\tREADY\tRESTARTS\tIMAGE\tCPU\tMEMORY\tK8S LOG\tELK")
			for _, pod := range result.Pods {
				fmt.Fprintf(tw, "%s\t%s\t%t\t%d\t%s\t%.3f\t%.1fMiB\t%dB\t%d\n",
					pod.Pod, pod.Status, pod.Ready, pod.RestartCount, imageTagHint(pod), pod.CPUCore, pod.MemoryMiB, pod.KubernetesLogBytes, pod.ElasticsearchLogHits)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(result.Next, "; "))
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		if len(result.CapabilityWarnings) > 0 {
			fmt.Fprintf(w, "Capability warnings: %s\n", strings.Join(result.CapabilityWarnings, "; "))
		}
		return nil
	})
}

func fetchInspectService(backendURL, service, envName, source, cluster string, tail, since int) (inspectServiceResult, error) {
	data, err := fetchReleaseStatusData(backendURL, service, cluster)
	if err != nil {
		return inspectServiceResult{}, err
	}
	result := inspectServiceResult{
		Service:     firstNonEmptyString(stringValue(data["service"]), service),
		Environment: firstNonEmptyString(stringValue(data["environment"]), envName),
		Namespace:   stringValue(data["namespace"]),
		Deployment:  stringValue(data["deployment"]),
		Status:      stringValue(data["status"]),
		Stage:       stringValue(data["stage"]),
		Image:       stringValue(data["image"]),
		ReleaseGaps: stringList(data["gaps"]),
		Next:        stringList(data["next_checks"]),
		Cluster:     cluster,
		Raw:         map[string]any{"release_status": data},
	}
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
	evidence := mapValue(data, "evidence")
	pods := mapValue(evidence, "pods")
	podItems := mapsFromItems(pods["items"])
	result.PodCount = intValue(pods["item_count"])
	if result.PodCount == 0 {
		result.PodCount = len(podItems)
	}
	if len(podItems) == 0 {
		result.EvidenceGaps = append(result.EvidenceGaps, "service_pods_missing")
		result.Findings = append(result.Findings, "No matching Pods were found from release evidence.")
	} else {
		for _, item := range podItems {
			podName := stringValue(item["name"])
			namespace := firstNonEmptyString(stringValue(item["namespace"]), result.Namespace)
			if podName == "" || namespace == "" {
				continue
			}
			pod, err := fetchInspectPod(backendURL, namespace, podName, source, cluster, tail, since)
			if err != nil {
				result.Warnings = append(result.Warnings, podName+": "+err.Error())
				result.EvidenceGaps = append(result.EvidenceGaps, "pod_inspection_failed")
				continue
			}
			result.Pods = append(result.Pods, pod)
			result.TotalCPUCore += pod.CPUCore
			result.TotalMemoryMiB += pod.MemoryMiB
			result.RestartCount += pod.RestartCount
			result.EvidenceGaps = append(result.EvidenceGaps, pod.EvidenceGaps...)
		}
	}
	result.TotalCPUCore = round3(result.TotalCPUCore)
	result.TotalMemoryMiB = round1(result.TotalMemoryMiB)
	result.ReleaseGaps = uniqueStrings(result.ReleaseGaps)
	result.EvidenceGaps = uniqueStrings(result.EvidenceGaps)
	result.Next = uniqueStrings(result.Next)
	result.Findings = append(result.Findings, serviceLogEvidenceFindings(result.EvidenceGaps)...)
	switch {
	case result.Status == "healthy" && result.RestartCount == 0:
		result.Findings = append(result.Findings, "Service rollout is healthy and no Pod restarts were found.")
	case result.Status != "" && result.Status != "healthy":
		result.Findings = append(result.Findings, "Service release status is "+result.Status+".")
	}
	if result.TotalCPUCore < 0.1 && result.TotalMemoryMiB < 256 && len(result.Pods) > 0 {
		result.Findings = append(result.Findings, "Current Pod resource usage is low.")
	}
	if len(result.ReleaseGaps) > 0 || len(result.EvidenceGaps) > 0 {
		result.Findings = append(result.Findings, "Some evidence is missing; treat the healthy checks as partial.")
	}
	recommendations, warning := fetchSkillRecommendations(backendURL, "service", serviceEvidenceStatus(result),
		uniqueStrings(append(append(result.MissingEvidence, result.EvidenceGaps...), result.ReleaseGaps...)),
		append(result.Findings, result.Next...))
	result.SkillRecommendations = recommendations
	if warning != "" {
		result.CapabilityWarnings = append(result.CapabilityWarnings, warning)
	}
	return result, nil
}

func serviceLogEvidenceFindings(gaps []string) []string {
	findings := []string{}
	gapSet := map[string]bool{}
	for _, gap := range gaps {
		gapSet[gap] = true
	}
	switch {
	case gapSet["kubernetes_logs_unavailable"]:
		findings = append(findings, "Kubernetes short-window logs could not be read for at least one Pod; Pod status, events, metrics, and release evidence remain usable.")
	case gapSet["kubernetes_logs_empty"]:
		findings = append(findings, "Kubernetes short-window logs are empty for at least one Pod; this does not block status, event, metric, or release checks.")
	}
	switch {
	case gapSet["elk_logs_unavailable"] || gapSet["elk_logs_missing_or_empty"] || gapSet["elk_logs_empty"] || gapSet["elk_logs_missing"]:
		findings = append(findings, "ELK/OpenSearch log evidence is missing or unavailable; historical logs are incomplete, but current Pod-level checks remain usable.")
	}
	return findings
}
