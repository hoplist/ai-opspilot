package assets

import (
	"net/netip"
	"sort"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
)

const Version = "v1"

type Zone struct {
	Name         string   `json:"name"`
	Region       string   `json:"region,omitempty"`
	Zone         string   `json:"zone,omitempty"`
	CIDRs        []string `json:"cidrs,omitempty"`
	EntryPoints  []string `json:"entrypoints,omitempty"`
	Coverage     string   `json:"coverage,omitempty"`
	ActionPolicy string   `json:"action_policy,omitempty"`
	Description  string   `json:"description,omitempty"`
	Source       string   `json:"source,omitempty"`
}

type Source struct {
	Name        string     `json:"name"`
	Kind        string     `json:"kind,omitempty"`
	Region      string     `json:"region,omitempty"`
	NetworkZone string     `json:"network_zone,omitempty"`
	URLSet      bool       `json:"url_set"`
	Enabled     bool       `json:"enabled"`
	Required    bool       `json:"required"`
	Coverage    string     `json:"coverage,omitempty"`
	Timeout     string     `json:"timeout,omitempty"`
	OnError     string     `json:"on_error,omitempty"`
	Sync        SourceSync `json:"sync,omitempty"`
	Note        string     `json:"note,omitempty"`
	Source      string     `json:"source,omitempty"`
}

type SourceSync struct {
	Enabled      bool   `json:"enabled"`
	Mode         string `json:"mode,omitempty"`
	DeletePolicy string `json:"delete_policy,omitempty"`
	Interval     string `json:"interval,omitempty"`
}

type Asset struct {
	Name            string            `json:"name"`
	Hostname        string            `json:"hostname,omitempty"`
	IPs             []string          `json:"ips,omitempty"`
	AssetType       string            `json:"asset_type,omitempty"`
	Region          string            `json:"region,omitempty"`
	NetworkZone     string            `json:"network_zone,omitempty"`
	BusinessLine    string            `json:"business_line,omitempty"`
	Business        string            `json:"business,omitempty"`
	Status          string            `json:"status,omitempty"`
	Owner           string            `json:"owner,omitempty"`
	Sources         []string          `json:"sources,omitempty"`
	ExpectedSources []string          `json:"expected_sources,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Source          string            `json:"source,omitempty"`
}

type Finding struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	TargetType  string `json:"target_type"`
	Target      string `json:"target"`
	NetworkZone string `json:"network_zone,omitempty"`
	IP          string `json:"ip,omitempty"`
	Summary     string `json:"summary"`
	Advice      string `json:"advice"`
}

type Catalog struct {
	Version  string         `json:"version"`
	Source   string         `json:"source"`
	Counts   map[string]int `json:"counts"`
	Zones    []Zone         `json:"zones"`
	Sources  []Source       `json:"sources"`
	Assets   []Asset        `json:"assets"`
	Warnings []string       `json:"warnings,omitempty"`
}

type InspectResult struct {
	Version         string    `json:"version"`
	QueryIP         string    `json:"query_ip,omitempty"`
	Zone            *Zone     `json:"zone,omitempty"`
	MatchedEntry    bool      `json:"matched_entrypoint,omitempty"`
	Assets          []Asset   `json:"assets,omitempty"`
	Sources         []Source  `json:"sources,omitempty"`
	MissingEvidence []string  `json:"missing_evidence,omitempty"`
	Findings        []Finding `json:"findings,omitempty"`
	Advice          []string  `json:"advice,omitempty"`
}

type DiffResult struct {
	Version  string    `json:"version"`
	Mode     string    `json:"mode"`
	Count    int       `json:"count"`
	Findings []Finding `json:"findings"`
	Warnings []string  `json:"warnings,omitempty"`
}

type SyncPlan struct {
	Version         string    `json:"version"`
	Mode            string    `json:"mode"`
	Source          string    `json:"source,omitempty"`
	Kind            string    `json:"kind,omitempty"`
	Ready           bool      `json:"ready"`
	DeletePolicy    string    `json:"delete_policy"`
	Actions         []string  `json:"actions"`
	Validation      []string  `json:"validation"`
	MissingEvidence []string  `json:"missing_evidence,omitempty"`
	Findings        []Finding `json:"findings,omitempty"`
	Warnings        []string  `json:"warnings,omitempty"`
}

func Build(cfg configloader.Config) Catalog {
	zones := zonesFromConfig(cfg.NetworkZones)
	sources := sourcesFromConfig(cfg.AssetSources)
	assetItems := assetsFromConfig(cfg.Assets, zones)
	warnings := []string{}
	if len(zones) == 0 {
		warnings = append(warnings, "network_zones_missing")
	}
	return Catalog{
		Version: Version,
		Source:  sourceName(cfg.Source),
		Counts: map[string]int{
			"zones":   len(zones),
			"sources": len(sources),
			"assets":  len(assetItems),
		},
		Zones:    zones,
		Sources:  sources,
		Assets:   assetItems,
		Warnings: warnings,
	}
}

func Zones(cfg configloader.Config) []Zone {
	return zonesFromConfig(cfg.NetworkZones)
}

func InspectIP(cfg configloader.Config, rawIP string) InspectResult {
	rawIP = strings.TrimSpace(rawIP)
	result := InspectResult{Version: Version, QueryIP: rawIP}
	ip, err := netip.ParseAddr(rawIP)
	if err != nil {
		result.MissingEvidence = append(result.MissingEvidence, "invalid_ip")
		result.Findings = append(result.Findings, Finding{
			Type:       "invalid_ip",
			Severity:   "error",
			TargetType: "ip",
			Target:     rawIP,
			IP:         rawIP,
			Summary:    "IP 格式不合法",
			Advice:     "确认输入的是单个 IPv4/IPv6 地址，不要传 CIDR 或主机名。",
		})
		return result
	}
	zones := zonesFromConfig(cfg.NetworkZones)
	if zone, entry := matchZone(zones, ip); zone != nil {
		result.Zone = zone
		result.MatchedEntry = entry
		result.Sources = sourcesForZone(sourcesFromConfig(cfg.AssetSources), zone.Name)
	} else {
		result.MissingEvidence = append(result.MissingEvidence, "network_zone_missing")
		result.Findings = append(result.Findings, Finding{
			Type:       "unknown_zone",
			Severity:   "warning",
			TargetType: "ip",
			Target:     rawIP,
			IP:         rawIP,
			Summary:    "IP 未命中已配置网络域",
			Advice:     "把该 IP 所属网段补入 opspilot-config 的 network_zones 后再关联 JumpServer/Prometheus。",
		})
		result.Advice = append(result.Advice, "先补网络域规则，再判断是否缺 JumpServer 或 Prometheus 覆盖。")
	}
	for _, asset := range assetsFromConfig(cfg.Assets, zones) {
		if assetHasIP(asset, rawIP) {
			result.Assets = append(result.Assets, asset)
		}
	}
	if result.Zone != nil {
		result.Findings = append(result.Findings, sourceFindingsForZone(result.Sources, result.Zone.Name)...)
		if !hasConfiguredKind(result.Sources, "jumpserver") {
			result.MissingEvidence = append(result.MissingEvidence, "jumpserver_source_missing")
		} else if !hasEnabledKind(result.Sources, "jumpserver") {
			result.MissingEvidence = append(result.MissingEvidence, "jumpserver_source_inactive")
		}
		if !hasConfiguredKind(result.Sources, "prometheus") {
			result.MissingEvidence = append(result.MissingEvidence, "prometheus_source_missing")
		} else if !hasEnabledKind(result.Sources, "prometheus") {
			result.MissingEvidence = append(result.MissingEvidence, "prometheus_source_inactive")
		}
		if result.MatchedEntry {
			result.Advice = append(result.Advice, "该 IP 是入口/堡垒类地址，先按 entrypoint 处理，不要当普通业务节点误删。")
		}
		if len(result.Sources) == 0 {
			result.Advice = append(result.Advice, "当前网络域还没有资产来源配置，先接 JumpServer/Prometheus 元数据即可。")
		}
	}
	return result
}

func Diff(cfg configloader.Config) DiffResult {
	zones := zonesFromConfig(cfg.NetworkZones)
	sources := sourcesFromConfig(cfg.AssetSources)
	assetItems := assetsFromConfig(cfg.Assets, zones)
	findings := []Finding{}
	for _, zone := range zones {
		zoneSources := sourcesForZone(sources, zone.Name)
		if !hasConfiguredKind(zoneSources, "jumpserver") {
			findings = append(findings, Finding{
				Type:        "jumpserver_source_missing",
				Severity:    "warning",
				TargetType:  "network_zone",
				Target:      zone.Name,
				NetworkZone: zone.Name,
				Summary:     "网络域未配置 JumpServer 来源",
				Advice:      "只提示缺失，不删除 Prometheus；接入对应 JumpServer API 后再做资产覆盖判断。",
			})
		}
		if !hasConfiguredKind(zoneSources, "prometheus") {
			findings = append(findings, Finding{
				Type:        "prometheus_source_missing",
				Severity:    "warning",
				TargetType:  "network_zone",
				Target:      zone.Name,
				NetworkZone: zone.Name,
				Summary:     "网络域未配置 Prometheus 来源",
				Advice:      "先补 Prometheus datasource 或 target 元数据；当前不会自动移除任何监控目标。",
			})
		}
		findings = append(findings, sourceFindingsForZone(zoneSources, zone.Name)...)
	}
	for _, asset := range assetItems {
		if asset.NetworkZone == "" {
			findings = append(findings, Finding{
				Type:       "unknown_zone",
				Severity:   "warning",
				TargetType: "asset",
				Target:     asset.Name,
				Summary:    "资产未能归属到网络域",
				Advice:     "补充资产 network_zone，或把资产 IP 所属网段加入 network_zones。",
			})
		}
		for _, expected := range asset.ExpectedSources {
			if !assetHasSourceKind(asset, expected) {
				findings = append(findings, Finding{
					Type:        "missing_" + normalizedKind(expected),
					Severity:    "warning",
					TargetType:  "asset",
					Target:      asset.Name,
					NetworkZone: asset.NetworkZone,
					Summary:     "资产缺少期望来源: " + expected,
					Advice:      "先补来源元数据；当前阶段只给建议，不执行注册或删除。",
				})
			}
		}
		if isRemoveCandidate(asset) {
			findings = append(findings, Finding{
				Type:        "remove_candidate",
				Severity:    "info",
				TargetType:  "asset",
				Target:      asset.Name,
				NetworkZone: asset.NetworkZone,
				Summary:     "资产只出现在 Prometheus，疑似遗留监控目标",
				Advice:      "人工确认 JumpServer、VM 平台和业务归属后再移除；OpsPilot 当前不会自动删除。",
			})
		}
	}
	sortFindings(findings)
	return DiffResult{
		Version:  Version,
		Mode:     "advisory_only",
		Count:    len(findings),
		Findings: findings,
	}
}

func SyncPlanForSource(cfg configloader.Config, name string) SyncPlan {
	sources := sourcesFromConfig(cfg.AssetSources)
	name = strings.TrimSpace(name)
	plan := SyncPlan{
		Version:      Version,
		Mode:         "readonly_plan",
		DeletePolicy: "mark_stale",
		Actions: []string{
			"Read assets from the configured CMDB/JMS source.",
			"Normalize hostname, IP, region, network zone, owner, and source labels.",
			"Compare remote assets with GitLab-managed asset metadata.",
			"Mark missing remote assets as stale instead of deleting them.",
			"Generate a config change plan; do not modify JumpServer, Prometheus, or GitLab automatically.",
		},
		Validation: []string{
			"opspilot cmdb catalog --output human",
			"opspilot cmdb diff --output human",
			"opspilot cmdb sync-plan --source <name> --output human",
		},
	}
	var selected *Source
	for idx := range sources {
		if name == "" && isCMDBKind(sources[idx].Kind) {
			selected = &sources[idx]
			break
		}
		if sources[idx].Name == name {
			selected = &sources[idx]
			break
		}
	}
	if selected == nil {
		plan.MissingEvidence = append(plan.MissingEvidence, "cmdb_source_missing")
		plan.Findings = append(plan.Findings, Finding{
			Type:       "cmdb_source_missing",
			Severity:   "warning",
			TargetType: "asset_source",
			Target:     firstNonEmpty(name, "default"),
			Summary:    "No CMDB/JMS asset source is configured.",
			Advice:     "Add an optional asset source with kind=jms, jumpserver, or cmdb. Missing CMDB must not block inspection.",
		})
		return plan
	}
	plan.Source = selected.Name
	plan.Kind = selected.Kind
	plan.Ready = selected.Enabled && selected.URLSet
	plan.DeletePolicy = firstNonEmpty(selected.Sync.DeletePolicy, "mark_stale")
	if !selected.Enabled {
		plan.MissingEvidence = append(plan.MissingEvidence, "cmdb_source_inactive")
	}
	if !selected.URLSet {
		plan.MissingEvidence = append(plan.MissingEvidence, "cmdb_source_url_missing")
	}
	if selected.Required {
		plan.Warnings = append(plan.Warnings, "cmdb source is marked required; OpsPilot recommends required=false so missing CMDB does not block troubleshooting")
	}
	if plan.DeletePolicy != "mark_stale" {
		plan.Warnings = append(plan.Warnings, "delete_policy should stay mark_stale in the first rollout; physical deletion remains plan-only")
	}
	return plan
}

func zonesFromConfig(items []configloader.NetworkZone) []Zone {
	out := make([]Zone, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		out = append(out, Zone{
			Name:         strings.TrimSpace(item.Name),
			Region:       strings.TrimSpace(item.Region),
			Zone:         strings.TrimSpace(item.Zone),
			CIDRs:        cleanStrings(item.CIDRs),
			EntryPoints:  cleanStrings(item.EntryPoints),
			Coverage:     firstNonEmpty(item.Coverage, "partial"),
			ActionPolicy: firstNonEmpty(item.ActionPolicy, "advisory_only"),
			Description:  strings.TrimSpace(item.Description),
			Source:       item.Source,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func sourcesFromConfig(items []configloader.AssetSource) []Source {
	out := make([]Source, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		out = append(out, Source{
			Name:        strings.TrimSpace(item.Name),
			Kind:        normalizedKind(item.Kind),
			Region:      strings.TrimSpace(item.Region),
			NetworkZone: strings.TrimSpace(item.NetworkZone),
			URLSet:      strings.TrimSpace(item.URL) != "",
			Enabled:     sourceEnabled(item),
			Required:    boolValue(item.Required),
			Coverage:    firstNonEmpty(item.Coverage, "partial"),
			Timeout:     firstNonEmpty(item.Timeout, "5s"),
			OnError:     firstNonEmpty(item.OnError, "warn"),
			Sync: SourceSync{
				Enabled:      boolValue(item.Sync.Enabled),
				Mode:         firstNonEmpty(item.Sync.Mode, "readonly"),
				DeletePolicy: firstNonEmpty(item.Sync.DeletePolicy, "mark_stale"),
				Interval:     strings.TrimSpace(item.Sync.Interval),
			},
			Note:   strings.TrimSpace(item.Note),
			Source: item.Source,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func assetsFromConfig(items []configloader.Asset, zones []Zone) []Asset {
	out := make([]Asset, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		networkZone := strings.TrimSpace(item.NetworkZone)
		if networkZone == "" {
			networkZone = zoneForAsset(item, zones)
		}
		out = append(out, Asset{
			Name:            strings.TrimSpace(item.Name),
			Hostname:        strings.TrimSpace(item.Hostname),
			IPs:             cleanStrings(item.IPs),
			AssetType:       strings.TrimSpace(item.AssetType),
			Region:          strings.TrimSpace(item.Region),
			NetworkZone:     networkZone,
			BusinessLine:    strings.TrimSpace(item.BusinessLine),
			Business:        strings.TrimSpace(item.Business),
			Status:          firstNonEmpty(item.Status, "active"),
			Owner:           strings.TrimSpace(item.Owner),
			Sources:         cleanStrings(item.Sources),
			ExpectedSources: cleanStrings(item.ExpectedSources),
			Labels:          item.Labels,
			Source:          item.Source,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func sourceFindingsForZone(sources []Source, zone string) []Finding {
	findings := []Finding{}
	for _, source := range sources {
		if !source.Enabled {
			findings = append(findings, Finding{
				Type:        "asset_source_disabled",
				Severity:    "info",
				TargetType:  "asset_source",
				Target:      source.Name,
				NetworkZone: zone,
				Summary:     "资产来源已配置但未启用",
				Advice:      "启用前只作为规划记录，不参与覆盖判断。",
			})
			continue
		}
		if !source.URLSet && (source.Kind == "jumpserver" || source.Kind == "jms" || source.Kind == "cmdb" || source.Kind == "prometheus") {
			findings = append(findings, Finding{
				Type:        source.Kind + "_url_missing",
				Severity:    "warning",
				TargetType:  "asset_source",
				Target:      source.Name,
				NetworkZone: zone,
				Summary:     "资产来源缺少 URL",
				Advice:      "补 URL 和凭证后才能做真实拉取；当前只保留网络域规划。",
			})
		}
	}
	return findings
}

func matchZone(zones []Zone, ip netip.Addr) (*Zone, bool) {
	for idx := range zones {
		for _, entry := range zones[idx].EntryPoints {
			entryIP, err := netip.ParseAddr(strings.TrimSpace(entry))
			if err == nil && entryIP == ip {
				return &zones[idx], true
			}
		}
		for _, raw := range zones[idx].CIDRs {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
			if err == nil && prefix.Contains(ip) {
				return &zones[idx], false
			}
		}
	}
	return nil, false
}

func sourcesForZone(sources []Source, zone string) []Source {
	out := []Source{}
	for _, source := range sources {
		if source.NetworkZone == zone {
			out = append(out, source)
		}
	}
	return out
}

func hasConfiguredKind(sources []Source, kind string) bool {
	kind = normalizedKind(kind)
	for _, source := range sources {
		if normalizedKind(source.Kind) == kind {
			return true
		}
	}
	return false
}

func hasEnabledKind(sources []Source, kind string) bool {
	kind = normalizedKind(kind)
	for _, source := range sources {
		if source.Enabled && normalizedKind(source.Kind) == kind {
			return true
		}
	}
	return false
}

func sourceEnabled(item configloader.AssetSource) bool {
	if item.Enabled == nil {
		return true
	}
	return *item.Enabled
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func isCMDBKind(kind string) bool {
	switch normalizedKind(kind) {
	case "jms", "jumpserver", "cmdb":
		return true
	default:
		return false
	}
}

func zoneForAsset(asset configloader.Asset, zones []Zone) string {
	for _, raw := range asset.IPs {
		ip, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		if zone, _ := matchZone(zones, ip); zone != nil {
			return zone.Name
		}
	}
	return ""
}

func assetHasIP(asset Asset, ip string) bool {
	for _, value := range asset.IPs {
		if strings.TrimSpace(value) == ip {
			return true
		}
	}
	return false
}

func assetHasSourceKind(asset Asset, kind string) bool {
	kind = normalizedKind(kind)
	for _, source := range asset.Sources {
		if normalizedKind(sourceKind(source)) == kind {
			return true
		}
	}
	return false
}

func sourceKind(source string) string {
	source = strings.TrimSpace(source)
	if before, _, ok := strings.Cut(source, ":"); ok {
		return before
	}
	return source
}

func isRemoveCandidate(asset Asset) bool {
	if len(asset.Sources) != 1 || !assetHasSourceKind(asset, "prometheus") {
		return false
	}
	for _, expected := range asset.ExpectedSources {
		if normalizedKind(expected) == "prometheus" {
			return false
		}
	}
	return true
}

func sortFindings(items []Finding) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Severity != items[j].Severity {
			return severityRank(items[i].Severity) < severityRank(items[j].Severity)
		}
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return items[i].Target < items[j].Target
	})
}

func severityRank(value string) int {
	switch value {
	case "error":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

func cleanStrings(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizedKind(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func sourceName(value string) string {
	if strings.TrimSpace(value) == "" {
		return "empty"
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
