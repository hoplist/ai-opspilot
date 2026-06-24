package main

import (
	"flag"
	"net/url"
	"strconv"
	"strings"
)

func errorsCommand(args []string) (string, url.Values) {
	if len(args) == 0 || args[0] != "recent" {
		fail("expected: errors recent")
	}
	fs := flag.NewFlagSet("errors recent", flag.ExitOnError)
	source := fs.String("source", "", "error source: kubernetes, argocd, release, middleware")
	service := fs.String("service", "", "service name")
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	limit := fs.Int("limit", 20, "result limit")
	_ = fs.Parse(args[1:])
	return "/api/errors/recent", url.Values{
		"source":    []string{*source},
		"service":   []string{*service},
		"namespace": []string{*namespace},
		"limit":     []string{strconv.Itoa(*limit)},
	}
}

func evidenceCommand(args []string) (string, url.Values) {
	if len(args) == 0 || args[0] != "request" {
		fail("expected: evidence request")
	}
	fs := flag.NewFlagSet("evidence request", flag.ExitOnError)
	host := fs.String("host", "", "gateway host/domain")
	uri := fs.String("uri", "", "request uri")
	status := fs.String("status", "", "HTTP status code")
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
	probeID := fs.String("probe-id", "", "probe id to narrow weak correlation")
	userAgent := fs.String("user-agent", "", "user agent to narrow APISIX correlation")
	traceID := fs.String("trace-id", "", "trace id to narrow service/APISIX correlation")
	var keywords repeatedFlags
	fs.Var(&keywords, "keyword", "extra keyword for service log correlation, repeatable")
	_ = fs.Parse(args[1:])
	values := url.Values{
		"host":              []string{*host},
		"uri":               []string{*uri},
		"status":            []string{*status},
		"at":                []string{*at},
		"since_seconds":     []string{strconv.Itoa(*since)},
		"window_seconds":    []string{strconv.Itoa(*window)},
		"limit":             []string{strconv.Itoa(*limit)},
		"include_options":   []string{strconv.FormatBool(*includeOptions)},
		"skip_apisix":       []string{strconv.FormatBool(*skipAPISIX || *serviceOnly)},
		"apisix_index":      []string{*apisixIndex},
		"service_index":     []string{*serviceIndex},
		"service_uri_field": []string{*serviceURIField},
		"probe_id":          []string{*probeID},
		"user_agent":        []string{*userAgent},
		"trace_id":          []string{*traceID},
	}
	for _, keyword := range keywords {
		values.Add("keyword", keyword)
	}
	return "/api/evidence/request", values
}

func logsCommand(args []string) (string, url.Values) {
	if len(args) == 0 {
		fail("expected: logs search or logs route")
	}
	if args[0] == "route" {
		return "/api/logs/route", logsRouteValues(args[1:])
	}
	if args[0] != "search" {
		fail("expected: logs search or logs route")
	}
	fs := flag.NewFlagSet("logs search", flag.ExitOnError)
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	pod := fs.String("pod", "", "pod")
	container := fs.String("container", "", "container")
	query := fs.String("q", "", "query string against log field")
	fs.StringVar(query, "query", "", "query string against log field")
	limit := fs.Int("limit", 20, "result limit")
	since := fs.Int("since", 1800, "look back seconds")
	_ = fs.Parse(args[1:])
	return "/api/logs/search", url.Values{
		"namespace":     []string{*namespace},
		"pod":           []string{*pod},
		"container":     []string{*container},
		"q":             []string{*query},
		"limit":         []string{strconv.Itoa(*limit)},
		"since_seconds": []string{strconv.Itoa(*since)},
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
		if arg == "--cluster" && i+1 < len(args) {
			opts.cluster = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--cluster=") {
			opts.cluster = strings.TrimPrefix(arg, "--cluster=")
			continue
		}
		out = append(out, arg)
	}
	return out
}

func addCluster(values url.Values, cluster string) url.Values {
	if values == nil {
		values = url.Values{}
	}
	if cluster != "" && values.Get("cluster") == "" {
		values.Set("cluster", cluster)
	}
	return values
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
		since := fs.Int("since", defaultPodLogSinceSeconds, "since seconds")
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
