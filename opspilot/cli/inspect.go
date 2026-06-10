package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
)

type apiEnvelope struct {
	OK   bool            `json:"ok"`
	Data json.RawMessage `json:"data"`
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

func inspectCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected inspect subcommand: service, pod, cluster, or release")
	}
	switch args[0] {
	case "service":
		return runInspectService(opts, args[1:], out)
	case "pod":
		return runInspectPod(opts, args[1:], out)
	case "cluster":
		return runInspectCluster(opts, args[1:], out)
	case "release":
		return runReleaseStatus(opts, args[1:], out)
	default:
		return fmt.Errorf("unknown inspect command: %s", args[0])
	}
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

func fixCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected fix subcommand: service or pod")
	}
	switch args[0] {
	case "service":
		return runFixService(opts, args[1:], out)
	case "pod":
		return runFixPod(opts, args[1:], out)
	default:
		return fmt.Errorf("unknown fix command: %s", args[0])
	}
}

func runFixService(opts globalOptions, args []string, out io.Writer) error {
	positionalService := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("fix service", flag.ExitOnError)
	service := fs.String("service", "", "service name")
	envName := fs.String("env", "test", "target environment")
	source := fs.String("source", "", "prometheus datasource")
	cluster := fs.String("cluster", "", "cluster name")
	tail := fs.Int("tail", 300, "tail lines")
	since := fs.Int("since", defaultPodLogSinceSeconds, "since seconds")
	dryRun := fs.Bool("dry-run", false, "plan only; do not mutate repositories or clusters")
	_ = fs.Parse(args)
	if *service == "" {
		*service = positionalService
	}
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("fix service requires --service")
	}
	if !*dryRun {
		return fmt.Errorf("fix service currently requires --dry-run")
	}
	inspection, err := fetchInspectService(opts.backendURL, *service, *envName, *source, firstNonEmptyString(*cluster, opts.cluster), *tail, *since)
	if err != nil {
		return err
	}
	pack := buildEvidencePack(inspection)
	result := fixPlanResult{
		TargetType:         "service",
		Target:             inspection.Service,
		Namespace:          inspection.Namespace,
		DryRun:             true,
		Status:             pack.Status,
		Summary:            firstNonEmptyString(pack.Summary, "Generated a dry-run service fix plan from OpsPilot evidence."),
		Evidence:           pack.Evidence,
		MissingEvidence:    pack.MissingEvidence,
		LikelyCauses:       pack.LikelyCauses,
		RecommendedActions: fixActionsFromEvidence("service", inspection.Service, pack),
		Warnings:           inspection.Warnings,
		Raw:                inspection,
	}
	recommendations, warning := fetchSkillRecommendations(opts.backendURL, "service", pack.Status, pack.MissingEvidence, append([]string{pack.Summary}, evidenceItemMessages(pack.Evidence)...))
	result.SkillRecommendations = recommendations
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	return writeOutput(out, opts.output, result, writeFixPlanHuman(result))
}

func runFixPod(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("fix pod", flag.ExitOnError)
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	pod := fs.String("pod", "", "pod")
	source := fs.String("source", "", "prometheus datasource")
	cluster := fs.String("cluster", "", "cluster name")
	tail := fs.Int("tail", 300, "tail lines")
	since := fs.Int("since", defaultPodLogSinceSeconds, "since seconds")
	dryRun := fs.Bool("dry-run", false, "plan only; do not mutate repositories or clusters")
	_ = fs.Parse(args)
	if *pod == "" && fs.NArg() > 0 {
		*pod = fs.Arg(0)
	}
	if *namespace == "" || *pod == "" {
		return fmt.Errorf("fix pod requires --namespace and --pod")
	}
	if !*dryRun {
		return fmt.Errorf("fix pod currently requires --dry-run")
	}
	inspection, err := fetchInspectPod(opts.backendURL, *namespace, *pod, *source, firstNonEmptyString(*cluster, opts.cluster), *tail, *since)
	if err != nil {
		return err
	}
	pack := buildEvidencePack(inspection)
	result := fixPlanResult{
		TargetType:         "pod",
		Target:             inspection.Pod,
		Namespace:          inspection.Namespace,
		DryRun:             true,
		Status:             pack.Status,
		Summary:            firstNonEmptyString(pack.Summary, "Generated a dry-run Pod fix plan from OpsPilot evidence."),
		Evidence:           pack.Evidence,
		MissingEvidence:    pack.MissingEvidence,
		LikelyCauses:       pack.LikelyCauses,
		RecommendedActions: fixActionsFromEvidence("pod", inspection.Pod, pack),
		Raw:                inspection,
	}
	recommendations, warning := fetchSkillRecommendations(opts.backendURL, "pod", pack.Status, pack.MissingEvidence, append([]string{pack.Summary}, evidenceItemMessages(pack.Evidence)...))
	result.SkillRecommendations = recommendations
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	return writeOutput(out, opts.output, result, writeFixPlanHuman(result))
}

func fixActionsFromEvidence(targetType, target string, pack evidencePack) []recommendedAction {
	actions := []recommendedAction{
		{Type: "ai_review", Target: "evidence_pack", Instruction: "Feed this evidence pack to AI before making code or configuration changes."},
	}
	if pack.Status != "healthy" {
		actions = append(actions,
			recommendedAction{Type: "code_or_config_review", Target: "repository", Instruction: "Inspect startup code, configuration loading, Dockerfile, probes, and deployment YAML for " + target + "."},
			recommendedAction{Type: "release_validation", Target: "pipeline", Instruction: "After a fix, publish through GitLab Runner -> BuildKit -> Registry -> GitOps -> Argo CD, then run check " + targetType + " again."},
		)
	} else {
		actions = append(actions, recommendedAction{Type: "no_code_change", Target: targetType, Instruction: "No direct code change is suggested from current evidence; fill missing evidence before changing code."})
	}
	if len(pack.MissingEvidence) > 0 {
		actions = append(actions, recommendedAction{Type: "missing_evidence", Target: "opspilot", Instruction: "The diagnosis is partial because evidence is missing: " + strings.Join(pack.MissingEvidence, ", ")})
	}
	return actions
}

func writeFixPlanHuman(result fixPlanResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Fix plan: %s %s dry_run=%t status=%s\n", result.TargetType, result.Target, result.DryRun, result.Status)
		if result.Namespace != "" {
			fmt.Fprintf(w, "Namespace: %s\n", result.Namespace)
		}
		if result.Summary != "" {
			fmt.Fprintf(w, "Summary: %s\n", result.Summary)
		}
		if len(result.Evidence) > 0 {
			fmt.Fprintln(w, "Evidence:")
			for _, item := range result.Evidence {
				fmt.Fprintf(w, "- %s: %s\n", item.Source, item.Message)
			}
		}
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, ", "))
		}
		if len(result.LikelyCauses) > 0 {
			fmt.Fprintln(w, "Likely causes:")
			for _, cause := range result.LikelyCauses {
				fmt.Fprintf(w, "- %s confidence=%.2f: %s\n", cause.Type, cause.Confidence, cause.Reason)
			}
		}
		if len(result.RecommendedActions) > 0 {
			fmt.Fprintln(w, "Recommended actions:")
			for _, action := range result.RecommendedActions {
				fmt.Fprintf(w, "- %s %s: %s\n", action.Type, action.Target, action.Instruction)
			}
		}
		writeSkillRecommendationsHuman(w, result.SkillRecommendations)
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	}
}

func writeSkillRecommendationsHuman(w io.Writer, recommendations []skillregistry.Recommendation) {
	if len(recommendations) == 0 {
		return
	}
	fmt.Fprintln(w, "Recommended skills:")
	for _, item := range recommendations {
		fmt.Fprintf(w, "- %s: %s\n", item.Name, item.Reason)
	}
}

func fetchSkillRecommendations(backendURL, targetType, status string, missingEvidence, findings []string) ([]skillregistry.Recommendation, string) {
	values := url.Values{
		"target_type": {targetType},
		"status":      {status},
	}
	for _, item := range missingEvidence {
		if strings.TrimSpace(item) != "" {
			values.Add("missing_evidence", item)
		}
	}
	for _, item := range findings {
		if strings.TrimSpace(item) != "" {
			values.Add("finding", item)
		}
	}
	body, err := get(backendURL, "/api/skills/recommend", values)
	if err != nil {
		return nil, "skills recommend: " + err.Error()
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "skills recommend: " + err.Error()
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, "skills recommend: response missing data"
	}
	raw, _ := json.Marshal(data["items"])
	var result []skillregistry.Recommendation
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, "skills recommend: " + err.Error()
	}
	return result, ""
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

func runInspectCluster(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("inspect cluster", flag.ExitOnError)
	source := fs.String("source", "all", "prometheus datasource, or all")
	cluster := fs.String("cluster", "", "cluster name")
	limit := fs.Int("limit", 10, "top result limit")
	_ = fs.Parse(args)
	result, err := fetchInspectCluster(opts.backendURL, *source, firstNonEmptyString(*cluster, opts.cluster), *limit)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintln(w, "Cluster inspection")
		if len(result.Findings) > 0 {
			fmt.Fprintf(w, "Findings: %s\n", strings.Join(result.Findings, "; "))
		}
		if len(result.AvailableEvidence) > 0 {
			fmt.Fprintf(w, "Available evidence: %s\n", strings.Join(result.AvailableEvidence, "; "))
		}
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, "; "))
		}
		if len(result.CapabilityWarnings) > 0 {
			fmt.Fprintf(w, "Capability warnings: %s\n", strings.Join(result.CapabilityWarnings, "; "))
		}
		writeSkillRecommendationsHuman(w, result.SkillRecommendations)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "\nNODES\tCPU\tMEMORY\tROOTFS")
		for _, node := range result.Nodes {
			fmt.Fprintf(tw, "%s\t%.1f%%\t%.1f%%\t%.1f%%\n",
				stringValue(node["node"]), floatValue(node["cpu_used_percent"]), floatValue(node["memory_used_percent"]), floatValue(node["rootfs_used_percent"]))
		}
		fmt.Fprintln(tw, "\nTOP CPU PODS\tNAMESPACE\tCPU")
		for _, pod := range result.TopCPU {
			fmt.Fprintf(tw, "%s\t%s\t%.3f cores\n", stringValue(pod["pod"]), stringValue(pod["namespace"]), floatValue(pod["cpu_cores"]))
		}
		fmt.Fprintln(tw, "\nTOP MEMORY PODS\tNAMESPACE\tMEMORY")
		for _, pod := range result.TopMemory {
			fmt.Fprintf(tw, "%s\t%s\t%.1fMiB\n", stringValue(pod["pod"]), stringValue(pod["namespace"]), floatValue(pod["memory_working_set_bytes"])/(1024*1024))
		}
		fmt.Fprintln(tw, "\nRESTARTS 24H\tNAMESPACE\tCONTAINER\tCOUNT")
		if len(result.Restarts24h) == 0 {
			fmt.Fprintln(tw, "-\t-\t-\t0")
		}
		for _, item := range result.Restarts24h {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%.1f\n", item.Metric["pod"], item.Metric["namespace"], item.Metric["container"], item.Value)
		}
		fmt.Fprintln(tw, "\nFILESYSTEMS\tMOUNT\tFREE\tTOTAL\tUSED")
		for _, row := range result.Filesystems {
			fmt.Fprintf(tw, "%s\t%s\t%.1fGiB\t%.1fGiB\t%.1f%%\n", row.Node, row.Mount, row.FreeGiB, row.TotalGiB, row.UsedPct)
		}
		return tw.Flush()
	})
}

func fetchInspectCluster(backendURL, source, cluster string, limit int) (inspectClusterResult, error) {
	result := inspectClusterResult{Cluster: cluster, Raw: map[string]any{}}
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
	abnormal, _ := getJSONMap(backendURL, "/api/k8s/pods", addCluster(url.Values{"status": {"abnormal"}, "limit": {strconv.Itoa(limit)}}, cluster))
	nodes, _ := getJSONMap(backendURL, "/api/metrics/nodes", url.Values{"source": {source}, "limit": {"100"}})
	topCPU, _ := getJSONMap(backendURL, "/api/metrics/pods", url.Values{"source": {source}, "sort": {"cpu"}, "limit": {strconv.Itoa(limit)}})
	topMemory, _ := getJSONMap(backendURL, "/api/metrics/pods", url.Values{"source": {source}, "sort": {"memory"}, "limit": {strconv.Itoa(limit)}})
	result.Raw["abnormal_pods"] = abnormal
	result.Raw["nodes"] = nodes
	result.Raw["top_cpu_pods"] = topCPU
	result.Raw["top_memory_pods"] = topMemory
	if data := mapValue(abnormal, "data"); data != nil {
		result.AbnormalPods = data
		if intValue(data["total_count"]) == 0 && intValue(data["item_count"]) == 0 {
			result.Findings = append(result.Findings, "No abnormal Pods found.")
		}
	}
	if data := mapValue(nodes, "data"); data != nil {
		result.Nodes = mapsFromItems(data["items"])
		for _, node := range result.Nodes {
			if floatValue(node["memory_used_percent"]) >= 80 {
				result.Findings = append(result.Findings, "High node memory: "+stringValue(node["node"]))
			}
			if floatValue(node["rootfs_used_percent"]) >= 80 {
				result.Findings = append(result.Findings, "High root filesystem usage: "+stringValue(node["node"]))
			}
		}
	}
	if data := mapValue(topCPU, "data"); data != nil {
		result.TopCPU = mapsFromItems(data["items"])
	}
	if data := mapValue(topMemory, "data"); data != nil {
		result.TopMemory = mapsFromItems(data["items"])
	}
	restarts, err := fetchMetricItems(backendURL, "topk(20, sum by (namespace,pod,container) (increase(kube_pod_container_status_restarts_total[24h])))", "node200-k8s")
	if err == nil {
		for _, item := range restarts {
			if item.Value > 0 {
				result.Restarts24h = append(result.Restarts24h, item)
			}
		}
	}
	filesystems, err := fetchFilesystems(backendURL, source)
	if err == nil {
		result.Filesystems = filesystems.Items
		for _, row := range result.Filesystems {
			if row.UsedPct >= 80 {
				result.Findings = append(result.Findings, "High filesystem usage: "+row.Node+" "+row.Mount)
			}
		}
	}
	if len(result.Restarts24h) > 0 {
		result.Findings = append(result.Findings, fmt.Sprintf("%d containers have restarts in the last 24h.", len(result.Restarts24h)))
	}
	recommendations, warning := fetchSkillRecommendations(backendURL, "cluster", clusterEvidenceStatus(result), result.MissingEvidence, result.Findings)
	result.SkillRecommendations = recommendations
	if warning != "" {
		result.CapabilityWarnings = append(result.CapabilityWarnings, warning)
	}
	return result, nil
}

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

func availableCapabilityCount(items []capabilityItem) int {
	count := 0
	for _, item := range items {
		if item.Available {
			count++
		}
	}
	return count
}
