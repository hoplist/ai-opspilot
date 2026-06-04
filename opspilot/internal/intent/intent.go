package intent

import (
	"regexp"
	"strings"
)

// Request describes a natural-language request that should be mapped to a
// deterministic OpsPilot action.
type Request struct {
	Query           string   `json:"query"`
	ServiceOverride string   `json:"service_override,omitempty"`
	Services        []string `json:"services,omitempty"`
}

// Intent is a plan-level interpretation of a user request. Execution policy is
// handled by the caller so parse stays side-effect free.
type Intent struct {
	Action      string   `json:"action"`
	Service     string   `json:"service,omitempty"`
	Target      string   `json:"target,omitempty"`
	Command     []string `json:"command"`
	Risk        string   `json:"risk"`
	Automation  string   `json:"automation"`
	Confidence  float64  `json:"confidence"`
	Warnings    []string `json:"warnings,omitempty"`
	Next        []string `json:"next,omitempty"`
	Explanation string   `json:"explanation,omitempty"`
}

// Interpret maps a small, auditable natural-language surface to stable
// OpsPilot commands. It is deliberately deterministic and conservative.
func Interpret(req Request) Intent {
	query := strings.TrimSpace(req.Query)
	lower := strings.ToLower(query)
	service := firstNonEmpty(req.ServiceOverride, serviceFromText(lower, req.Services))
	intent := Intent{
		Action:      "inspect_service",
		Service:     service,
		Command:     []string{"inspect", "service", service},
		Risk:        "read_only",
		Automation:  "auto_execute",
		Confidence:  0.65,
		Explanation: "Defaulted to service inspection because the request does not clearly ask for release, rollback, or history.",
	}
	if service == "" {
		intent.Command = []string{}
		intent.Confidence = 0.25
		intent.Warnings = append(intent.Warnings, "service could not be identified from the request")
		intent.Next = append(intent.Next, "Add --service or include a configured service name in the request.")
	}

	switch {
	case containsAny(lower, []string{"rollback", "roll back", "revert", "回退", "退回", "回滚", "鍥為", "閫€鍥"}):
		target := rollbackTargetFromText(query)
		intent.Action = "rollback_service"
		intent.Target = target
		intent.Command = []string{"release", "rollback", service, target, "--confirm"}
		intent.Risk = "controlled_mutate"
		intent.Automation = "plan_first"
		intent.Confidence = confidenceWithService(service, 0.75)
		intent.Explanation = "Rollback changes GitOps desired state, so OpsPilot should show a plan before executing."
		if target == "" {
			intent.Warnings = append(intent.Warnings, "rollback target could not be identified")
			intent.Next = append(intent.Next, "Ask for release history, then provide the target revision or tag.")
		}
	case containsAny(lower, []string{"history", "release history", "version history", "发布记录", "版本记录", "历史", "鍘嗗彶", "鍙戝竷璁板綍", "鐗堟湰璁板綍"}):
		intent.Action = "release_history"
		intent.Command = []string{"release", "history", service}
		intent.Risk = "read_only"
		intent.Automation = "auto_execute"
		intent.Confidence = confidenceWithService(service, 0.8)
		intent.Explanation = "Release history is read-only evidence."
	case containsAny(lower, []string{"release", "deploy", "publish", "上线", "发布", "发版", "鍙戝竷", "涓婄嚎", "鍙戠増"}):
		intent.Action = "release_service"
		intent.Command = []string{"release", "service", service, "--trigger"}
		intent.Risk = "controlled_mutate"
		intent.Automation = "plan_first"
		intent.Confidence = confidenceWithService(service, 0.75)
		intent.Explanation = "Release can mutate CI/GitOps state, so natural-language entrypoints default to plan-first or dry-run."
	}
	return intent
}

func confidenceWithService(service string, base float64) float64 {
	if service == "" {
		return 0.3
	}
	return base
}

func serviceFromText(text string, services []string) string {
	for _, service := range services {
		if service != "" && strings.Contains(text, strings.ToLower(service)) {
			return service
		}
	}
	matches := regexp.MustCompile(`[a-zA-Z0-9][a-zA-Z0-9._/]*-[a-zA-Z0-9][a-zA-Z0-9._/-]*`).FindAllString(text, -1)
	if len(matches) > 0 {
		return strings.Trim(matches[0], `"'.,，。；;:：`)
	}
	return ""
}

func rollbackTargetFromText(query string) string {
	fields := strings.Fields(query)
	for i, field := range fields {
		lower := strings.ToLower(strings.Trim(field, `"'.,，。；;:：`))
		if (lower == "to" || lower == "target" || lower == "到" || lower == "至") && i+1 < len(fields) {
			return strings.Trim(fields[i+1], `"'.,，。；;:：`)
		}
	}
	return ""
}

func containsAny(text string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
