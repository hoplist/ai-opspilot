package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return NewClientWithToken(baseURL, "")
}

func NewClientWithToken(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) Health(ctx context.Context) map[string]any {
	out := map[string]any{
		"configured": true,
		"url":        c.baseURL,
		"ready":      false,
	}
	body, status, err := c.getRaw(ctx, "/health", nil)
	out["status_code"] = status
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		out["agent"] = payload
	}
	out["ready"] = status >= 200 && status < 300
	return out
}

func (c *Client) Containers(ctx context.Context) (map[string]any, error) {
	return c.getJSON(ctx, "/api/containers", nil)
}

func (c *Client) Inspect(ctx context.Context, container string) (map[string]any, error) {
	return c.getJSON(ctx, "/api/containers/"+url.PathEscape(container)+"/inspect", nil)
}

func (c *Client) Logs(ctx context.Context, req LogRequest) (ContainerLog, error) {
	values := url.Values{
		"tail":          []string{strconv.Itoa(req.TailLines)},
		"since_seconds": []string{strconv.Itoa(req.SinceSeconds)},
		"limit_bytes":   []string{strconv.Itoa(req.LimitBytes)},
		"timestamps":    []string{strconv.FormatBool(req.Timestamps)},
	}
	result, err := c.getJSON(ctx, "/api/containers/"+url.PathEscape(req.Container)+"/logs", values)
	if err != nil {
		return ContainerLog{}, err
	}
	body, _ := json.Marshal(result)
	var log ContainerLog
	if err := json.Unmarshal(body, &log); err != nil {
		return ContainerLog{}, err
	}
	return log, nil
}

func (c *Client) Stats(ctx context.Context, container string) (map[string]any, error) {
	return c.getJSON(ctx, "/api/containers/"+url.PathEscape(container)+"/stats", nil)
}

func (c *Client) getJSON(ctx context.Context, path string, values url.Values) (map[string]any, error) {
	body, status, err := c.getRaw(ctx, path, values)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("node agent returned %d: %s", status, strings.TrimSpace(string(body)))
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) getRaw(ctx context.Context, path string, values url.Values) ([]byte, int, error) {
	target := c.baseURL + path
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, resp.Body)
	return buf.Bytes(), resp.StatusCode, nil
}
