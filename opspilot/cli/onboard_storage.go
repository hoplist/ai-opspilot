package main

import (
	"sort"
	"strings"
)

func normalizeStorageRequirements(c onboardServiceConfig, items []onboardStorageConfig) []onboardStorageConfig {
	normalized := []onboardStorageConfig{}
	seen := map[string]bool{}
	for _, item := range items {
		name := sanitizeDNSLabel(firstNonEmpty(item.Name, item.Purpose))
		if name == "" {
			continue
		}
		purpose := sanitizeDNSLabel(firstNonEmpty(item.Purpose, name))
		mode := canonicalStorageMode(firstNonEmpty(item.Mode, defaultStorageMode(purpose)))
		normalizedItem := onboardStorageConfig{
			Name:          name,
			Purpose:       purpose,
			Mode:          mode,
			MountPath:     firstNonEmpty(item.MountPath, defaultStorageMountPath(purpose)),
			HostPath:      item.HostPath,
			SizeHint:      item.SizeHint,
			SizeLimit:     item.SizeLimit,
			RetentionDays: item.RetentionDays,
			ReadOnly:      item.ReadOnly,
			Reason:        item.Reason,
			Evidence:      item.Evidence,
		}
		switch mode {
		case "emptyDir":
			normalizedItem.HostPath = ""
			normalizedItem.SizeHint = ""
			if normalizedItem.SizeLimit == "" {
				normalizedItem.SizeLimit = defaultStorageSizeLimit(purpose)
			}
		default:
			normalizedItem.Mode = "hostPath"
			if !isPlatformHostPath(normalizedItem.HostPath) {
				normalizedItem.HostPath = platformHostPath(c, name)
			}
			if normalizedItem.SizeHint == "" {
				normalizedItem.SizeHint = defaultStorageSizeHint(purpose)
			}
			if purpose == "logs" && normalizedItem.RetentionDays == 0 {
				normalizedItem.RetentionDays = 7
			}
		}
		if normalizedItem.MountPath == "" || seen[normalizedItem.Name] {
			continue
		}
		seen[normalizedItem.Name] = true
		normalized = append(normalized, normalizedItem)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return storageRank(normalized[i].Purpose, normalized[i].Name) < storageRank(normalized[j].Purpose, normalized[j].Name)
	})
	return normalized
}

func defaultStorageRequirement(c onboardServiceConfig, purpose string) onboardStorageConfig {
	purpose = sanitizeDNSLabel(purpose)
	if purpose == "" {
		purpose = "data"
	}
	mode := defaultStorageMode(purpose)
	item := onboardStorageConfig{
		Name:      purpose,
		Purpose:   purpose,
		Mode:      mode,
		MountPath: defaultStorageMountPath(purpose),
	}
	if mode == "emptyDir" {
		item.SizeLimit = defaultStorageSizeLimit(purpose)
	} else {
		item.HostPath = platformHostPath(c, purpose)
		item.SizeHint = defaultStorageSizeHint(purpose)
		if purpose == "logs" {
			item.RetentionDays = 7
		}
	}
	return item
}

func defaultStorageMode(purpose string) string {
	switch sanitizeDNSLabel(purpose) {
	case "cache", "tmp", "temp":
		return "emptyDir"
	default:
		return "hostPath"
	}
}

func canonicalStorageMode(mode string) string {
	switch strings.ToLower(strings.ReplaceAll(mode, "-", "")) {
	case "emptydir":
		return "emptyDir"
	default:
		return "hostPath"
	}
}

func defaultStorageMountPath(purpose string) string {
	switch sanitizeDNSLabel(purpose) {
	case "logs":
		return "/app/logs"
	case "runtime", "uploads", "upload", "files", "data":
		return "/app/runtime"
	case "cache", "tmp", "temp":
		return "/tmp/cache"
	default:
		return "/app/" + sanitizeDNSLabel(purpose)
	}
}

func defaultStorageSizeHint(purpose string) string {
	switch sanitizeDNSLabel(purpose) {
	case "runtime", "uploads", "upload", "files", "data":
		return "20Gi"
	default:
		return "10Gi"
	}
}

func defaultStorageSizeLimit(purpose string) string {
	switch sanitizeDNSLabel(purpose) {
	case "cache", "tmp", "temp":
		return "1Gi"
	default:
		return "1Gi"
	}
}

func storageRank(purpose, name string) int {
	switch sanitizeDNSLabel(firstNonEmpty(purpose, name)) {
	case "logs":
		return 0
	case "runtime", "uploads", "upload", "files", "data":
		return 1
	case "cache", "tmp", "temp":
		return 2
	default:
		return 10
	}
}

func platformHostPath(c onboardServiceConfig, name string) string {
	namespace := sanitizeDNSLabel(firstNonEmpty(c.Namespace, defaultNamespace(c.Group, c.Project)))
	service := sanitizeDNSLabel(firstNonEmpty(c.Name, c.Container, "service"))
	volume := sanitizeDNSLabel(firstNonEmpty(name, "data"))
	return strings.TrimRight(defaultHostPathRoot, "/") + "/" + namespace + "/" + service + "/" + volume
}

func platformHostPathRoot(c onboardServiceConfig) string {
	namespace := sanitizeDNSLabel(firstNonEmpty(c.Namespace, defaultNamespace(c.Group, c.Project)))
	service := sanitizeDNSLabel(firstNonEmpty(c.Name, c.Container, "service"))
	return strings.TrimRight(defaultHostPathRoot, "/") + "/" + namespace + "/" + service
}

func isPlatformHostPath(value string) bool {
	value = strings.TrimSpace(value)
	root := strings.TrimRight(defaultHostPathRoot, "/") + "/"
	return strings.HasPrefix(value, root)
}
