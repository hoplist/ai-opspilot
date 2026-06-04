package intent

import "testing"

func TestInterpretReleasePlanFirst(t *testing.T) {
	got := Interpret(Request{
		Query:    "deploy opspilot-core",
		Services: []string{"opspilot-core"},
	})
	if got.Action != "release_service" || got.Service != "opspilot-core" {
		t.Fatalf("unexpected intent: %#v", got)
	}
	if got.Risk != "controlled_mutate" || got.Automation != "plan_first" {
		t.Fatalf("release should be plan-first controlled mutation: %#v", got)
	}
}

func TestInterpretInspectDefault(t *testing.T) {
	got := Interpret(Request{
		Query:    "check opspilot-core status",
		Services: []string{"opspilot-core"},
	})
	if got.Action != "inspect_service" || got.Risk != "read_only" || got.Automation != "auto_execute" {
		t.Fatalf("unexpected inspect intent: %#v", got)
	}
}

func TestInterpretMissingServiceWarns(t *testing.T) {
	got := Interpret(Request{Query: "check current service"})
	if got.Service != "" || len(got.Warnings) == 0 {
		t.Fatalf("expected missing service warning: %#v", got)
	}
}
