package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
)

func runConfigCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected config subcommand: validate or status")
	}
	switch args[0] {
	case "validate":
		return runConfigValidate(opts, args[1:], out)
	case "status":
		return runConfigStatus(opts, args[1:], out)
	default:
		return fmt.Errorf("expected config subcommand: validate or status")
	}
}

func runConfigValidate(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("config validate", flag.ExitOnError)
	dir := fs.String("dir", env("OPSPILOT_CONFIG_DIR", ""), "OpsPilot YAML config directory")
	_ = fs.Parse(args)
	cfg := configloader.Load(*dir)
	summary := cfg.Summary()
	err := writeOutput(out, opts.output, summary, writeConfigStatusHuman(summary))
	if err != nil {
		return err
	}
	if !summary.Valid {
		return fmt.Errorf("config validation failed")
	}
	return nil
}

func runConfigStatus(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("config status", flag.ExitOnError)
	_ = fs.Parse(args)
	body, err := get(opts.backendURL, "/api/config/status", url.Values{})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return fmt.Errorf("config status response missing data")
	}
	return writeOutput(out, opts.output, data, writeConfigStatusMapHuman(data, stringList(payload["warnings"])))
}

func writeConfigStatusHuman(summary configloader.Summary) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Config: source=%s valid=%t dir=%s commit=%s\n", summary.Source, summary.Valid, summary.Directory, summary.Commit)
		writeCounts(w, summary.Counts)
		writeMessages(w, "Warnings", summary.Warnings)
		writeMessages(w, "Errors", summary.Errors)
		return nil
	}
}

func writeConfigStatusMapHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Config: source=%s valid=%t dir=%s commit=%s\n",
			stringValue(data["source"]), boolValue(data["valid"]), stringValue(data["directory"]), stringValue(data["commit"]))
		counts := map[string]int{}
		for key, value := range mapValue(data, "counts") {
			counts[key] = intValue(value)
		}
		writeCounts(w, counts)
		writeMessages(w, "Warnings", append(warnings, stringList(data["warnings"])...))
		writeMessages(w, "Errors", stringList(data["errors"]))
		return nil
	}
}

func writeCounts(w io.Writer, counts map[string]int) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ITEM\tCOUNT")
	keys := []string{"services", "datasources", "credentials", "clusters", "agents", "network_zones", "asset_sources", "assets", "flows", "inspections", "topology_regions", "correlation_rules"}
	for _, key := range keys {
		fmt.Fprintf(tw, "%s\t%d\n", key, counts[key])
	}
	_ = tw.Flush()
}

func writeMessages(w io.Writer, label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "%s: %s\n", label, strings.Join(items, "; "))
}
