package logsearch

import (
	"strings"
	"testing"
)

func TestParseCorrelationRoutes(t *testing.T) {
	routes := ParseCorrelationRoutes("devops-steps|devops.tpo.xzoa.com|/cis/api/internal/jobserver/steps/|devops-server-*|msg|evtName|cis_steps_${id}|evtName:cis_jobserver_steps")
	if len(routes) != 1 {
		t.Fatalf("routes = %d", len(routes))
	}
	route := routes[0]
	if route.Name != "devops-steps" {
		t.Fatalf("name = %s", route.Name)
	}
	if route.ServiceIndex != "devops-server-*" {
		t.Fatalf("service index = %s", route.ServiceIndex)
	}
	if route.ServiceEventTemplate != "cis_steps_${id}" {
		t.Fatalf("event template = %s", route.ServiceEventTemplate)
	}
}

func TestServiceLogQueryUsesBusinessIDTemplate(t *testing.T) {
	route := &CorrelationRoute{
		ServiceURIField:      "msg",
		ServiceEventField:    "evtName",
		ServiceEventTemplate: "cis_steps_${id}",
	}
	query, mode := serviceLogQuery(route, "msg", "/cis/api/internal/jobserver/steps/19635751")
	if mode != "business_id" {
		t.Fatalf("mode = %s", mode)
	}
	if !containsAll(query, "evtName:\"cis_steps_19635751\"", "msg:\"/cis/api/internal/jobserver/steps/19635751\"") {
		t.Fatalf("query = %s", query)
	}
}

func TestCorrelationStrengthAllowsServiceOnlyInvestigation(t *testing.T) {
	apisix := map[string]any{
		"status": "skipped",
	}
	service := map[string]any{
		"status": "available",
		"total":  1,
		"items":  []any{map[string]any{"msg": "GET /api/hr/queryUserScheduleList"}},
	}
	if mode := investigationMode(apisix, service); mode != "service_only" {
		t.Fatalf("mode = %s", mode)
	}
	if strength := correlationStrength(apisix, service); strength != "weak" {
		t.Fatalf("strength = %s", strength)
	}
	gaps := strings.Join(correlationGaps(apisix, service), ",")
	if !strings.Contains(gaps, "apisix_log_skipped") {
		t.Fatalf("gaps = %s", gaps)
	}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
