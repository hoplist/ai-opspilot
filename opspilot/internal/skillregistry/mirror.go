package skillregistry

import (
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
