package main

import (
	"fmt"
	"sort"
	"strings"
)

func detectConfigSources(c onboardServiceConfig) []onboardConfigSourceConfig {
	signals := collectRepoSignals()
	evidence := configSourceEvidence(signals, []string{"apollo", "APOLLO_META", "apollo.meta", "apolloconfig", "--cfg=", "--env="})
	if len(evidence) == 0 {
		return nil
	}
	item := defaultApolloConfigSource(c)
	item.Meta = firstDetectedURL(evidence, item.Meta)
	item.Env = firstDetectedFlagValue(evidence, "--env", item.Env)
	if firstDetectedFlagValue(evidence, "--cfg", "") != "" || firstDetectedFlagValue(evidence, "--env", "") != "" {
		item.InjectMode = "args"
	}
	item.Evidence = evidence
	item.Reason = "detected Apollo configuration usage; generate platform-managed Apollo config references"
	return []onboardConfigSourceConfig{item}
}

func normalizeConfigSources(c onboardServiceConfig, items []onboardConfigSourceConfig) []onboardConfigSourceConfig {
	normalized := []onboardConfigSourceConfig{}
	seen := map[string]bool{}
	for _, item := range items {
		sourceType := sanitizeDNSLabel(firstNonEmpty(item.Type, item.Name))
		if sourceType == "" {
			continue
		}
		defaults := onboardConfigSourceConfig{}
		switch sourceType {
		case "apollo":
			defaults = defaultApolloConfigSource(c)
		default:
			defaults = onboardConfigSourceConfig{
				Name:       sourceType,
				Type:       sourceType,
				ConfigMap:  sanitizeDNSLabel(c.Name + "-" + sourceType + "-config"),
				InjectMode: "env",
			}
		}
		defaults.Name = firstNonEmpty(item.Name, defaults.Name)
		defaults.Type = firstNonEmpty(item.Type, defaults.Type)
		defaults.Required = item.Required
		defaults.AppID = firstNonEmpty(item.AppID, defaults.AppID)
		defaults.Env = firstNonEmpty(item.Env, defaults.Env)
		defaults.Cluster = firstNonEmpty(item.Cluster, defaults.Cluster)
		if len(item.Namespaces) > 0 {
			defaults.Namespaces = item.Namespaces
		}
		defaults.Meta = firstNonEmpty(item.Meta, defaults.Meta)
		defaults.ConfigMap = firstNonEmpty(item.ConfigMap, defaults.ConfigMap)
		defaults.TokenSecret = firstNonEmpty(item.TokenSecret, defaults.TokenSecret)
		defaults.InjectMode = canonicalConfigInjectMode(firstNonEmpty(item.InjectMode, defaults.InjectMode))
		defaults.EnvFlag = firstNonEmpty(item.EnvFlag, defaults.EnvFlag)
		defaults.MetaFlag = firstNonEmpty(item.MetaFlag, defaults.MetaFlag)
		defaults.MountPath = firstNonEmpty(item.MountPath, defaults.MountPath)
		defaults.Reason = item.Reason
		defaults.Evidence = item.Evidence
		key := defaults.Name
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, defaults)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].Name < normalized[j].Name
	})
	return normalized
}

func defaultApolloConfigSource(c onboardServiceConfig) onboardConfigSourceConfig {
	return onboardConfigSourceConfig{
		Name:       "apollo",
		Type:       "apollo",
		AppID:      firstNonEmpty(c.Name, c.Container),
		Cluster:    "default",
		Namespaces: []string{"application"},
		ConfigMap:  sanitizeDNSLabel(c.Name + "-apollo-config"),
		InjectMode: "env",
		EnvFlag:    "--env",
		MetaFlag:   "--cfg",
		MountPath:  "/app/config/apollo.yaml",
	}
}

func canonicalConfigInjectMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "args", "arg", "command", "command-args":
		return "args"
	case "file", "yaml", "mount":
		return "file"
	default:
		return "env"
	}
}

func configSourceCredentialPlans(c onboardServiceConfig) []string {
	out := []string{}
	for _, item := range c.ConfigSources {
		if item.TokenSecret == "" {
			continue
		}
		out = append(out, fmt.Sprintf("%s: token secret=%s keys=token", item.Type, item.TokenSecret))
	}
	return out
}

func configSourceEvidence(signals []repoSignal, tokens []string) []string {
	evidence := []string{}
	for _, signal := range signals {
		for _, line := range strings.Split(signal.Text, "\n") {
			lower := strings.ToLower(line)
			for _, token := range tokens {
				if token == "" || !strings.Contains(lower, strings.ToLower(token)) {
					continue
				}
				snippet := strings.Join(strings.Fields(line), " ")
				if len(snippet) > 160 {
					snippet = snippet[:160]
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

func firstDetectedURL(evidence []string, fallback string) string {
	for _, item := range evidence {
		for _, marker := range []string{"--cfg=", "APOLLO_META=", "apollo.meta=", "meta:"} {
			if value := extractValueAfterMarker(item, marker); strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
				return value
			}
		}
	}
	return fallback
}

func firstDetectedFlagValue(evidence []string, flag, fallback string) string {
	for _, item := range evidence {
		for _, marker := range []string{flag + "=", flag + " "} {
			if value := extractValueAfterMarker(item, marker); value != "" {
				return value
			}
		}
	}
	return fallback
}

func extractValueAfterMarker(text, marker string) string {
	idx := strings.Index(strings.ToLower(text), strings.ToLower(marker))
	if idx < 0 {
		return ""
	}
	raw := text[idx+len(marker):]
	raw = strings.TrimLeft(raw, " \t\"'")
	end := len(raw)
	for i, r := range raw {
		if r == ' ' || r == '\t' || r == ',' || r == ';' || r == '"' || r == '\'' || r == '#' || r == '\r' || r == '\n' {
			end = i
			break
		}
	}
	return strings.TrimRight(strings.TrimSpace(raw[:end]), "\\")
}
