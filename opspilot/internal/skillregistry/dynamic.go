package skillregistry

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const defaultDynamicSkillsDir = "/opt/opspilot/skills/current"

type Options struct {
	DynamicEnabled bool
	SkillsDir      string
}

func RegistryFromEnv(category string, integratedOnly bool) (Catalog, []string) {
	return RegistryWithOptions(category, integratedOnly, Options{
		DynamicEnabled: boolEnv("OPSPILOT_SKILLS_DYNAMIC_ENABLED", true),
		SkillsDir:      env("OPSPILOT_SKILLS_DIR", defaultDynamicSkillsDir),
	})
}

func RegistryWithOptions(category string, integratedOnly bool, opts Options) (Catalog, []string) {
	base := allSkills()
	if !opts.DynamicEnabled {
		return registryFromItems(base, category, integratedOnly, Catalog{
			Version: Version,
			Source:  "embedded",
		}), nil
	}
	dir := strings.TrimSpace(opts.SkillsDir)
	if dir == "" {
		dir = defaultDynamicSkillsDir
	}
	dynamic, sourceVersion, warnings := loadDynamicSkills(dir)
	if len(dynamic) == 0 {
		return registryFromItems(base, category, integratedOnly, Catalog{
			Version:    Version,
			Source:     "embedded",
			SourcePath: dir,
		}), warnings
	}
	merged := mergeSkills(base, dynamic)
	return registryFromItems(merged, category, integratedOnly, Catalog{
		Version:       Version,
		Source:        "dynamic+embedded",
		SourcePath:    dir,
		SourceVersion: sourceVersion,
		DynamicCount:  len(dynamic),
	}), warnings
}

func loadDynamicSkills(root string) ([]Skill, string, []string) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", []string{"skills: dynamic directory not found; using embedded registry"}
		}
		return nil, "", []string{"skills: cannot read dynamic directory: " + err.Error()}
	}
	if !info.IsDir() {
		return nil, "", []string{"skills: dynamic path is not a directory; using embedded registry"}
	}
	walkRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		walkRoot = resolved
	}
	version := readTrimmed(filepath.Join(root, ".opspilot-skills-version"))
	warnings := []string{}
	files := []string{}
	err = filepath.WalkDir(walkRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			warnings = append(warnings, "skills: cannot walk "+path+": "+walkErr.Error())
			return nil
		}
		name := entry.Name()
		if entry.IsDir() && (name == ".git" || strings.HasPrefix(name, ".")) && path != root {
			return filepath.SkipDir
		}
		if entry.IsDir() || name != "skill.yaml" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		warnings = append(warnings, "skills: cannot scan dynamic directory: "+err.Error())
	}
	sort.Strings(files)
	skills := []Skill{}
	for _, file := range files {
		skill, enabled, err := readDynamicSkill(file)
		if err != nil {
			warnings = append(warnings, "skills: "+err.Error())
			continue
		}
		if !enabled {
			continue
		}
		skills = append(skills, skill)
	}
	if len(files) == 0 {
		warnings = append(warnings, "skills: no skill.yaml files found; using embedded registry")
	}
	return skills, version, warnings
}

func readDynamicSkill(path string) (Skill, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, false, fmt.Errorf("%s cannot be read: %w", path, err)
	}
	skill, enabled, err := parseSkillYAML(string(data))
	if err != nil {
		return Skill{}, false, fmt.Errorf("%s is invalid: %w", path, err)
	}
	if !enabled {
		return skill, false, nil
	}
	if strings.TrimSpace(skill.Name) == "" {
		return Skill{}, false, fmt.Errorf("%s is invalid: name is required", path)
	}
	if strings.TrimSpace(skill.Category) == "" {
		return Skill{}, false, fmt.Errorf("%s is invalid: category is required", path)
	}
	if strings.TrimSpace(skill.Label) == "" {
		skill.Label = skill.Name
	}
	if strings.TrimSpace(skill.IntegrationTier) == "" {
		skill.IntegrationTier = "dynamic"
	}
	if skill.Priority == 0 {
		skill.Priority = 50
	}
	if skill.Commands == nil {
		skill.Commands = []string{}
	}
	if skill.UseWhen == nil {
		skill.UseWhen = []string{}
	}
	if skill.Evidence == nil {
		skill.Evidence = []string{}
	}
	if skill.Boundaries == nil {
		skill.Boundaries = []string{}
	}
	if skill.SourceSkillPath == "" {
		skill.SourceSkillPath = path
	}
	return skill, true, nil
}

func parseSkillYAML(text string) (Skill, bool, error) {
	var skill Skill
	enabled := true
	currentList := ""
	for lineNo, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			if currentList == "" {
				return Skill{}, false, fmt.Errorf("line %d list item has no key", lineNo+1)
			}
			appendListValue(&skill, currentList, cleanYAMLValue(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		currentList = ""
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return Skill{}, false, fmt.Errorf("line %d must be key: value", lineNo+1)
		}
		key = normalizeYAMLKey(key)
		value = cleanYAMLValue(value)
		switch key {
		case "name":
			skill.Name = value
		case "label":
			skill.Label = value
		case "category":
			skill.Category = value
		case "integration_tier":
			skill.IntegrationTier = value
		case "integrated":
			skill.Integrated = parseBool(value, true)
		case "enabled":
			enabled = parseBool(value, true)
		case "priority":
			if value == "" {
				continue
			}
			priority, err := strconv.Atoi(value)
			if err != nil {
				return Skill{}, false, fmt.Errorf("line %d priority must be an integer", lineNo+1)
			}
			skill.Priority = priority
		case "summary":
			skill.Summary = value
		case "source_skill_path":
			skill.SourceSkillPath = value
		case "source_description":
			skill.SourceDescription = value
		case "use_when", "evidence", "commands", "boundaries", "next_integration":
			if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
				for _, item := range parseInlineList(value) {
					appendListValue(&skill, key, item)
				}
				continue
			}
			if value != "" {
				appendListValue(&skill, key, value)
				continue
			}
			currentList = key
		default:
			// Unknown keys are ignored so the skills repo can add AI-only metadata
			// without forcing an OpsPilot release.
		}
	}
	return skill, enabled, nil
}

func appendListValue(skill *Skill, key, value string) {
	if value == "" {
		return
	}
	switch key {
	case "use_when":
		skill.UseWhen = append(skill.UseWhen, value)
	case "evidence":
		skill.Evidence = append(skill.Evidence, value)
	case "commands":
		skill.Commands = append(skill.Commands, value)
	case "boundaries":
		skill.Boundaries = append(skill.Boundaries, value)
	case "next_integration":
		skill.NextIntegration = append(skill.NextIntegration, value)
	}
}

func parseInlineList(value string) []string {
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if inner == "" {
		return nil
	}
	items := []string{}
	for _, part := range strings.Split(inner, ",") {
		item := cleanYAMLValue(part)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func mergeSkills(base, dynamic []Skill) []Skill {
	merged := make([]Skill, 0, len(base)+len(dynamic))
	index := map[string]int{}
	for _, skill := range base {
		key := strings.ToLower(skill.Name)
		index[key] = len(merged)
		merged = append(merged, skill)
	}
	for _, skill := range dynamic {
		key := strings.ToLower(skill.Name)
		if existing, ok := index[key]; ok {
			merged[existing] = skill
			continue
		}
		index[key] = len(merged)
		merged = append(merged, skill)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Priority == merged[j].Priority {
			return merged[i].Name < merged[j].Name
		}
		return merged[i].Priority > merged[j].Priority
	})
	return merged
}

func normalizeYAMLKey(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
}

func cleanYAMLValue(value string) string {
	value = strings.TrimSpace(value)
	if comment := strings.Index(value, " #"); comment >= 0 {
		value = strings.TrimSpace(value[:comment])
	}
	value = strings.Trim(value, `"'`)
	return value
}

func parseBool(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	case "":
		return fallback
	default:
		return fallback
	}
}

func readTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return parseBool(value, fallback)
}
