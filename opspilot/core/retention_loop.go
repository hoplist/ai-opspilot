package main

import (
	"fmt"
	"os"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/evidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/retention"
)

func startRetentionCleanupLoop(store *evidence.Store, collector *errorevidence.Collector, eventPolicy retention.Policy, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		for {
			<-timer.C
			if store != nil {
				if err := store.Cleanup(); err != nil {
					fmt.Fprintf(os.Stderr, "evidence_retention_cleanup_failed error=%v\n", err)
				}
			}
			if collector != nil {
				if err := collector.Cleanup(eventPolicy); err != nil {
					fmt.Fprintf(os.Stderr, "error_event_retention_cleanup_failed error=%v\n", err)
				}
			}
			timer.Reset(interval)
		}
	}()
}
