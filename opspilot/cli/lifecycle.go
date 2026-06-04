package main

import (
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
)

type lifecyclePolicy struct {
	Mode                  string   `json:"mode"`
	HumanApprovalRequired bool     `json:"human_approval_required"`
	AllowedRisk           []string `json:"allowed_risk"`
	PlanOnlyRisk          []string `json:"plan_only_risk"`
	Forbidden             []string `json:"forbidden"`
}

type lifecycleAction struct {
	ID           string   `json:"id"`
	Category     string   `json:"category"`
	Risk         string   `json:"risk"`
	Action       string   `json:"action"`
	Target       string   `json:"target"`
	Reason       string   `json:"reason"`
	Automation   string   `json:"automation"`
	Evidence     []string `json:"evidence,omitempty"`
	Requires     []string `json:"requires,omitempty"`
	BlockedBy    []string `json:"blocked_by,omitempty"`
	RollbackHint string   `json:"rollback_hint,omitempty"`
}

type janitorPlanResult struct {
	Scope                string                         `json:"scope"`
	DryRun               bool                           `json:"dry_run"`
	Policy               lifecyclePolicy                `json:"policy"`
	Summary              map[string]any                 `json:"summary"`
	Actions              []lifecycleAction              `json:"actions"`
	Findings             []string                       `json:"findings,omitempty"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	Warnings             []string                       `json:"warnings,omitempty"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Raw                  any                            `json:"raw,omitempty"`
}

type healerDiagnosisResult struct {
	Service              string                         `json:"service"`
	Environment          string                         `json:"environment"`
	DryRun               bool                           `json:"dry_run"`
	Policy               lifecyclePolicy                `json:"policy"`
	Status               string                         `json:"status"`
	Summary              string                         `json:"summary"`
	FailureClass         string                         `json:"failure_class,omitempty"`
	Actions              []lifecycleAction              `json:"actions"`
	Evidence             []evidenceItem                 `json:"evidence,omitempty"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	Warnings             []string                       `json:"warnings,omitempty"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Raw                  any                            `json:"raw,omitempty"`
}

type decommissionPlanResult struct {
	Service              string                         `json:"service"`
	Environment          string                         `json:"environment"`
	DryRun               bool                           `json:"dry_run"`
	KeepData             bool                           `json:"keep_data"`
	Policy               lifecyclePolicy                `json:"policy"`
	Risk                 string                         `json:"risk"`
	Summary              string                         `json:"summary"`
	Inventory            map[string]any                 `json:"inventory"`
	Actions              []lifecycleAction              `json:"actions"`
	BlockedActions       []lifecycleAction              `json:"blocked_actions,omitempty"`
	EvidenceGaps         []string                       `json:"evidence_gaps,omitempty"`
	Warnings             []string                       `json:"warnings,omitempty"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Raw                  any                            `json:"raw,omitempty"`
}

func defaultLifecyclePolicy() lifecyclePolicy {
	return lifecyclePolicy{
		Mode:                  "plan_first_controlled_mutation",
		HumanApprovalRequired: false,
		AllowedRisk:           []string{"read_only", "safe_mutate", "controlled_mutate"},
		PlanOnlyRisk:          []string{"high_risk"},
		Forbidden: []string{
			"delete persistent data automatically",
			"delete production namespace automatically",
			"delete GitLab projects automatically",
			"change cluster-level RBAC, CNI, StorageClass, or ingress controller automatically",
		},
	}
}

func janitorCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected janitor subcommand: plan")
	}
	switch args[0] {
	case "plan":
		return runJanitorPlan(opts, args[1:], out)
	case "run":
		return fmt.Errorf("janitor run is not enabled in the first version; use janitor plan and implement allow-listed --confirm execution later")
	default:
		return fmt.Errorf("unknown janitor command: %s", args[0])
	}
}

func healerCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected healer subcommand: diagnose")
	}
	switch args[0] {
	case "diagnose":
		return runHealerDiagnose(opts, args[1:], out)
	case "fix":
		return fmt.Errorf("healer fix is not enabled in the first version; use healer diagnose and fix service --dry-run")
	default:
		return fmt.Errorf("unknown healer command: %s", args[0])
	}
}

func appCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected app subcommand: decommission")
	}
	switch args[0] {
	case "decommission":
		return appDecommissionCommand(opts, args[1:], out)
	default:
		return fmt.Errorf("unknown app command: %s", args[0])
	}
}

func appDecommissionCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected app decommission subcommand: plan")
	}
	switch args[0] {
	case "plan":
		return runAppDecommissionPlan(opts, args[1:], out)
	case "run":
		return fmt.Errorf("app decommission run is not enabled in the first version; use app decommission plan and keep data by default")
	default:
		return fmt.Errorf("unknown app decommission command: %s", args[0])
	}
}

func runJanitorPlan(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("janitor plan", flag.ExitOnError)
	scope := fs.String("scope", "cluster", "cleanup scope: cluster, sandbox, or all")
	source := fs.String("source", "all", "prometheus datasource, or all")
	limit := fs.Int("limit", 10, "inspection limit")
	_ = fs.Parse(args)
	inspection, err := fetchInspectCluster(opts.backendURL, *source, *limit)
	if err != nil {
		return err
	}
	actions := janitorActionsFromCluster(*scope, inspection)
	result := janitorPlanResult{
		Scope:                *scope,
		DryRun:               true,
		Policy:               defaultLifecyclePolicy(),
		Summary:              lifecycleActionSummary(actions),
		Actions:              actions,
		Findings:             inspection.Findings,
		MissingEvidence:      inspection.MissingEvidence,
		Warnings:             inspection.CapabilityWarnings,
		SkillRecommendations: skillregistry.Recommend("cluster", clusterEvidenceStatus(inspection), inspection.MissingEvidence, inspection.Findings),
		Raw:                  inspection,
	}
	if len(result.Actions) == 0 {
		result.Findings = append(result.Findings, "No automatic cleanup candidates matched the current allow-list.")
	}
	return writeOutput(out, opts.output, result, writeJanitorPlanHuman(result))
}

func janitorActionsFromCluster(scope string, inspection inspectClusterResult) []lifecycleAction {
	actions := []lifecycleAction{}
	for _, pod := range mapsFromItems(mapValue(inspection.AbnormalPods, "data")["items"]) {
		namespace := stringValue(pod["namespace"])
		name := stringValue(pod["name"])
		status := stringValue(pod["status"])
		if name == "" || namespace == "" {
			continue
		}
		target := namespace + "/" + name
		if strings.EqualFold(status, "Succeeded") || strings.EqualFold(status, "Completed") {
			actions = append(actions, lifecycleAction{
				ID:         "cleanup_completed_pod",
				Category:   "kubernetes",
				Risk:       riskForNamespace(namespace, scope),
				Action:     "delete completed Pod only when owner is a Job/CronJob and TTL/labels confirm it is temporary",
				Target:     target,
				Reason:     "Completed Pods can accumulate and add visual noise to inspections.",
				Automation: automationForNamespace(namespace, scope),
				Evidence:   []string{"pod_status=" + status},
				Requires:   []string{"temporary label or Job owner", "not in protected namespace"},
			})
		}
		if strings.Contains(strings.ToLower(status), "imagepull") || strings.Contains(strings.ToLower(status), "crashloop") {
			actions = append(actions, lifecycleAction{
				ID:         "investigate_abnormal_pod",
				Category:   "kubernetes",
				Risk:       "read_only",
				Action:     "inspect pod and route to healer before cleanup",
				Target:     target,
				Reason:     "Abnormal running workload should be diagnosed, not deleted blindly.",
				Automation: "auto_plan_only",
				Evidence:   []string{"pod_status=" + status},
				Requires:   []string{"inspect pod", "recent logs", "events"},
			})
		}
	}
	for _, fs := range inspection.Filesystems {
		if fs.UsedPct >= 85 {
			actions = append(actions, lifecycleAction{
				ID:         "filesystem_pressure_plan",
				Category:   "storage",
				Risk:       "high_risk",
				Action:     "generate hostPath/log retention cleanup plan only",
				Target:     fs.Node + ":" + fs.Mount,
				Reason:     fmt.Sprintf("Filesystem usage is %.1f%%.", fs.UsedPct),
				Automation: "plan_only",
				Evidence:   []string{fmt.Sprintf("free=%.1fGiB total=%.1fGiB", fs.FreeGiB, fs.TotalGiB)},
				BlockedBy:  []string{"hostPath/PV data deletion is high risk"},
			})
		}
	}
	sortLifecycleActions(actions)
	return actions
}

func runHealerDiagnose(opts globalOptions, args []string, out io.Writer) error {
	positionalService := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("healer diagnose", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	envName := fs.String("env", "test", "target environment")
	source := fs.String("source", "", "prometheus datasource")
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
		return fmt.Errorf("healer diagnose requires --service")
	}
	release, err := fetchReleaseService(opts.backendURL, *service, *envName, 5)
	if err != nil {
		return err
	}
	inspection, inspectErr := fetchInspectService(opts.backendURL, *service, *envName, *source, *tail, *since)
	actions := healerActionsFromRelease(release, inspection, inspectErr)
	missing := []string{}
	warnings := append([]string{}, release.Warnings...)
	if inspectErr != nil {
		warnings = append(warnings, "inspect service: "+inspectErr.Error())
		missing = append(missing, "service_inspection")
	} else {
		missing = uniqueStrings(append(append(inspection.MissingEvidence, inspection.EvidenceGaps...), inspection.ReleaseGaps...))
	}
	status := release.Status
	if status == "" {
		status = "unknown"
	}
	result := healerDiagnosisResult{
		Service:              release.Service,
		Environment:          release.Environment,
		DryRun:               true,
		Policy:               defaultLifecyclePolicy(),
		Status:               status,
		Summary:              healerSummary(release, inspection, inspectErr),
		FailureClass:         healerFailureClass(release, inspection, inspectErr),
		Actions:              actions,
		Evidence:             healerEvidenceItems(release, inspection, inspectErr),
		MissingEvidence:      missing,
		Warnings:             warnings,
		SkillRecommendations: skillregistry.Recommend("release", status, missing, append(release.Gaps, release.Next...)),
		Raw:                  map[string]any{"release": release, "inspection": inspection},
	}
	return writeOutput(out, opts.output, result, writeHealerDiagnoseHuman(result))
}

func healerActionsFromRelease(release releaseServiceResult, inspection inspectServiceResult, inspectErr error) []lifecycleAction {
	actions := []lifecycleAction{}
	for _, job := range release.Jobs {
		if stringValue(job["status"]) == "failed" {
			name := stringValue(job["name"])
			actions = append(actions, lifecycleAction{
				ID:         "diagnose_failed_job",
				Category:   "pipeline",
				Risk:       "read_only",
				Action:     "read bounded job logs and classify failure",
				Target:     name,
				Reason:     "Latest release pipeline contains a failed job.",
				Automation: "auto_allowed",
				Evidence:   []string{"stage=" + stringValue(job["stage"]), "failure=" + stringValue(job["failure_reason"])},
				Requires:   []string{"release logs --service " + release.Service + " --job " + name},
			})
		}
	}
	if release.Pipeline != nil && stringValue(release.Pipeline["status"]) == "failed" {
		actions = append(actions, lifecycleAction{
			ID:         "rerun_pipeline_after_fix",
			Category:   "pipeline",
			Risk:       "safe_mutate",
			Action:     "rerun release pipeline after a generated fix is committed",
			Target:     release.Service,
			Reason:     "Pipeline failed; retry is safe only after evidence-based fix or transient-runner classification.",
			Automation: "confirm_allowed",
			Requires:   []string{"failure class is transient or fix commit exists"},
		})
	}
	if inspectErr == nil && inspection.Status != "" && inspection.Status != "healthy" {
		actions = append(actions, lifecycleAction{
			ID:           "rollback_unhealthy_release",
			Category:     "release",
			Risk:         "controlled_mutate",
			Action:       "rollback to previous healthy GitOps image through release rollback",
			Target:       release.Service,
			Reason:       "Release is unhealthy after deployment.",
			Automation:   "confirm_allowed",
			Requires:     []string{"release history has previous healthy image", "no data migration risk"},
			RollbackHint: "release rollback --service " + release.Service + " --to <previous-tag> --confirm",
		})
	}
	if inspectErr == nil && inspection.RestartCount > 0 {
		actions = append(actions, lifecycleAction{
			ID:         "review_restarting_pods",
			Category:   "kubernetes",
			Risk:       "read_only",
			Action:     "inspect recent logs and events before restart or rollback",
			Target:     inspection.Namespace + "/" + inspection.Deployment,
			Reason:     fmt.Sprintf("Service Pods have %d restarts.", inspection.RestartCount),
			Automation: "auto_allowed",
			Evidence:   []string{fmt.Sprintf("restarts=%d", inspection.RestartCount)},
		})
	}
	if len(actions) == 0 {
		actions = append(actions, lifecycleAction{
			ID:         "no_healing_needed",
			Category:   "release",
			Risk:       "read_only",
			Action:     "no mutation suggested",
			Target:     release.Service,
			Reason:     "Current evidence does not show a failed pipeline or unhealthy rollout.",
			Automation: "auto_plan_only",
		})
	}
	sortLifecycleActions(actions)
	return actions
}

func runAppDecommissionPlan(opts globalOptions, args []string, out io.Writer) error {
	positionalService := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("app decommission plan", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	envName := fs.String("env", "test", "target environment")
	keepData := fs.Bool("keep-data", true, "keep PVC, PV, hostPath, database, and middleware data")
	_ = fs.Parse(args)
	if *service == "" {
		*service = positionalService
	}
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("app decommission plan requires --service")
	}
	release, err := fetchReleaseService(opts.backendURL, *service, *envName, 5)
	if err != nil {
		return err
	}
	inspection, inspectErr := fetchInspectService(opts.backendURL, *service, *envName, "", 120, defaultPodLogSinceSeconds)
	actions, blocked := decommissionActions(release, inspection, inspectErr, *keepData)
	gaps := append([]string{}, release.Gaps...)
	warnings := append([]string{}, release.Warnings...)
	if inspectErr != nil {
		warnings = append(warnings, "inspect service: "+inspectErr.Error())
		gaps = append(gaps, "service_inspection")
	} else {
		gaps = uniqueStrings(append(append(gaps, inspection.EvidenceGaps...), inspection.ReleaseGaps...))
	}
	inventory := map[string]any{
		"gitlab_pipeline": release.Pipeline,
		"gitops":          release.GitOps,
		"argocd":          release.ArgoCD,
		"namespace":       release.Namespace,
		"deployment":      release.Deployment,
		"image":           release.Image,
		"pods":            inspection.Pods,
		"history_count":   release.HistoryCount,
	}
	result := decommissionPlanResult{
		Service:              release.Service,
		Environment:          release.Environment,
		DryRun:               true,
		KeepData:             *keepData,
		Policy:               defaultLifecyclePolicy(),
		Risk:                 decommissionRisk(release, inspection, inspectErr, *keepData),
		Summary:              "Generated decommission plan only. Data-bearing resources are kept by default and high-risk deletion is plan-only.",
		Inventory:            inventory,
		Actions:              actions,
		BlockedActions:       blocked,
		EvidenceGaps:         gaps,
		Warnings:             warnings,
		SkillRecommendations: skillregistry.Recommend("release", "decommission_plan", gaps, []string{"GitOps and Argo CD are the source of truth for application removal."}),
		Raw:                  map[string]any{"release": release, "inspection": inspection},
	}
	return writeOutput(out, opts.output, result, writeDecommissionPlanHuman(result))
}

func decommissionActions(release releaseServiceResult, inspection inspectServiceResult, inspectErr error, keepData bool) ([]lifecycleAction, []lifecycleAction) {
	actions := []lifecycleAction{}
	blocked := []lifecycleAction{
		{
			ID:         "delete_persistent_data",
			Category:   "data",
			Risk:       "high_risk",
			Action:     "delete PVC/PV/hostPath/database data",
			Target:     release.Service,
			Reason:     "Persistent data deletion can be irreversible and may affect shared middleware.",
			Automation: "plan_only",
			BlockedBy:  []string{"data deletion is high risk", "keep-data defaults to true"},
		},
		{
			ID:         "delete_gitlab_project",
			Category:   "gitlab",
			Risk:       "high_risk",
			Action:     "delete GitLab project",
			Target:     release.Service,
			Reason:     "Source repository deletion is not part of app runtime decommission.",
			Automation: "plan_only",
			BlockedBy:  []string{"GitLab project deletion is forbidden for automatic execution"},
		},
	}

	mappingGaps := decommissionMappingGaps(release, keepData)
	if len(mappingGaps) == 0 {
		actions = append(actions,
			lifecycleAction{
				ID:           "remove_gitops_application",
				Category:     "gitops",
				Risk:         "controlled_mutate",
				Action:       "remove Argo Application manifest from GitOps",
				Target:       firstNonEmptyString(argocdAppName(release), release.Service),
				Reason:       "Argo CD prune should delete desired workload resources after GitOps commit.",
				Automation:   "confirm_allowed",
				Requires:     []string{"service mapping exists", "GitOps diff reviewed", "keep-data=true"},
				RollbackHint: "revert the GitOps decommission commit",
			},
			lifecycleAction{
				ID:           "remove_gitops_workload_manifests",
				Category:     "gitops",
				Risk:         "controlled_mutate",
				Action:       "remove service workload manifests from GitOps",
				Target:       stringValue(release.GitOps["path"]),
				Reason:       "GitOps is the source of truth for workload desired state.",
				Automation:   "confirm_allowed",
				Requires:     []string{"GitOps path mapping exists", "no shared namespace guardrail removal"},
				RollbackHint: "revert the GitOps decommission commit",
			},
		)
	} else {
		blocked = append(blocked,
			lifecycleAction{
				ID:         "remove_gitops_application",
				Category:   "gitops",
				Risk:       "high_risk",
				Action:     "remove Argo Application manifest from GitOps",
				Target:     firstNonEmptyString(argocdAppName(release), release.Service),
				Reason:     "Application removal is unsafe until service ownership and Argo CD mapping are complete.",
				Automation: "plan_only",
				BlockedBy:  mappingGaps,
			},
			lifecycleAction{
				ID:         "remove_gitops_workload_manifests",
				Category:   "gitops",
				Risk:       "high_risk",
				Action:     "remove service workload manifests from GitOps",
				Target:     stringValue(release.GitOps["path"]),
				Reason:     "Workload manifest removal is unsafe without an exact GitOps path.",
				Automation: "plan_only",
				BlockedBy:  mappingGaps,
			},
		)
	}

	if release.Namespace != "" && strings.HasPrefix(release.Namespace, "cicd-demo") && len(mappingGaps) == 0 {
		actions = append(actions, lifecycleAction{
			ID:         "delete_demo_namespace_when_empty",
			Category:   "kubernetes",
			Risk:       "controlled_mutate",
			Action:     "delete demo namespace only after it is empty and labelled temporary",
			Target:     release.Namespace,
			Reason:     "Sandbox/demo namespace cleanup is allowed after ownership and emptiness checks.",
			Automation: "confirm_allowed",
			Requires:   []string{"opspilot.io/temporary=true or sandbox namespace policy", "no PVC/PV/data resources"},
		})
	} else if release.Namespace != "" {
		blockedBy := []string{"namespace deletion is high risk outside fully mapped sandbox/demo scope"}
		if len(mappingGaps) > 0 {
			blockedBy = append(blockedBy, mappingGaps...)
		}
		blocked = append(blocked, lifecycleAction{
			ID:         "delete_namespace",
			Category:   "kubernetes",
			Risk:       "high_risk",
			Action:     "delete namespace",
			Target:     release.Namespace,
			Reason:     "Namespaces may contain shared services, secrets, PVCs, or multiple applications.",
			Automation: "plan_only",
			BlockedBy:  uniqueStrings(blockedBy),
		})
	}
	if inspectErr == nil && inspection.PodCount > 0 {
		actions = append(actions, lifecycleAction{
			ID:         "verify_no_live_traffic",
			Category:   "evidence",
			Risk:       "read_only",
			Action:     "verify traffic, recent logs, and release history before decommission",
			Target:     release.Service,
			Reason:     fmt.Sprintf("Service still has %d Pod(s) in release evidence.", inspection.PodCount),
			Automation: "auto_allowed",
			Requires:   []string{"traffic evidence", "recent logs", "owner confirmation not required but recommended for non-sandbox services"},
		})
	}
	sortLifecycleActions(actions)
	sortLifecycleActions(blocked)
	return actions, blocked
}

func decommissionMappingGaps(release releaseServiceResult, keepData bool) []string {
	gaps := []string{}
	if !keepData {
		gaps = append(gaps, "keep-data=false requires a separate data deletion review")
	}
	if release.Namespace == "" {
		gaps = append(gaps, "namespace mapping missing")
	}
	if release.Deployment == "" {
		gaps = append(gaps, "deployment mapping missing")
	}
	if stringValue(release.GitOps["path"]) == "" {
		gaps = append(gaps, "gitops path mapping missing")
	}
	if argocdAppName(release) == "" {
		gaps = append(gaps, "argocd application mapping missing")
	}
	return uniqueStrings(gaps)
}

func argocdAppName(release releaseServiceResult) string {
	return firstNonEmptyString(stringValue(release.ArgoCD["name"]), stringValue(release.ArgoCD["app"]))
}

func writeJanitorPlanHuman(result janitorPlanResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Janitor plan: scope=%s dry_run=%t mode=%s\n", result.Scope, result.DryRun, result.Policy.Mode)
		writeLifecycleSummary(w, result.Summary, result.Actions)
		if len(result.Findings) > 0 {
			fmt.Fprintf(w, "Findings: %s\n", strings.Join(result.Findings, "; "))
		}
		writeLifecycleActions(w, "Actions", result.Actions)
		writeSkillRecommendationsHuman(w, result.SkillRecommendations)
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, ", "))
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	}
}

func writeHealerDiagnoseHuman(result healerDiagnosisResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Healer diagnosis: service=%s env=%s dry_run=%t status=%s class=%s\n", result.Service, result.Environment, result.DryRun, result.Status, result.FailureClass)
		if result.Summary != "" {
			fmt.Fprintf(w, "Summary: %s\n", result.Summary)
		}
		writeLifecycleActions(w, "Actions", result.Actions)
		if len(result.Evidence) > 0 {
			fmt.Fprintln(w, "Evidence:")
			for _, item := range result.Evidence {
				fmt.Fprintf(w, "- %s: %s\n", item.Source, item.Message)
			}
		}
		writeSkillRecommendationsHuman(w, result.SkillRecommendations)
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, ", "))
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	}
}

func writeDecommissionPlanHuman(result decommissionPlanResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Decommission plan: service=%s env=%s dry_run=%t keep_data=%t risk=%s\n", result.Service, result.Environment, result.DryRun, result.KeepData, result.Risk)
		if result.Summary != "" {
			fmt.Fprintf(w, "Summary: %s\n", result.Summary)
		}
		fmt.Fprintf(w, "Inventory: namespace=%s deployment=%s image=%s\n",
			stringValue(result.Inventory["namespace"]), stringValue(result.Inventory["deployment"]), stringValue(result.Inventory["image"]))
		writeLifecycleActions(w, "Allowed plan actions", result.Actions)
		writeLifecycleActions(w, "Blocked/high-risk actions", result.BlockedActions)
		writeSkillRecommendationsHuman(w, result.SkillRecommendations)
		if len(result.EvidenceGaps) > 0 {
			fmt.Fprintf(w, "Evidence gaps: %s\n", strings.Join(result.EvidenceGaps, ", "))
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	}
}

func writeLifecycleSummary(w io.Writer, summary map[string]any, actions []lifecycleAction) {
	if summary == nil {
		summary = lifecycleActionSummary(actions)
	}
	fmt.Fprintf(w, "Summary: total=%d read_only=%d safe=%d controlled=%d high_risk=%d\n",
		intValue(summary["total"]), intValue(summary["read_only"]), intValue(summary["safe_mutate"]),
		intValue(summary["controlled_mutate"]), intValue(summary["high_risk"]))
}

func writeLifecycleActions(w io.Writer, title string, actions []lifecycleAction) {
	if len(actions) == 0 {
		return
	}
	fmt.Fprintln(w, title+":")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RISK\tAUTOMATION\tCATEGORY\tTARGET\tACTION")
	for _, action := range actions {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", action.Risk, action.Automation, action.Category, action.Target, oneLine(action.Action, 80))
	}
	_ = tw.Flush()
}

func lifecycleActionSummary(actions []lifecycleAction) map[string]any {
	summary := map[string]any{"total": len(actions), "read_only": 0, "safe_mutate": 0, "controlled_mutate": 0, "high_risk": 0}
	for _, action := range actions {
		key := action.Risk
		if _, ok := summary[key]; ok {
			summary[key] = intValue(summary[key]) + 1
		}
	}
	return summary
}

func riskForNamespace(namespace, scope string) string {
	if scope == "sandbox" || strings.HasPrefix(namespace, "cicd-demo") || strings.Contains(namespace, "demo") {
		return "controlled_mutate"
	}
	return "high_risk"
}

func automationForNamespace(namespace, scope string) string {
	if riskForNamespace(namespace, scope) == "controlled_mutate" {
		return "confirm_allowed"
	}
	return "plan_only"
}

func sortLifecycleActions(actions []lifecycleAction) {
	sort.SliceStable(actions, func(i, j int) bool {
		if actions[i].Risk != actions[j].Risk {
			return lifecycleRiskRank(actions[i].Risk) < lifecycleRiskRank(actions[j].Risk)
		}
		if actions[i].Category != actions[j].Category {
			return actions[i].Category < actions[j].Category
		}
		return actions[i].Target < actions[j].Target
	})
}

func lifecycleRiskRank(risk string) int {
	switch risk {
	case "read_only":
		return 0
	case "safe_mutate":
		return 1
	case "controlled_mutate":
		return 2
	case "high_risk":
		return 3
	default:
		return 9
	}
}

func healerSummary(release releaseServiceResult, inspection inspectServiceResult, inspectErr error) string {
	parts := []string{}
	if release.Pipeline != nil {
		parts = append(parts, "pipeline="+stringValue(release.Pipeline["status"]))
	}
	if release.ArgoCD != nil {
		parts = append(parts, "argocd="+stringValue(release.ArgoCD["sync_status"])+"/"+stringValue(release.ArgoCD["health_status"]))
	}
	if inspectErr == nil {
		parts = append(parts, fmt.Sprintf("pods=%d restarts=%d", inspection.PodCount, inspection.RestartCount))
	} else {
		parts = append(parts, "service inspection unavailable")
	}
	if len(parts) == 0 {
		return "No release evidence was available."
	}
	return strings.Join(parts, "; ")
}

func healerFailureClass(release releaseServiceResult, inspection inspectServiceResult, inspectErr error) string {
	for _, job := range release.Jobs {
		if stringValue(job["status"]) == "failed" {
			stage := stringValue(job["stage"])
			if stage == "build" {
				return "build_failure"
			}
			if strings.Contains(stage, "test") || strings.Contains(stage, "precheck") {
				return "quality_gate_failure"
			}
			if strings.Contains(stage, "gitops") {
				return "gitops_update_failure"
			}
			return "pipeline_failure"
		}
	}
	if release.ArgoCD != nil && stringValue(release.ArgoCD["health_status"]) != "Healthy" {
		return "argocd_rollout_failure"
	}
	if inspectErr == nil && inspection.RestartCount > 0 {
		return "runtime_restart"
	}
	return "none"
}

func healerEvidenceItems(release releaseServiceResult, inspection inspectServiceResult, inspectErr error) []evidenceItem {
	items := []evidenceItem{}
	if release.Pipeline != nil {
		items = append(items, evidenceItem{Source: "gitlab", Message: fmt.Sprintf("pipeline status=%s id=%d", stringValue(release.Pipeline["status"]), intValue(release.Pipeline["id"]))})
	}
	if release.GitOps != nil {
		items = append(items, evidenceItem{Source: "gitops", Message: "desired_image=" + stringValue(release.GitOps["desired_image"])})
	}
	if release.ArgoCD != nil {
		items = append(items, evidenceItem{Source: "argocd", Message: fmt.Sprintf("sync=%s health=%s", stringValue(release.ArgoCD["sync_status"]), stringValue(release.ArgoCD["health_status"]))})
	}
	if inspectErr == nil {
		items = append(items, evidenceItem{Source: "kubernetes", Message: fmt.Sprintf("namespace=%s deployment=%s pods=%d restarts=%d", inspection.Namespace, inspection.Deployment, inspection.PodCount, inspection.RestartCount)})
	}
	return items
}

func decommissionRisk(release releaseServiceResult, inspection inspectServiceResult, inspectErr error, keepData bool) string {
	if !keepData {
		return "high_risk"
	}
	if release.Namespace == "" || stringValue(release.GitOps["path"]) == "" {
		return "high_risk"
	}
	if strings.HasPrefix(release.Namespace, "cicd-demo") {
		return "controlled_mutate"
	}
	if inspectErr == nil && inspection.PodCount > 0 {
		return "controlled_mutate"
	}
	return "controlled_mutate"
}
