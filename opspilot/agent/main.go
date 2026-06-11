package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/version"
)

type config struct {
	host              string
	port              string
	dockerSocket      string
	allowedContainers map[string]bool
	token             string
}

type dockerClient struct {
	socket string
	http   *http.Client
}

func main() {
	cfg := loadConfig()
	if err := validateConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	docker := newDockerClient(cfg.dockerSocket)
	mux := http.NewServeMux()
	registerRoutes(mux, docker, cfg)
	addr := cfg.host + ":" + cfg.port
	fmt.Printf("opspilot-agent %s listening on http://%s\n", version.Version, addr)
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

func validateConfig(cfg config) error {
	if cfg.token != "" || isLocalListenHost(cfg.host) {
		return nil
	}
	return fmt.Errorf("OPSPILOT_AGENT_TOKEN is required when listening on non-local host %q", cfg.host)
}

func isLocalListenHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loadConfig() config {
	host := flag.String("host", env("OPSPILOT_AGENT_HOST", "0.0.0.0"), "listen host")
	port := flag.String("port", env("OPSPILOT_AGENT_PORT", "19080"), "listen port")
	socket := flag.String("docker-socket", env("OPSPILOT_AGENT_DOCKER_SOCKET", "/var/run/docker.sock"), "docker socket path")
	allowed := flag.String("allowed-containers", env("OPSPILOT_AGENT_ALLOWED_CONTAINERS", ""), "comma separated allowed container names or ids")
	token := flag.String("token", env("OPSPILOT_AGENT_TOKEN", ""), "optional bearer token")
	flag.Parse()
	return config{
		host:              *host,
		port:              *port,
		dockerSocket:      *socket,
		allowedContainers: parseAllowList(*allowed),
		token:             *token,
	}
}

func registerRoutes(mux *http.ServeMux, docker *dockerClient, cfg config) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(w, r, cfg) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":                 true,
			"version":            version.Version,
			"docker_socket":      cfg.dockerSocket,
			"allowed_containers": len(cfg.allowedContainers),
		})
	})
	mux.HandleFunc("/api/containers", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(w, r, cfg) || !onlyGET(w, r) {
			return
		}
		if r.URL.Path != "/api/containers" {
			http.NotFound(w, r)
			return
		}
		containers, err := docker.containers(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		items := []any{}
		for _, raw := range containers {
			item := containerSummary(raw)
			if allowedContainer(cfg.allowedContainers, item) {
				items = append(items, item)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "item_count": len(items)})
	})
	mux.HandleFunc("/api/containers/", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(w, r, cfg) || !onlyGET(w, r) {
			return
		}
		container, action, ok := parseContainerAction(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if !allowedName(cfg.allowedContainers, container) {
			writeError(w, http.StatusForbidden, fmt.Errorf("container is not allowed: %s", container))
			return
		}
		switch action {
		case "inspect":
			payload, err := docker.inspect(r.Context(), container)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, inspectSummary(payload))
		case "logs":
			req := nodeagent.BoundedLogRequest(nodeagent.LogRequest{
				Container:    container,
				TailLines:    intQuery(r, "tail", 300),
				SinceSeconds: intQueryAliases(r, []string{"since_seconds", "since"}, 1800),
				LimitBytes:   intQuery(r, "limit_bytes", 1024*1024),
				Timestamps:   boolQuery(r, "timestamps"),
			})
			text, err := docker.logs(r.Context(), req)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			truncated := false
			if len([]byte(text)) > req.LimitBytes {
				text = string([]byte(text)[:req.LimitBytes])
				truncated = true
			}
			writeJSON(w, http.StatusOK, nodeagent.ContainerLog{
				Container:    container,
				TailLines:    req.TailLines,
				SinceSeconds: req.SinceSeconds,
				LimitBytes:   req.LimitBytes,
				Truncated:    truncated,
				Text:         text,
			})
		case "stats":
			payload, err := docker.stats(r.Context(), container)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, statsSummary(payload))
		default:
			http.NotFound(w, r)
		}
	})
}

func newDockerClient(socket string) *dockerClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
	}
	return &dockerClient{
		socket: socket,
		http:   &http.Client{Timeout: 30 * time.Second, Transport: transport},
	}
}

func (d *dockerClient) containers(ctx context.Context) ([]map[string]any, error) {
	var out []map[string]any
	err := d.getJSON(ctx, "/containers/json?all=1", &out)
	return out, err
}

func (d *dockerClient) inspect(ctx context.Context, container string) (map[string]any, error) {
	var out map[string]any
	err := d.getJSON(ctx, "/containers/"+url.PathEscape(container)+"/json", &out)
	return out, err
}

func (d *dockerClient) logs(ctx context.Context, req nodeagent.LogRequest) (string, error) {
	values := url.Values{
		"stdout":     []string{"1"},
		"stderr":     []string{"1"},
		"tail":       []string{strconv.Itoa(req.TailLines)},
		"since":      []string{strconv.FormatInt(time.Now().Add(-time.Duration(req.SinceSeconds)*time.Second).Unix(), 10)},
		"timestamps": []string{strconv.FormatBool(req.Timestamps)},
	}
	body, err := d.getRaw(ctx, "/containers/"+url.PathEscape(req.Container)+"/logs?"+values.Encode())
	if err != nil {
		return "", err
	}
	return stripDockerLogHeaders(body), nil
}

func (d *dockerClient) stats(ctx context.Context, container string) (map[string]any, error) {
	var out map[string]any
	err := d.getJSON(ctx, "/containers/"+url.PathEscape(container)+"/stats?stream=false", &out)
	return out, err
}

func (d *dockerClient) getJSON(ctx context.Context, path string, target any) error {
	body, err := d.getRaw(ctx, path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func (d *dockerClient) getRaw(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("docker api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func containerSummary(raw map[string]any) map[string]any {
	names := []string{}
	for _, item := range anySlice(raw["Names"]) {
		names = append(names, strings.TrimPrefix(fmt.Sprint(item), "/"))
	}
	return map[string]any{
		"id":      fmt.Sprint(raw["Id"]),
		"names":   names,
		"image":   fmt.Sprint(raw["Image"]),
		"state":   fmt.Sprint(raw["State"]),
		"status":  fmt.Sprint(raw["Status"]),
		"created": raw["Created"],
	}
}

func inspectSummary(raw map[string]any) map[string]any {
	config := mapValue(raw["Config"])
	state := mapValue(raw["State"])
	hostConfig := mapValue(raw["HostConfig"])
	networkSettings := mapValue(raw["NetworkSettings"])
	return map[string]any{
		"id":      raw["Id"],
		"name":    strings.TrimPrefix(fmt.Sprint(raw["Name"]), "/"),
		"created": raw["Created"],
		"path":    raw["Path"],
		"args":    raw["Args"],
		"image":   config["Image"],
		"state": map[string]any{
			"status":      state["Status"],
			"running":     state["Running"],
			"restarting":  state["Restarting"],
			"oom_killed":  state["OOMKilled"],
			"dead":        state["Dead"],
			"pid":         state["Pid"],
			"exit_code":   state["ExitCode"],
			"error":       state["Error"],
			"started_at":  state["StartedAt"],
			"finished_at": state["FinishedAt"],
			"health":      state["Health"],
		},
		"restart_policy": mapValue(hostConfig["RestartPolicy"]),
		"mounts":         raw["Mounts"],
		"network":        networkSettings["Networks"],
	}
}

func statsSummary(raw map[string]any) map[string]any {
	return map[string]any{
		"id":          raw["id"],
		"name":        strings.TrimPrefix(fmt.Sprint(raw["name"]), "/"),
		"read":        raw["read"],
		"cpu_percent": cpuPercent(raw),
		"memory":      memoryStats(raw),
		"network":     networkStats(raw),
		"blkio":       raw["blkio_stats"],
	}
}

func cpuPercent(raw map[string]any) float64 {
	cpu := mapValue(raw["cpu_stats"])
	precpu := mapValue(raw["precpu_stats"])
	cpuUsage := mapValue(cpu["cpu_usage"])
	precpuUsage := mapValue(precpu["cpu_usage"])
	cpuDelta := floatValue(cpuUsage["total_usage"]) - floatValue(precpuUsage["total_usage"])
	systemDelta := floatValue(cpu["system_cpu_usage"]) - floatValue(precpu["system_cpu_usage"])
	onlineCPUs := floatValue(cpu["online_cpus"])
	if onlineCPUs == 0 {
		onlineCPUs = float64(len(anySlice(cpuUsage["percpu_usage"])))
	}
	if cpuDelta <= 0 || systemDelta <= 0 || onlineCPUs <= 0 {
		return 0
	}
	return (cpuDelta / systemDelta) * onlineCPUs * 100
}

func memoryStats(raw map[string]any) map[string]any {
	memory := mapValue(raw["memory_stats"])
	usage := floatValue(memory["usage"])
	limit := floatValue(memory["limit"])
	percent := 0.0
	if limit > 0 {
		percent = usage / limit * 100
	}
	return map[string]any{
		"usage_bytes":   usage,
		"limit_bytes":   limit,
		"usage_percent": percent,
		"stats":         memory["stats"],
	}
}

func networkStats(raw map[string]any) map[string]any {
	networks := mapValue(raw["networks"])
	totalRx := 0.0
	totalTx := 0.0
	for _, value := range networks {
		net := mapValue(value)
		totalRx += floatValue(net["rx_bytes"])
		totalTx += floatValue(net["tx_bytes"])
	}
	return map[string]any{"rx_bytes": totalRx, "tx_bytes": totalTx, "interfaces": networks}
}

func parseContainerAction(path string) (string, string, bool) {
	rest := strings.TrimPrefix(path, "/api/containers/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	container, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", false
	}
	return container, parts[1], true
}

func parseAllowList(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = true
		}
	}
	return out
}

func allowedContainer(allowed map[string]bool, item map[string]any) bool {
	id := fmt.Sprint(item["id"])
	if allowedName(allowed, id) {
		return true
	}
	for _, name := range stringSlice(item["names"]) {
		if allowedName(allowed, name) {
			return true
		}
	}
	return false
}

func allowedName(allowed map[string]bool, name string) bool {
	if len(allowed) == 0 {
		return false
	}
	if allowed[name] {
		return true
	}
	for candidate := range allowed {
		if strings.HasPrefix(name, candidate) || strings.HasPrefix(candidate, name) {
			return true
		}
	}
	return false
}

func authorize(w http.ResponseWriter, r *http.Request, cfg config) bool {
	if cfg.token == "" {
		return true
	}
	header := r.Header.Get("Authorization")
	got := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if subtle.ConstantTimeCompare([]byte(got), []byte(cfg.token)) == 1 {
		return true
	}
	writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
	return false
}

func onlyGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
	return false
}

func intQuery(r *http.Request, name string, fallback int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func intQueryAliases(r *http.Request, names []string, fallback int) int {
	for _, name := range names {
		if r.URL.Query().Get(name) != "" {
			return intQuery(r, name, fallback)
		}
	}
	return fallback
}

func boolQuery(r *http.Request, name string) bool {
	raw := r.URL.Query().Get(name)
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
}

func stripDockerLogHeaders(body []byte) string {
	var out bytes.Buffer
	for len(body) >= 8 {
		size := int(body[4])<<24 | int(body[5])<<16 | int(body[6])<<8 | int(body[7])
		if size < 0 || size > len(body)-8 {
			break
		}
		out.Write(body[8 : 8+size])
		body = body[8+size:]
	}
	if out.Len() == 0 {
		return string(body)
	}
	return out.String()
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

func stringSlice(value any) []string {
	if slice, ok := value.([]string); ok {
		return slice
	}
	out := []string{}
	for _, item := range anySlice(value) {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
