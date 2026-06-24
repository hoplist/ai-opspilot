package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

type repeatedFlags []string

func (f *repeatedFlags) String() string {
	return strings.Join(*f, ",")
}

func (f *repeatedFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func runProbeCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "http" {
		return fmt.Errorf("expected: probe http")
	}
	fs := flag.NewFlagSet("probe http", flag.ExitOnError)
	method := fs.String("method", "GET", "HTTP method: GET, POST, HEAD, OPTIONS")
	targetURL := fs.String("url", "", "target HTTP or HTTPS URL")
	bodyText := fs.String("body", "", "request body for POST")
	timeout := fs.Int("timeout", 10, "request timeout seconds, max 30")
	bodyLimit := fs.Int("body-limit-bytes", 16*1024, "response body preview limit, max 64KiB")
	includeResponse := fs.Bool("include-response", false, "include truncated response body preview")
	probeID := fs.String("probe-id", "", "optional probe id")
	host := fs.String("host", "", "override gateway host used for log correlation")
	uri := fs.String("uri", "", "override URI used for log correlation")
	status := fs.String("status", "", "override status used for log correlation")
	traceID := fs.String("trace-id", "", "trace id keyword for service/APISIX log correlation")
	namespace := fs.String("namespace", "", "optional pod namespace for Kubernetes evidence")
	fs.StringVar(namespace, "n", "", "optional pod namespace for Kubernetes evidence")
	pod := fs.String("pod", "", "optional pod name for Kubernetes evidence")
	source := fs.String("source", "", "optional Prometheus source for pod metrics")
	window := fs.Int("window", 300, "seconds before and after probe time for log correlation")
	limit := fs.Int("limit", 20, "result limit per log evidence section")
	apisixIndex := fs.String("apisix-index", "", "APISIX Elasticsearch index pattern")
	serviceIndex := fs.String("service-index", "", "service log Elasticsearch index pattern")
	serviceURIField := fs.String("service-uri-field", "", "service log field containing URI text")
	skipLogs := fs.Bool("skip-logs", false, "skip APISIX/service log correlation")
	skipAPISIX := fs.Bool("skip-apisix", false, "skip APISIX lookup")
	serviceOnly := fs.Bool("service-only", false, "alias for --skip-apisix")
	persist := fs.Bool("persist", false, "persist evidence pack on the server")
	var headers repeatedFlags
	var keywords repeatedFlags
	fs.Var(&headers, "header", "request header, repeatable, format 'Name: value'")
	fs.Var(&keywords, "keyword", "extra keyword for service log correlation, repeatable")
	_ = fs.Parse(args[1:])

	values := url.Values{
		"method":            {*method},
		"url":               {*targetURL},
		"body":              {*bodyText},
		"timeout_seconds":   {strconv.Itoa(*timeout)},
		"body_limit_bytes":  {strconv.Itoa(*bodyLimit)},
		"include_response":  {strconv.FormatBool(*includeResponse)},
		"probe_id":          {*probeID},
		"host":              {*host},
		"uri":               {*uri},
		"status":            {*status},
		"trace_id":          {*traceID},
		"namespace":         {*namespace},
		"pod":               {*pod},
		"source":            {*source},
		"window_seconds":    {strconv.Itoa(*window)},
		"limit":             {strconv.Itoa(*limit)},
		"apisix_index":      {*apisixIndex},
		"service_index":     {*serviceIndex},
		"service_uri_field": {*serviceURIField},
		"skip_logs":         {strconv.FormatBool(*skipLogs)},
		"skip_apisix":       {strconv.FormatBool(*skipAPISIX || *serviceOnly)},
		"persist":           {strconv.FormatBool(*persist)},
	}
	for _, header := range headers {
		values.Add("header", header)
	}
	for _, keyword := range keywords {
		values.Add("keyword", keyword)
	}
	body, err := post(opts.backendURL, "/api/probe/http", addCluster(values, opts.cluster))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return fmt.Errorf("probe response missing data")
	}
	return writeOutput(out, opts.output, data, writeProbeHuman(data, stringList(payload["warnings"])))
}

func writeProbeHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		probe := mapValue(data, "probe")
		pack := mapValue(data, "evidence_pack")
		correlation := mapValue(data, "correlation")
		fmt.Fprintf(w, "Probe: id=%s %s %s status=%d duration_ms=%d\n",
			stringValue(probe["probe_id"]),
			stringValue(probe["method"]),
			stringValue(probe["url"]),
			intValue(probe["status_code"]),
			intValue(probe["duration_ms"]),
		)
		if errText := stringValue(probe["error"]); errText != "" {
			fmt.Fprintf(w, "Error: %s\n", errText)
		}
		if correlation != nil {
			fmt.Fprintf(w, "Correlation: mode=%s strength=%s gaps=%s\n",
				stringValue(correlation["investigation_mode"]),
				stringValue(correlation["evidence_strength"]),
				strings.Join(stringList(correlation["gaps"]), ","),
			)
		} else {
			fmt.Fprintln(w, "Correlation: unavailable")
		}
		fmt.Fprintf(w, "Evidence Pack: id=%s status=%s trigger=%s\n",
			stringValue(pack["id"]),
			stringValue(pack["status"]),
			stringValue(pack["trigger"]),
		)
		if summary := stringValue(pack["summary"]); summary != "" {
			fmt.Fprintf(w, "Summary: %s\n", summary)
		}
		if len(warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(warnings, "; "))
		}
		return nil
	}
}
