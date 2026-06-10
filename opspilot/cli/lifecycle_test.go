package main

import (
	"testing"
)

func TestDecommissionPlanBlocksMutationsWhenMappingsAreMissing(t *testing.T) {
	release := releaseServiceResult{
		Service:    "fullstack-vue-web",
		Namespace:  "cicd-demo-fullstack",
		Deployment: "fullstack-vue-web",
		GitOps:     map[string]any{},
		ArgoCD:     map[string]any{},
	}
	inspection := inspectServiceResult{PodCount: 1}
	actions, blocked := decommissionActions(release, inspection, nil, true)

	for _, action := range actions {
		if action.Risk != "read_only" {
			t.Fatalf("missing mappings must not produce mutable action: %#v", action)
		}
	}
	blockedByID := map[string]lifecycleAction{}
	for _, action := range blocked {
		blockedByID[action.ID] = action
	}
	for _, id := range []string{"remove_gitops_application", "remove_gitops_workload_manifests", "delete_namespace"} {
		action, ok := blockedByID[id]
		if !ok {
			t.Fatalf("expected %s to be blocked: %#v", id, blocked)
		}
		if action.Risk != "high_risk" || action.Automation != "plan_only" {
			t.Fatalf("expected %s to be high-risk plan-only: %#v", id, action)
		}
	}
}

func TestDecommissionPlanAllowsGitOpsPlanOnlyWhenFullyMapped(t *testing.T) {
	release := releaseServiceResult{
		Service:    "opspilot-core",
		Namespace:  "opspilot",
		Deployment: "opspilot-core",
		GitOps:     map[string]any{"path": "clusters/test/apps/opspilot-core/deployment.yaml"},
		ArgoCD:     map[string]any{"app": "opspilot-core"},
	}
	inspection := inspectServiceResult{PodCount: 1}
	actions, blocked := decommissionActions(release, inspection, nil, true)

	allowedByID := map[string]lifecycleAction{}
	for _, action := range actions {
		allowedByID[action.ID] = action
	}
	for _, id := range []string{"remove_gitops_application", "remove_gitops_workload_manifests"} {
		action, ok := allowedByID[id]
		if !ok {
			t.Fatalf("expected %s to be allowed in the plan: %#v", id, actions)
		}
		if action.Risk != "controlled_mutate" || action.Automation != "confirm_allowed" {
			t.Fatalf("expected %s to be controlled and confirmation-gated: %#v", id, action)
		}
	}
	for _, action := range blocked {
		if action.ID == "delete_namespace" && action.Target != "opspilot" {
			t.Fatalf("unexpected namespace block target: %#v", action)
		}
	}
}
