package skillregistry

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type MirrorEntry struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	Source       string `json:"source,omitempty"`
	UpstreamPath string `json:"upstream_path,omitempty"`
	RuntimePath  string `json:"runtime_path,omitempty"`
	Category     string `json:"category,omitempty"`
	Priority     int    `json:"priority,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type MirrorIndex struct {
	Ready            bool          `json:"ready"`
	Root             string        `json:"root"`
	RegistryPath     string        `json:"registry_path,omitempty"`
	ImportRulesPath  string        `json:"import_rules_path,omitempty"`
	SkillsCount      int           `json:"skills_count"`
	CandidateCount   int           `json:"candidate_count"`
	UnsupportedCount int           `json:"unsupported_count"`
	UpstreamCount    int           `json:"upstream_count"`
	Integrated       []MirrorEntry `json:"integrated,omitempty"`
	Candidates       []MirrorEntry `json:"candidates,omitempty"`
	Unsupported      []MirrorEntry `json:"unsupported,omitempty"`
	Sources          []MirrorEntry `json:"sources,omitempty"`
	Warnings         []string      `json:"warnings,omitempty"`
}

type GeneratedSkillFile struct {
	Path   string `json:"path"`
	Body   string `json:"body"`
	Exists bool   `json:"exists"`
}

type ImportPlan struct {
	Ready       bool                 `json:"ready"`
	DryRun      bool                 `json:"dry_run"`
	Name        string               `json:"name"`
	Status      string               `json:"status"`
	Source      string               `json:"source,omitempty"`
	Category    string               `json:"category,omitempty"`
	Reason      string               `json:"reason,omitempty"`
	RuntimePath string               `json:"runtime_path,omitempty"`
	Candidate   MirrorEntry          `json:"candidate,omitempty"`
	Files       []GeneratedSkillFile `json:"files,omitempty"`
	Warnings    []string             `json:"warnings,omitempty"`
	Next        []string             `json:"next,omitempty"`
}

func MirrorFromEnv() MirrorIndex {
	return MirrorWithSkillsDir(env("OPSPILOT_SKILLS_DIR", defaultDynamicSkillsDir))
}

func MirrorWithSkillsDir(skillsDir string) MirrorIndex {
	root := mirrorRootFromSkillsDir(skillsDir)
	index := MirrorIndex{
		Root:            root,
		RegistryPath:    filepath.Join(root, "registry.yaml"),
		ImportRulesPath: filepath.Join(root, "import-rules.yaml"),
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		index.Warnings = append(index.Warnings, "skills mirror: root is not readable")
		return index
	}
	index.Ready = true
	index.SkillsCount = countSkillYAML(filepath.Join(root, "skills"))
	index.UpstreamCount = countDirs(filepath.Join(root, "upstream"))
	entries := parseMirrorRegistry(index.RegistryPath)
	if len(entries) == 0 {
		index.Warnings = append(index.Warnings, "skills mirror: registry.yaml has no entries")
		entries = append(entries, entriesFromDir(filepath.Join(root, "skills"), "integrated")...)
		entries = append(entries, entriesFromDir(filepath.Join(root, "candidates"), "candidate")...)
	}
	for _, entry := range entries {
		switch strings.ToLower(entry.Status) {
		case "integrated":
			index.Integrated = append(index.Integrated, entry)
		case "unsupported":
			index.Unsupported = append(index.Unsupported, entry)
		default:
			if entry.Status == "" {
				entry.Status = "candidate"
			}
			index.Candidates = append(index.Candidates, entry)
		}
	}
	index.CandidateCount = len(index.Candidates)
	index.UnsupportedCount = len(index.Unsupported)
	index.Sources = parseMirrorSources(index.RegistryPath)
	sortMirrorEntries(index.Integrated)
	sortMirrorEntries(index.Candidates)
	sortMirrorEntries(index.Unsupported)
	sortMirrorEntries(index.Sources)
	return index
}

func ImportPlanFromEnv(name string) ImportPlan {
	return ImportPlanWithSkillsDir(env("OPSPILOT_SKILLS_DIR", defaultDynamicSkillsDir), name)
}

func ImportPlanWithSkillsDir(skillsDir, name string) ImportPlan {
	name = strings.TrimSpace(name)
	plan := ImportPlan{
		DryRun: true,
		Name:   name,
		Status: "not_found",
		Next: []string{
			"Check skills candidates to confirm the candidate name.",
			"Only commit generated files to the GitLab skills repository after review.",
		},
	}
	if name == "" {
		plan.Warnings = append(plan.Warnings, "skill name is required")
		return plan
	}
	index := MirrorWithSkillsDir(skillsDir)
	plan.Warnings = append(plan.Warnings, index.Warnings...)
	if !index.Ready {
		plan.Status = "mirror_unavailable"
		plan.Next = []string{"Fix the server-side skills mirror mount or OPSPILOT_SKILLS_DIR before importing candidates."}
		return plan
	}
	for _, entry := range index.Integrated {
		if strings.EqualFold(entry.Name, name) {
			plan.Ready = true
			plan.Status = "already_integrated"
			plan.Source = entry.Source
			plan.Category = entry.Category
			plan.Reason = entry.Reason
			plan.RuntimePath = firstNonEmptyString(entry.RuntimePath, filepath.ToSlash(filepath.Join("skills", entry.Name)))
			plan.Candidate = entry
			plan.Next = []string{"No import is needed; this skill is already enabled under skills/."}
			return plan
		}
	}
	for _, entry := range index.Unsupported {
		if strings.EqualFold(entry.Name, name) {
			plan.Status = "unsupported"
			plan.Source = entry.Source
			plan.Category = entry.Category
			plan.Reason = entry.Reason
			plan.Candidate = entry
			plan.Warnings = append(plan.Warnings, firstNonEmptyString(entry.Reason, "candidate requires unsupported runtime capabilities"))
			plan.Next = []string{"Do not promote this candidate until its runtime dependency can execute server-side in OpsPilot."}
			return plan
		}
	}
	for _, entry := range index.Candidates {
		if strings.EqualFold(entry.Name, name) {
			return buildImportPlan(index.Root, entry)
		}
	}
	return plan
}

func buildImportPlan(root string, entry MirrorEntry) ImportPlan {
	runtimePath := firstNonEmptyString(entry.RuntimePath, filepath.ToSlash(filepath.Join("skills", entry.Name)))
	category := firstNonEmptyString(entry.Category, "workflow")
	files := []GeneratedSkillFile{
		generatedSkillFile(root, filepath.ToSlash(filepath.Join(runtimePath, "skill.yaml")), renderCandidateSkillYAML(entry, category)),
		generatedSkillFile(root, filepath.ToSlash(filepath.Join(runtimePath, "SKILL.md")), renderCandidateSkillMarkdown(entry, category)),
		generatedSkillFile(root, filepath.ToSlash(filepath.Join(runtimePath, "examples", entry.Name+"-example.md")), renderCandidateExample(entry)),
	}
	return ImportPlan{
		Ready:       true,
		DryRun:      true,
		Name:        entry.Name,
		Status:      "candidate_plan",
		Source:      entry.Source,
		Category:    category,
		Reason:      entry.Reason,
		RuntimePath: runtimePath,
		Candidate:   entry,
		Files:       files,
		Next: []string{
			"Review the generated draft and adjust commands/boundaries for OpsPilot server-side execution.",
			"Commit the reviewed files under skills/ in the GitLab skills repository.",
			"Run opspilot skills validate after GitLab sync; only then treat the skill as enabled.",
		},
	}
}

func generatedSkillFile(root, relPath, body string) GeneratedSkillFile {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(relPath)))
	return GeneratedSkillFile{Path: relPath, Body: body, Exists: err == nil}
}

func mirrorRootFromSkillsDir(skillsDir string) string {
	dir := strings.TrimSpace(skillsDir)
	if dir == "" {
		dir = defaultDynamicSkillsDir
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	if filepath.Base(dir) == "skills" {
		return filepath.Dir(dir)
	}
	return dir
}

func countSkillYAML(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == "skill.yaml" {
			count++
		}
		return nil
	})
	return count
}

func countDirs(root string) int {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			count++
		}
	}
	return count
}

func parseMirrorRegistry(path string) []MirrorEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	entries := []MirrorEntry{}
	var current *MirrorEntry
	inSkills := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "skills:" {
			inSkills = true
			continue
		}
		if line == "sources:" {
			inSkills = false
			continue
		}
		if !inSkills {
			continue
		}
		if strings.HasPrefix(line, "- name:") {
			if current != nil {
				entries = append(entries, *current)
			}
			current = &MirrorEntry{Name: cleanYAMLValue(strings.TrimPrefix(line, "- name:"))}
			continue
		}
		if current == nil {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		setMirrorEntryField(current, normalizeYAMLKey(key), cleanYAMLValue(value))
	}
	if current != nil {
		entries = append(entries, *current)
	}
	return entries
}

func parseMirrorSources(path string) []MirrorEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	sources := []MirrorEntry{}
	var current *MirrorEntry
	inSources := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "sources:" {
			inSources = true
			continue
		}
		if line == "skills:" {
			inSources = false
			continue
		}
		if !inSources {
			continue
		}
		if strings.HasPrefix(line, "- name:") {
			if current != nil {
				sources = append(sources, *current)
			}
			current = &MirrorEntry{Name: cleanYAMLValue(strings.TrimPrefix(line, "- name:"))}
			continue
		}
		if current == nil {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		setMirrorEntryField(current, normalizeYAMLKey(key), cleanYAMLValue(value))
	}
	if current != nil {
		sources = append(sources, *current)
	}
	return sources
}

func entriesFromDir(root, status string) []MirrorEntry {
	entries := []MirrorEntry{}
	for _, dir := range listDirs(root) {
		entries = append(entries, MirrorEntry{Name: dir, Status: status})
	}
	return entries
}

func listDirs(root string) []string {
	items, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	names := []string{}
	for _, item := range items {
		if item.IsDir() && !strings.HasPrefix(item.Name(), ".") {
			names = append(names, item.Name())
		}
	}
	sort.Strings(names)
	return names
}

func setMirrorEntryField(entry *MirrorEntry, key, value string) {
	switch key {
	case "status":
		entry.Status = value
	case "source":
		entry.Source = value
	case "upstream_path":
		entry.UpstreamPath = value
	case "runtime_path":
		entry.RuntimePath = value
	case "category":
		entry.Category = value
	case "priority":
		if parsed, err := strconv.Atoi(value); err == nil {
			entry.Priority = parsed
		}
	case "reason":
		entry.Reason = value
	}
}

func sortMirrorEntries(entries []MirrorEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Status == entries[j].Status {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Status < entries[j].Status
	})
}

func renderCandidateSkillYAML(entry MirrorEntry, category string) string {
	return fmt.Sprintf(`name: %s
label: %s
category: %s
integration_tier: candidate
integrated: false
priority: %d
summary: Review draft for server-side OpsPilot adaptation from %s.
use_when:
  - review whether %s should become an OpsPilot runtime skill
evidence:
  - upstream source: %s
  - candidate reason: %s
commands:
%s
boundaries:
  - dry-run draft only until reviewed and committed in GitLab
  - server-side execution only; no client-local dependencies
  - no destructive actions without an explicit OpsPilot-controlled guard
source_description: Generated import draft from skills mirror candidate metadata.
`, entry.Name, titleFromName(entry.Name), category, candidatePriority(entry), firstNonEmptyString(entry.Source, "unknown source"), entry.Name, firstNonEmptyString(entry.UpstreamPath, entry.Source, "unknown"), firstNonEmptyString(entry.Reason, "not specified"), yamlList(candidateCommands(entry)))
}

func renderCandidateSkillMarkdown(entry MirrorEntry, category string) string {
	return fmt.Sprintf(`# %s

Generated review draft for OpsPilot server-side skill promotion.

## Source

- Source: %s
- Upstream path: %s
- Category: %s
- Reason: %s

## Runtime Intent

Use this skill only after adapting it to OpsPilot's backend capabilities. The client CLI should call OpsPilot APIs; it should not carry its own copy of this skill.

## Suggested Commands

%s

## Review Checklist

- Confirm every command maps to an existing OpsPilot CLI/API command or a safe new backend capability.
- Remove any browser, desktop, local shell, or user-session dependency.
- Keep risky actions plan-only unless OpsPilot has an explicit confirmation and permission gate.
- Run `+"`opspilot skills validate`"+` after GitLab sync before marking this skill integrated.
`, titleFromName(entry.Name), firstNonEmptyString(entry.Source, "unknown"), firstNonEmptyString(entry.UpstreamPath, "not specified"), category, firstNonEmptyString(entry.Reason, "not specified"), markdownList(candidateCommands(entry)))
}

func renderCandidateExample(entry MirrorEntry) string {
	return fmt.Sprintf(`# %s Example

User asks OpsPilot a question that may benefit from %s.

OpsPilot should:

1. Pick this skill only if it is integrated in the server-side skills registry.
2. Gather read-only evidence first.
3. Return a concise plan or result with missing evidence called out.
`, titleFromName(entry.Name), entry.Name)
}

func candidatePriority(entry MirrorEntry) int {
	if entry.Priority > 0 {
		return entry.Priority
	}
	return 60
}

func candidateCommands(entry MirrorEntry) []string {
	switch strings.ToLower(entry.Name) {
	case "gstack-health":
		return []string{"doctor", "inspect cluster", "repo precheck"}
	case "gstack-canary":
		return []string{"release status", "quality status", "inspect service"}
	case "gstack-careful", "gstack-guard":
		return []string{"repo preflight", "app decommission plan", "janitor plan"}
	case "gstack-qa":
		return []string{"quality run", "quality status", "repo precheck"}
	case "gstack-benchmark":
		return []string{"quality run", "metrics query", "inspect service"}
	case "gstack-document-release":
		return []string{"release history", "release status"}
	case "gstack-autoplan", "gstack-spec":
		return []string{"onboard plan", "repo autofix --dry-run", "repo preflight"}
	default:
		return []string{"inspect service", "repo precheck"}
	}
}

func yamlList(items []string) string {
	var b strings.Builder
	for _, item := range items {
		b.WriteString("  - ")
		b.WriteString(item)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func markdownList(items []string) string {
	var b strings.Builder
	for _, item := range items {
		b.WriteString("- `")
		b.WriteString(item)
		b.WriteString("`\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func titleFromName(name string) string {
	parts := strings.Fields(strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " "))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
