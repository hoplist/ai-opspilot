package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	Enabled  bool        `json:"enabled"`
	Optional bool        `json:"optional"`
	BaseURL  string      `json:"base_url,omitempty"`
	Smoke    SmokeConfig `json:"smoke"`
}

type SmokeConfig struct {
	TimeoutSeconds int        `json:"timeout_seconds"`
	LatencyP95Ms   int        `json:"latency_p95_ms"`
	Endpoints      []Endpoint `json:"endpoints"`
}

type Endpoint struct {
	Name         string `json:"name"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	ExpectStatus int    `json:"expect_status"`
}

type Report struct {
	Status        string        `json:"status"`
	Optional      bool          `json:"optional"`
	BaseURL       string        `json:"base_url,omitempty"`
	StartedAt     string        `json:"started_at"`
	FinishedAt    string        `json:"finished_at"`
	DurationMs    int64         `json:"duration_ms"`
	CheckCount    int           `json:"check_count"`
	PassedCount   int           `json:"passed_count"`
	FailedCount   int           `json:"failed_count"`
	SkippedReason string        `json:"skipped_reason,omitempty"`
	Summary       string        `json:"summary"`
	Checks        []CheckResult `json:"checks,omitempty"`
}

type CheckResult struct {
	Name            string `json:"name"`
	Method          string `json:"method"`
	URL             string `json:"url"`
	ExpectStatus    int    `json:"expect_status"`
	StatusCode      int    `json:"status_code,omitempty"`
	DurationMs      int64  `json:"duration_ms"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	LatencyExceeded bool   `json:"latency_exceeded,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:  true,
		Optional: true,
		Smoke: SmokeConfig{
			TimeoutSeconds: 3,
			LatencyP95Ms:   1000,
		},
	}
}

func ParseJSON(raw string) (Config, error) {
	cfg := DefaultConfig()
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return Config{}, err
	}
	cfg.defaults()
	return cfg, nil
}

func ParseYAML(raw string) (Config, error) {
	cfg := DefaultConfig()
	current := ""
	inEndpoints := false
	currentEndpoint := -1
	for _, line := range strings.Split(raw, "\n") {
		line = stripComment(line)
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := leadingSpaces(line)
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			key := strings.TrimSuffix(trimmed, ":")
			switch {
			case indent == 0:
				current = key
				inEndpoints = false
			case current == "quality" && key == "smoke":
				current = "smoke"
				inEndpoints = false
			case current == "smoke" && key == "endpoints":
				inEndpoints = true
			}
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && inEndpoints {
			cfg.Smoke.Endpoints = append(cfg.Smoke.Endpoints, Endpoint{Method: "GET", ExpectStatus: 200})
			currentEndpoint = len(cfg.Smoke.Endpoints) - 1
			applyEndpointPair(&cfg.Smoke.Endpoints[currentEndpoint], strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = cleanValue(value)
		switch {
		case inEndpoints && currentEndpoint >= 0:
			applyEndpointValue(&cfg.Smoke.Endpoints[currentEndpoint], key, value)
		case current == "quality":
			switch key {
			case "enabled":
				cfg.Enabled = parseBool(value, cfg.Enabled)
			case "optional":
				cfg.Optional = parseBool(value, cfg.Optional)
			case "baseURL", "baseUrl", "base_url":
				cfg.BaseURL = value
			}
		case current == "smoke":
			switch key {
			case "timeoutSeconds", "timeout_seconds":
				cfg.Smoke.TimeoutSeconds = parseInt(value, cfg.Smoke.TimeoutSeconds)
			case "latencyP95Ms", "latency_p95_ms":
				cfg.Smoke.LatencyP95Ms = parseInt(value, cfg.Smoke.LatencyP95Ms)
			}
		}
	}
	cfg.defaults()
	return cfg, nil
}

func Run(ctx context.Context, cfg Config, baseURL string, client *http.Client) (report Report) {
	cfg.defaults()
	if baseURL == "" {
		baseURL = cfg.BaseURL
	}
	start := time.Now()
	report = Report{
		Status:     "running",
		Optional:   cfg.Optional,
		BaseURL:    baseURL,
		StartedAt:  start.UTC().Format(time.RFC3339),
		FinishedAt: start.UTC().Format(time.RFC3339),
	}
	defer func() {
		report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		report.DurationMs = time.Since(start).Milliseconds()
	}()
	if !cfg.Enabled {
		report.Status = "skipped"
		report.SkippedReason = "quality_config_disabled"
		report.Summary = "Quality checks are disabled for this service."
		return report
	}
	if baseURL == "" {
		report.Status = "skipped"
		report.SkippedReason = "quality_base_url_missing"
		report.Summary = "Quality checks were skipped because baseURL is empty."
		return report
	}
	if len(cfg.Smoke.Endpoints) == 0 {
		report.Status = "skipped"
		report.SkippedReason = "quality_endpoints_missing"
		report.Summary = "Quality checks were skipped because no endpoints are configured."
		return report
	}
	if client == nil {
		client = &http.Client{}
	}
	for _, endpoint := range cfg.Smoke.Endpoints {
		check := runEndpoint(ctx, client, cfg, baseURL, endpoint)
		report.Checks = append(report.Checks, check)
		report.CheckCount++
		if check.Status == "pass" {
			report.PassedCount++
		} else {
			report.FailedCount++
		}
	}
	if report.FailedCount > 0 {
		report.Status = "failed"
		report.Summary = fmt.Sprintf("%d/%d quality checks failed.", report.FailedCount, report.CheckCount)
		return report
	}
	report.Status = "passed"
	report.Summary = fmt.Sprintf("%d/%d quality checks passed.", report.PassedCount, report.CheckCount)
	return report
}

func WriteReport(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func runEndpoint(ctx context.Context, client *http.Client, cfg Config, baseURL string, endpoint Endpoint) CheckResult {
	endpoint.defaults()
	target := endpointURL(baseURL, endpoint.Path)
	check := CheckResult{
		Name:         endpoint.Name,
		Method:       endpoint.Method,
		URL:          target,
		ExpectStatus: endpoint.ExpectStatus,
		Status:       "fail",
	}
	timeout := time.Duration(cfg.Smoke.TimeoutSeconds) * time.Second
	reqCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, endpoint.Method, target, nil)
	if err != nil {
		check.Error = err.Error()
		return check
	}
	start := time.Now()
	resp, err := client.Do(req)
	check.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		check.Error = err.Error()
		return check
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	check.StatusCode = resp.StatusCode
	if resp.StatusCode != endpoint.ExpectStatus {
		check.Error = fmt.Sprintf("unexpected status %d", resp.StatusCode)
		return check
	}
	if cfg.Smoke.LatencyP95Ms > 0 && check.DurationMs > int64(cfg.Smoke.LatencyP95Ms) {
		check.LatencyExceeded = true
		check.Error = fmt.Sprintf("duration %dms exceeded threshold %dms", check.DurationMs, cfg.Smoke.LatencyP95Ms)
		return check
	}
	check.Status = "pass"
	return check
}

func (c *Config) defaults() {
	if c.Smoke.TimeoutSeconds <= 0 {
		c.Smoke.TimeoutSeconds = 3
	}
	if c.Smoke.LatencyP95Ms < 0 {
		c.Smoke.LatencyP95Ms = 0
	}
	for i := range c.Smoke.Endpoints {
		c.Smoke.Endpoints[i].defaults()
	}
}

func (e *Endpoint) defaults() {
	if e.Name == "" {
		e.Name = strings.TrimPrefix(e.Path, "/")
	}
	if e.Name == "" {
		e.Name = "endpoint"
	}
	if e.Method == "" {
		e.Method = "GET"
	}
	e.Method = strings.ToUpper(e.Method)
	if e.ExpectStatus == 0 {
		e.ExpectStatus = 200
	}
}

func endpointURL(baseURL, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if path == "" {
		return baseURL
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path
}

func applyEndpointPair(endpoint *Endpoint, raw string) {
	key, value, ok := strings.Cut(raw, ":")
	if !ok {
		return
	}
	applyEndpointValue(endpoint, strings.TrimSpace(key), cleanValue(value))
}

func applyEndpointValue(endpoint *Endpoint, key, value string) {
	switch key {
	case "name":
		endpoint.Name = value
	case "method":
		endpoint.Method = strings.ToUpper(value)
	case "path":
		endpoint.Path = value
	case "expectStatus", "expect_status":
		endpoint.ExpectStatus = parseInt(value, endpoint.ExpectStatus)
	}
}

func stripComment(line string) string {
	if idx := strings.Index(line, "#"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return value
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func parseBool(raw string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "yes", "on", "1":
		return true
	case "false", "no", "off", "0":
		return false
	default:
		return fallback
	}
}

func parseInt(raw string, fallback int) int {
	var value int
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &value); err != nil {
		return fallback
	}
	return value
}
