package configloader

import "strings"

const DefaultProbePolicyName = "default-http-probe"

func (c Config) ResolveProbePolicy(name string) ProbePolicy {
	name = strings.TrimSpace(name)
	if name != "" {
		for _, item := range c.ProbePolicies {
			if item.Name == name {
				return normalizeProbePolicy(item)
			}
		}
	}
	for _, item := range c.ProbePolicies {
		if item.Default {
			return normalizeProbePolicy(item)
		}
	}
	if len(c.ProbePolicies) > 0 {
		return normalizeProbePolicy(c.ProbePolicies[0])
	}
	return DefaultHTTPProbePolicy()
}

func DefaultHTTPProbePolicy() ProbePolicy {
	return normalizeProbePolicy(ProbePolicy{
		Name:    DefaultProbePolicyName,
		Default: true,
		Target:  "http",
		Window: ProbePolicyWindow{
			SinceSeconds:  900,
			WindowSeconds: 300,
			Limit:         20,
		},
		Evidence: []ProbeEvidencePolicy{
			{
				Name:        "gateway_logs",
				Type:        "gateway_logs",
				OnMissing:   "warn",
				MatchFields: []string{"host", "uri", "status", "probe_id", "user_agent"},
			},
			{
				Name:        "service_logs",
				Type:        "service_logs",
				OnMissing:   "warn",
				MatchFields: []string{"uri", "trace_id", "probe_id", "keyword"},
			},
			{
				Name:      "pod",
				Type:      "kubernetes_pod",
				OnMissing: "skip",
			},
			{
				Name:      "metrics",
				Type:      "prometheus_pod",
				OnMissing: "warn",
			},
		},
	})
}

func normalizeProbePolicy(policy ProbePolicy) ProbePolicy {
	if policy.Name == "" {
		policy.Name = DefaultProbePolicyName
	}
	if policy.Target == "" {
		policy.Target = "http"
	}
	if policy.Window.SinceSeconds <= 0 {
		policy.Window.SinceSeconds = 900
	}
	if policy.Window.WindowSeconds <= 0 {
		policy.Window.WindowSeconds = 300
	}
	if policy.Window.Limit <= 0 {
		policy.Window.Limit = 20
	}
	for idx := range policy.Evidence {
		if policy.Evidence[idx].OnMissing == "" {
			policy.Evidence[idx].OnMissing = "warn"
		}
	}
	return policy
}

func ProbeEvidenceEnabled(item ProbeEvidencePolicy) bool {
	return item.Enabled == nil || *item.Enabled
}

func ProbeEvidenceRequired(item ProbeEvidencePolicy) bool {
	return item.Required != nil && *item.Required
}
