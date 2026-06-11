package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/evidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
)

func startEventPackLoop(k8sRegistry *k8s.Registry, promRegistry *prom.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, collector *errorevidence.Collector, store *evidence.Store, interval time.Duration) {
	if k8sRegistry == nil || collector == nil || store == nil || !store.Enabled() || interval <= 0 {
		return
	}
	go func() {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		for {
			<-timer.C
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			if err := writeRecentEventPacks(ctx, k8sRegistry, promRegistry, logClient, releaseRegistry, collector, store); err != nil {
				fmt.Fprintf(os.Stderr, "event_pack_scan_failed error=%v\n", err)
			}
			cancel()
			timer.Reset(interval)
		}
	}()
}

func writeRecentEventPacks(ctx context.Context, k8sRegistry *k8s.Registry, promRegistry *prom.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, collector *errorevidence.Collector, store *evidence.Store) error {
	client, _, err := k8sRegistry.ClientFor("")
	if err != nil {
		return err
	}
	events, warnings, err := collector.Recent(ctx, client, releaseRegistry, promRegistry, logClient, errorevidence.Request{Limit: 50})
	if err != nil {
		return err
	}
	for _, event := range events.Items {
		pack := evidence.Pack{
			ID:       "event-" + event.ID,
			Trigger:  "scheduled_event_scan",
			Target:   evidence.Target{Type: targetTypeFromEvent(event), Name: targetNameFromEvent(event), Namespace: event.Namespace, Cluster: client.ClusterName()},
			Status:   statusFromSeverity(event.Severity),
			Summary:  event.Message,
			Sources:  []evidence.Source{{Name: event.Source, Status: "available", Detail: event.Stage}},
			Evidence: map[string]any{"event": event},
			Warnings: warnings,
			RecommendedActions: []evidence.Action{
				evidence.ReadOnlyNextCheck(firstEventNextCheck(event), "Use the generated event evidence pack before planning any mutation."),
			},
		}
		if _, err := store.Write(pack); err != nil {
			return err
		}
	}
	return nil
}

func targetTypeFromEvent(event errorevidence.Event) string {
	if event.Resource != "" && len(event.Resource) >= 4 && event.Resource[:4] == "pod/" {
		return "pod"
	}
	if event.Service != "" {
		return "service"
	}
	return "cluster"
}

func targetNameFromEvent(event errorevidence.Event) string {
	if strings.HasPrefix(event.Resource, "pod/") {
		return strings.TrimPrefix(event.Resource, "pod/")
	}
	if event.Service != "" {
		return event.Service
	}
	if parts := strings.SplitN(event.Resource, "/", 2); len(parts) == 2 {
		return parts[1]
	}
	return event.Resource
}

func statusFromSeverity(severity string) string {
	switch severity {
	case "critical":
		return "unhealthy"
	case "warning":
		return "degraded"
	default:
		return "unknown"
	}
}

func firstEventNextCheck(event errorevidence.Event) string {
	if len(event.NextChecks) > 0 {
		return event.NextChecks[0]
	}
	if event.Service != "" {
		return "inspect service --service " + event.Service
	}
	return "inspect cluster"
}
