package main

import (
	"fmt"
	"sort"
	"strings"
)

func normalizeMiddlewareRequirements(c onboardServiceConfig, items []onboardMiddlewareConfig) []onboardMiddlewareConfig {
	normalized := []onboardMiddlewareConfig{}
	seen := map[string]bool{}
	for _, item := range items {
		kind := firstNonEmpty(item.Kind, item.Name)
		entry, ok := middlewareCatalogByKind(kind)
		if !ok {
			entry = middlewareCatalogEntry{
				Kind:       sanitizeDNSLabel(kind),
				Display:    firstNonEmpty(item.Display, kind),
				Mode:       firstNonEmpty(item.Mode, "shared"),
				Allocation: firstNonEmpty(item.Allocation, "logical-resource"),
				Env:        item.Env,
			}
		}
		defaults := defaultMiddlewareRequirement(c, entry)
		defaults.Name = firstNonEmpty(item.Name, defaults.Name)
		defaults.Kind = firstNonEmpty(item.Kind, defaults.Kind)
		defaults.Display = firstNonEmpty(item.Display, defaults.Display)
		defaults.Mode = firstNonEmpty(item.Mode, defaults.Mode)
		defaults.Allocation = firstNonEmpty(item.Allocation, defaults.Allocation)
		defaults.Provision = firstNonEmpty(item.Provision, defaults.Provision)
		defaults.Resource = firstNonEmpty(item.Resource, defaults.Resource)
		defaults.Secret = firstNonEmpty(item.Secret, defaults.Secret)
		if len(item.Env) > 0 {
			defaults.Env = item.Env
		}
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
		return middlewareCatalogRank(normalized[i].Kind) < middlewareCatalogRank(normalized[j].Kind)
	})
	return normalized
}

func defaultMiddlewareRequirement(c onboardServiceConfig, entry middlewareCatalogEntry) onboardMiddlewareConfig {
	name := sanitizeDNSLabel(entry.Kind)
	item := onboardMiddlewareConfig{
		Name:       name,
		Kind:       entry.Kind,
		Display:    entry.Display,
		Mode:       entry.Mode,
		Allocation: entry.Allocation,
		Resource:   middlewareResourceName(c, entry.Kind),
		Secret:     sanitizeDNSLabel(c.Name + "-" + entry.Kind + "-conn"),
		Env:        append([]string{}, entry.Env...),
	}
	if middlewareKindAutoProvisioned(entry.Kind) {
		item.Provision = "auto"
	} else {
		item.Provision = "external"
	}
	return item
}

func middlewareCatalogByKind(kind string) (middlewareCatalogEntry, bool) {
	kind = sanitizeDNSLabel(kind)
	for _, entry := range middlewareCatalog {
		if entry.Kind == kind {
			return entry, true
		}
	}
	return middlewareCatalogEntry{}, false
}

func middlewareCatalogRank(kind string) int {
	for i, entry := range middlewareCatalog {
		if entry.Kind == kind {
			return i
		}
	}
	return len(middlewareCatalog) + 1
}

func middlewareResourceName(c onboardServiceConfig, kind string) string {
	parts := []string{
		firstNonEmpty(c.Group, defaultGroup),
		firstNonEmpty(c.Project, projectNameFromService(c.Name)),
		c.Name,
		kind,
	}
	return strings.ReplaceAll(sanitizeDNSLabel(strings.Join(parts, "-")), "-", "_")
}

func middlewareCredentialPlans(c onboardServiceConfig) []string {
	out := []string{}
	for _, item := range c.Middleware {
		if item.Kind == "" {
			continue
		}
		secret := firstNonEmpty(item.Secret, sanitizeDNSLabel(c.Name+"-"+item.Kind+"-credentials"))
		out = append(out, fmt.Sprintf("%s: secret=%s mode=%s allocation=%s keys=%s",
			item.Kind, secret, firstNonEmpty(item.Mode, "shared"), firstNonEmpty(item.Allocation, "service-scoped"), strings.Join(item.Env, "|")))
	}
	return out
}
