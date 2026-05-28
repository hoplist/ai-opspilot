package skillregistry

import "testing"

func TestRegistryIntegratedCoreSkills(t *testing.T) {
	catalog := Registry("", true)
	if catalog.IntegratedCount < 6 {
		t.Fatalf("integrated count = %d", catalog.IntegratedCount)
	}
	required := []string{
		"opspilot-ops",
		"auto-inspection-rca",
		"kubernetes-specialist",
		"monitoring-expert",
		"devops-engineer",
		"debugging-wizard",
	}
	for _, name := range required {
		if !hasSkill(catalog.Items, name) {
			t.Fatalf("missing skill %s", name)
		}
	}
}

func TestRecommendPodIncludesKubernetesAndDebugging(t *testing.T) {
	recommendations := Recommend("pod", "unhealthy", []string{"elk_logs_missing"}, []string{"Pod is not ready", "restart count is high"})
	for _, name := range []string{"opspilot-ops", "kubernetes-specialist", "monitoring-expert", "debugging-wizard", "auto-inspection-rca"} {
		if !hasRecommendation(recommendations, name) {
			t.Fatalf("missing recommendation %s: %#v", name, recommendations)
		}
	}
}

func hasSkill(items []Skill, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func hasRecommendation(items []Recommendation, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
