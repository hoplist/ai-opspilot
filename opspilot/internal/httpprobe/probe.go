package httpprobe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultTimeoutSeconds = 10
	DefaultBodyLimitBytes = 16 * 1024
	MaxTimeoutSeconds     = 30
	MaxBodyLimitBytes     = 64 * 1024
	UserAgentPrefix       = "OpsPilot-Probe"
	ProbeHeaderName       = "X-OpsPilot-Probe-Id"
)

type Request struct {
	Method          string
	URL             string
	Headers         map[string]string
	Body            string
	ProbeID         string
	TimeoutSeconds  int
	BodyLimitBytes  int
	IncludeResponse bool
}

type Result struct {
	ProbeID         string            `json:"probe_id"`
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	Host            string            `json:"host"`
	Path            string            `json:"path"`
	UserAgent       string            `json:"user_agent"`
	RequestHeaders  map[string]string `json:"request_headers,omitempty"`
	StatusCode      int               `json:"status_code,omitempty"`
	Status          string            `json:"status,omitempty"`
	DurationMs      int64             `json:"duration_ms"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	BodyPreview     string            `json:"body_preview,omitempty"`
	BodyTruncated   bool              `json:"body_truncated,omitempty"`
	StartedAt       string            `json:"started_at"`
	CompletedAt     string            `json:"completed_at"`
	Error           string            `json:"error,omitempty"`
}

func Run(ctx context.Context, req Request) (Result, error) {
	parsed, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil {
		return Result{}, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Result{}, fmt.Errorf("only http and https URLs are supported")
	}
	if parsed.Host == "" {
		return Result{}, fmt.Errorf("url host is required")
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	if !allowedMethod(method) {
		return Result{}, fmt.Errorf("method must be GET, POST, HEAD, or OPTIONS")
	}
	probeID := strings.TrimSpace(req.ProbeID)
	if probeID == "" {
		probeID = newProbeID()
	}
	timeout := clamp(req.TimeoutSeconds, DefaultTimeoutSeconds, MaxTimeoutSeconds)
	limit := clamp(req.BodyLimitBytes, DefaultBodyLimitBytes, MaxBodyLimitBytes)
	userAgent := UserAgentPrefix + "/" + probeID

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	started := time.Now().UTC()
	httpReq, err := http.NewRequestWithContext(ctx, method, parsed.String(), strings.NewReader(req.Body))
	if err != nil {
		return Result{}, err
	}
	for key, value := range req.Headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		httpReq.Header.Set(key, value)
	}
	httpReq.Header.Set(ProbeHeaderName, probeID)
	httpReq.Header.Set("User-Agent", userAgent)

	result := Result{
		ProbeID:        probeID,
		Method:         method,
		URL:            parsed.String(),
		Host:           parsed.Hostname(),
		Path:           pathWithQuery(parsed),
		UserAgent:      userAgent,
		RequestHeaders: redactHeaders(httpReq.Header),
		StartedAt:      started.Format(time.RFC3339Nano),
	}
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Do(httpReq)
	completed := time.Now().UTC()
	result.CompletedAt = completed.Format(time.RFC3339Nano)
	result.DurationMs = completed.Sub(started).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	defer resp.Body.Close()
	result.StatusCode = resp.StatusCode
	result.Status = resp.Status
	result.ResponseHeaders = redactHeaders(resp.Header)
	if req.IncludeResponse && resp.Body != nil {
		body, truncated := readLimited(resp.Body, int64(limit))
		result.BodyPreview = string(body)
		result.BodyTruncated = truncated
	}
	return result, nil
}

func allowedMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func pathWithQuery(parsed *url.URL) string {
	if parsed.RawQuery == "" {
		return parsed.EscapedPath()
	}
	return parsed.EscapedPath() + "?" + parsed.RawQuery
}

func readLimited(reader io.Reader, limit int64) ([]byte, bool) {
	limited := io.LimitReader(reader, limit+1)
	body, _ := io.ReadAll(limited)
	if int64(len(body)) <= limit {
		return body, false
	}
	return body[:limit], true
}

func redactHeaders(headers http.Header) map[string]string {
	out := map[string]string{}
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		if sensitiveHeader(key) {
			out[key] = "[redacted]"
			continue
		}
		out[key] = strings.Join(values, ",")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sensitiveHeader(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "cookie") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "api-key")
}

func clamp(value, fallback, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func newProbeID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "probe-" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("probe-%d", time.Now().UnixNano())
}
