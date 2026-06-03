package k8s

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTailLines    = 300
	DefaultSinceSeconds = 36000
	DefaultLimitBytes   = 1024 * 1024
	MaxTailLines        = 1000
	MaxSinceSeconds     = 86400
	MaxLimitBytes       = 5 * 1024 * 1024
)

type Client struct {
	kubectl   string
	mode      string
	host      string
	port      string
	tokenPath string
	caPath    string
	http      *http.Client
}

type LogRequest struct {
	Namespace    string
	Pod          string
	Container    string
	TailLines    int
	SinceSeconds int
	LimitBytes   int
	Previous     bool
	Timestamps   bool
}

type ListResult struct {
	Items      []map[string]any `json:"items"`
	ItemCount  int              `json:"item_count"`
	TotalCount int              `json:"total_count"`
	Truncated  bool             `json:"truncated"`
}

type PodLog struct {
	Namespace    string `json:"namespace"`
	Pod          string `json:"pod"`
	Container    string `json:"container"`
	Previous     bool   `json:"previous"`
	TailLines    int    `json:"tail_lines"`
	SinceSeconds int    `json:"since_seconds"`
	LimitBytes   int    `json:"limit_bytes"`
	Truncated    bool   `json:"truncated"`
	Text         string `json:"text"`
}

type JobRequest struct {
	Namespace string
	Job       map[string]any
}

func NewClient() *Client {
	tokenPath := env("OPSPILOT_SERVICEACCOUNT_TOKEN", "/var/run/secrets/kubernetes.io/serviceaccount/token")
	caPath := env("OPSPILOT_SERVICEACCOUNT_CA", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := env("KUBERNETES_SERVICE_PORT", "443")
	mode := "kubectl"
	if host != "" && fileExists(tokenPath) {
		mode = "in-cluster"
	}
	return &Client{
		kubectl:   env("OPSPILOT_KUBECTL", "kubectl"),
		mode:      mode,
		host:      host,
		port:      port,
		tokenPath: tokenPath,
		caPath:    caPath,
		http:      &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) Health() map[string]any {
	out := map[string]any{
		"mode": c.mode,
	}
	if c.mode == "kubectl" {
		out["kubectl"] = c.kubectl
	} else {
		out["in_cluster_host"] = c.host
	}
	return out
}

func (c *Client) InventoryOverview(ctx context.Context, limit int) map[string]any {
	warnings := []string{}
	counts := map[string]any{}
	checks := []struct {
		name string
		path string
		args []string
	}{
		{"namespace_count", "/api/v1/namespaces", []string{"get", "namespaces", "-o", "json"}},
		{"node_count", "/api/v1/nodes", []string{"get", "nodes", "-o", "json"}},
		{"pod_count", "/api/v1/pods", []string{"get", "pods", "-A", "-o", "json"}},
		{"service_count", "/api/v1/services", []string{"get", "services", "-A", "-o", "json"}},
		{"deployment_count", "/apis/apps/v1/deployments", []string{"get", "deployments", "-A", "-o", "json"}},
		{"statefulset_count", "/apis/apps/v1/statefulsets", []string{"get", "statefulsets", "-A", "-o", "json"}},
		{"daemonset_count", "/apis/apps/v1/daemonsets", []string{"get", "daemonsets", "-A", "-o", "json"}},
	}
	for _, check := range checks {
		payload, err := c.json(ctx, check.path, check.args)
		if err != nil {
			counts[check.name] = nil
			warnings = append(warnings, fmt.Sprintf("%s: %v", check.name, err))
			continue
		}
		counts[check.name] = len(items(payload))
	}
	abnormal, err := c.ListPods(ctx, "", "abnormal", "", limit)
	if err != nil {
		counts["abnormal_pod_count"] = nil
		warnings = append(warnings, fmt.Sprintf("abnormal_pods: %v", err))
		abnormal = ListResult{}
	} else {
		counts["abnormal_pod_count"] = abnormal.TotalCount
	}
	return map[string]any{
		"clusters":          []any{},
		"counts":            counts,
		"top_abnormal_pods": abnormal.Items,
		"warnings":          warnings,
	}
}

func (c *Client) ListPods(ctx context.Context, namespace, status, q string, limit int) (ListResult, error) {
	path := "/api/v1/pods"
	args := []string{"get", "pods", "-A", "-o", "json"}
	if namespace != "" {
		path = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods"
		args = []string{"get", "pods", "-n", namespace, "-o", "json"}
	}
	payload, err := c.json(ctx, path, args)
	if err != nil {
		return ListResult{}, err
	}
	pods := []map[string]any{}
	for _, item := range items(payload) {
		summary := PodSummary(item)
		if status != "" && !MatchesStatus(summary, status) {
			continue
		}
		if q != "" && !matchesQuery(summary, q) {
			continue
		}
		pods = append(pods, summary)
	}
	if limit < 0 {
		limit = 0
	}
	total := len(pods)
	if limit == 0 || limit > total {
		limit = total
	}
	return ListResult{
		Items:      pods[:limit],
		ItemCount:  limit,
		TotalCount: total,
		Truncated:  total > limit,
	}, nil
}

func (c *Client) GetPod(ctx context.Context, namespace, pod string) (map[string]any, error) {
	if namespace == "" || pod == "" {
		return nil, errors.New("namespace and pod are required")
	}
	path := "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods/" + url.PathEscape(pod)
	return c.json(ctx, path, []string{"get", "pod", pod, "-n", namespace, "-o", "json"})
}

func (c *Client) DeploymentStatus(ctx context.Context, namespace, name string) (map[string]any, error) {
	if namespace == "" || name == "" {
		return nil, errors.New("namespace and deployment are required")
	}
	path := "/apis/apps/v1/namespaces/" + url.PathEscape(namespace) + "/deployments/" + url.PathEscape(name)
	raw, err := c.json(ctx, path, []string{"get", "deployment", name, "-n", namespace, "-o", "json"})
	if err != nil {
		return nil, err
	}
	meta := object(raw, "metadata")
	spec := object(raw, "spec")
	status := object(raw, "status")
	templateSpec := object(object(spec, "template"), "spec")
	selector := object(object(spec, "selector"), "matchLabels")
	containers := []any{}
	for _, rawContainer := range array(templateSpec, "containers") {
		container := asMap(rawContainer)
		containers = append(containers, map[string]any{
			"name":  stringValue(container, "name"),
			"image": stringValue(container, "image"),
		})
	}
	conditions := []any{}
	for _, rawCond := range array(status, "conditions") {
		cond := asMap(rawCond)
		conditions = append(conditions, map[string]any{
			"type":    stringValue(cond, "type"),
			"status":  stringValue(cond, "status"),
			"reason":  stringValue(cond, "reason"),
			"message": stringValue(cond, "message"),
		})
	}
	return map[string]any{
		"namespace":             stringValue(meta, "namespace"),
		"name":                  stringValue(meta, "name"),
		"generation":            intValue(meta, "generation"),
		"observed_generation":   intValue(status, "observedGeneration"),
		"replicas":              intValue(status, "replicas"),
		"updated_replicas":      intValue(status, "updatedReplicas"),
		"ready_replicas":        intValue(status, "readyReplicas"),
		"available_replicas":    intValue(status, "availableReplicas"),
		"unavailable_replicas":  intValue(status, "unavailableReplicas"),
		"desired_replicas":      intValue(spec, "replicas"),
		"selector_match_labels": selector,
		"containers":            containers,
		"conditions":            conditions,
		"labels":                object(meta, "labels"),
	}, nil
}

func (c *Client) FindDeploymentByName(ctx context.Context, name string) (map[string]any, error) {
	if name == "" {
		return nil, errors.New("deployment name is required")
	}
	raw, err := c.json(ctx, "/apis/apps/v1/deployments", []string{"get", "deployments", "-A", "-o", "json"})
	if err != nil {
		return nil, err
	}
	for _, item := range items(raw) {
		meta := object(item, "metadata")
		if stringValue(meta, "name") != name {
			continue
		}
		namespace := stringValue(meta, "namespace")
		return c.DeploymentStatus(ctx, namespace, name)
	}
	return nil, fmt.Errorf("deployment not found: %s", name)
}

func (c *Client) ArgoApplicationStatus(ctx context.Context, namespace, name string) (map[string]any, error) {
	if namespace == "" || name == "" {
		return nil, errors.New("namespace and application are required")
	}
	path := "/apis/argoproj.io/v1alpha1/namespaces/" + url.PathEscape(namespace) + "/applications/" + url.PathEscape(name)
	raw, err := c.json(ctx, path, []string{"get", "application", name, "-n", namespace, "-o", "json"})
	if err != nil {
		return nil, err
	}
	status := object(raw, "status")
	sync := object(status, "sync")
	health := object(status, "health")
	operation := object(status, "operationState")
	return map[string]any{
		"app":             name,
		"namespace":       namespace,
		"sync_status":     stringValue(sync, "status"),
		"health_status":   stringValue(health, "status"),
		"revision":        stringValue(sync, "revision"),
		"operation_phase": stringValue(operation, "phase"),
		"message":         stringValue(operation, "message"),
	}, nil
}

func (c *Client) ListPodsByLabels(ctx context.Context, namespace string, labels map[string]any, limit int) (ListResult, error) {
	result, err := c.ListPods(ctx, namespace, "", "", 0)
	if err != nil {
		return ListResult{}, err
	}
	filtered := []map[string]any{}
	for _, pod := range result.Items {
		podLabels, _ := pod["labels"].(map[string]any)
		if labelsMatch(podLabels, labels) {
			filtered = append(filtered, pod)
		}
	}
	total := len(filtered)
	if limit < 0 {
		limit = 0
	}
	if limit == 0 || limit > total {
		limit = total
	}
	return ListResult{Items: filtered[:limit], ItemCount: limit, TotalCount: total, Truncated: total > limit}, nil
}

func (c *Client) ListEvents(ctx context.Context, namespace, involvedName string, limit int) (ListResult, error) {
	if namespace == "" {
		return ListResult{}, errors.New("namespace is required")
	}
	path := "/api/v1/namespaces/" + url.PathEscape(namespace) + "/events"
	args := []string{"get", "events", "-n", namespace, "-o", "json"}
	if involvedName != "" {
		selector := "involvedObject.name=" + involvedName
		path += "?" + url.Values{"fieldSelector": []string{selector}}.Encode()
		args = []string{"get", "events", "-n", namespace, "--field-selector", selector, "-o", "json"}
	}
	payload, err := c.json(ctx, path, args)
	if err != nil {
		return ListResult{}, err
	}
	events := []map[string]any{}
	for _, item := range items(payload) {
		events = append(events, EventSummary(item))
	}
	total := len(events)
	if limit < 0 {
		limit = 0
	}
	if limit == 0 || limit > total {
		limit = total
	}
	return ListResult{Items: events[:limit], ItemCount: limit, TotalCount: total, Truncated: total > limit}, nil
}

func (c *Client) CreateJob(ctx context.Context, namespace string, job map[string]any) (map[string]any, error) {
	if namespace == "" {
		return nil, errors.New("namespace is required")
	}
	if job == nil {
		return nil, errors.New("job is required")
	}
	if c.mode == "in-cluster" {
		path := "/apis/batch/v1/namespaces/" + url.PathEscape(namespace) + "/jobs"
		return c.postJSON(ctx, path, job)
	}
	body, err := json.Marshal(job)
	if err != nil {
		return nil, err
	}
	text, err := c.kubectlTextInput(ctx, []string{"create", "-f", "-", "-o", "json"}, string(body))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetJob(ctx context.Context, namespace, name string) (map[string]any, error) {
	if namespace == "" || name == "" {
		return nil, errors.New("namespace and job are required")
	}
	path := "/apis/batch/v1/namespaces/" + url.PathEscape(namespace) + "/jobs/" + url.PathEscape(name)
	return c.json(ctx, path, []string{"get", "job", name, "-n", namespace, "-o", "json"})
}

func (c *Client) ListJobsByLabels(ctx context.Context, namespace string, labels map[string]string, limit int) (ListResult, error) {
	if namespace == "" {
		return ListResult{}, errors.New("namespace is required")
	}
	selector := labelSelector(labels)
	path := "/apis/batch/v1/namespaces/" + url.PathEscape(namespace) + "/jobs"
	args := []string{"get", "jobs", "-n", namespace, "-o", "json"}
	if selector != "" {
		path += "?" + url.Values{"labelSelector": []string{selector}}.Encode()
		args = []string{"get", "jobs", "-n", namespace, "-l", selector, "-o", "json"}
	}
	payload, err := c.json(ctx, path, args)
	if err != nil {
		return ListResult{}, err
	}
	jobs := []map[string]any{}
	for _, item := range items(payload) {
		jobs = append(jobs, JobSummary(item))
	}
	sort.SliceStable(jobs, func(i, j int) bool {
		return fmt.Sprint(jobs[i]["created_at"]) > fmt.Sprint(jobs[j]["created_at"])
	})
	if limit < 0 {
		limit = 0
	}
	total := len(jobs)
	if limit == 0 || limit > total {
		limit = total
	}
	return ListResult{Items: jobs[:limit], ItemCount: limit, TotalCount: total, Truncated: total > limit}, nil
}

func (c *Client) ReadJobLog(ctx context.Context, namespace, jobName string, limitBytes int) (PodLog, error) {
	if namespace == "" || jobName == "" {
		return PodLog{}, errors.New("namespace and job are required")
	}
	pods, err := c.ListPodsByLabels(ctx, namespace, map[string]any{"job-name": jobName}, 1)
	if err != nil {
		return PodLog{}, err
	}
	if len(pods.Items) == 0 {
		return PodLog{}, errors.New("job pod not found")
	}
	podName := fmt.Sprint(pods.Items[0]["name"])
	return c.ReadPodLog(ctx, LogRequest{
		Namespace:  namespace,
		Pod:        podName,
		TailLines:  MaxTailLines,
		LimitBytes: limitBytes,
	})
}

func (c *Client) ReadPodLog(ctx context.Context, req LogRequest) (PodLog, error) {
	req = BoundedLogRequest(req)
	if req.Namespace == "" || req.Pod == "" {
		return PodLog{}, errors.New("namespace and pod are required")
	}
	if req.Container == "" {
		if rawPod, err := c.GetPod(ctx, req.Namespace, req.Pod); err == nil {
			req.Container = firstContainerName(PodSummary(rawPod))
		}
	}
	var text string
	var err error
	if c.mode == "in-cluster" {
		params := url.Values{}
		params.Set("tailLines", strconv.Itoa(req.TailLines))
		params.Set("sinceSeconds", strconv.Itoa(req.SinceSeconds))
		params.Set("limitBytes", strconv.Itoa(req.LimitBytes))
		params.Set("previous", strconv.FormatBool(req.Previous))
		params.Set("timestamps", strconv.FormatBool(req.Timestamps))
		if req.Container != "" {
			params.Set("container", req.Container)
		}
		path := "/api/v1/namespaces/" + url.PathEscape(req.Namespace) + "/pods/" + url.PathEscape(req.Pod) + "/log?" + params.Encode()
		text, err = c.raw(ctx, path)
	} else {
		args := []string{
			"logs", "-n", req.Namespace, req.Pod,
			"--tail=" + strconv.Itoa(req.TailLines),
			"--since=" + strconv.Itoa(req.SinceSeconds) + "s",
			"--limit-bytes=" + strconv.Itoa(req.LimitBytes),
		}
		if req.Container != "" {
			args = append(args, "-c", req.Container)
		}
		if req.Previous {
			args = append(args, "--previous")
		}
		if req.Timestamps {
			args = append(args, "--timestamps")
		}
		text, err = c.kubectlText(ctx, args)
	}
	if err != nil {
		return PodLog{}, err
	}
	truncated := false
	if len([]byte(text)) > req.LimitBytes {
		text = string([]byte(text)[:req.LimitBytes])
		truncated = true
	}
	return PodLog{
		Namespace:    req.Namespace,
		Pod:          req.Pod,
		Container:    req.Container,
		Previous:     req.Previous,
		TailLines:    req.TailLines,
		SinceSeconds: req.SinceSeconds,
		LimitBytes:   req.LimitBytes,
		Truncated:    truncated,
		Text:         text,
	}, nil
}

func (c *Client) PodContext(ctx context.Context, namespace, pod string) (map[string]any, error) {
	rawPod, err := c.GetPod(ctx, namespace, pod)
	if err != nil {
		return nil, err
	}
	warnings := []string{}
	summary := PodSummary(rawPod)
	eventList, err := c.ListEvents(ctx, namespace, pod, 20)
	if err != nil {
		warnings = append(warnings, "events: "+err.Error())
		eventList = ListResult{Items: []map[string]any{}}
	}
	container := firstContainerName(summary)
	logs := []any{}
	for _, previous := range logModes(summary) {
		log, err := c.ReadPodLog(ctx, LogRequest{
			Namespace:    namespace,
			Pod:          pod,
			Container:    container,
			TailLines:    120,
			SinceSeconds: DefaultSinceSeconds,
			LimitBytes:   256 * 1024,
			Previous:     previous,
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("logs previous=%v: %v", previous, err))
			continue
		}
		logs = append(logs, log)
	}
	return map[string]any{
		"target": map[string]any{
			"type":      "pod",
			"namespace": namespace,
			"name":      pod,
			"cluster":   env("OPSPILOT_CLUSTER", "default"),
		},
		"summary": summary,
		"evidence": map[string]any{
			"inventory": map[string]any{"pod": summary},
			"metrics":   []any{},
			"events":    eventList.Items,
			"logs":      logs,
			"release":   map[string]any{},
		},
		"warnings": warnings,
	}, nil
}

func (c *Client) DiagnosePod(ctx context.Context, namespace, pod string) (map[string]any, error) {
	contextPack, err := c.PodContext(ctx, namespace, pod)
	if err != nil {
		return nil, err
	}
	summary, _ := contextPack["summary"].(map[string]any)
	findings := []string{}
	if reasons, ok := summary["waiting_reasons"].([]any); ok && len(reasons) > 0 {
		parts := []string{}
		for _, reason := range reasons {
			parts = append(parts, fmt.Sprint(reason))
		}
		findings = append(findings, "Pod has waiting containers: "+strings.Join(parts, ", "))
	}
	if restarts, ok := summary["restart_count"].(int); ok && restarts > 0 {
		findings = append(findings, fmt.Sprintf("Pod has container restarts: %d", restarts))
	}
	if ready, _ := summary["ready"].(bool); !ready {
		findings = append(findings, "Pod is not ready")
	}
	confidence := "medium"
	if len(findings) == 0 {
		findings = append(findings, "No obvious pod-level failure signal found in MVP evidence")
		confidence = "low"
	}
	contextPack["diagnosis"] = map[string]any{
		"findings":   findings,
		"confidence": confidence,
		"next_steps": []string{
			"Review Kubernetes events",
			"Review current and previous short-window pod logs",
			"Review Prometheus pod CPU, memory, and restart metrics when configured",
		},
	}
	return contextPack, nil
}

func (c *Client) json(ctx context.Context, path string, kubectlArgs []string) (map[string]any, error) {
	var text string
	var err error
	if c.mode == "in-cluster" {
		text, err = c.raw(ctx, path)
	} else {
		text, err = c.kubectlText(ctx, kubectlArgs)
	}
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) raw(ctx context.Context, path string) (string, error) {
	tokenBytes, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return "", err
	}
	endpoint := "https://" + c.host + ":" + c.port + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tokenBytes)))
	client := c.http
	if fileExists(c.caPath) {
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		ca, err := os.ReadFile(c.caPath)
		if err == nil {
			pool.AppendCertsFromPEM(ca)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		client = &http.Client{Timeout: 20 * time.Second, Transport: transport}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("kubernetes api %d: %s", resp.StatusCode, string(bytes.TrimSpace(body)))
	}
	return string(body), nil
}

func (c *Client) postJSON(ctx context.Context, path string, payload any) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	tokenBytes, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return nil, err
	}
	endpoint := "https://" + c.host + ":" + c.port + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tokenBytes)))
	req.Header.Set("Content-Type", "application/json")
	client := c.http
	if fileExists(c.caPath) {
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		ca, err := os.ReadFile(c.caPath)
		if err == nil {
			pool.AppendCertsFromPEM(ca)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		client = &http.Client{Timeout: 20 * time.Second, Transport: transport}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kubernetes api %d: %s", resp.StatusCode, string(bytes.TrimSpace(respBody)))
	}
	var out map[string]any
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) kubectlText(ctx context.Context, args []string) (string, error) {
	return c.kubectlTextInput(ctx, args, "")
}

func (c *Client) kubectlTextInput(ctx context.Context, args []string, stdin string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.kubectl, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("kubectl timeout: %w", ctx.Err())
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("kubectl failed: %s", msg)
	}
	return stdout.String(), nil
}

func labelSelector(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, key := range keys {
		value := strings.TrimSpace(labels[key])
		if key == "" || value == "" {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, ",")
}

func items(payload map[string]any) []map[string]any {
	raw, _ := payload["items"].([]any)
	out := []map[string]any{}
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
