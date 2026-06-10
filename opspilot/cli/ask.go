package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	intentpkg "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/intent"
)

type naturalLanguageResult struct {
	Query    string   `json:"query"`
	Action   string   `json:"action"`
	Service  string   `json:"service,omitempty"`
	Command  []string `json:"command"`
	Executed bool     `json:"executed"`
	DryRun   bool     `json:"dry_run"`
	Message  string   `json:"message,omitempty"`
	Result   any      `json:"result,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type naturalLanguageIntent = intentpkg.Intent

func runNaturalLanguage(opts globalOptions, args []string, out io.Writer) error {
	service := ""
	ref := "main"
	dryRun := false
	queryParts := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--dry-run":
			dryRun = true
		case arg == "--service" && i+1 < len(args):
			service = args[i+1]
			i++
		case strings.HasPrefix(arg, "--service="):
			service = strings.TrimPrefix(arg, "--service=")
		case arg == "--ref" && i+1 < len(args):
			ref = args[i+1]
			i++
		case strings.HasPrefix(arg, "--ref="):
			ref = strings.TrimPrefix(arg, "--ref=")
		default:
			queryParts = append(queryParts, arg)
		}
	}
	query := strings.TrimSpace(strings.Join(queryParts, " "))
	if query == "" {
		return fmt.Errorf("ask requires natural language text")
	}
	intent, warnings := fetchNaturalLanguageIntent(opts.backendURL, query, service)
	if intent.Service == "" {
		result := naturalLanguageResult{
			Query:    query,
			Action:   intent.Action,
			Command:  intent.Command,
			Executed: false,
			DryRun:   dryRun,
			Message:  "service could not be identified from the request",
			Warnings: warnings,
		}
		_ = writeOutput(out, opts.output, result, writeNaturalLanguageHuman(result))
		return fmt.Errorf("service could not be identified from natural language")
	}
	result := naturalLanguageResult{
		Query:    query,
		Action:   intent.Action,
		Service:  intent.Service,
		Command:  intent.Command,
		Executed: false,
		DryRun:   dryRun,
		Warnings: warnings,
	}
	if dryRun {
		result.Message = "dry run only; no action executed"
		return writeOutput(out, opts.output, result, writeNaturalLanguageHuman(result))
	}
	switch intent.Action {
	case "inspect_service":
		payload, err := fetchInspectService(opts.backendURL, intent.Service, "test", "", opts.cluster, 300, defaultPodLogSinceSeconds)
		if err != nil {
			return err
		}
		result.Executed = true
		result.Result = payload
	case "release_service":
		payload, err := triggerReleaseService(opts.backendURL, intent.Service, ref, opts.cluster, nil)
		if err != nil {
			return err
		}
		result.Executed = true
		result.Result = payload
	case "release_history":
		payload, err := fetchReleaseHistoryData(opts.backendURL, intent.Service, opts.cluster, 10)
		if err != nil {
			return err
		}
		result.Executed = true
		result.Result = payload
	case "rollback_service":
		if intent.Target == "" {
			return fmt.Errorf("rollback target could not be identified from natural language")
		}
		payload, err := rollbackReleaseService(opts.backendURL, intent.Service, intent.Target, opts.cluster)
		if err != nil {
			return err
		}
		result.Executed = true
		result.Result = payload
	default:
		return fmt.Errorf("unsupported natural language action: %s", intent.Action)
	}
	return writeOutput(out, opts.output, result, writeNaturalLanguageHuman(result))
}

func writeNaturalLanguageHuman(result naturalLanguageResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Ask: %s\n", result.Query)
		fmt.Fprintf(w, "Intent: %s service=%s executed=%t\n", result.Action, result.Service, result.Executed)
		if len(result.Command) > 0 {
			fmt.Fprintf(w, "Command: opspilot %s\n", strings.Join(result.Command, " "))
		}
		if result.Message != "" {
			fmt.Fprintf(w, "Message: %s\n", result.Message)
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		if result.Result != nil {
			switch payload := result.Result.(type) {
			case inspectServiceResult:
				fmt.Fprintf(w, "Status: %s stage=%s namespace=%s deployment=%s\n", payload.Status, payload.Stage, payload.Namespace, payload.Deployment)
				fmt.Fprintf(w, "Usage: pods=%d restarts=%d CPU %.3f cores memory %.1f MiB\n", payload.PodCount, payload.RestartCount, payload.TotalCPUCore, payload.TotalMemoryMiB)
				if len(payload.Findings) > 0 {
					fmt.Fprintf(w, "Findings: %s\n", strings.Join(payload.Findings, "; "))
				}
				if len(payload.EvidenceGaps) > 0 {
					fmt.Fprintf(w, "Evidence gaps: %s\n", strings.Join(payload.EvidenceGaps, ", "))
				}
				if len(payload.AvailableEvidence) > 0 {
					fmt.Fprintf(w, "Available evidence: %s\n", strings.Join(payload.AvailableEvidence, "; "))
				}
				if len(payload.MissingEvidence) > 0 {
					fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(payload.MissingEvidence, "; "))
				}
				return nil
			case map[string]any:
				if pipeline := mapValue(payload, "pipeline"); pipeline != nil {
					fmt.Fprintf(w, "Pipeline: id=%d status=%s ref=%s sha=%s\n",
						intValue(pipeline["id"]), stringValue(pipeline["status"]), stringValue(pipeline["ref"]), stringValue(pipeline["sha"]))
					if checks := stringList(payload["next_checks"]); len(checks) > 0 {
						fmt.Fprintf(w, "Next: %s\n", strings.Join(checks, "; "))
					}
					return nil
				}
			}
			body, err := json.MarshalIndent(result.Result, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(w, string(body))
		}
		return nil
	}
}

func fetchNaturalLanguageIntent(backendURL, query, service string) (intentpkg.Intent, []string) {
	body, err := get(backendURL, "/api/intent/parse", url.Values{
		"query":   {query},
		"service": {service},
	})
	if err == nil {
		var payload map[string]any
		if jsonErr := json.Unmarshal(body, &payload); jsonErr != nil {
			return intentpkg.Intent{}, []string{"intent parse: " + jsonErr.Error()}
		}
		data := mapValue(payload, "data")
		if data == nil {
			return intentpkg.Intent{}, []string{"intent parse: response missing data"}
		}
		raw, _ := json.Marshal(data)
		var parsed intentpkg.Intent
		if jsonErr := json.Unmarshal(raw, &parsed); jsonErr != nil {
			return intentpkg.Intent{}, []string{"intent parse: " + jsonErr.Error()}
		}
		warnings := append(stringList(payload["warnings"]), parsed.Warnings...)
		return parsed, warnings
	}
	services, warnings := fetchConfiguredServices(backendURL)
	warnings = append(warnings, "backend intent parser unavailable; used CLI compatibility parser: "+err.Error())
	parsed := intentpkg.Interpret(intentpkg.Request{
		Query:           query,
		ServiceOverride: service,
		Services:        services,
	})
	return parsed, append(warnings, parsed.Warnings...)
}

func interpretNaturalLanguage(query, serviceOverride string, services []string) naturalLanguageIntent {
	lower := strings.ToLower(query)
	service := firstNonEmptyString(serviceOverride, serviceFromText(lower, services))
	action := "inspect_service"
	command := []string{"inspect", "service", service}
	if containsAny(lower, []string{"回退", "rollback", "退回"}) {
		target := rollbackTargetFromText(query)
		action = "rollback_service"
		command = []string{"release", "rollback", service, target, "--confirm"}
		return naturalLanguageIntent{Action: action, Service: service, Target: target, Command: command}
	}
	if containsAny(lower, []string{"历史", "history", "版本记录", "发布记录"}) {
		action = "release_history"
		command = []string{"release", "history", service}
		return naturalLanguageIntent{Action: action, Service: service, Command: command}
	}
	if containsAny(lower, []string{"发布", "上线", "release", "deploy", "发版"}) {
		action = "release_service"
		command = []string{"release", "service", service, "--trigger"}
		return naturalLanguageIntent{Action: action, Service: service, Command: command}
	}
	return naturalLanguageIntent{Action: action, Service: service, Command: command}
}

func fetchConfiguredServices(backendURL string) ([]string, []string) {
	body, err := get(backendURL, "/api/health", url.Values{})
	if err != nil {
		return nil, []string{"health: " + err.Error()}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, []string{"health: " + err.Error()}
	}
	data := mapValue(payload, "data")
	release := mapValue(data, "release")
	out := []string{}
	for _, item := range stringList(release["services"]) {
		if item != "" {
			out = append(out, item)
		}
	}
	return out, nil
}

func serviceFromText(text string, services []string) string {
	for _, service := range services {
		if service != "" && strings.Contains(text, strings.ToLower(service)) {
			return service
		}
	}
	matches := regexp.MustCompile(`[a-zA-Z0-9][a-zA-Z0-9._/]*-[a-zA-Z0-9][a-zA-Z0-9._/-]*`).FindAllString(text, -1)
	if len(matches) > 0 {
		return strings.Trim(matches[0], `"'.,，。;；:：`)
	}
	return ""
}

func rollbackTargetFromText(query string) string {
	fields := strings.Fields(query)
	for i, field := range fields {
		lower := strings.ToLower(strings.Trim(field, `"'.,，。;；:：`))
		if (lower == "到" || lower == "to" || lower == "target") && i+1 < len(fields) {
			return strings.Trim(fields[i+1], `"'.,，。;；:：`)
		}
	}
	return ""
}
