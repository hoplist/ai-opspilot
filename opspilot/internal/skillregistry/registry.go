package skillregistry

import "strings"

const Version = "2026-06-02-code-precheck-skills"

type Skill struct {
	Name              string   `json:"name"`
	Label             string   `json:"label"`
	Category          string   `json:"category"`
	IntegrationTier   string   `json:"integration_tier"`
	Integrated        bool     `json:"integrated"`
	Priority          int      `json:"priority"`
	Summary           string   `json:"summary"`
	UseWhen           []string `json:"use_when"`
	Evidence          []string `json:"evidence"`
	Commands          []string `json:"commands"`
	Boundaries        []string `json:"boundaries"`
	NextIntegration   []string `json:"next_integration,omitempty"`
	SourceSkillPath   string   `json:"source_skill_path,omitempty"`
	SourceDescription string   `json:"source_description,omitempty"`
}

type Catalog struct {
	Version         string  `json:"version"`
	Source          string  `json:"source"`
	SourcePath      string  `json:"source_path,omitempty"`
	SourceVersion   string  `json:"source_version,omitempty"`
	ItemCount       int     `json:"item_count"`
	IntegratedCount int     `json:"integrated_count"`
	DynamicCount    int     `json:"dynamic_count,omitempty"`
	Items           []Skill `json:"items"`
}

type Recommendation struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Category string   `json:"category"`
	Reason   string   `json:"reason"`
	Commands []string `json:"commands,omitempty"`
}

func Registry(category string, integratedOnly bool) Catalog {
	return registryFromItems(allSkills(), category, integratedOnly, Catalog{
		Version: Version,
		Source:  "embedded",
	})
}

func registryFromItems(sourceItems []Skill, category string, integratedOnly bool, catalog Catalog) Catalog {
	items := []Skill{}
	for _, skill := range sourceItems {
		if category != "" && !strings.EqualFold(skill.Category, category) {
			continue
		}
		if integratedOnly && !skill.Integrated {
			continue
		}
		items = append(items, skill)
	}
	catalog.ItemCount = len(items)
	catalog.IntegratedCount = countIntegrated(items)
	catalog.Items = items
	if catalog.Version == "" {
		catalog.Version = Version
	}
	if catalog.Source == "" {
		catalog.Source = "embedded"
	}
	return catalog
}

func Summary(catalog Catalog) map[string]any {
	names := []string{}
	categories := map[string]int{}
	for _, skill := range catalog.Items {
		names = append(names, skill.Name)
		categories[skill.Category]++
	}
	return map[string]any{
		"version":          catalog.Version,
		"source":           catalog.Source,
		"source_path":      catalog.SourcePath,
		"source_version":   catalog.SourceVersion,
		"item_count":       catalog.ItemCount,
		"integrated_count": catalog.IntegratedCount,
		"dynamic_count":    catalog.DynamicCount,
		"names":            names,
		"categories":       categories,
	}
}

func Recommend(targetType, status string, missingEvidence, findings []string) []Recommendation {
	return RecommendFromCatalog(Registry("", true), targetType, status, missingEvidence, findings)
}

func RecommendFromCatalog(catalog Catalog, targetType, status string, missingEvidence, findings []string) []Recommendation {
	want := map[string]string{
		"opspilot-ops":     "default OpsPilot CLI entry for read-only checks",
		"debugging-wizard": "turn evidence into a hypothesis-driven fix plan",
	}
	switch strings.ToLower(targetType) {
	case "pod":
		want["kubernetes-specialist"] = "pod status, events, probes, restarts, and logs are primary evidence"
		want["monitoring-expert"] = "pod CPU, memory, restart, and log-source gaps need observability context"
		want["auto-inspection-rca"] = "pod evidence should be converted into an RCA-style evidence pack"
	case "cluster":
		want["kubernetes-specialist"] = "cluster inspection depends on workloads, events, nodes, and scheduling evidence"
		want["monitoring-expert"] = "cluster inspection depends on node, top pod, restart, and filesystem metrics"
	case "service":
		want["devops-engineer"] = "service checks include release mapping, GitLab, BuildKit, GitOps, and Argo CD"
		want["kubernetes-specialist"] = "service health is ultimately verified through Kubernetes workloads and Pods"
		want["monitoring-expert"] = "service checks need metrics and log-source evidence"
		want["auto-inspection-rca"] = "service evidence should be summarized for AI RCA and follow-up fixes"
	case "release":
		want["devops-engineer"] = "release status, logs, history, rollback, and GitOps evidence are the main workflow"
	case "repo", "code", "code-precheck":
		want["code-reviewer"] = "repo code precheck needs broad correctness and maintainability review"
		want["security-reviewer"] = "repo code precheck needs secrets, unsafe execution, and injection risk review"
		want["secure-code-guardian"] = "repo code precheck findings need secure remediation patterns"
		want["database-optimizer"] = "repo code precheck includes full-table, destructive SQL, and N+1 risk"
		want["devops-engineer"] = "repo code precheck is part of the GitLab CI release workflow"
	}
	if status != "" && status != "healthy" {
		want["auto-inspection-rca"] = "non-healthy status needs RCA evidence grouping"
	}
	if len(missingEvidence) > 0 {
		want["monitoring-expert"] = "missing evidence must be called out and mapped to unavailable integrations"
	}
	if status != "healthy" && containsAny(findings, "restart", "not ready", "crash", "failed", "BackOff") {
		want["kubernetes-specialist"] = "runtime failures need Kubernetes event and container-state reasoning"
		want["debugging-wizard"] = "failure signals need root-cause hypotheses and safe next checks"
	}

	recommendations := []Recommendation{}
	for _, skill := range catalog.Items {
		reason, ok := want[skill.Name]
		if !ok || !skill.Integrated {
			continue
		}
		recommendations = append(recommendations, Recommendation{
			Name:     skill.Name,
			Label:    skill.Label,
			Category: skill.Category,
			Reason:   reason,
			Commands: skill.Commands,
		})
	}
	return recommendations
}

func countIntegrated(items []Skill) int {
	count := 0
	for _, item := range items {
		if item.Integrated {
			count++
		}
	}
	return count
}

func containsAny(values []string, needles ...string) bool {
	for _, value := range values {
		lower := strings.ToLower(value)
		for _, needle := range needles {
			if strings.Contains(lower, strings.ToLower(needle)) {
				return true
			}
		}
	}
	return false
}

func allSkills() []Skill {
	return []Skill{
		{
			Name:            "opspilot-ops",
			Label:           "OpsPilot Ops",
			Category:        "platform",
			IntegrationTier: "core",
			Integrated:      true,
			Priority:        100,
			Summary:         "CLI-first OpsPilot workflow for Kubernetes, Prometheus, Docker node-agent, logs, evidence, and release checks.",
			UseWhen:         []string{"start any OpsPilot investigation", "inspect current platform health", "summarize evidence without bypassing the CLI"},
			Evidence:        []string{"capabilities", "doctor", "inspect", "release status", "metrics", "logs"},
			Commands:        []string{"doctor", "capabilities", "check cluster", "check pod", "check service", "release status"},
			Boundaries:      []string{"CLI first", "no direct cluster mutation", "report missing evidence explicitly"},
			SourceSkillPath: "C:/Users/Administrator/.codex/skills/opspilot-ops/SKILL.md",
		},
		{
			Name:            "auto-inspection-rca",
			Label:           "Auto Inspection RCA",
			Category:        "rca",
			IntegrationTier: "core",
			Integrated:      true,
			Priority:        90,
			Summary:         "RCA evidence grouping for Pods, services, events, logs, release context, and AI-readable fix planning.",
			UseWhen:         []string{"turn raw evidence into an RCA pack", "explain partial evidence", "prepare AI follow-up fixes"},
			Evidence:        []string{"events", "logs", "metrics", "release gaps", "likely causes"},
			Commands:        []string{"errors recent", "inspect pod", "inspect service", "fix pod --dry-run", "fix service --dry-run"},
			Boundaries:      []string{"read-only evidence first", "fix commands generate plans unless explicitly extended"},
			SourceSkillPath: "C:/Users/Administrator/.codex/skills/auto-inspection-rca/SKILL.md",
		},
		{
			Name:            "kubernetes-specialist",
			Label:           "Kubernetes Specialist",
			Category:        "kubernetes",
			IntegrationTier: "core",
			Integrated:      true,
			Priority:        80,
			Summary:         "Kubernetes workload and Pod troubleshooting rules for status, readiness, restarts, probes, events, RBAC, and scheduling.",
			UseWhen:         []string{"Pod is not ready", "containers are restarting", "events mention scheduling, image pull, probe, CNI, or RBAC issues"},
			Evidence:        []string{"Pod status", "Pod events", "current and previous logs", "Deployment/ReplicaSet/Node metadata"},
			Commands:        []string{"k8s pods --status abnormal", "context pod", "diagnose pod", "k8s logs pod", "inspect cluster"},
			Boundaries:      []string{"prefer declarative/GitOps fixes", "do not use imperative kubectl mutation from OpsPilot"},
			SourceSkillPath: "C:/Users/Administrator/.codex/skills/kubernetes-specialist/SKILL.md",
		},
		{
			Name:            "monitoring-expert",
			Label:           "Monitoring Expert",
			Category:        "observability",
			IntegrationTier: "core",
			Integrated:      true,
			Priority:        70,
			Summary:         "Prometheus, filesystem, log-source, and capability-gap reasoning for runtime troubleshooting.",
			UseWhen:         []string{"CPU, memory, restart, or disk evidence is needed", "log source is missing", "cluster or service health needs resource context"},
			Evidence:        []string{"Prometheus node metrics", "Pod metrics", "filesystem metrics", "host disk attribution", "ELK/OpenSearch readiness", "capability gaps"},
			Commands:        []string{"metrics health", "metrics nodes --source all", "metrics pods --source all", "metrics filesystems --source all", "host disk --host node206", "logs search"},
			Boundaries:      []string{"metrics enrich Kubernetes evidence but do not block Pod-first checks"},
			SourceSkillPath: "C:/Users/Administrator/.codex/skills/monitoring-expert/SKILL.md",
		},
		{
			Name:            "devops-engineer",
			Label:           "DevOps Engineer",
			Category:        "release",
			IntegrationTier: "core",
			Integrated:      true,
			Priority:        65,
			Summary:         "Release and onboarding workflow for Dockerfile, GitLab CI, BuildKit, Registry, GitOps, Argo CD, rollback, and repo governance.",
			UseWhen:         []string{"service release status is needed", "build or deploy failed", "repository needs platform onboarding", "rollback evidence is needed"},
			Evidence:        []string{"GitLab pipeline", "BuildKit job", "Registry tag", "GitOps desired image", "Argo CD sync/health", "repository preflight"},
			Commands:        []string{"release service", "release status", "release jobs", "release logs", "release history", "release rollback", "repo preflight", "repo autofix"},
			Boundaries:      []string{"publish through GitLab Runner -> BuildKit -> Registry -> GitOps -> Argo CD", "avoid direct Kubernetes mutation"},
			SourceSkillPath: "C:/Users/Administrator/.codex/skills/devops-engineer/SKILL.md",
		},
		{
			Name:            "code-reviewer",
			Label:           "Code Reviewer",
			Category:        "code-quality",
			IntegrationTier: "ci-evidence",
			Integrated:      true,
			Priority:        58,
			Summary:         "Explains OpsPilot code-precheck evidence for correctness, dangerous logic, N+1 patterns, and maintainability risks.",
			UseWhen:         []string{"GitLab code-precheck has warnings or blockers", "repository code needs safe review before release", "AI needs to explain source-level findings"},
			Evidence:        []string{"code-precheck.json", "repo precheck", "file path", "line number", "rule id", "recommendation"},
			Commands:        []string{"repo precheck", "repo precheck --write", "release jobs", "release logs"},
			Boundaries:      []string{"do not block on style preferences", "only high-confidence blocker rules should stop CI"},
			SourceSkillPath: "C:/Users/Administrator/.codex/skills/code-reviewer/SKILL.md",
		},
		{
			Name:            "security-reviewer",
			Label:           "Security Reviewer",
			Category:        "security",
			IntegrationTier: "ci-evidence",
			Integrated:      true,
			Priority:        57,
			Summary:         "Explains code-precheck security evidence such as hardcoded secrets, unsafe shell execution, injection risk, and dependency scan gaps.",
			UseWhen:         []string{"code-precheck reports secret_leak or dangerous_shell", "release is blocked by a security finding", "security remediation needs prioritization"},
			Evidence:        []string{"code-precheck.json", "GitLab artifact", "rule severity", "security category"},
			Commands:        []string{"repo precheck", "release jobs", "release logs"},
			Boundaries:      []string{"report critical findings immediately", "do not expose sensitive secret values in summaries"},
			SourceSkillPath: "C:/Users/Administrator/.codex/skills/security-reviewer/SKILL.md",
		},
		{
			Name:            "secure-code-guardian",
			Label:           "Secure Code Guardian",
			Category:        "security",
			IntegrationTier: "ci-evidence",
			Integrated:      true,
			Priority:        56,
			Summary:         "Provides secure remediation guidance for precheck findings: parameterized SQL, input validation, authentication, and safe secret handling.",
			UseWhen:         []string{"security finding needs a concrete code fix", "raw SQL construction needs parameterization", "input validation or auth hardening is needed"},
			Evidence:        []string{"code-precheck.json", "raw_sql_construction", "secret_leak", "api_query_writes_data"},
			Commands:        []string{"repo precheck", "fix service --dry-run"},
			Boundaries:      []string{"prefer minimal safe patches", "never store secrets in source code"},
			SourceSkillPath: "C:/Users/Administrator/.codex/skills/secure-code-guardian/SKILL.md",
		},
		{
			Name:            "database-optimizer",
			Label:           "Database Optimizer",
			Category:        "database",
			IntegrationTier: "ci-evidence",
			Integrated:      true,
			Priority:        55,
			Summary:         "Explains database-related precheck evidence including destructive SQL, unguarded writes, full-table scans, and possible N+1 queries.",
			UseWhen:         []string{"code-precheck reports database blockers", "query may scan full table", "loop contains database access", "pagination or index guidance is needed"},
			Evidence:        []string{"code-precheck.json", "db_destructive_sql", "db_unguarded_write", "db_full_table_read", "possible_n_plus_one"},
			Commands:        []string{"repo precheck", "release logs", "fix service --dry-run"},
			Boundaries:      []string{"do not run production EXPLAIN from OpsPilot without explicit scope", "precheck rules are static hints unless runtime DB evidence is attached"},
			SourceSkillPath: "C:/Users/Administrator/.codex/skills/database-optimizer/SKILL.md",
		},
		{
			Name:            "debugging-wizard",
			Label:           "Debugging Wizard",
			Category:        "ai-debugging",
			IntegrationTier: "core",
			Integrated:      true,
			Priority:        60,
			Summary:         "Hypothesis-driven debugging loop that turns logs, stack traces, events, metrics, and release evidence into safe next actions.",
			UseWhen:         []string{"evidence contains errors", "root cause is unclear", "AI needs a bounded fix plan"},
			Evidence:        []string{"findings", "likely causes", "recommended actions", "missing evidence"},
			Commands:        []string{"check service --output evidence", "check pod --output evidence", "fix service --dry-run --output evidence", "fix pod --dry-run --output evidence"},
			Boundaries:      []string{"do not guess beyond evidence", "separate code fixes from platform fixes"},
			SourceSkillPath: "C:/Users/Administrator/.codex/skills/debugging-wizard/SKILL.md",
		},
	}
}
