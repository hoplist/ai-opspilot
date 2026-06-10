package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func detectMiddlewareRequirements(c onboardServiceConfig) []onboardMiddlewareConfig {
	signals := collectRepoSignals()
	items := []onboardMiddlewareConfig{}
	for _, entry := range middlewareCatalog {
		evidence := middlewareEvidence(signals, entry.Tokens)
		if len(evidence) == 0 {
			continue
		}
		item := defaultMiddlewareRequirement(c, entry)
		item.Evidence = evidence
		item.Reason = fmt.Sprintf("detected %s dependency; use %s and allocate %s", entry.Display, entry.Mode, entry.Allocation)
		items = append(items, item)
	}
	return items
}

func detectStorageRequirements(c onboardServiceConfig) []onboardStorageConfig {
	signals := collectRepoSignals()
	items := []onboardStorageConfig{}
	if evidence := storageEvidence(signals, []string{"LOG_DIR", "LOG_PATH", "log.dir", "logging.file", "/logs", "logs/"}); len(evidence) > 0 {
		item := defaultStorageRequirement(c, "logs")
		item.MountPath = firstDetectedPath(evidence, []string{"LOG_DIR", "LOG_PATH"}, item.MountPath)
		item.Evidence = evidence
		item.Reason = "detected log path; use platform-managed hostPath with retention metadata"
		items = append(items, item)
	}
	if evidence := storageEvidence(signals, []string{"upload", "uploads", "runtime", "files", "conversations"}); len(evidence) > 0 {
		item := defaultStorageRequirement(c, "runtime")
		item.MountPath = firstDetectedPath(evidence, []string{"UPLOAD_DIR", "UPLOAD_PATH", "RUNTIME_DIR", "FILES_DIR"}, item.MountPath)
		item.Evidence = evidence
		item.Reason = "detected runtime/upload file path; use platform-managed hostPath"
		items = append(items, item)
	}
	if evidence := storageEvidence(signals, []string{"CACHE_DIR", "cache.dir", "/cache", "tmp/cache", "temp"}); len(evidence) > 0 {
		item := defaultStorageRequirement(c, "cache")
		item.MountPath = firstDetectedPath(evidence, []string{"CACHE_DIR", "TMP_DIR", "TEMP_DIR"}, item.MountPath)
		item.Evidence = evidence
		item.Reason = "detected cache/temp path; use bounded emptyDir"
		items = append(items, item)
	}
	return normalizeStorageRequirements(c, items)
}

type repoSignal struct {
	Path string
	Text string
}

func collectRepoSignals() []repoSignal {
	signals := []repoSignal{}
	seen := map[string]bool{}
	maxFiles := 200
	_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || len(signals) >= maxFiles {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if shouldSkipScanDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldScanDependencyFile(path) || seen[path] {
			return nil
		}
		seen[path] = true
		if body, ok := readSmallTextFile(path); ok {
			signals = append(signals, repoSignal{Path: filepath.ToSlash(path), Text: string(body)})
		}
		return nil
	})
	return signals
}

func shouldSkipScanDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", "target", ".next", ".venv", "venv", "__pycache__":
		return true
	default:
		return false
	}
}

func shouldScanDependencyFile(path string) bool {
	slashPath := filepath.ToSlash(path)
	if strings.HasPrefix(slashPath, "deploy/k8s/") {
		return false
	}
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "opspilot.service.yaml", "opspilot.namespaces.yaml", "opspilot.release-service.txt", ".gitlab-ci.yml":
		return false
	}
	switch base {
	case "go.mod", "package.json", "requirements.txt", "pyproject.toml", "pom.xml",
		".env", ".env.example", "application.yml", "application.yaml",
		"bootstrap.yml", "bootstrap.yaml", "config.yml", "config.yaml",
		"application.properties":
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".js", ".ts", ".py", ".java", ".yml", ".yaml", ".properties", ".toml":
		return true
	default:
		return false
	}
}

func readSmallTextFile(path string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 256*1024 {
		return nil, false
	}
	body, err := os.ReadFile(path)
	if err != nil || strings.ContainsRune(string(body), '\x00') {
		return nil, false
	}
	return body, true
}

func middlewareEvidence(signals []repoSignal, tokens []string) []string {
	evidence := []string{}
	for _, signal := range signals {
		lower := strings.ToLower(signal.Text)
		for _, token := range tokens {
			if token == "" || !strings.Contains(lower, strings.ToLower(token)) {
				continue
			}
			evidence = append(evidence, fmt.Sprintf("%s contains %s", signal.Path, token))
			if len(evidence) >= 3 {
				return evidence
			}
		}
	}
	return evidence
}

func storageEvidence(signals []repoSignal, tokens []string) []string {
	evidence := []string{}
	for _, signal := range signals {
		for _, line := range strings.Split(signal.Text, "\n") {
			lower := strings.ToLower(line)
			for _, token := range tokens {
				if token == "" || !strings.Contains(lower, strings.ToLower(token)) {
					continue
				}
				snippet := strings.Join(strings.Fields(line), " ")
				if len(snippet) > 120 {
					snippet = snippet[:120]
				}
				if snippet == "" {
					snippet = token
				}
				evidence = append(evidence, fmt.Sprintf("%s contains %s: %s", signal.Path, token, snippet))
				if len(evidence) >= 3 {
					return evidence
				}
			}
		}
	}
	return evidence
}

func firstDetectedPath(evidence []string, keys []string, fallback string) string {
	for _, item := range evidence {
		for _, key := range keys {
			if path := extractPathAfterKey(item, key); path != "" {
				return path
			}
		}
	}
	return fallback
}

func extractPathAfterKey(text, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	lower := strings.ToLower(text)
	markers := []string{
		strings.ToLower(key) + "=",
		strings.ToLower(key) + ":",
		strings.ToLower(key) + " =",
		strings.ToLower(key) + " :",
	}
	for _, marker := range markers {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		raw := text[idx+len(marker):]
		raw = strings.TrimLeft(raw, " \t\"'")
		if strings.HasPrefix(raw, "=") || strings.HasPrefix(raw, ":") {
			raw = strings.TrimLeft(raw[1:], " \t\"'")
		}
		end := len(raw)
		for i, r := range raw {
			if r == ' ' || r == '\t' || r == ',' || r == ';' || r == '"' || r == '\'' || r == '#' || r == '\r' {
				end = i
				break
			}
		}
		candidate := strings.TrimRight(strings.TrimSpace(raw[:end]), "/")
		if strings.HasPrefix(candidate, "/") {
			return candidate
		}
	}
	return ""
}
