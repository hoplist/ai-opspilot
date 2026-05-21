package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type Sample struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
}

type ListResult struct {
	Items     []map[string]any `json:"items"`
	ItemCount int              `json:"item_count"`
	Source    string           `json:"source,omitempty"`
	Sources   []string         `json:"sources,omitempty"`
	Warnings  []string         `json:"warnings,omitempty"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) Configured() bool {
	return c != nil && c.baseURL != ""
}

func (c *Client) Health(ctx context.Context) map[string]any {
	out := map[string]any{
		"configured": c.Configured(),
	}
	if !c.Configured() {
		return out
	}
	out["url"] = c.baseURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/-/ready", nil)
	if err != nil {
		out["ready"] = false
		out["error"] = err.Error()
		return out
	}
	resp, err := c.http.Do(req)
	if err != nil {
		out["ready"] = false
		out["error"] = err.Error()
		return out
	}
	defer resp.Body.Close()
	out["ready"] = resp.StatusCode >= 200 && resp.StatusCode < 300
	out["status_code"] = resp.StatusCode
	return out
}

func (c *Client) Query(ctx context.Context, query string) ([]Sample, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("prometheus is not configured")
	}
	endpoint := c.baseURL + "/api/v1/query?" + url.Values{"query": []string{query}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("prometheus returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string   `json:"resultType"`
			Result     []Sample `json:"result"`
		} `json:"data"`
		ErrorType string `json:"errorType"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s %s", payload.ErrorType, payload.Error)
	}
	if payload.Data.ResultType != "vector" {
		return nil, fmt.Errorf("unsupported prometheus result type: %s", payload.Data.ResultType)
	}
	return payload.Data.Result, nil
}

func (c *Client) QueryRaw(ctx context.Context, query string) (map[string]any, error) {
	samples, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	for _, sample := range samples {
		items = append(items, map[string]any{
			"metric": sample.Metric,
			"value":  sample.FloatValue(),
			"raw":    sample.Value,
		})
	}
	return map[string]any{
		"query":      query,
		"items":      items,
		"item_count": len(items),
	}, nil
}

func (c *Client) NodeMetrics(ctx context.Context, limit int) (ListResult, error) {
	cpu, err := c.Query(ctx, `100 * (1 - avg by (instance, node, nodename, host) (rate(node_cpu_seconds_total{mode="idle"}[5m])))`)
	if err != nil {
		return ListResult{}, err
	}
	memoryUsed, err := c.Query(ctx, `100 * (1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes))`)
	if err != nil {
		return ListResult{}, err
	}
	memoryTotal, err := c.Query(ctx, `node_memory_MemTotal_bytes`)
	if err != nil {
		return ListResult{}, err
	}
	memoryAvailable, err := c.Query(ctx, `node_memory_MemAvailable_bytes`)
	if err != nil {
		return ListResult{}, err
	}
	fsUsed, _ := c.Query(ctx, `100 * (1 - (node_filesystem_avail_bytes{mountpoint="/",fstype!~"tmpfs|overlay|squashfs"} / node_filesystem_size_bytes{mountpoint="/",fstype!~"tmpfs|overlay|squashfs"}))`)

	nodes := map[string]map[string]any{}
	merge := func(samples []Sample, field string) {
		for _, sample := range samples {
			name := nodeName(sample)
			if name == "" {
				continue
			}
			if _, ok := nodes[name]; !ok {
				nodes[name] = map[string]any{
					"node":     name,
					"instance": sample.Metric["instance"],
				}
			}
			nodes[name][field] = round(sample.FloatValue(), 4)
		}
	}
	merge(cpu, "cpu_used_percent")
	merge(memoryUsed, "memory_used_percent")
	merge(memoryTotal, "memory_total_bytes")
	merge(memoryAvailable, "memory_available_bytes")
	merge(fsUsed, "rootfs_used_percent")

	items := values(nodes)
	sort.Slice(items, func(i, j int) bool {
		return floatField(items[i], "cpu_used_percent") > floatField(items[j], "cpu_used_percent")
	})
	return limited(items, limit), nil
}

func (c *Client) PodMetrics(ctx context.Context, namespace, sortBy string, limit int) (ListResult, error) {
	filter := podFilter(namespace, "")
	cpu, err := c.Query(ctx, `sum by (namespace, pod) (rate(container_cpu_usage_seconds_total`+filter+`[5m]))`)
	if err != nil {
		return ListResult{}, err
	}
	memory, err := c.Query(ctx, `sum by (namespace, pod) (container_memory_working_set_bytes`+filter+`)`)
	if err != nil {
		return ListResult{}, err
	}
	pods := map[string]map[string]any{}
	mergePod := func(samples []Sample, field string) {
		for _, sample := range samples {
			ns := sample.Metric["namespace"]
			pod := sample.Metric["pod"]
			if ns == "" || pod == "" {
				continue
			}
			key := ns + "/" + pod
			if _, ok := pods[key]; !ok {
				pods[key] = map[string]any{"namespace": ns, "pod": pod}
			}
			pods[key][field] = round(sample.FloatValue(), 6)
		}
	}
	mergePod(cpu, "cpu_cores")
	mergePod(memory, "memory_working_set_bytes")

	items := values(pods)
	sortField := "cpu_cores"
	if sortBy == "memory" || sortBy == "mem" {
		sortField = "memory_working_set_bytes"
	}
	sort.Slice(items, func(i, j int) bool {
		return floatField(items[i], sortField) > floatField(items[j], sortField)
	})
	return limited(items, limit), nil
}

func (c *Client) ContainerMetrics(ctx context.Context, sortBy string, limit int) (ListResult, error) {
	filter := `{name!="",image!=""}`
	cpu, err := c.Query(ctx, `sum by (name, image) (rate(container_cpu_usage_seconds_total`+filter+`[5m]))`)
	if err != nil {
		return ListResult{}, err
	}
	memory, err := c.Query(ctx, `sum by (name, image) (container_memory_working_set_bytes`+filter+`)`)
	if err != nil {
		return ListResult{}, err
	}
	containers := map[string]map[string]any{}
	mergeContainer := func(samples []Sample, field string) {
		for _, sample := range samples {
			name := sample.Metric["name"]
			image := sample.Metric["image"]
			if name == "" || image == "" {
				continue
			}
			key := name + "|" + image
			if _, ok := containers[key]; !ok {
				containers[key] = map[string]any{"container": name, "image": image}
			}
			containers[key][field] = round(sample.FloatValue(), 6)
		}
	}
	mergeContainer(cpu, "cpu_cores")
	mergeContainer(memory, "memory_working_set_bytes")

	items := values(containers)
	sortField := "cpu_cores"
	if sortBy == "memory" || sortBy == "mem" {
		sortField = "memory_working_set_bytes"
	}
	sort.Slice(items, func(i, j int) bool {
		return floatField(items[i], sortField) > floatField(items[j], sortField)
	})
	return limited(items, limit), nil
}

func (c *Client) SinglePodMetrics(ctx context.Context, namespace, pod string) (map[string]any, error) {
	if namespace == "" || pod == "" {
		return nil, fmt.Errorf("namespace and pod are required")
	}
	filter := podFilter(namespace, pod)
	cpu, err := c.Query(ctx, `sum by (namespace, pod) (rate(container_cpu_usage_seconds_total`+filter+`[5m]))`)
	if err != nil {
		return nil, err
	}
	memory, err := c.Query(ctx, `sum by (namespace, pod) (container_memory_working_set_bytes`+filter+`)`)
	if err != nil {
		return nil, err
	}
	restarts, _ := c.Query(ctx, `sum by (namespace, pod) (kube_pod_container_status_restarts_total`+filter+`)`)
	out := map[string]any{
		"namespace": namespace,
		"pod":       pod,
		"window":    "5m",
	}
	if len(cpu) > 0 {
		out["cpu_cores"] = round(cpu[0].FloatValue(), 6)
	}
	if len(memory) > 0 {
		out["memory_working_set_bytes"] = round(memory[0].FloatValue(), 2)
	}
	if len(restarts) > 0 {
		out["restart_count"] = round(restarts[0].FloatValue(), 0)
	}
	return out, nil
}

func (s Sample) FloatValue() float64 {
	if len(s.Value) < 2 {
		return 0
	}
	switch value := s.Value[1].(type) {
	case string:
		f, _ := strconv.ParseFloat(value, 64)
		return f
	case float64:
		return value
	default:
		f, _ := strconv.ParseFloat(fmt.Sprint(value), 64)
		return f
	}
}

func nodeName(sample Sample) string {
	if sample.Metric["node"] != "" {
		return sample.Metric["node"]
	}
	if sample.Metric["nodename"] != "" {
		return sample.Metric["nodename"]
	}
	if sample.Metric["host"] != "" {
		return sample.Metric["host"]
	}
	return sample.Metric["instance"]
}

func podFilter(namespace, pod string) string {
	parts := []string{`container!=""`, `pod!=""`}
	if namespace != "" {
		parts = append(parts, `namespace=`+strconv.Quote(namespace))
	}
	if pod != "" {
		parts = append(parts, `pod=`+strconv.Quote(pod))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func values(in map[string]map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, value := range in {
		out = append(out, value)
	}
	return out
}

func limited(items []map[string]any, limit int) ListResult {
	if limit < 0 {
		limit = 0
	}
	if limit == 0 || limit > len(items) {
		limit = len(items)
	}
	return ListResult{Items: items[:limit], ItemCount: limit}
}

func floatField(item map[string]any, key string) float64 {
	switch value := item[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	default:
		return 0
	}
}

func round(value float64, places int) float64 {
	if places <= 0 {
		return math.Round(value)
	}
	scale := math.Pow(10, float64(places))
	return math.Round(value*scale) / scale
}
