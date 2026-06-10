package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/quality"
)

func qualityCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected quality command: run, status, or runner")
	}
	switch args[0] {
	case "run":
		return runQualityRun(opts, args[1:], out)
	case "status":
		return runQualityStatus(opts, args[1:], out)
	case "runner":
		return runQualityRunner(args[1:], out)
	default:
		return fmt.Errorf("unknown quality command: %s", args[0])
	}
}

func runQualityRun(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "service" {
		return fmt.Errorf("expected: quality run service")
	}
	positionalService := ""
	args = args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("quality run service", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	baseURL := fs.String("base-url", "", "override quality base URL")
	cluster := fs.String("cluster", "", "cluster name")
	_ = fs.Parse(args)
	if *service == "" {
		*service = positionalService
	}
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("quality run service requires --service")
	}
	body, err := post(opts.backendURL, "/api/quality/run", addCluster(url.Values{"service": {*service}, "base_url": {*baseURL}}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	data, err := unwrapData(body, "quality run")
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, data, writeQualityHuman("Quality run", data))
}

func runQualityStatus(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "service" {
		return fmt.Errorf("expected: quality status service")
	}
	positionalService := ""
	args = args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("quality status service", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	_ = fs.Parse(args)
	if *service == "" {
		*service = positionalService
	}
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("quality status service requires --service")
	}
	body, err := get(opts.backendURL, "/api/quality/status", addCluster(url.Values{"service": {*service}}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	data, err := unwrapData(body, "quality status")
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, data, writeQualityHuman("Quality status", data))
}

func runQualityRunner(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("quality runner", flag.ExitOnError)
	configPath := fs.String("config", "", "quality config path, YAML or JSON")
	baseURL := fs.String("base-url", env("OPSPILOT_QUALITY_BASE_URL", ""), "quality base URL override")
	_ = fs.Parse(args)
	cfg, err := readQualityRunnerConfig(*configPath)
	if err != nil {
		return err
	}
	report := quality.Run(context.Background(), cfg, *baseURL, nil)
	return quality.WriteReport(out, report)
}

func readQualityRunnerConfig(path string) (quality.Config, error) {
	if raw := env("OPSPILOT_QUALITY_CONFIG_JSON", ""); raw != "" {
		return quality.ParseJSON(raw)
	}
	if path == "" {
		return quality.DefaultConfig(), nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return quality.Config{}, err
	}
	text := string(body)
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		return quality.ParseJSON(text)
	}
	return quality.ParseYAML(text)
}

func writeQualityHuman(title string, data map[string]any) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "%s: service=%s status=%s optional=%t\n",
			title, stringValue(data["service"]), stringValue(data["status"]), boolValue(data["optional"]))
		if reason := stringValue(data["reason"]); reason != "" {
			fmt.Fprintf(w, "Reason: %s\n", reason)
		}
		if namespace := stringValue(data["namespace"]); namespace != "" {
			fmt.Fprintf(w, "Namespace: %s\n", namespace)
		}
		if jobName := firstNonEmptyString(stringValue(data["job_name"]), stringValue(mapValue(data, "job")["name"])); jobName != "" {
			fmt.Fprintf(w, "Job: %s\n", jobName)
		}
		if report := mapValue(data, "report"); report != nil {
			fmt.Fprintf(w, "Report: status=%s checks=%d passed=%d failed=%d duration=%dms\n",
				stringValue(report["status"]), intValue(report["check_count"]), intValue(report["passed_count"]), intValue(report["failed_count"]), intValue(report["duration_ms"]))
			if summary := stringValue(report["summary"]); summary != "" {
				fmt.Fprintf(w, "Summary: %s\n", summary)
			}
		}
		if checks := stringList(data["next_checks"]); len(checks) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(checks, "; "))
		}
		if logsTail := stringValue(data["logs_tail"]); logsTail != "" {
			fmt.Fprintf(w, "Logs tail:\n%s\n", logsTail)
		}
		return nil
	}
}
