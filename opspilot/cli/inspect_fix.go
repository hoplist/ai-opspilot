package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
	"io"
	"net/url"
	"strings"
)

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
