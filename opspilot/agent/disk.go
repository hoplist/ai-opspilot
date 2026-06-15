package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
)

const gib = 1024 * 1024 * 1024

type mountRecord struct {
	device     string
	mountpoint string
	fstype     string
}

func collectHostDisk(ctx context.Context, docker *dockerClient, cfg config, req nodeagent.HostDiskRequest) (nodeagent.HostDiskResponse, error) {
	req = nodeagent.BoundedHostDiskRequest(req)
	warnings := []string{}
	scanPaths := expandHostPathPatterns(cfg.hostRoot, cfg.diskAllowedPaths)
	filesystems, fsWarnings := collectFilesystems(cfg.hostRoot, scanPaths)
	warnings = append(warnings, fsWarnings...)
	topPaths, scanWarnings := collectTopPaths(ctx, cfg.hostRoot, scanPaths, req.Depth, req.Limit)
	warnings = append(warnings, scanWarnings...)
	dockerDisk, dockerWarnings := collectDockerDisk(ctx, docker)
	warnings = append(warnings, dockerWarnings...)
	containerLogs, logWarnings := collectContainerLogs(ctx, docker, cfg, req.Limit)
	warnings = append(warnings, logWarnings...)

	return nodeagent.HostDiskResponse{
		Filesystems:   filesystems,
		TopPaths:      topPaths,
		Docker:        dockerDisk,
		ContainerLogs: containerLogs,
		CleanupPlan:   buildHostCleanupPlan(topPaths, dockerDisk, containerLogs),
		Limits: nodeagent.HostDiskLimits{
			AllowedPaths: append([]string{}, scanPaths...),
			MaxDepth:     req.Depth,
			TopLimit:     req.Limit,
			ReadOnly:     true,
		},
		Warnings: warnings,
	}, nil
}

func expandHostPathPatterns(hostRoot string, specs []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, spec := range specs {
		if !strings.Contains(spec, "*") {
			if !seen[spec] {
				seen[spec] = true
				out = append(out, spec)
			}
			continue
		}
		actualPattern := mapHostPath(hostRoot, spec)
		matches, err := filepath.Glob(actualPattern)
		if err != nil || len(matches) == 0 {
			continue
		}
		sort.Strings(matches)
		for _, match := range matches {
			display := displayHostPath(hostRoot, match)
			if display == "" || seen[display] {
				continue
			}
			seen[display] = true
			out = append(out, display)
		}
	}
	return out
}

func collectFilesystems(hostRoot string, allowedPaths []string) ([]nodeagent.HostFilesystem, []string) {
	mounts := readMounts(hostRoot)
	warnings := []string{}
	out := []nodeagent.HostFilesystem{}
	seen := map[string]bool{}
	for _, displayPath := range allowedPaths {
		actualPath := mapHostPath(hostRoot, displayPath)
		if _, err := os.Stat(actualPath); err != nil {
			continue
		}
		total, free, avail, err := filesystemUsage(actualPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("filesystem %s: %v", displayPath, err))
			continue
		}
		record := bestMountRecord(displayPath, mounts)
		key := fmt.Sprintf("%s|%s|%s", record.device, record.mountpoint, displayPath)
		if seen[key] {
			continue
		}
		seen[key] = true
		usedPercent := 0.0
		if total > 0 {
			usedPercent = (1 - float64(avail)/float64(total)) * 100
		}
		out = append(out, nodeagent.HostFilesystem{
			Path:        displayPath,
			Mountpoint:  record.mountpoint,
			Device:      record.device,
			FSType:      record.fstype,
			TotalBytes:  total,
			FreeBytes:   free,
			AvailBytes:  avail,
			UsedPercent: roundDiskPercent(usedPercent),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out, warnings
}

func collectTopPaths(ctx context.Context, hostRoot string, allowedPaths []string, depth, limit int) ([]nodeagent.HostPathUsage, []string) {
	items := []nodeagent.HostPathUsage{}
	warnings := []string{}
	for _, displayPath := range allowedPaths {
		actualPath := mapHostPath(hostRoot, displayPath)
		if _, err := os.Stat(actualPath); err != nil {
			continue
		}
		if err := collectPathUsage(ctx, actualPath, displayPath, depth, 0, &items); err != nil {
			warnings = append(warnings, fmt.Sprintf("scan %s: %v", displayPath, err))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SizeBytes == items[j].SizeBytes {
			return items[i].Path < items[j].Path
		}
		return items[i].SizeBytes > items[j].SizeBytes
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, warnings
}

func collectPathUsage(ctx context.Context, actualPath, displayPath string, depthRemaining, currentDepth int, items *[]nodeagent.HostPathUsage) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	info, err := os.Lstat(actualPath)
	if err != nil {
		*items = append(*items, nodeagent.HostPathUsage{Path: displayPath, Depth: currentDepth, Error: err.Error()})
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if !info.IsDir() {
		*items = append(*items, nodeagent.HostPathUsage{Path: displayPath, SizeBytes: info.Size(), Depth: currentDepth})
		return nil
	}
	entries, err := os.ReadDir(actualPath)
	if err != nil {
		*items = append(*items, nodeagent.HostPathUsage{Path: displayPath, Depth: currentDepth, Error: err.Error()})
		return nil
	}
	size := int64(0)
	for _, entry := range entries {
		entryInfo, err := entry.Info()
		if err != nil {
			continue
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}
		childActual := filepath.Join(actualPath, entry.Name())
		childDisplay := path.Join(displayPath, entry.Name())
		if entryInfo.IsDir() {
			if depthRemaining > 0 {
				before := len(*items)
				if err := collectPathUsage(ctx, childActual, childDisplay, depthRemaining-1, currentDepth+1, items); err != nil {
					return err
				}
				if len(*items) > before {
					size += (*items)[len(*items)-1].SizeBytes
				}
			}
			continue
		}
		size += entryInfo.Size()
	}
	*items = append(*items, nodeagent.HostPathUsage{Path: displayPath, SizeBytes: size, Depth: currentDepth})
	return nil
}

func collectDockerDisk(ctx context.Context, docker *dockerClient) (nodeagent.HostDockerDisk, []string) {
	raw, err := docker.systemDF(ctx)
	if err != nil {
		return nodeagent.HostDockerDisk{Available: false}, []string{"docker system df: " + err.Error()}
	}
	out := nodeagent.HostDockerDisk{
		Available:       true,
		LayersSizeBytes: int64Value(raw["LayersSize"]),
	}
	for _, item := range anySlice(raw["Images"]) {
		image := mapValue(item)
		size := int64Value(image["Size"])
		out.ImagesSizeBytes += size
		if int64Value(image["Containers"]) == 0 {
			out.ImagesReclaimableBytes += size
		}
	}
	for _, item := range anySlice(raw["Containers"]) {
		container := mapValue(item)
		out.ContainersSizeRwBytes += int64Value(container["SizeRw"])
		out.ContainersSizeRootFsBytes += int64Value(container["SizeRootFs"])
	}
	for _, item := range anySlice(raw["Volumes"]) {
		volume := mapValue(item)
		usage := mapValue(volume["UsageData"])
		size := int64Value(usage["Size"])
		out.VolumesSizeBytes += size
		if int64Value(usage["RefCount"]) == 0 {
			out.VolumesReclaimableBytes += size
		}
	}
	for _, item := range anySlice(raw["BuildCache"]) {
		cache := mapValue(item)
		size := int64Value(cache["Size"])
		out.BuildCacheSizeBytes += size
		if !boolValue(cache["InUse"]) {
			out.BuildCacheReclaimableBytes += size
		}
	}
	out.ApproxReclaimableBytes = out.ImagesReclaimableBytes + out.VolumesReclaimableBytes + out.BuildCacheReclaimableBytes
	return out, nil
}

func collectContainerLogs(ctx context.Context, docker *dockerClient, cfg config, limit int) ([]nodeagent.HostContainerLogUsage, []string) {
	containers, err := docker.containers(ctx)
	if err != nil {
		return nil, []string{"container log usage: " + err.Error()}
	}
	out := []nodeagent.HostContainerLogUsage{}
	warnings := []string{}
	for _, raw := range containers {
		item := containerSummary(raw)
		if !allowedContainer(cfg.allowedContainers, item) {
			continue
		}
		id := fmt.Sprint(item["id"])
		inspect, err := docker.inspect(ctx, id)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("inspect %s: %v", firstContainerName(item), err))
			continue
		}
		logPath := strings.TrimSpace(fmt.Sprint(inspect["LogPath"]))
		hostConfig := mapValue(inspect["HostConfig"])
		logConfig := mapValue(hostConfig["LogConfig"])
		entry := nodeagent.HostContainerLogUsage{
			Container:  firstContainerName(item),
			ID:         shortID(id),
			LogPath:    logPath,
			LogDriver:  strings.TrimSpace(fmt.Sprint(logConfig["Type"])),
			LogOptions: stringMap(logConfig["Config"]),
		}
		if logPath != "" {
			entry.MappedPath = mapHostPath(cfg.hostRoot, logPath)
			if stat, err := os.Stat(entry.MappedPath); err == nil {
				entry.SizeBytes = stat.Size()
			} else {
				entry.Warning = err.Error()
			}
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SizeBytes > out[j].SizeBytes
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, warnings
}

func buildHostCleanupPlan(topPaths []nodeagent.HostPathUsage, dockerDisk nodeagent.HostDockerDisk, logs []nodeagent.HostContainerLogUsage) []nodeagent.HostCleanupPlanItem {
	plans := []nodeagent.HostCleanupPlanItem{}
	for _, log := range logs {
		if log.LogDriver == "json-file" && strings.TrimSpace(log.LogOptions["max-size"]) == "" {
			plans = append(plans, nodeagent.HostCleanupPlanItem{
				ID:                "docker-json-log-rotation",
				Risk:              "controlled_mutation",
				Summary:           "Docker json-file log rotation is not configured for at least one allowed container.",
				Evidence:          fmt.Sprintf("%s log_driver=json-file max-size=empty size=%s", log.Container, humanBytes(log.SizeBytes)),
				Recommendation:    "Configure Docker daemon or container log-opts with max-size and max-file, then restart affected containers during a maintenance window.",
				MinValidation:     "Run host disk again and verify log_options.max-size is present and log size stops growing unbounded.",
				ExecutionBoundary: "plan_only; OpsPilot does not edit Docker daemon config or truncate logs.",
			})
			break
		}
	}
	for _, log := range logs {
		if log.SizeBytes >= 1*gib {
			plans = append(plans, nodeagent.HostCleanupPlanItem{
				ID:                "large-container-log",
				Risk:              "controlled_mutation",
				Summary:           "A container log file is larger than 1GiB.",
				Evidence:          fmt.Sprintf("%s %s %s", log.Container, log.LogPath, humanBytes(log.SizeBytes)),
				Recommendation:    "Prefer log rotation first. If emergency cleanup is needed, stop or rotate through Docker-safe procedures after confirming service impact.",
				MinValidation:     "Re-run host disk and confirm filesystem available bytes increased and the container keeps writing new logs normally.",
				ExecutionBoundary: "plan_only; OpsPilot only reports the candidate and does not truncate files.",
			})
			break
		}
	}
	if dockerDisk.Available && dockerDisk.ApproxReclaimableBytes >= 1*gib {
		plans = append(plans, nodeagent.HostCleanupPlanItem{
			ID:                "docker-reclaimable-review",
			Risk:              "controlled_mutation",
			Summary:           "Docker reports reclaimable image, volume, or build-cache bytes.",
			Evidence:          humanBytes(dockerDisk.ApproxReclaimableBytes),
			Recommendation:    "Review unused images/build cache before pruning; keep release images that may be needed for rollback.",
			MinValidation:     "Run docker system df and OpsPilot host disk before/after the manual prune to confirm reclaimed bytes.",
			ExecutionBoundary: "plan_only; OpsPilot does not run docker prune.",
		})
	}
	for _, item := range topPaths {
		if item.Path == "/var/log/journal" && item.SizeBytes >= 1*gib {
			plans = append(plans, nodeagent.HostCleanupPlanItem{
				ID:                "journald-retention-review",
				Risk:              "controlled_mutation",
				Summary:           "journald storage is larger than 1GiB.",
				Evidence:          humanBytes(item.SizeBytes),
				Recommendation:    "Set journald retention or vacuum old logs after confirming audit/log retention requirements.",
				MinValidation:     "Re-run host disk and verify /var/log/journal decreases without losing required audit evidence.",
				ExecutionBoundary: "plan_only; OpsPilot does not run journalctl vacuum.",
			})
			break
		}
	}
	return plans
}

func parseDiskPaths(raw string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		clean := cleanHostPath(part)
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

func cleanHostPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "/") {
		return ""
	}
	return path.Clean(raw)
}

func mapHostPath(hostRoot, displayPath string) string {
	clean := cleanHostPath(displayPath)
	if clean == "" {
		return ""
	}
	hostRoot = strings.TrimRight(strings.TrimSpace(hostRoot), `/\`)
	if hostRoot == "" {
		return clean
	}
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	return filepath.Join(append([]string{hostRoot}, parts...)...)
}

func displayHostPath(hostRoot, actualPath string) string {
	hostRoot = strings.TrimRight(strings.TrimSpace(hostRoot), `/\`)
	if hostRoot == "" {
		return cleanHostPath(filepath.ToSlash(actualPath))
	}
	rel, err := filepath.Rel(hostRoot, actualPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return cleanHostPath("/" + filepath.ToSlash(rel))
}

func readMounts(hostRoot string) []mountRecord {
	mountPath := "/proc/mounts"
	if strings.TrimSpace(hostRoot) != "" {
		mountPath = filepath.Join(hostRoot, "proc", "mounts")
	}
	body, err := os.ReadFile(mountPath)
	if err != nil {
		return nil
	}
	out := []mountRecord{}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		out = append(out, mountRecord{
			device:     unescapeMountField(fields[0]),
			mountpoint: unescapeMountField(fields[1]),
			fstype:     unescapeMountField(fields[2]),
		})
	}
	return out
}

func bestMountRecord(displayPath string, mounts []mountRecord) mountRecord {
	best := mountRecord{mountpoint: displayPath}
	for _, record := range mounts {
		mp := record.mountpoint
		if displayPath == mp || strings.HasPrefix(displayPath, strings.TrimRight(mp, "/")+"/") {
			if len(mp) > len(best.mountpoint) || best.device == "" {
				best = record
			}
		}
	}
	return best
}

func unescapeMountField(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(value)
}

func firstContainerName(item map[string]any) string {
	names := stringSlice(item["names"])
	if len(names) > 0 {
		return names[0]
	}
	return shortID(fmt.Sprint(item["id"]))
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func stringMap(value any) map[string]string {
	raw := mapValue(value)
	if len(raw) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, item := range raw {
		out[key] = fmt.Sprint(item)
	}
	return out
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func boolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func roundDiskPercent(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func humanBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%dB", value)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	size := float64(value)
	for _, unit := range units {
		size = size / 1024
		if size < 1024 {
			return fmt.Sprintf("%.1f%s", size, unit)
		}
	}
	return fmt.Sprintf("%.1fPiB", size/1024)
}
