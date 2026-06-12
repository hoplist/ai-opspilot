package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/audit"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/evidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/retention"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/version"
)

func main() {
	host := flag.String("host", env("OPSPILOT_HOST", "0.0.0.0"), "listen host")
	port := flag.String("port", env("OPSPILOT_PORT", "18080"), "listen port")
	flag.Parse()

	runtimeConfig := loadRuntimeConfig()
	state := newRuntimeState(runtimeConfig)
	qualitySettings := release.QualitySettings{
		Enabled:         boolEnv("OPSPILOT_QUALITY_ENABLED", true),
		RunnerImage:     env("OPSPILOT_QUALITY_RUNNER_IMAGE", ""),
		ImagePullSecret: env("OPSPILOT_QUALITY_IMAGE_PULL_SECRET", ""),
		Ref:             env("OPSPILOT_QUALITY_REF", ""),
		TTLSeconds:      intEnv("OPSPILOT_QUALITY_JOB_TTL_SECONDS", 3600),
		DeadlineSeconds: intEnv("OPSPILOT_QUALITY_DEADLINE_SECONDS", 120),
	}
	errorCollector := errorevidence.NewCollector(env("OPSPILOT_ERROR_EVENT_DIR", "/var/lib/opspilot/error-events"))
	auditRecorder := audit.NewRecorderWithRetention(env("OPSPILOT_AUDIT_LOG_PATH", "/var/lib/opspilot/audit/audit.jsonl"), audit.RetentionPolicy{
		MaxBytes: int64(intEnv("OPSPILOT_AUDIT_MAX_BYTES", 33554432)),
		MaxAge:   time.Duration(intEnv("OPSPILOT_AUDIT_RETENTION_DAYS", 7)) * 24 * time.Hour,
	})
	evidenceStore := evidence.NewStoreWithRetention(env("OPSPILOT_EVIDENCE_PACK_DIR", "/var/lib/opspilot/evidence-packs"), retention.Policy{
		MaxItems:  intEnv("OPSPILOT_EVIDENCE_PACK_MAX_ITEMS", 200),
		MaxAge:    time.Duration(intEnv("OPSPILOT_EVIDENCE_PACK_RETENTION_DAYS", 3)) * 24 * time.Hour,
		MaxBytes:  int64(intEnv("OPSPILOT_EVIDENCE_PACK_MAX_BYTES", 100663296)),
		Extension: []string{".json"},
	})
	errorEventRetention := retention.Policy{
		MaxItems:  intEnv("OPSPILOT_ERROR_EVENT_MAX_ITEMS", 100),
		MaxAge:    time.Duration(intEnv("OPSPILOT_ERROR_EVENT_RETENTION_DAYS", 3)) * 24 * time.Hour,
		MaxBytes:  int64(intEnv("OPSPILOT_ERROR_EVENT_MAX_BYTES", 33554432)),
		Extension: []string{".json", ".jsonl"},
	}
	if boolEnv("OPSPILOT_EVENT_PACK_ENABLED", true) {
		startEventPackLoopWithState(state, errorCollector, evidenceStore, time.Duration(intEnv("OPSPILOT_EVENT_PACK_INTERVAL_SECONDS", 300))*time.Second)
	}
	startConfigReloadLoop(state, time.Duration(intEnv("OPSPILOT_CONFIG_RELOAD_SECONDS", 0))*time.Second)
	startRetentionCleanupLoop(evidenceStore, errorCollector, errorEventRetention, time.Duration(intEnv("OPSPILOT_RETENTION_CLEANUP_INTERVAL_SECONDS", 300))*time.Second)
	mux := http.NewServeMux()
	registerRoutes(mux, state, errorCollector, qualitySettings, auditRecorder, evidenceStore)
	addr := *host + ":" + *port
	fmt.Printf("opspilot-core %s listening on http://%s\n", version.Version, addr)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
