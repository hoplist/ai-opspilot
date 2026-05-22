package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/contracts"
)

const defaultBackend = "http://127.0.0.1:18080"
const realFilesystemFilter = `fstype!~"tmpfs|overlay|squashfs|ramfs|cgroup2?|proc|sysfs|devtmpfs|devpts|securityfs|pstore|bpf|tracefs|debugfs|configfs|fusectl|mqueue|hugetlbfs"`

type globalOptions struct {
	backendURL string
	output     string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, `{"ok":false,"error":`+strconv.Quote(err.Error())+`}`)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	opts := globalOptions{
		backendURL: env("OPSPILOT_BACKEND_URL", defaultBackend),
		output:     env("OPSPILOT_OUTPUT", "json"),
	}
	args = consumeGlobalFlags(args, &opts)
	if len(args) == 0 {
		return fmt.Errorf("missing command")
	}
	var endpoint string
	var values url.Values
	switch args[0] {
	case "schema":
		_, err := out.Write(contracts.CLISchema)
		if err == nil {
			_, err = fmt.Fprintln(out)
		}
		return err
	case "inventory":
		endpoint, values = inventoryCommand(args[1:])
	case "metrics":
		if len(args) > 1 && args[1] == "filesystems" {
			return runMetricsFilesystems(opts, args[2:], out)
		}
		endpoint, values = metricsCommand(args[1:])
	case "k8s":
		endpoint, values = k8sCommand(args[1:])
	case "docker":
		endpoint, values = dockerCommand(args[1:])
	case "logs":
		endpoint, values = logsCommand(args[1:])
	case "evidence":
		endpoint, values = evidenceCommand(args[1:])
	case "release":
		if len(args) > 1 && args[1] == "status" {
			return runReleaseStatus(opts, args[2:], out)
		}
		if len(args) > 1 && args[1] == "jobs" {
			return runReleaseJobs(opts, args[2:], out)
		}
		if len(args) > 1 && args[1] == "logs" {
			return runReleaseLogs(opts, args[2:], out)
		}
		if len(args) > 1 && args[1] == "history" {
			return runReleaseHistory(opts, args[2:], out)
		}
		if len(args) > 1 && args[1] == "rollback" {
			return runReleaseRollback(opts, args[2:], out)
		}
		endpoint, values = releaseCommand(args[1:])
	case "context":
		endpoint, values = podRefCommand(args[1:], "/api/context/pod")
	case "diagnose":
		endpoint, values = diagnoseCommand(args[1:])
	case "inspect", "check":
		return inspectCommand(opts, args[1:], out)
	case "onboard":
		return onboardCommand(args[1:], out)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
	body, err := get(opts.backendURL, endpoint, values)
	if err != nil {
		return err
	}
	_, err = out.Write(body)
	if err == nil {
		_, err = fmt.Fprintln(out)
	}
	return err
}

func evidenceCommand(args []string) (string, url.Values) {
	if len(args) == 0 || args[0] != "request" {
		fail("expected: evidence request")
	}
	fs := flag.NewFlagSet("evidence request", flag.ExitOnError)
	host := fs.String("host", "", "gateway host/domain")
	uri := fs.String("uri", "", "request uri")
	at := fs.String("at", "", "center time, RFC3339 or yyyy-mm-dd HH:MM[:SS]")
	since := fs.Int("since", 900, "look back seconds when --at is empty")
	window := fs.Int("window", 300, "seconds before and after --at")
	limit := fs.Int("limit", 20, "result limit per evidence section")
	includeOptions := fs.Bool("include-options", false, "include CORS OPTIONS requests")
	skipAPISIX := fs.Bool("skip-apisix", false, "skip APISIX lookup and run service-log-only investigation")
	serviceOnly := fs.Bool("service-only", false, "alias for --skip-apisix")
	apisixIndex := fs.String("apisix-index", "", "APISIX Elasticsearch index pattern")
	serviceIndex := fs.String("service-index", "", "service log Elasticsearch index pattern")
	serviceURIField := fs.String("service-uri-field", "", "service log field containing URI text")
	_ = fs.Parse(args[1:])
	return "/api/evidence/request", url.Values{
		"host":              []string{*host},
		"uri":               []string{*uri},
		"at":                []string{*at},
		"since_seconds":     []string{strconv.Itoa(*since)},
		"window_seconds":    []string{strconv.Itoa(*window)},
		"limit":             []string{strconv.Itoa(*limit)},
		"include_options":   []string{strconv.FormatBool(*includeOptions)},
		"skip_apisix":       []string{strconv.FormatBool(*skipAPISIX || *serviceOnly)},
		"apisix_index":      []string{*apisixIndex},
		"service_index":     []string{*serviceIndex},
		"service_uri_field": []string{*serviceURIField},
	}
}

func logsCommand(args []string) (string, url.Values) {
	if len(args) == 0 || args[0] != "search" {
		fail("expected: logs search")
	}
	fs := flag.NewFlagSet("logs search", flag.ExitOnError)
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	pod := fs.String("pod", "", "pod")
	container := fs.String("container", "", "container")
	query := fs.String("q", "", "query string against log field")
	fs.StringVar(query, "query", "", "query string against log field")
	limit := fs.Int("limit", 20, "result limit")
	_ = fs.Parse(args[1:])
	return "/api/logs/search", url.Values{
		"namespace": []string{*namespace},
		"pod":       []string{*pod},
		"container": []string{*container},
		"q":         []string{*query},
		"limit":     []string{strconv.Itoa(*limit)},
	}
}

func dockerCommand(args []string) (string, url.Values) {
	if len(args) == 0 {
		fail("expected docker command")
	}
	switch args[0] {
	case "agents":
		return "/api/node-agents", url.Values{}
	case "containers":
		fs := flag.NewFlagSet("docker containers", flag.ExitOnError)
		host := fs.String("host", "", "node agent host, or all")
		_ = fs.Parse(args[1:])
		return "/api/docker/containers", url.Values{"host": []string{*host}}
	case "inspect":
		fs := flag.NewFlagSet("docker inspect", flag.ExitOnError)
		host := fs.String("host", "", "node agent host")
		container := fs.String("container", "", "container name or id")
		_ = fs.Parse(args[1:])
		return "/api/docker/inspect", url.Values{"host": []string{*host}, "container": []string{*container}}
	case "logs":
		fs := flag.NewFlagSet("docker logs", flag.ExitOnError)
		host := fs.String("host", "", "node agent host")
		container := fs.String("container", "", "container name or id")
		tail := fs.Int("tail", 300, "tail lines")
		since := fs.Int("since", 1800, "since seconds")
		limitBytes := fs.Int("limit-bytes", 1024*1024, "limit bytes")
		timestamps := fs.Bool("timestamps", false, "timestamps")
		_ = fs.Parse(args[1:])
		return "/api/docker/logs", url.Values{
			"host":          []string{*host},
			"container":     []string{*container},
			"tail_lines":    []string{strconv.Itoa(*tail)},
			"since_seconds": []string{strconv.Itoa(*since)},
			"limit_bytes":   []string{strconv.Itoa(*limitBytes)},
			"timestamps":    []string{strconv.FormatBool(*timestamps)},
		}
	case "stats":
		fs := flag.NewFlagSet("docker stats", flag.ExitOnError)
		host := fs.String("host", "", "node agent host")
		container := fs.String("container", "", "container name or id")
		_ = fs.Parse(args[1:])
		return "/api/docker/stats", url.Values{"host": []string{*host}, "container": []string{*container}}
	default:
		fail("unknown docker command: " + args[0])
	}
	return "", nil
}

func diagnoseCommand(args []string) (string, url.Values) {
	if len(args) == 0 {
		fail("expected diagnose subcommand")
	}
	switch args[0] {
	case "pod":
		return podRefCommand(args, "/api/diagnose/pod")
	case "docker":
		fs := flag.NewFlagSet("diagnose docker", flag.ExitOnError)
		host := fs.String("host", "", "node agent host")
		container := fs.String("container", "", "container name or id")
		tail := fs.Int("tail", 300, "tail lines")
		since := fs.Int("since", 1800, "since seconds")
		limitBytes := fs.Int("limit-bytes", 1024*1024, "limit bytes")
		timestamps := fs.Bool("timestamps", false, "timestamps")
		_ = fs.Parse(args[1:])
		return "/api/diagnose/docker", url.Values{
			"host":          []string{*host},
			"container":     []string{*container},
			"tail_lines":    []string{strconv.Itoa(*tail)},
			"since_seconds": []string{strconv.Itoa(*since)},
			"limit_bytes":   []string{strconv.Itoa(*limitBytes)},
			"timestamps":    []string{strconv.FormatBool(*timestamps)},
		}
	default:
		fail("unknown diagnose subcommand: " + args[0])
	}
	return "", nil
}

func releaseCommand(args []string) (string, url.Values) {
	if len(args) == 0 {
		fail("expected release command")
	}
	switch args[0] {
	case "status", "evidence", "diagnose":
		fs := flag.NewFlagSet("release "+args[0], flag.ExitOnError)
		service := fs.String("service", "", "release service name")
		_ = fs.Parse(args[1:])
		return "/api/release/status", url.Values{"service": []string{*service}}
	case "jobs":
		fs := flag.NewFlagSet("release jobs", flag.ExitOnError)
		service := fs.String("service", "", "release service name")
		_ = fs.Parse(args[1:])
		return "/api/release/jobs", url.Values{"service": []string{*service}}
	case "logs":
		fs := flag.NewFlagSet("release logs", flag.ExitOnError)
		service := fs.String("service", "", "release service name")
		job := fs.String("job", "", "GitLab job name")
		jobID := fs.String("job-id", "", "GitLab job id")
		tail := fs.Int("tail", 200, "tail lines")
		limitBytes := fs.Int("limit-bytes", 128*1024, "limit bytes")
		_ = fs.Parse(args[1:])
		return "/api/release/logs", url.Values{
			"service":     []string{*service},
			"job":         []string{*job},
			"job_id":      []string{*jobID},
			"tail_lines":  []string{strconv.Itoa(*tail)},
			"limit_bytes": []string{strconv.Itoa(*limitBytes)},
		}
	case "history":
		fs := flag.NewFlagSet("release history", flag.ExitOnError)
		service := fs.String("service", "", "release service name")
		limit := fs.Int("limit", 10, "history item limit")
		_ = fs.Parse(args[1:])
		return "/api/release/history", url.Values{"service": []string{*service}, "limit": []string{strconv.Itoa(*limit)}}
	default:
		fail("unknown release command: " + args[0])
	}
	return "", nil
}

func consumeGlobalFlags(args []string, opts *globalOptions) []string {
	out := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--backend-url" && i+1 < len(args) {
			opts.backendURL = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--backend-url=") {
			opts.backendURL = strings.TrimPrefix(arg, "--backend-url=")
			continue
		}
		if arg == "--output" && i+1 < len(args) {
			opts.output = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--output=") {
			opts.output = strings.TrimPrefix(arg, "--output=")
			continue
		}
		out = append(out, arg)
	}
	return out
}

func inventoryCommand(args []string) (string, url.Values) {
	if len(args) == 0 || args[0] != "overview" {
		fail("expected: inventory overview")
	}
	fs := flag.NewFlagSet("inventory overview", flag.ExitOnError)
	limit := fs.Int("limit", 10, "result limit")
	_ = fs.Parse(args[1:])
	return "/api/inventory/overview", url.Values{"limit": []string{strconv.Itoa(*limit)}}
}

func metricsCommand(args []string) (string, url.Values) {
	if len(args) == 0 {
		fail("expected metrics command")
	}
	switch args[0] {
	case "health":
		return "/api/metrics/health", url.Values{}
	case "datasources":
		return "/api/metrics/datasources", url.Values{}
	case "query":
		fs := flag.NewFlagSet("metrics query", flag.ExitOnError)
		query := fs.String("query", "", "promql query")
		fs.StringVar(query, "q", "", "promql query")
		source := fs.String("source", "", "prometheus datasource")
		_ = fs.Parse(args[1:])
		return "/api/metrics/query", url.Values{"query": []string{*query}, "source": []string{*source}}
	case "filesystems":
		fs := flag.NewFlagSet("metrics filesystems", flag.ExitOnError)
		source := fs.String("source", "", "prometheus datasource, or all")
		_ = fs.Parse(args[1:])
		return "/api/metrics/query", url.Values{"query": []string{"node_filesystem_avail_bytes{" + realFilesystemFilter + "}"}, "source": []string{*source}}
	case "nodes":
		fs := flag.NewFlagSet("metrics nodes", flag.ExitOnError)
		limit := fs.Int("limit", 20, "result limit")
		source := fs.String("source", "", "prometheus datasource, or all")
		_ = fs.Parse(args[1:])
		return "/api/metrics/nodes", url.Values{"limit": []string{strconv.Itoa(*limit)}, "source": []string{*source}}
	case "pods":
		fs := flag.NewFlagSet("metrics pods", flag.ExitOnError)
		namespace := fs.String("namespace", "", "namespace")
		fs.StringVar(namespace, "n", "", "namespace")
		sortBy := fs.String("sort", "cpu", "sort by cpu or memory")
		limit := fs.Int("limit", 20, "result limit")
		source := fs.String("source", "", "prometheus datasource, or all")
		_ = fs.Parse(args[1:])
		return "/api/metrics/pods", url.Values{
			"namespace": []string{*namespace},
			"sort":      []string{*sortBy},
			"limit":     []string{strconv.Itoa(*limit)},
			"source":    []string{*source},
		}
	case "containers":
		fs := flag.NewFlagSet("metrics containers", flag.ExitOnError)
		sortBy := fs.String("sort", "cpu", "sort by cpu or memory")
		limit := fs.Int("limit", 20, "result limit")
		source := fs.String("source", "", "prometheus datasource, or all")
		_ = fs.Parse(args[1:])
		return "/api/metrics/containers", url.Values{
			"sort":   []string{*sortBy},
			"limit":  []string{strconv.Itoa(*limit)},
			"source": []string{*source},
		}
	case "pod":
		fs := flag.NewFlagSet("metrics pod", flag.ExitOnError)
		namespace := fs.String("namespace", "", "namespace")
		fs.StringVar(namespace, "n", "", "namespace")
		pod := fs.String("pod", "", "pod")
		source := fs.String("source", "", "prometheus datasource")
		_ = fs.Parse(args[1:])
		return "/api/metrics/pod", url.Values{"namespace": []string{*namespace}, "pod": []string{*pod}, "source": []string{*source}}
	default:
		fail("unknown metrics command: " + args[0])
	}
	return "", nil
}

func k8sCommand(args []string) (string, url.Values) {
	if len(args) == 0 {
		fail("expected k8s command")
	}
	switch args[0] {
	case "pods":
		fs := flag.NewFlagSet("k8s pods", flag.ExitOnError)
		namespace := fs.String("namespace", "", "namespace")
		fs.StringVar(namespace, "n", "", "namespace")
		status := fs.String("status", "", "status")
		query := fs.String("q", "", "query")
		limit := fs.Int("limit", 100, "result limit")
		_ = fs.Parse(args[1:])
		return "/api/k8s/pods", url.Values{
			"namespace": []string{*namespace},
			"status":    []string{*status},
			"q":         []string{*query},
			"limit":     []string{strconv.Itoa(*limit)},
		}
	case "logs":
		if len(args) < 2 || args[1] != "pod" {
			fail("expected: k8s logs pod")
		}
		fs := flag.NewFlagSet("k8s logs pod", flag.ExitOnError)
		namespace := fs.String("namespace", "", "namespace")
		fs.StringVar(namespace, "n", "", "namespace")
		pod := fs.String("pod", "", "pod")
		container := fs.String("container", "", "container")
		fs.StringVar(container, "c", "", "container")
		tail := fs.Int("tail", 300, "tail lines")
		since := fs.Int("since", 1800, "since seconds")
		limitBytes := fs.Int("limit-bytes", 1024*1024, "limit bytes")
		previous := fs.Bool("previous", false, "previous logs")
		timestamps := fs.Bool("timestamps", false, "timestamps")
		_ = fs.Parse(args[2:])
		return "/api/k8s/logs/pod", url.Values{
			"namespace":     []string{*namespace},
			"pod":           []string{*pod},
			"container":     []string{*container},
			"tail_lines":    []string{strconv.Itoa(*tail)},
			"since_seconds": []string{strconv.Itoa(*since)},
			"limit_bytes":   []string{strconv.Itoa(*limitBytes)},
			"previous":      []string{strconv.FormatBool(*previous)},
			"timestamps":    []string{strconv.FormatBool(*timestamps)},
		}
	default:
		fail("unknown k8s command: " + args[0])
	}
	return "", nil
}

func podRefCommand(args []string, endpoint string) (string, url.Values) {
	if len(args) == 0 || args[0] != "pod" {
		fail("expected pod subcommand")
	}
	fs := flag.NewFlagSet(strings.TrimPrefix(endpoint, "/api/"), flag.ExitOnError)
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	pod := fs.String("pod", "", "pod")
	source := fs.String("source", "", "prometheus datasource for metric evidence")
	_ = fs.Parse(args[1:])
	return endpoint, url.Values{"namespace": []string{*namespace}, "pod": []string{*pod}, "source": []string{*source}}
}

type apiEnvelope struct {
	OK   bool            `json:"ok"`
	Data json.RawMessage `json:"data"`
}

type metricItem struct {
	Metric map[string]string `json:"metric"`
	Source string            `json:"source"`
	Value  float64           `json:"value"`
}

type metricItemsData struct {
	Items []metricItem `json:"items"`
}

type filesystemRow struct {
	Source   string  `json:"source"`
	Node     string  `json:"node"`
	Mount    string  `json:"mount"`
	Device   string  `json:"device"`
	FSType   string  `json:"fstype"`
	FreeGiB  float64 `json:"free_gib"`
	TotalGiB float64 `json:"total_gib"`
	UsedPct  float64 `json:"used_percent"`
}

type filesystemsResult struct {
	Items []filesystemRow `json:"items"`
	Count int             `json:"item_count"`
}

func inspectCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected inspect subcommand: pod or cluster")
	}
	switch args[0] {
	case "pod":
		return runInspectPod(opts, args[1:], out)
	case "cluster":
		return runInspectCluster(opts, args[1:], out)
	default:
		return fmt.Errorf("unknown inspect command: %s", args[0])
	}
}

func runMetricsFilesystems(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("metrics filesystems", flag.ExitOnError)
	source := fs.String("source", "", "prometheus datasource, or all")
	_ = fs.Parse(args)
	result, err := fetchFilesystems(opts.backendURL, *source)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SOURCE\tNODE\tMOUNT\tDEVICE\tFSTYPE\tFREE\tTOTAL\tUSED")
		for _, row := range result.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%.1fGiB\t%.1fGiB\t%.1f%%\n",
				row.Source, row.Node, row.Mount, row.Device, row.FSType, row.FreeGiB, row.TotalGiB, row.UsedPct)
		}
		return tw.Flush()
	})
}

func fetchFilesystems(backendURL, source string) (filesystemsResult, error) {
	avail, err := fetchMetricItems(backendURL, "node_filesystem_avail_bytes{"+realFilesystemFilter+"}", source)
	if err != nil {
		return filesystemsResult{}, err
	}
	size, err := fetchMetricItems(backendURL, "node_filesystem_size_bytes{"+realFilesystemFilter+"}", source)
	if err != nil {
		return filesystemsResult{}, err
	}
	sizes := map[string]float64{}
	for _, item := range size {
		sizes[filesystemKey(item)] = item.Value
	}
	rows := make([]filesystemRow, 0, len(avail))
	for _, item := range avail {
		total := sizes[filesystemKey(item)]
		if total <= 0 {
			continue
		}
		usedPct := (1 - item.Value/total) * 100
		rows = append(rows, filesystemRow{
			Source:   item.Source,
			Node:     metricNode(item.Metric),
			Mount:    item.Metric["mountpoint"],
			Device:   item.Metric["device"],
			FSType:   item.Metric["fstype"],
			FreeGiB:  round1(item.Value / (1024 * 1024 * 1024)),
			TotalGiB: round1(total / (1024 * 1024 * 1024)),
			UsedPct:  round1(usedPct),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Node == rows[j].Node {
			return rows[i].Mount < rows[j].Mount
		}
		return rows[i].Node < rows[j].Node
	})
	return filesystemsResult{Items: rows, Count: len(rows)}, nil
}

type inspectPodResult struct {
	Namespace            string         `json:"namespace"`
	Pod                  string         `json:"pod"`
	Node                 string         `json:"node,omitempty"`
	Status               string         `json:"status,omitempty"`
	Ready                bool           `json:"ready"`
	RestartCount         int            `json:"restart_count"`
	CPUCore              float64        `json:"cpu_cores"`
	MemoryMiB            float64        `json:"memory_mib"`
	KubernetesLogBytes   int            `json:"kubernetes_log_bytes"`
	ElasticsearchLogHits int            `json:"elasticsearch_log_hits"`
	EvidenceGaps         []string       `json:"evidence_gaps"`
	Findings             []string       `json:"findings"`
	Raw                  map[string]any `json:"raw,omitempty"`
}

func runInspectPod(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("inspect pod", flag.ExitOnError)
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	pod := fs.String("pod", "", "pod")
	source := fs.String("source", "", "prometheus datasource")
	tail := fs.Int("tail", 300, "tail lines")
	since := fs.Int("since", 1800, "since seconds")
	_ = fs.Parse(args)
	if *pod == "" && fs.NArg() > 0 {
		*pod = fs.Arg(0)
	}
	if *namespace == "" || *pod == "" {
		return fmt.Errorf("inspect pod requires --namespace and --pod")
	}
	result, err := fetchInspectPod(opts.backendURL, *namespace, *pod, *source, *tail, *since)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Pod: %s/%s\n", result.Namespace, result.Pod)
		fmt.Fprintf(w, "Status: %s ready=%t restarts=%d node=%s\n", result.Status, result.Ready, result.RestartCount, result.Node)
		fmt.Fprintf(w, "Usage: CPU %.3f cores, memory %.1f MiB\n", result.CPUCore, result.MemoryMiB)
		fmt.Fprintf(w, "Logs: Kubernetes %d bytes, ELK hits %d\n", result.KubernetesLogBytes, result.ElasticsearchLogHits)
		if len(result.EvidenceGaps) > 0 {
			fmt.Fprintf(w, "Evidence gaps: %s\n", strings.Join(result.EvidenceGaps, ", "))
		}
		if len(result.Findings) > 0 {
			fmt.Fprintf(w, "Findings: %s\n", strings.Join(result.Findings, "; "))
		}
		return nil
	})
}

func fetchInspectPod(backendURL, namespace, pod, source string, tail, since int) (inspectPodResult, error) {
	result := inspectPodResult{Namespace: namespace, Pod: pod, Raw: map[string]any{}}
	contextBody, err := get(backendURL, "/api/context/pod", url.Values{"namespace": {namespace}, "pod": {pod}, "source": {source}})
	if err != nil {
		return result, err
	}
	var contextPayload map[string]any
	_ = json.Unmarshal(contextBody, &contextPayload)
	result.Raw["context"] = contextPayload
	if data := mapValue(contextPayload, "data"); data != nil {
		if summary := mapValue(data, "summary"); summary != nil {
			result.Node = stringValue(summary["node"])
			result.Status = stringValue(summary["status"])
			result.Ready = boolValue(summary["ready"])
			result.RestartCount = intValue(summary["restart_count"])
		}
	}
	metricsBody, err := get(backendURL, "/api/metrics/pod", url.Values{"namespace": {namespace}, "pod": {pod}, "source": {source}})
	if err == nil {
		var metricsPayload map[string]any
		_ = json.Unmarshal(metricsBody, &metricsPayload)
		result.Raw["metrics"] = metricsPayload
		if data := mapValue(metricsPayload, "data"); data != nil {
			result.CPUCore = floatValue(data["cpu_cores"])
			result.MemoryMiB = round1(floatValue(data["memory_working_set_bytes"]) / (1024 * 1024))
			if result.RestartCount == 0 {
				result.RestartCount = intValue(data["restart_count"])
			}
		}
	}
	logBody, err := get(backendURL, "/api/k8s/logs/pod", url.Values{
		"namespace":     {namespace},
		"pod":           {pod},
		"tail_lines":    {strconv.Itoa(tail)},
		"since_seconds": {strconv.Itoa(since)},
	})
	if err == nil {
		var logPayload map[string]any
		_ = json.Unmarshal(logBody, &logPayload)
		result.Raw["kubernetes_logs"] = logPayload
		if data := mapValue(logPayload, "data"); data != nil {
			result.KubernetesLogBytes = len(stringValue(data["text"]))
		}
	}
	elkBody, err := get(backendURL, "/api/logs/search", url.Values{"namespace": {namespace}, "pod": {pod}, "limit": {"1"}})
	if err == nil {
		var elkPayload map[string]any
		_ = json.Unmarshal(elkBody, &elkPayload)
		result.Raw["elk_logs"] = elkPayload
		if data := mapValue(elkPayload, "data"); data != nil {
			result.ElasticsearchLogHits = intValue(data["total"])
			if result.ElasticsearchLogHits == 0 {
				result.ElasticsearchLogHits = intValue(data["item_count"])
			}
		}
	}
	if result.KubernetesLogBytes == 0 {
		result.EvidenceGaps = append(result.EvidenceGaps, "kubernetes_logs_empty")
	}
	if result.ElasticsearchLogHits == 0 {
		result.EvidenceGaps = append(result.EvidenceGaps, "elk_logs_missing_or_empty")
	}
	if result.Ready {
		result.Findings = append(result.Findings, "Pod is currently ready.")
	}
	if result.Ready && result.KubernetesLogBytes == 0 && result.ElasticsearchLogHits == 0 {
		result.Findings = append(result.Findings, "Pod is ready, but no log evidence was found in Kubernetes logs or ELK.")
	}
	if result.RestartCount > 0 {
		result.Findings = append(result.Findings, fmt.Sprintf("Pod has historical restarts: %d.", result.RestartCount))
	}
	return result, nil
}

type inspectClusterResult struct {
	AbnormalPods map[string]any   `json:"abnormal_pods"`
	Nodes        []map[string]any `json:"nodes"`
	TopCPU       []map[string]any `json:"top_cpu_pods"`
	TopMemory    []map[string]any `json:"top_memory_pods"`
	Restarts24h  []metricItem     `json:"restarts_24h"`
	Filesystems  []filesystemRow  `json:"filesystems"`
	Findings     []string         `json:"findings"`
	Raw          map[string]any   `json:"raw,omitempty"`
}

func runInspectCluster(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("inspect cluster", flag.ExitOnError)
	source := fs.String("source", "all", "prometheus datasource, or all")
	limit := fs.Int("limit", 10, "top result limit")
	_ = fs.Parse(args)
	result, err := fetchInspectCluster(opts.backendURL, *source, *limit)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintln(w, "Cluster inspection")
		if len(result.Findings) > 0 {
			fmt.Fprintf(w, "Findings: %s\n", strings.Join(result.Findings, "; "))
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "\nNODES\tCPU\tMEMORY\tROOTFS")
		for _, node := range result.Nodes {
			fmt.Fprintf(tw, "%s\t%.1f%%\t%.1f%%\t%.1f%%\n",
				stringValue(node["node"]), floatValue(node["cpu_used_percent"]), floatValue(node["memory_used_percent"]), floatValue(node["rootfs_used_percent"]))
		}
		fmt.Fprintln(tw, "\nTOP CPU PODS\tNAMESPACE\tCPU")
		for _, pod := range result.TopCPU {
			fmt.Fprintf(tw, "%s\t%s\t%.3f cores\n", stringValue(pod["pod"]), stringValue(pod["namespace"]), floatValue(pod["cpu_cores"]))
		}
		fmt.Fprintln(tw, "\nTOP MEMORY PODS\tNAMESPACE\tMEMORY")
		for _, pod := range result.TopMemory {
			fmt.Fprintf(tw, "%s\t%s\t%.1fMiB\n", stringValue(pod["pod"]), stringValue(pod["namespace"]), floatValue(pod["memory_working_set_bytes"])/(1024*1024))
		}
		fmt.Fprintln(tw, "\nRESTARTS 24H\tNAMESPACE\tCONTAINER\tCOUNT")
		if len(result.Restarts24h) == 0 {
			fmt.Fprintln(tw, "-\t-\t-\t0")
		}
		for _, item := range result.Restarts24h {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%.1f\n", item.Metric["pod"], item.Metric["namespace"], item.Metric["container"], item.Value)
		}
		fmt.Fprintln(tw, "\nFILESYSTEMS\tMOUNT\tFREE\tTOTAL\tUSED")
		for _, row := range result.Filesystems {
			fmt.Fprintf(tw, "%s\t%s\t%.1fGiB\t%.1fGiB\t%.1f%%\n", row.Node, row.Mount, row.FreeGiB, row.TotalGiB, row.UsedPct)
		}
		return tw.Flush()
	})
}

func runReleaseStatus(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release status", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release status requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/status", url.Values{"service": []string{*service}})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release: %s\n", stringValue(data["service"]))
		fmt.Fprintf(w, "Status: %s stage=%s namespace=%s deployment=%s\n",
			stringValue(data["status"]), stringValue(data["stage"]), stringValue(data["namespace"]), stringValue(data["deployment"]))
		if image := stringValue(data["image"]); image != "" {
			fmt.Fprintf(w, "Image: %s\n", image)
		}
		if evidence := mapValue(data, "evidence"); evidence != nil {
			if k8s := mapValue(evidence, "kubernetes"); k8s != nil {
				fmt.Fprintf(w, "Kubernetes: ready=%d desired=%d updated=%d available=%d\n",
					intValue(k8s["ready_replicas"]), intValue(k8s["desired_replicas"]), intValue(k8s["updated_replicas"]), intValue(k8s["available_replicas"]))
			}
			if pods := mapValue(evidence, "pods"); pods != nil {
				fmt.Fprintf(w, "Pods: %d/%d listed\n", intValue(pods["item_count"]), intValue(pods["total_count"]))
			}
			if registry := mapValue(evidence, "registry"); registry != nil {
				fmt.Fprintf(w, "Registry: %s tag=%s\n", stringValue(registry["status"]), stringValue(registry["tag"]))
			}
			if pipeline := mapValue(evidence, "gitlab_pipeline"); pipeline != nil {
				fmt.Fprintf(w, "GitLab: %s ref=%s sha=%s\n", stringValue(pipeline["status"]), stringValue(pipeline["ref"]), stringValue(pipeline["sha"]))
			}
			if gitops := mapValue(evidence, "gitops"); gitops != nil {
				fmt.Fprintf(w, "GitOps: %s image=%s\n", stringValue(gitops["status"]), stringValue(gitops["desired_image"]))
			}
			if argocd := mapValue(evidence, "argocd"); argocd != nil {
				fmt.Fprintf(w, "Argo CD: sync=%s health=%s\n", stringValue(argocd["sync_status"]), stringValue(argocd["health_status"]))
			}
		}
		if gaps := stringList(data["gaps"]); len(gaps) > 0 {
			fmt.Fprintf(w, "Gaps: %s\n", strings.Join(gaps, ", "))
		}
		if checks := stringList(data["next_checks"]); len(checks) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(checks, "; "))
		}
		return nil
	})
}

func runReleaseJobs(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release jobs", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release jobs requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/jobs", url.Values{"service": []string{*service}})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release jobs: %s\n", stringValue(data["service"]))
		if pipeline := mapValue(data, "pipeline"); pipeline != nil {
			fmt.Fprintf(w, "Pipeline: %s id=%d ref=%s sha=%s\n",
				stringValue(pipeline["status"]), intValue(pipeline["id"]), stringValue(pipeline["ref"]), stringValue(pipeline["sha"]))
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tSTAGE\tNAME\tSTATUS\tDURATION\tFAILURE")
		for _, job := range mapsFromItems(data["items"]) {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%.1fs\t%s\n",
				intValue(job["id"]), stringValue(job["stage"]), stringValue(job["name"]), stringValue(job["status"]), floatValue(job["duration"]), stringValue(job["failure_reason"]))
		}
		return tw.Flush()
	})
}

func runReleaseLogs(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release logs", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	job := fs.String("job", "", "GitLab job name")
	jobID := fs.String("job-id", "", "GitLab job id")
	tail := fs.Int("tail", 200, "tail lines")
	limitBytes := fs.Int("limit-bytes", 128*1024, "limit bytes")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release logs requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/logs", url.Values{
		"service":     []string{*service},
		"job":         []string{*job},
		"job_id":      []string{*jobID},
		"tail_lines":  []string{strconv.Itoa(*tail)},
		"limit_bytes": []string{strconv.Itoa(*limitBytes)},
	})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release log: %s job=%s id=%d truncated=%t\n",
			stringValue(data["service"]), stringValue(data["job_name"]), intValue(data["job_id"]), boolValue(data["truncated"]))
		fmt.Fprintln(w, stringValue(data["text"]))
		return nil
	})
}

func runReleaseHistory(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release history", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	limit := fs.Int("limit", 10, "history item limit")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release history requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/history", url.Values{"service": []string{*service}, "limit": []string{strconv.Itoa(*limit)}})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release history: %s\n", stringValue(data["service"]))
		if image := stringValue(data["current_image"]); image != "" {
			fmt.Fprintf(w, "Current image: %s\n", image)
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "CURRENT\tREVISION\tDATE\tTAG\tMESSAGE")
		for _, item := range mapsFromItems(data["items"]) {
			current := ""
			if boolValue(item["current"]) {
				current = "*"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				current,
				stringValue(item["short_revision"]),
				shortTime(stringValue(item["committed_at"])),
				stringValue(item["tag"]),
				oneLine(stringValue(item["message"]), 80),
			)
		}
		return tw.Flush()
	})
}

func runReleaseRollback(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release rollback", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	target := fs.String("to", "", "target tag, full image, or GitOps revision")
	fs.StringVar(target, "target", "", "target tag, full image, or GitOps revision")
	confirm := fs.Bool("confirm", false, "confirm GitOps rollback commit")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *target == "" && fs.NArg() > 1 {
		*target = fs.Arg(1)
	}
	if *service == "" {
		return fmt.Errorf("release rollback requires --service")
	}
	if *target == "" {
		return fmt.Errorf("release rollback requires --to")
	}
	if !*confirm {
		return fmt.Errorf("release rollback requires --confirm")
	}
	body, err := post(opts.backendURL, "/api/release/rollback", url.Values{
		"service": []string{*service},
		"to":      []string{*target},
		"confirm": []string{strconv.FormatBool(*confirm)},
	})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Rollback: %s status=%s\n", stringValue(data["service"]), stringValue(data["status"]))
		fmt.Fprintf(w, "Previous: %s\n", stringValue(data["previous_image"]))
		fmt.Fprintf(w, "Target: %s\n", stringValue(data["target_image"]))
		fmt.Fprintf(w, "GitOps: %s %s branch=%s\n",
			stringValue(data["gitops_project"]), stringValue(data["gitops_path"]), stringValue(data["branch"]))
		if commit := stringValue(data["commit_short_id"]); commit != "" {
			fmt.Fprintf(w, "Commit: %s %s\n", commit, stringValue(data["commit_message"]))
		}
		if reason := stringValue(data["reason"]); reason != "" {
			fmt.Fprintf(w, "Reason: %s\n", reason)
		}
		if checks := stringList(data["next_checks"]); len(checks) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(checks, "; "))
		}
		return nil
	})
}

func fetchInspectCluster(backendURL, source string, limit int) (inspectClusterResult, error) {
	result := inspectClusterResult{Raw: map[string]any{}}
	abnormal, _ := getJSONMap(backendURL, "/api/k8s/pods", url.Values{"status": {"abnormal"}, "limit": {strconv.Itoa(limit)}})
	nodes, _ := getJSONMap(backendURL, "/api/metrics/nodes", url.Values{"source": {source}, "limit": {"100"}})
	topCPU, _ := getJSONMap(backendURL, "/api/metrics/pods", url.Values{"source": {source}, "sort": {"cpu"}, "limit": {strconv.Itoa(limit)}})
	topMemory, _ := getJSONMap(backendURL, "/api/metrics/pods", url.Values{"source": {source}, "sort": {"memory"}, "limit": {strconv.Itoa(limit)}})
	result.Raw["abnormal_pods"] = abnormal
	result.Raw["nodes"] = nodes
	result.Raw["top_cpu_pods"] = topCPU
	result.Raw["top_memory_pods"] = topMemory
	if data := mapValue(abnormal, "data"); data != nil {
		result.AbnormalPods = data
		if intValue(data["total_count"]) == 0 && intValue(data["item_count"]) == 0 {
			result.Findings = append(result.Findings, "No abnormal Pods found.")
		}
	}
	if data := mapValue(nodes, "data"); data != nil {
		result.Nodes = mapsFromItems(data["items"])
		for _, node := range result.Nodes {
			if floatValue(node["memory_used_percent"]) >= 80 {
				result.Findings = append(result.Findings, "High node memory: "+stringValue(node["node"]))
			}
			if floatValue(node["rootfs_used_percent"]) >= 80 {
				result.Findings = append(result.Findings, "High root filesystem usage: "+stringValue(node["node"]))
			}
		}
	}
	if data := mapValue(topCPU, "data"); data != nil {
		result.TopCPU = mapsFromItems(data["items"])
	}
	if data := mapValue(topMemory, "data"); data != nil {
		result.TopMemory = mapsFromItems(data["items"])
	}
	restarts, err := fetchMetricItems(backendURL, "topk(20, sum by (namespace,pod,container) (increase(kube_pod_container_status_restarts_total[24h])))", "node200-k8s")
	if err == nil {
		for _, item := range restarts {
			if item.Value > 0 {
				result.Restarts24h = append(result.Restarts24h, item)
			}
		}
	}
	filesystems, err := fetchFilesystems(backendURL, source)
	if err == nil {
		result.Filesystems = filesystems.Items
		for _, row := range result.Filesystems {
			if row.UsedPct >= 80 {
				result.Findings = append(result.Findings, "High filesystem usage: "+row.Node+" "+row.Mount)
			}
		}
	}
	if len(result.Restarts24h) > 0 {
		result.Findings = append(result.Findings, fmt.Sprintf("%d containers have restarts in the last 24h.", len(result.Restarts24h)))
	}
	return result, nil
}

func fetchMetricItems(backendURL, query, source string) ([]metricItem, error) {
	body, err := get(backendURL, "/api/metrics/query", url.Values{"query": {query}, "source": {source}})
	if err != nil {
		return nil, err
	}
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	var data metricItemsData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, err
	}
	return data.Items, nil
}

func getJSONMap(backendURL, endpoint string, values url.Values) (map[string]any, error) {
	body, err := get(backendURL, endpoint, values)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeOutput(out io.Writer, output string, payload any, table func(io.Writer) error) error {
	switch strings.ToLower(output) {
	case "", "json":
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(body))
		return err
	case "pretty":
		body, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(body))
		return err
	case "table", "human":
		return table(out)
	default:
		return fmt.Errorf("unknown output: %s", output)
	}
}

func filesystemKey(item metricItem) string {
	return item.Source + "|" + metricNode(item.Metric) + "|" + item.Metric["device"] + "|" + item.Metric["mountpoint"]
}

func metricNode(metric map[string]string) string {
	if metric["node"] != "" {
		return metric["node"]
	}
	return metric["host"]
}

func mapValue(m map[string]any, key string) map[string]any {
	if value, ok := m[key].(map[string]any); ok {
		return value
	}
	return nil
}

func mapsFromItems(value any) []map[string]any {
	items, _ := value.([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func stringList(value any) []string {
	out := []string{}
	items, _ := value.([]any)
	for _, item := range items {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func boolValue(value any) bool {
	v, _ := value.(bool)
	return v
}

func intValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

func floatValue(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}

func round1(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func get(baseURL, endpoint string, values url.Values) ([]byte, error) {
	clean := url.Values{}
	for key, vals := range values {
		if len(vals) > 0 && vals[0] != "" {
			clean.Set(key, vals[0])
		}
	}
	target := strings.TrimRight(baseURL, "/") + endpoint
	if encoded := clean.Encode(); encoded != "" {
		target += "?" + encoded
	}
	resp, err := http.Get(target)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backend returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if json.Valid(body) {
		return body, nil
	}
	return nil, fmt.Errorf("backend returned non-json response")
}

func post(baseURL, endpoint string, values url.Values) ([]byte, error) {
	clean := url.Values{}
	for key, vals := range values {
		if len(vals) > 0 && vals[0] != "" {
			clean.Set(key, vals[0])
		}
	}
	target := strings.TrimRight(baseURL, "/") + endpoint
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(clean.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backend returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if json.Valid(body) {
		return body, nil
	}
	return nil, fmt.Errorf("backend returned non-json response")
}

func shortTime(value string) string {
	if len(value) >= len("2006-01-02T15:04:05") {
		return strings.ReplaceAll(value[:16], "T", " ")
	}
	return value
}

func oneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}
