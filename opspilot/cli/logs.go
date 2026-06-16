package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
)

func runLogsCommand(opts globalOptions, args []string, out io.Writer) (bool, error) {
	if len(args) == 0 || args[0] != "route" {
		return false, nil
	}
	values := logsRouteValues(args[1:])
	body, err := get(opts.backendURL, "/api/logs/route", values)
	if err != nil {
		return true, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return true, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return true, fmt.Errorf("logs route response missing data")
	}
	return true, writeOutput(out, opts.output, data, writeLogsRouteHuman(data, stringList(payload["warnings"])))
}

func logsRouteValues(args []string) url.Values {
	fs := flag.NewFlagSet("logs route", flag.ExitOnError)
	service := fs.String("service", "", "service catalog name")
	host := fs.String("host", "", "gateway host/domain")
	region := fs.String("region", "", "region name")
	cluster := fs.String("cluster", "", "cluster name")
	global := fs.Bool("global", false, "include all log datasources after local route candidates")
	_ = fs.Parse(args)
	return url.Values{
		"service": []string{*service},
		"host":    []string{*host},
		"region":  []string{*region},
		"cluster": []string{*cluster},
		"global":  []string{strconv.FormatBool(*global)},
	}
}

func writeLogsRouteHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Logs route: status=%s\n", stringValue(data["status"]))
		if service := mapValue(data, "service"); service != nil {
			fmt.Fprintf(w, "Service: %s cluster=%s namespace=%s deployment=%s\n",
				stringValue(service["name"]), stringValue(service["cluster"]), stringValue(service["namespace"]), stringValue(service["deployment"]))
		}
		if selected := mapValue(data, "selected"); selected != nil {
			fmt.Fprintf(w, "Selected: %s kind=%s region=%s confidence=%s\n",
				stringValue(selected["name"]), stringValue(selected["kind"]), stringValue(selected["region"]), stringValue(selected["confidence"]))
		}
		candidates := mapsFromItems(data["candidates"])
		if len(candidates) > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tKIND\tREGION\tCONFIDENCE\tAPISIX\tSERVICE_INDEXES\tREASON")
			for _, item := range candidates {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					stringValue(item["name"]),
					stringValue(item["kind"]),
					stringValue(item["region"]),
					stringValue(item["confidence"]),
					stringValue(item["apisix_index"]),
					strings.Join(stringList(item["service_indexes"]), ","),
					stringValue(item["reason"]),
				)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if ui := mapsFromItems(data["ui_candidates"]); len(ui) > 0 {
			names := []string{}
			for _, item := range ui {
				names = append(names, stringValue(item["name"]))
			}
			fmt.Fprintf(w, "UI metadata: %s\n", strings.Join(names, ", "))
		}
		if gaps := stringList(data["gaps"]); len(gaps) > 0 {
			fmt.Fprintf(w, "Gaps: %s\n", strings.Join(gaps, "; "))
		}
		if warnings != nil && len(warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(warnings, "; "))
		}
		return nil
	}
}
