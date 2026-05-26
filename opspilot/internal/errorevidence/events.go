package errorevidence

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
)

type Event struct {
	ID            string         `json:"id"`
	Time          string         `json:"time,omitempty"`
	Source        string         `json:"source"`
	Stage         string         `json:"stage"`
	Service       string         `json:"service,omitempty"`
	Namespace     string         `json:"namespace,omitempty"`
	Resource      string         `json:"resource,omitempty"`
	Severity      string         `json:"severity"`
	Status        string         `json:"status"`
	Message       string         `json:"message"`
	Evidence      []string       `json:"evidence,omitempty"`
	ProbableCause string         `json:"probable_cause,omitempty"`
	NextChecks    []string       `json:"next_checks,omitempty"`
	Raw           map[string]any `json:"raw,omitempty"`
}

type Request struct {
	Source    string
	Service   string
	Namespace string
	Limit     int
}

type Result struct {
	Items     []Event  `json:"items"`
	ItemCount int      `json:"item_count"`
	Truncated bool     `json:"truncated"`
	Sources   []string `json:"sources"`
}

type Collector struct {
	eventDir string
}

func NewCollector(eventDir string) *Collector {
	return &Collector{eventDir: eventDir}
}

func (c *Collector) Recent(ctx context.Context, client *k8s.Client, releases *release.Registry, promRegistry *prom.Registry, logClient *logsearch.Client, req Request) (Result, []string, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	warnings := []string{}
	events := []Event{}

	if req.Source == "" || req.Source == "kubernetes" {
		k8sEvents, err := kubernetesEvents(ctx, client, req.Limit)
		if err != nil {
			warnings = append(warnings, "kubernetes: "+err.Error())
		}
		events = append(events, k8sEvents...)
	}
	if req.Source == "" || req.Source == "argocd" {
		argoEvents, err := argoEvents(ctx, client, req.Limit)
		if err != nil {
			warnings = append(warnings, "argocd: "+err.Error())
		}
		events = append(events, argoEvents...)
	}
	if releases != nil && releases.Configured() && (req.Source == "" || req.Source == "release") {
		releaseEvents, releaseWarnings := releaseEvents(ctx, releases, client, promRegistry, logClient)
		warnings = append(warnings, releaseWarnings...)
		events = append(events, releaseEvents...)
	}
	if c.eventDir != "" && (req.Source == "" || req.Source == "middleware" || req.Source == "file") {
		fileEvents, err := c.fileEvents(req.Limit * 5)
		if err != nil {
			warnings = append(warnings, "file events: "+err.Error())
		}
		events = append(events, fileEvents...)
	}

	events = filterEvents(events, req)
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Time > events[j].Time
	})
	total := len(events)
	if req.Limit > 0 && req.Limit < total {
		events = events[:req.Limit]
	}
	return Result{
		Items:     events,
		ItemCount: len(events),
		Truncated: total > len(events),
		Sources:   eventSources(events),
	}, unique(warnings), nil
}

func kubernetesEvents(ctx context.Context, client *k8s.Client, limit int) ([]Event, error) {
	result, err := client.ListPods(ctx, "", "abnormal", "", limit)
	if err != nil {
		return nil, err
	}
	events := []Event{}
	for _, pod := range result.Items {
		namespace := stringValue(pod, "namespace")
		name := stringValue(pod, "name")
		reasons := stringSlice(pod["waiting_reasons"])
		message := fmt.Sprintf("Pod %s/%s is abnormal", namespace, name)
		if len(reasons) > 0 {
			message += ": " + strings.Join(reasons, ", ")
		}
		evidence := []string{
			"phase=" + stringValue(pod, "phase"),
			"status=" + stringValue(pod, "status"),
			fmt.Sprintf("ready=%v", boolValue(pod, "ready")),
			fmt.Sprintf("restart_count=%d", intValue(pod, "restart_count")),
			"node=" + stringValue(pod, "node"),
		}
		events = append(events, Event{
			ID:            stableID("kubernetes", namespace, name),
			Time:          firstNonEmpty(stringValue(pod, "start_time"), time.Now().Format(time.RFC3339)),
			Source:        "kubernetes",
			Stage:         "runtime",
			Service:       serviceFromPod(pod),
			Namespace:     namespace,
			Resource:      "pod/" + name,
			Severity:      podSeverity(pod),
			Status:        "open",
			Message:       message,
			Evidence:      evidence,
			ProbableCause: probablePodCause(reasons),
			NextChecks: []string{
				fmt.Sprintf("context pod --namespace %s --pod %s", namespace, name),
				fmt.Sprintf("diagnose pod --namespace %s --pod %s", namespace, name),
			},
			Raw: pod,
		})
	}
	return events, nil
}

func argoEvents(ctx context.Context, client *k8s.Client, limit int) ([]Event, error) {
	result, err := client.ListArgoApplications(ctx, "argocd", limit)
	if err != nil {
		return nil, err
	}
	events := []Event{}
	for _, app := range result.Items {
		syncStatus := stringValue(app, "sync_status")
		healthStatus := stringValue(app, "health_status")
		if syncStatus == "Synced" && healthStatus == "Healthy" {
			continue
		}
		name := stringValue(app, "name")
		namespace := stringValue(app, "dest_namespace")
		evidence := []string{
			"sync_status=" + syncStatus,
			"health_status=" + healthStatus,
			"operation_phase=" + stringValue(app, "operation_phase"),
			"message=" + firstNonEmpty(stringValue(app, "message"), stringValue(app, "health_message")),
		}
		events = append(events, Event{
			ID:            stableID("argocd", name),
			Time:          time.Now().Format(time.RFC3339),
			Source:        "argocd",
			Stage:         "sync",
			Service:       name,
			Namespace:     namespace,
			Resource:      "application/" + name,
			Severity:      argoSeverity(syncStatus, healthStatus),
			Status:        "open",
			Message:       fmt.Sprintf("Argo CD application %s sync=%s health=%s", name, syncStatus, healthStatus),
			Evidence:      compactEvidence(evidence),
			ProbableCause: "desired state is not fully reconciled",
			NextChecks:    []string{"release status --service " + name, "inspect cluster"},
			Raw:           app,
		})
	}
	return events, nil
}

func releaseEvents(ctx context.Context, releases *release.Registry, client *k8s.Client, promRegistry *prom.Registry, logClient *logsearch.Client) ([]Event, []string) {
	events := []Event{}
	warnings := []string{}
	for _, service := range releases.Services() {
		status, releaseWarnings, err := releases.Status(ctx, service, client, promRegistry, logClient)
		warnings = append(warnings, prefixWarnings("release "+service, releaseWarnings)...)
		if err != nil {
			events = append(events, Event{
				ID:            stableID("release", service, "status_error"),
				Time:          time.Now().Format(time.RFC3339),
				Source:        "release",
				Stage:         "evidence",
				Service:       service,
				Severity:      "warning",
				Status:        "open",
				Message:       "release evidence read failed: " + err.Error(),
				ProbableCause: "release datasource or service mapping is incomplete",
				NextChecks:    []string{"release status --service " + service},
			})
			continue
		}
		releaseStatus := stringValue(status, "status")
		gaps := stringSlice(status["gaps"])
		if releaseStatus == "healthy" && len(gaps) == 0 {
			continue
		}
		namespace := stringValue(status, "namespace")
		deployment := stringValue(status, "deployment")
		events = append(events, Event{
			ID:            stableID("release", service, releaseStatus, strings.Join(gaps, ",")),
			Time:          time.Now().Format(time.RFC3339),
			Source:        "release",
			Stage:         firstNonEmpty(stringValue(status, "stage"), "evidence"),
			Service:       service,
			Namespace:     namespace,
			Resource:      "deployment/" + deployment,
			Severity:      releaseSeverity(releaseStatus, gaps),
			Status:        "open",
			Message:       fmt.Sprintf("Release %s status=%s gaps=%s", service, releaseStatus, strings.Join(gaps, ",")),
			Evidence:      gaps,
			ProbableCause: releaseProbableCause(releaseStatus, gaps),
			NextChecks:    stringSlice(status["next_checks"]),
			Raw:           status,
		})
	}
	return events, unique(warnings)
}

func (c *Collector) fileEvents(maxFiles int) ([]Event, error) {
	if maxFiles <= 0 {
		maxFiles = 100
	}
	entries, err := os.ReadDir(c.eventDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, err
	}
	events := []Event{}
	for _, entry := range entries {
		if entry.IsDir() || len(events) >= maxFiles {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		path := filepath.Join(c.eventDir, name)
		info, err := entry.Info()
		if err != nil || info.Size() > 1024*1024 {
			continue
		}
		if strings.HasSuffix(name, ".jsonl") {
			loaded, err := readJSONLines(path)
			if err != nil {
				return events, err
			}
			events = append(events, loaded...)
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return events, err
		}
		var event Event
		if err := json.Unmarshal(body, &event); err != nil {
			return events, err
		}
		events = append(events, normalizeFileEvent(event, path))
	}
	return events, nil
}

func readJSONLines(path string) ([]Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	events := []Event{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return events, err
		}
		events = append(events, normalizeFileEvent(event, path))
	}
	return events, scanner.Err()
}

func normalizeFileEvent(event Event, path string) Event {
	if event.Source == "" {
		event.Source = "middleware"
	}
	if event.Stage == "" {
		event.Stage = "provision"
	}
	if event.Severity == "" {
		event.Severity = "warning"
	}
	if event.Status == "" {
		event.Status = "open"
	}
	if event.Time == "" {
		event.Time = time.Now().Format(time.RFC3339)
	}
	if event.ID == "" {
		event.ID = stableID(event.Source, event.Stage, event.Service, event.Namespace, event.Resource, event.Message, path)
	}
	if event.Raw == nil {
		event.Raw = map[string]any{"path": path}
	}
	return event
}

func filterEvents(events []Event, req Request) []Event {
	out := []Event{}
	for _, event := range events {
		if req.Source != "" && event.Source != req.Source {
			continue
		}
		if req.Service != "" && event.Service != req.Service {
			continue
		}
		if req.Namespace != "" && event.Namespace != req.Namespace {
			continue
		}
		out = append(out, event)
	}
	return out
}

func eventSources(events []Event) []string {
	values := []string{}
	for _, event := range events {
		values = append(values, event.Source)
	}
	return unique(values)
}

func serviceFromPod(pod map[string]any) string {
	labels, _ := pod["labels"].(map[string]any)
	for _, key := range []string{"app.kubernetes.io/name", "app", "service"} {
		if value := fmt.Sprint(labels[key]); value != "" && value != "<nil>" {
			return value
		}
	}
	owner := stringValue(pod, "owner_name")
	if owner != "" {
		return owner
	}
	return stringValue(pod, "name")
}

func podSeverity(pod map[string]any) string {
	reasons := strings.ToLower(strings.Join(stringSlice(pod["waiting_reasons"]), ","))
	switch {
	case strings.Contains(reasons, "crashloop") || strings.Contains(reasons, "imagepull"):
		return "critical"
	case intValue(pod, "restart_count") > 0:
		return "warning"
	default:
		return "warning"
	}
}

func probablePodCause(reasons []string) string {
	lower := strings.ToLower(strings.Join(reasons, ","))
	switch {
	case strings.Contains(lower, "imagepull") || strings.Contains(lower, "errimagepull"):
		return "image cannot be pulled or registry credentials are missing"
	case strings.Contains(lower, "crashloop"):
		return "container is repeatedly crashing after start"
	case strings.Contains(lower, "containercreating"):
		return "container is waiting for image, network, volume, or runtime setup"
	default:
		return ""
	}
}

func argoSeverity(syncStatus, healthStatus string) string {
	if healthStatus == "Degraded" {
		return "critical"
	}
	if syncStatus == "OutOfSync" {
		return "warning"
	}
	return "warning"
}

func releaseSeverity(status string, gaps []string) string {
	if status == "degraded" || status == "failed" {
		return "critical"
	}
	if len(gaps) > 0 {
		return "warning"
	}
	return "info"
}

func releaseProbableCause(status string, gaps []string) string {
	if len(gaps) > 0 {
		return "release evidence gaps: " + strings.Join(gaps, ",")
	}
	if status != "" && status != "healthy" {
		return "release is not healthy"
	}
	return ""
}

func prefixWarnings(prefix string, warnings []string) []string {
	out := []string{}
	for _, warning := range warnings {
		if warning != "" {
			out = append(out, prefix+": "+warning)
		}
	}
	return out
}

func compactEvidence(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !strings.HasSuffix(value, "=") {
			out = append(out, value)
		}
	}
	return out
}

func stableID(parts ...string) string {
	joined := strings.Join(parts, "|")
	sum := sha1.Sum([]byte(joined))
	return hex.EncodeToString(sum[:])[:16]
}

func stringValue(m map[string]any, key string) string {
	if value, ok := m[key]; ok {
		return fmt.Sprint(value)
	}
	return ""
}

func boolValue(m map[string]any, key string) bool {
	value, _ := m[key].(bool)
	return value
}

func intValue(m map[string]any, key string) int {
	switch value := m[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func stringSlice(value any) []string {
	out := []string{}
	switch raw := value.(type) {
	case []string:
		return raw
	case []any:
		for _, item := range raw {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				out = append(out, text)
			}
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
