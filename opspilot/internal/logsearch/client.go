package logsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL     string
	index       string
	correlation CorrelationConfig
	username    string
	password    string
	http        *http.Client
}

type SearchRequest struct {
	Namespace string
	Pod       string
	Container string
	Query     string
	Limit     int
}

func NewClient(baseURL, index string) *Client {
	return NewClientWithConfig(baseURL, index, CorrelationConfig{})
}

func NewClientWithConfig(baseURL, index string, correlation CorrelationConfig) *Client {
	return NewClientWithConfigAndAuth(baseURL, index, correlation, "", "")
}

func NewClientWithConfigAndAuth(baseURL, index string, correlation CorrelationConfig, username, password string) *Client {
	if index == "" {
		index = "opspilot-k8s-*"
	}
	correlation = correlation.withDefaults()
	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		index:       index,
		correlation: correlation,
		username:    username,
		password:    password,
		http:        &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) Configured() bool {
	return c != nil && c.baseURL != ""
}

func (c *Client) APISIXConfigured() bool {
	return c != nil && c.Configured() && !c.correlation.DisableAPISIX && c.correlation.APISIXIndex != ""
}

func (c *Client) ServiceLogsConfigured() bool {
	return c != nil && c.Configured() && c.correlation.ServiceIndex != ""
}

func (c *Client) Health(ctx context.Context) map[string]any {
	out := map[string]any{
		"configured": c.Configured(),
		"url":        c.baseURL,
		"index":      c.index,
		"correlation": map[string]any{
			"apisix_configured":       c.APISIXConfigured(),
			"apisix_disabled":         c.correlation.DisableAPISIX,
			"apisix_index":            c.correlation.APISIXIndex,
			"service_logs_configured": c.ServiceLogsConfigured(),
			"service_index":           c.correlation.ServiceIndex,
			"service_uri_field":       c.correlation.ServiceURIField,
			"routes":                  len(c.correlation.Routes),
		},
		"ready": false,
	}
	if !c.Configured() {
		return out
	}
	status, err := c.getStatus(ctx, "/_cluster/health")
	out["status_code"] = status
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	out["ready"] = status >= 200 && status < 300
	return out
}

func (c *Client) Search(ctx context.Context, req SearchRequest) (map[string]any, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("log search is not configured")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	filters := []any{}
	if req.Namespace != "" {
		filters = append(filters, matchPhrase("kubernetes.namespace_name", req.Namespace))
	}
	if req.Pod != "" {
		filters = append(filters, matchPhrase("kubernetes.pod_name", req.Pod))
	}
	if req.Container != "" {
		filters = append(filters, matchPhrase("kubernetes.container_name", req.Container))
	}
	must := []any{}
	if req.Query != "" {
		must = append(must, map[string]any{
			"query_string": map[string]any{
				"query":            req.Query,
				"default_field":    "log",
				"analyze_wildcard": true,
			},
		})
	}
	body := map[string]any{
		"size": limit,
		"sort": []any{map[string]any{
			"@timestamp": map[string]any{"order": "desc"},
		}},
		"query": map[string]any{
			"bool": map[string]any{
				"filter": filters,
				"must":   must,
			},
		},
	}
	payload, err := c.postJSON(ctx, "/"+c.index+"/_search", body)
	if err != nil {
		return nil, err
	}
	hits := mapValue(payload["hits"])
	items := []map[string]any{}
	for _, raw := range anySlice(hits["hits"]) {
		hit := mapValue(raw)
		source := mapValue(hit["_source"])
		kubernetes := mapValue(source["kubernetes"])
		items = append(items, map[string]any{
			"timestamp": source["@timestamp"],
			"namespace": kubernetes["namespace_name"],
			"pod":       kubernetes["pod_name"],
			"container": kubernetes["container_name"],
			"host":      kubernetes["host"],
			"stream":    source["stream"],
			"log":       source["log"],
			"index":     hit["_index"],
		})
	}
	total := mapValue(hits["total"])
	return map[string]any{
		"items":        items,
		"item_count":   len(items),
		"total":        total["value"],
		"index":        c.index,
		"query":        req.Query,
		"namespace":    req.Namespace,
		"pod":          req.Pod,
		"container":    req.Container,
		"result_order": "timestamp_desc",
	}, nil
}

func matchPhrase(field, value string) map[string]any {
	return map[string]any{"match_phrase": map[string]any{field: value}}
}

func (c *Client) getStatus(ctx context.Context, path string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, err
	}
	c.setAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func (c *Client) postJSON(ctx context.Context, path string, body any) (map[string]any, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("log search returned %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) setAuth(req *http.Request) {
	if c != nil && c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
}

func mapValue(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func anySlice(value any) []any {
	if slice, ok := value.([]any); ok {
		return slice
	}
	return []any{}
}
