package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"text/tabwriter"
)

func runAssetsCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		args = []string{"catalog"}
	}
	switch args[0] {
	case "zones", "zone":
		return runAssetsEndpoint(opts, "/api/assets/zones", url.Values{}, out, writeAssetsZonesHuman)
	case "catalog", "list":
		return runAssetsEndpoint(opts, "/api/assets/catalog", url.Values{}, out, writeAssetsCatalogHuman)
	case "inspect":
		fs := flag.NewFlagSet("assets inspect", flag.ExitOnError)
		ip := fs.String("ip", "", "IP address to classify")
		_ = fs.Parse(args[1:])
		if *ip == "" && fs.NArg() > 0 {
			*ip = fs.Arg(0)
		}
		if *ip == "" {
			return fmt.Errorf("assets inspect requires --ip")
		}
		return runAssetsEndpoint(opts, "/api/assets/inspect", url.Values{"ip": {*ip}}, out, writeAssetsInspectHuman)
	case "diff":
		return runAssetsEndpoint(opts, "/api/assets/diff", url.Values{}, out, writeAssetsDiffHuman)
	case "sync-plan", "sync":
		fs := flag.NewFlagSet("assets sync-plan", flag.ExitOnError)
		source := fs.String("source", "", "optional CMDB/JMS asset source name")
		_ = fs.Parse(args[1:])
		if *source == "" && fs.NArg() > 0 {
			*source = fs.Arg(0)
		}
		return runAssetsEndpoint(opts, "/api/assets/sync-plan", url.Values{"source": {*source}}, out, writeAssetsSyncPlanHuman)
	default:
		return fmt.Errorf("expected assets subcommand: zones, catalog, inspect, diff, or sync-plan")
	}
}

func runAssetsEndpoint(opts globalOptions, endpoint string, values url.Values, out io.Writer, table func(map[string]any, []string) func(io.Writer) error) error {
	body, err := get(opts.backendURL, endpoint, values)
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return fmt.Errorf("assets response missing data")
	}
	return writeOutput(out, opts.output, data, table(data, stringList(payload["warnings"])))
}

func writeAssetsZonesHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Asset zones: count=%d\n", intValue(data["count"]))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tREGION\tZONE\tCIDRS\tENTRYPOINTS\tCOVERAGE\tPOLICY")
		for _, item := range mapsFromItems(data["items"]) {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				stringValue(item["name"]), stringValue(item["region"]), stringValue(item["zone"]),
				strings.Join(stringList(item["cidrs"]), ","),
				strings.Join(stringList(item["entrypoints"]), ","),
				stringValue(item["coverage"]), stringValue(item["action_policy"]))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		writeMessages(w, "Warnings", warnings)
		return nil
	}
}

func writeAssetsCatalogHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		counts := mapValue(data, "counts")
		fmt.Fprintf(w, "Asset catalog: source=%s zones=%d sources=%d assets=%d\n",
			stringValue(data["source"]), intValue(counts["zones"]), intValue(counts["sources"]), intValue(counts["assets"]))
		if zones := mapsFromItems(data["zones"]); len(zones) > 0 {
			fmt.Fprintln(w, "Zones:")
			writeZoneRows(w, zones)
		}
		if sources := mapsFromItems(data["sources"]); len(sources) > 0 {
			fmt.Fprintln(w, "Sources:")
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tKIND\tZONE\tENABLED\tREQUIRED\tURL\tCOVERAGE\tON_ERROR")
			for _, item := range sources {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%t\t%t\t%s\t%s\n",
					stringValue(item["name"]), stringValue(item["kind"]), stringValue(item["network_zone"]),
					boolValue(item["enabled"]), boolValue(item["required"]), boolValue(item["url_set"]),
					stringValue(item["coverage"]), stringValue(item["on_error"]))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if assets := mapsFromItems(data["assets"]); len(assets) > 0 {
			fmt.Fprintln(w, "Assets:")
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tHOSTNAME\tIPS\tTYPE\tZONE\tBUSINESS_LINE\tOWNER\tSTATUS\tSOURCES\tEXPECTED")
			for _, item := range assets {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					stringValue(item["name"]), stringValue(item["hostname"]), strings.Join(stringList(item["ips"]), ","),
					stringValue(item["asset_type"]), stringValue(item["network_zone"]),
					stringValue(item["business_line"]), stringValue(item["owner"]), stringValue(item["status"]),
					strings.Join(stringList(item["sources"]), ","), strings.Join(stringList(item["expected_sources"]), ","))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		writeMessages(w, "Warnings", append(warnings, stringList(data["warnings"])...))
		return nil
	}
}

func writeAssetsInspectHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Asset inspect: ip=%s\n", stringValue(data["query_ip"]))
		if zone := mapValue(data, "zone"); zone != nil {
			fmt.Fprintf(w, "Zone: %s region=%s type=%s entrypoint=%t coverage=%s policy=%s\n",
				stringValue(zone["name"]), stringValue(zone["region"]), stringValue(zone["zone"]),
				boolValue(data["matched_entrypoint"]), stringValue(zone["coverage"]), stringValue(zone["action_policy"]))
		} else {
			fmt.Fprintln(w, "Zone: unknown")
		}
		if sources := mapsFromItems(data["sources"]); len(sources) > 0 {
			fmt.Fprintln(w, "Sources:")
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tKIND\tENABLED\tREQUIRED\tURL\tCOVERAGE\tON_ERROR")
			for _, item := range sources {
				fmt.Fprintf(tw, "%s\t%s\t%t\t%t\t%t\t%s\t%s\n",
					stringValue(item["name"]), stringValue(item["kind"]), boolValue(item["enabled"]),
					boolValue(item["required"]), boolValue(item["url_set"]), stringValue(item["coverage"]),
					stringValue(item["on_error"]))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if assets := mapsFromItems(data["assets"]); len(assets) > 0 {
			fmt.Fprintf(w, "Matched assets: %d\n", len(assets))
		}
		writeMessages(w, "Missing evidence", stringList(data["missing_evidence"]))
		writeFindingRows(w, mapsFromItems(data["findings"]))
		writeMessages(w, "Advice", stringList(data["advice"]))
		writeMessages(w, "Warnings", warnings)
		return nil
	}
}

func writeAssetsDiffHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Asset diff: mode=%s findings=%d\n", stringValue(data["mode"]), intValue(data["count"]))
		writeFindingRows(w, mapsFromItems(data["findings"]))
		writeMessages(w, "Warnings", append(warnings, stringList(data["warnings"])...))
		return nil
	}
}

func writeAssetsSyncPlanHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "CMDB sync plan: mode=%s source=%s kind=%s ready=%t delete_policy=%s\n",
			stringValue(data["mode"]), stringValue(data["source"]), stringValue(data["kind"]),
			boolValue(data["ready"]), stringValue(data["delete_policy"]))
		writeMessages(w, "Missing evidence", stringList(data["missing_evidence"]))
		writeMessages(w, "Actions", stringList(data["actions"]))
		writeFindingRows(w, mapsFromItems(data["findings"]))
		writeMessages(w, "Validation", stringList(data["validation"]))
		writeMessages(w, "Warnings", append(warnings, stringList(data["warnings"])...))
		return nil
	}
}

func writeZoneRows(w io.Writer, zones []map[string]any) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tREGION\tZONE\tCIDRS\tENTRYPOINTS\tCOVERAGE\tPOLICY")
	for _, item := range zones {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			stringValue(item["name"]), stringValue(item["region"]), stringValue(item["zone"]),
			strings.Join(stringList(item["cidrs"]), ","),
			strings.Join(stringList(item["entrypoints"]), ","),
			stringValue(item["coverage"]), stringValue(item["action_policy"]))
	}
	_ = tw.Flush()
}

func writeFindingRows(w io.Writer, findings []map[string]any) {
	if len(findings) == 0 {
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SEVERITY\tTYPE\tTARGET\tZONE\tSUMMARY\tADVICE")
	for _, item := range findings {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			stringValue(item["severity"]), stringValue(item["type"]), stringValue(item["target"]),
			stringValue(item["network_zone"]), stringValue(item["summary"]), stringValue(item["advice"]))
	}
	_ = tw.Flush()
}
