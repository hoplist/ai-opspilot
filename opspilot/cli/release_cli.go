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
)

type releaseServiceResult struct {
	Cluster          string           `json:"cluster,omitempty"`
	Service          string           `json:"service"`
	Environment      string           `json:"environment"`
	Status           string           `json:"status,omitempty"`
	Stage            string           `json:"stage,omitempty"`
	Namespace        string           `json:"namespace,omitempty"`
	Deployment       string           `json:"deployment,omitempty"`
	Image            string           `json:"image,omitempty"`
	TriggerSupported bool             `json:"trigger_supported"`
	TriggerHint      string           `json:"trigger_hint"`
	Gaps             []string         `json:"gaps,omitempty"`
	Next             []string         `json:"next,omitempty"`
	Pipeline         map[string]any   `json:"pipeline,omitempty"`
	BuildKit         map[string]any   `json:"buildkit,omitempty"`
	Registry         map[string]any   `json:"registry,omitempty"`
	GitOps           map[string]any   `json:"gitops,omitempty"`
	ArgoCD           map[string]any   `json:"argocd,omitempty"`
	Quality          map[string]any   `json:"quality,omitempty"`
	Jobs             []map[string]any `json:"jobs,omitempty"`
	JobCount         int              `json:"job_count"`
	History          []map[string]any `json:"history,omitempty"`
	HistoryCount     int              `json:"history_count"`
	Triggered        bool             `json:"triggered"`
	Trigger          map[string]any   `json:"trigger,omitempty"`
	Warnings         []string         `json:"warnings,omitempty"`
	Raw              map[string]any   `json:"raw,omitempty"`
}

func runReleaseService(opts globalOptions, args []string, out io.Writer) error {
	positionalService := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("release service", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	envName := fs.String("env", "test", "target environment")
	cluster := fs.String("cluster", "", "cluster name")
	historyLimit := fs.Int("history", 5, "release history item limit")
	trigger := fs.Bool("trigger", false, "trigger a new release pipeline")
	ref := fs.String("ref", "main", "GitLab ref to trigger")
	_ = fs.Parse(args)
	if *service == "" {
		*service = positionalService
	}
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release service requires --service")
	}
	activeCluster := firstNonEmptyString(*cluster, opts.cluster)
	result, err := fetchReleaseService(opts.backendURL, *service, *envName, activeCluster, *historyLimit)
	if err != nil {
		return err
	}
	if *trigger {
		triggerResult, err := triggerReleaseService(opts.backendURL, *service, *ref, activeCluster, nil)
		if err != nil {
			return err
		}
		result.Triggered = true
		result.TriggerSupported = true
		result.TriggerHint = "submitted GitLab pipeline through OpsPilot"
		result.Trigger = triggerResult
		if result.Raw == nil {
			result.Raw = map[string]any{}
		}
		result.Raw["trigger"] = triggerResult
		if checks := stringList(triggerResult["next_checks"]); len(checks) > 0 {
			result.Next = uniqueStrings(append(result.Next, checks...))
		}
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Release service: %s env=%s\n", result.Service, result.Environment)
		fmt.Fprintf(w, "Status: %s stage=%s namespace=%s deployment=%s\n", result.Status, result.Stage, result.Namespace, result.Deployment)
		if result.Image != "" {
			fmt.Fprintf(w, "Image: %s\n", result.Image)
		}
		if result.Trigger != nil {
			if pipeline := mapValue(result.Trigger, "pipeline"); pipeline != nil {
				fmt.Fprintf(w, "Triggered: pipeline id=%d status=%s ref=%s sha=%s\n",
					intValue(pipeline["id"]), stringValue(pipeline["status"]), stringValue(pipeline["ref"]), stringValue(pipeline["sha"]))
			} else {
				fmt.Fprintf(w, "Triggered: %s\n", stringValue(result.Trigger["status"]))
			}
		}
		if result.Pipeline != nil {
			fmt.Fprintf(w, "GitLab pipeline: %s id=%d ref=%s sha=%s\n",
				stringValue(result.Pipeline["status"]), intValue(result.Pipeline["id"]), stringValue(result.Pipeline["ref"]), stringValue(result.Pipeline["sha"]))
		}
		if result.GitOps != nil {
			fmt.Fprintf(w, "GitOps: %s image=%s\n", stringValue(result.GitOps["status"]), stringValue(result.GitOps["desired_image"]))
		}
		if result.ArgoCD != nil {
			fmt.Fprintf(w, "Argo CD: sync=%s health=%s\n", stringValue(result.ArgoCD["sync_status"]), stringValue(result.ArgoCD["health_status"]))
		}
		if result.Quality != nil {
			fmt.Fprintf(w, "Quality: %s reason=%s optional=%t\n",
				stringValue(result.Quality["status"]), stringValue(result.Quality["reason"]), boolValue(result.Quality["optional"]))
		}
		if len(result.Jobs) > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "JOB\tSTAGE\tSTATUS\tDURATION\tFAILURE")
			for _, job := range result.Jobs {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%.1fs\t%s\n",
					stringValue(job["name"]), stringValue(job["stage"]), stringValue(job["status"]), floatValue(job["duration"]), stringValue(job["failure_reason"]))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if len(result.History) > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "HISTORY\tREVISION\tDATE\tTAG\tMESSAGE")
			for _, item := range result.History {
				current := ""
				if boolValue(item["current"]) {
					current = "*"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					current, stringValue(item["short_revision"]), shortTime(stringValue(item["committed_at"])), stringValue(item["tag"]), oneLine(stringValue(item["message"]), 80))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if len(result.Gaps) > 0 {
			fmt.Fprintf(w, "Gaps: %s\n", strings.Join(result.Gaps, ", "))
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(result.Next, "; "))
		}
		fmt.Fprintf(w, "Trigger: %s\n", result.TriggerHint)
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	})
}

func fetchReleaseService(backendURL, service, envName, cluster string, historyLimit int) (releaseServiceResult, error) {
	status, err := fetchReleaseStatusData(backendURL, service, cluster)
	if err != nil {
		return releaseServiceResult{}, err
	}
	result := releaseServiceResult{
		Service:          firstNonEmptyString(stringValue(status["service"]), service),
		Cluster:          cluster,
		Environment:      firstNonEmptyString(stringValue(status["environment"]), envName),
		Status:           stringValue(status["status"]),
		Stage:            stringValue(status["stage"]),
		Namespace:        stringValue(status["namespace"]),
		Deployment:       stringValue(status["deployment"]),
		Image:            stringValue(status["image"]),
		TriggerSupported: true,
		TriggerHint:      "use release service --trigger to submit a GitLab pipeline through OpsPilot",
		Gaps:             stringList(status["gaps"]),
		Next:             stringList(status["next_checks"]),
		Raw:              map[string]any{"status": status},
	}
	if evidence := mapValue(status, "evidence"); evidence != nil {
		result.Pipeline = mapValue(evidence, "gitlab_pipeline")
		result.BuildKit = mapValue(evidence, "buildkit")
		result.Registry = mapValue(evidence, "registry")
		result.GitOps = mapValue(evidence, "gitops")
		result.ArgoCD = mapValue(evidence, "argocd")
		result.Quality = mapValue(evidence, "quality")
	}
	if jobs, err := fetchReleaseJobsData(backendURL, service, cluster); err != nil {
		result.Warnings = append(result.Warnings, "release jobs: "+err.Error())
	} else {
		result.Raw["jobs"] = jobs
		result.Jobs = mapsFromItems(jobs["items"])
		result.JobCount = intValue(jobs["item_count"])
	}
	if historyLimit > 0 {
		if history, err := fetchReleaseHistoryData(backendURL, service, cluster, historyLimit); err != nil {
			result.Warnings = append(result.Warnings, "release history: "+err.Error())
		} else {
			result.Raw["history"] = history
			result.History = mapsFromItems(history["items"])
			result.HistoryCount = intValue(history["item_count"])
		}
	}
	return result, nil
}

func triggerReleaseService(backendURL, service, ref, cluster string, variables map[string]string) (map[string]any, error) {
	values := addCluster(url.Values{"service": {service}, "ref": {ref}}, cluster)
	for key, value := range variables {
		values.Set("var."+key, value)
	}
	body, err := post(backendURL, "/api/release/trigger", values)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("release trigger response missing data")
	}
	return data, nil
}

func rollbackReleaseService(backendURL, service, target, cluster string) (map[string]any, error) {
	body, err := post(backendURL, "/api/release/rollback", addCluster(url.Values{
		"service": {service},
		"to":      {target},
		"confirm": {"true"},
	}, cluster))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("release rollback response missing data")
	}
	return data, nil
}

func unwrapData(body []byte, label string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("%s response missing data", label)
	}
	return data, nil
}

func fetchReleaseStatusData(backendURL, service, cluster string) (map[string]any, error) {
	body, err := get(backendURL, "/api/release/status", addCluster(url.Values{"service": {service}}, cluster))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("release status response missing data")
	}
	return data, nil
}

func fetchReleaseJobsData(backendURL, service, cluster string) (map[string]any, error) {
	body, err := get(backendURL, "/api/release/jobs", addCluster(url.Values{"service": {service}}, cluster))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("release jobs response missing data")
	}
	return data, nil
}

func fetchReleaseHistoryData(backendURL, service, cluster string, limit int) (map[string]any, error) {
	body, err := get(backendURL, "/api/release/history", addCluster(url.Values{"service": {service}, "limit": {strconv.Itoa(limit)}}, cluster))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("release history response missing data")
	}
	return data, nil
}

func runReleaseStatus(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release status", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release status requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/status", addCluster(url.Values{"service": []string{*service}}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release: %s\n", stringValue(data["service"]))
		fmt.Fprintf(w, "Status: %s stage=%s namespace=%s deployment=%s\n",
			stringValue(data["status"]), stringValue(data["stage"]), stringValue(data["namespace"]), stringValue(data["deployment"]))
		if image := stringValue(data["image"]); image != "" {
			fmt.Fprintf(w, "Image: %s\n", image)
		}
		if evidence := mapValue(data, "evidence"); evidence != nil {
			if k8s := mapValue(evidence, "kubernetes"); k8s != nil {
				fmt.Fprintf(w, "Kubernetes: ready=%d desired=%d updated=%d available=%d\n",
					intValue(k8s["ready_replicas"]), intValue(k8s["desired_replicas"]), intValue(k8s["updated_replicas"]), intValue(k8s["available_replicas"]))
			}
			if pods := mapValue(evidence, "pods"); pods != nil {
				fmt.Fprintf(w, "Pods: %d/%d listed\n", intValue(pods["item_count"]), intValue(pods["total_count"]))
			}
			if registry := mapValue(evidence, "registry"); registry != nil {
				fmt.Fprintf(w, "Registry: %s tag=%s\n", stringValue(registry["status"]), stringValue(registry["tag"]))
			}
			if pipeline := mapValue(evidence, "gitlab_pipeline"); pipeline != nil {
				fmt.Fprintf(w, "GitLab: %s ref=%s sha=%s\n", stringValue(pipeline["status"]), stringValue(pipeline["ref"]), stringValue(pipeline["sha"]))
			}
			if gitops := mapValue(evidence, "gitops"); gitops != nil {
				fmt.Fprintf(w, "GitOps: %s image=%s\n", stringValue(gitops["status"]), stringValue(gitops["desired_image"]))
			}
			if argocd := mapValue(evidence, "argocd"); argocd != nil {
				fmt.Fprintf(w, "Argo CD: sync=%s health=%s\n", stringValue(argocd["sync_status"]), stringValue(argocd["health_status"]))
			}
			if quality := mapValue(evidence, "quality"); quality != nil {
				fmt.Fprintf(w, "Quality: %s reason=%s optional=%t\n",
					stringValue(quality["status"]), stringValue(quality["reason"]), boolValue(quality["optional"]))
			}
		}
		if gaps := stringList(data["gaps"]); len(gaps) > 0 {
			fmt.Fprintf(w, "Gaps: %s\n", strings.Join(gaps, ", "))
			for _, detail := range mapsFromItems(data["gap_details"]) {
				fmt.Fprintf(w, "  - %s: %s Action: %s\n",
					stringValue(detail["code"]), stringValue(detail["impact"]), stringValue(detail["action"]))
			}
		}
		if checks := stringList(data["next_checks"]); len(checks) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(checks, "; "))
		}
		return nil
	})
}

func runReleaseJobs(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release jobs", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release jobs requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/jobs", addCluster(url.Values{"service": []string{*service}}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release jobs: %s\n", stringValue(data["service"]))
		if pipeline := mapValue(data, "pipeline"); pipeline != nil {
			fmt.Fprintf(w, "Pipeline: %s id=%d ref=%s sha=%s\n",
				stringValue(pipeline["status"]), intValue(pipeline["id"]), stringValue(pipeline["ref"]), stringValue(pipeline["sha"]))
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tSTAGE\tNAME\tSTATUS\tDURATION\tFAILURE")
		for _, job := range mapsFromItems(data["items"]) {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%.1fs\t%s\n",
				intValue(job["id"]), stringValue(job["stage"]), stringValue(job["name"]), stringValue(job["status"]), floatValue(job["duration"]), stringValue(job["failure_reason"]))
		}
		return tw.Flush()
	})
}

func runReleaseLogs(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release logs", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	job := fs.String("job", "", "GitLab job name")
	jobID := fs.String("job-id", "", "GitLab job id")
	tail := fs.Int("tail", 200, "tail lines")
	limitBytes := fs.Int("limit-bytes", 128*1024, "limit bytes")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release logs requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/logs", addCluster(url.Values{
		"service":     []string{*service},
		"job":         []string{*job},
		"job_id":      []string{*jobID},
		"tail_lines":  []string{strconv.Itoa(*tail)},
		"limit_bytes": []string{strconv.Itoa(*limitBytes)},
	}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release log: %s job=%s id=%d truncated=%t\n",
			stringValue(data["service"]), stringValue(data["job_name"]), intValue(data["job_id"]), boolValue(data["truncated"]))
		fmt.Fprintln(w, stringValue(data["text"]))
		return nil
	})
}

func runReleaseHistory(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release history", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	limit := fs.Int("limit", 10, "history item limit")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release history requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/history", addCluster(url.Values{"service": []string{*service}, "limit": []string{strconv.Itoa(*limit)}}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release history: %s\n", stringValue(data["service"]))
		if image := stringValue(data["current_image"]); image != "" {
			fmt.Fprintf(w, "Current image: %s\n", image)
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "CURRENT\tREVISION\tDATE\tTAG\tMESSAGE")
		for _, item := range mapsFromItems(data["items"]) {
			current := ""
			if boolValue(item["current"]) {
				current = "*"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				current,
				stringValue(item["short_revision"]),
				shortTime(stringValue(item["committed_at"])),
				stringValue(item["tag"]),
				oneLine(stringValue(item["message"]), 80),
			)
		}
		return tw.Flush()
	})
}

func runReleaseRollback(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release rollback", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	target := fs.String("to", "", "target tag, full image, or GitOps revision")
	fs.StringVar(target, "target", "", "target tag, full image, or GitOps revision")
	confirm := fs.Bool("confirm", false, "confirm GitOps rollback commit")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *target == "" && fs.NArg() > 1 {
		*target = fs.Arg(1)
	}
	if *service == "" {
		return fmt.Errorf("release rollback requires --service")
	}
	if *target == "" {
		return fmt.Errorf("release rollback requires --to")
	}
	if !*confirm {
		return fmt.Errorf("release rollback requires --confirm")
	}
	body, err := post(opts.backendURL, "/api/release/rollback", addCluster(url.Values{
		"service": []string{*service},
		"to":      []string{*target},
		"confirm": []string{strconv.FormatBool(*confirm)},
	}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Rollback: %s status=%s\n", stringValue(data["service"]), stringValue(data["status"]))
		fmt.Fprintf(w, "Previous: %s\n", stringValue(data["previous_image"]))
		fmt.Fprintf(w, "Target: %s\n", stringValue(data["target_image"]))
		fmt.Fprintf(w, "GitOps: %s %s branch=%s\n",
			stringValue(data["gitops_project"]), stringValue(data["gitops_path"]), stringValue(data["branch"]))
		if commit := stringValue(data["commit_short_id"]); commit != "" {
			fmt.Fprintf(w, "Commit: %s %s\n", commit, stringValue(data["commit_message"]))
		}
		if reason := stringValue(data["reason"]); reason != "" {
			fmt.Fprintf(w, "Reason: %s\n", reason)
		}
		if checks := stringList(data["next_checks"]); len(checks) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(checks, "; "))
		}
		return nil
	})
}
