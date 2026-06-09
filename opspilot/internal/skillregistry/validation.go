package skillregistry

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ValidationIssue struct {
	Level   string `json:"level"`
	Skill   string `json:"skill,omitempty"`
	Path    string `json:"path,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type ValidationResult struct {
	Ready       bool              `json:"ready"`
	Root        string            `json:"root"`
	SkillCount  int               `json:"skill_count"`
	ErrorCount  int               `json:"error_count"`
	WarnCount   int               `json:"warn_count"`
	Issues      []ValidationIssue `json:"issues,omitempty"`
	SkillNames  []string          `json:"skill_names,omitempty"`
	ExampleGaps []string          `json:"example_gaps,omitempty"`
}

func ValidateDirectory(root string) ValidationResult {
	root = strings.TrimSpace(root)
	if root == "" {
		root = defaultDynamicSkillsDir
	}
	result := ValidationResult{Root: root}
	info, err := os.Stat(root)
	if err != nil {
		result.addIssue("error", "", root, "", "skills directory is not readable: "+err.Error())
		result.finish()
		return result
	}
	if !info.IsDir() {
		result.addIssue("error", "", root, "", "skills path is not a directory")
		result.finish()
		return result
	}

	files := []string{}
	walkRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		walkRoot = resolved
	}
	err = filepath.WalkDir(walkRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			result.addIssue("warning", "", path, "", "cannot walk path: "+walkErr.Error())
			return nil
		}
		name := entry.Name()
		if entry.IsDir() && (name == ".git" || strings.HasPrefix(name, ".")) && path != walkRoot {
			return filepath.SkipDir
		}
		if !entry.IsDir() && name == "skill.yaml" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		result.addIssue("error", "", root, "", "cannot scan skills directory: "+err.Error())
		result.finish()
		return result
	}
	sort.Strings(files)
	if len(files) == 0 {
		result.addIssue("error", "", root, "", "no skill.yaml files found")
		result.finish()
		return result
	}

	seen := map[string]string{}
	for _, file := range files {
		skill, enabled, err := readDynamicSkill(file)
		if err != nil {
			result.addIssue("error", "", file, "skill.yaml", err.Error())
			continue
		}
		if !enabled {
			continue
		}
		result.SkillCount++
		result.SkillNames = append(result.SkillNames, skill.Name)
		lowerName := strings.ToLower(skill.Name)
		if previous := seen[lowerName]; previous != "" {
			result.addIssue("error", skill.Name, file, "name", "duplicate skill name also defined at "+previous)
		}
		seen[lowerName] = file
		validateSkillFile(&result, skill, file)
	}
	sort.Strings(result.SkillNames)
	sort.Strings(result.ExampleGaps)
	result.finish()
	return result
}

func validateSkillFile(result *ValidationResult, skill Skill, yamlPath string) {
	dir := filepath.Dir(yamlPath)
	if strings.TrimSpace(skill.Name) == "" {
		result.addIssue("error", "", yamlPath, "name", "name is required")
	}
	if strings.TrimSpace(skill.Category) == "" {
		result.addIssue("error", skill.Name, yamlPath, "category", "category is required")
	}
	if strings.TrimSpace(skill.Summary) == "" {
		result.addIssue("warning", skill.Name, yamlPath, "summary", "summary should explain the runtime purpose")
	}
	if len(skill.UseWhen) == 0 {
		result.addIssue("warning", skill.Name, yamlPath, "use_when", "use_when should contain routing hints")
	}
	if len(skill.Evidence) == 0 {
		result.addIssue("warning", skill.Name, yamlPath, "evidence", "evidence should list required evidence")
	}
	if len(skill.Commands) == 0 {
		result.addIssue("warning", skill.Name, yamlPath, "commands", "commands should map to approved OpsPilot commands")
	}
	if len(skill.Boundaries) == 0 {
		result.addIssue("warning", skill.Name, yamlPath, "boundaries", "boundaries should describe safety limits")
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		result.addIssue("warning", skill.Name, filepath.Join(dir, "SKILL.md"), "SKILL.md", "SKILL.md is missing or unreadable")
	}
	for _, command := range skill.Commands {
		if looksLikeArbitraryShell(command) {
			result.addIssue("error", skill.Name, yamlPath, "commands", "command must map to OpsPilot capabilities, not arbitrary shell: "+command)
		}
	}
	if skill.Priority >= 80 && !dirExists(filepath.Join(dir, "examples")) {
		result.ExampleGaps = append(result.ExampleGaps, skill.Name)
		result.addIssue("warning", skill.Name, filepath.Join(dir, "examples"), "examples", "high-priority skill should include examples")
	}
}

func looksLikeArbitraryShell(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return false
	}
	blocked := []string{"bash ", "sh ", "powershell", "cmd /c", "kubectl ", "docker ", "rm ", "curl ", "wget "}
	for _, item := range blocked {
		if strings.HasPrefix(lower, item) || strings.Contains(lower, "&&") || strings.Contains(lower, "|") {
			return true
		}
	}
	return false
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (r *ValidationResult) addIssue(level, skill, path, field, message string) {
	r.Issues = append(r.Issues, ValidationIssue{
		Level:   level,
		Skill:   skill,
		Path:    path,
		Field:   field,
		Message: message,
	})
}

func (r *ValidationResult) finish() {
	for _, issue := range r.Issues {
		switch issue.Level {
		case "error":
			r.ErrorCount++
		case "warning", "warn":
			r.WarnCount++
		}
	}
	r.Ready = r.ErrorCount == 0
}
