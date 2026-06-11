package main

import (
	"testing"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
)

func TestTargetNameFromEventUsesPodResource(t *testing.T) {
	event := errorevidence.Event{
		Service:  "orders-api",
		Resource: "pod/orders-api-6f9d7",
	}
	if got := targetNameFromEvent(event); got != "orders-api-6f9d7" {
		t.Fatalf("targetNameFromEvent() = %q", got)
	}
}
