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

func runFlowsCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		args = []string{"catalog"}
	}
	switch args[0] {
	case "catalog", "list":
		return runFlowsEndpoint(opts, "/api/flows/catalog", url.Values{}, out, writeFlowsCatalogHuman)
	case "inspect", "check":
		fs := flag.NewFlagSet("flows inspect", flag.ExitOnError)
		name := fs.String("name", "", "flow name")
		stage := fs.String("stage", "", "optional stage name")
		window := fs.String("window", "", "inspection window")
		_ = fs.Parse(args[1:])
		if *name == "" && fs.NArg() > 0 {
			*name = fs.Arg(0)
		}
		return runFlowsEndpoint(opts, "/api/flows/inspect", url.Values{
			"name":   {*name},
			"stage":  {*stage},
			"window": {*window},
		}, out, writeFlowInspectHuman)
	default:
		return fmt.Errorf("expected flows subcommand: catalog or inspect")
	}
}

func runFlowsEndpoint(opts globalOptions, endpoint string, values url.Values, out io.Writer, table func(map[string]any, []string) func(io.Writer) error) error {
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
		return fmt.Errorf("flows response missing data")
	}
	return writeOutput(out, opts.output, data, table(data, stringList(payload["warnings"])))
}

func writeFlowsCatalogHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Flows: source=%s count=%d\n", stringValue(data["source"]), intValue(data["count"]))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tCLUSTER\tREGION\tTYPE\tSERVICE\tSTAGES\tMATCH_KEYS")
		for _, item := range mapsFromItems(data["items"]) {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
				stringValue(item["name"]),
				stringValue(item["cluster"]),
				stringValue(item["region"]),
				stringValue(item["type"]),
				stringValue(item["service"]),
				intValue(item["stage_count"]),
				strings.Join(stringList(item["match_keys"]), ","),
			)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		writeMessages(w, "Warnings", warnings)
		return nil
	}
}

func writeFlowInspectHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Flow: name=%s cluster=%s ready=%t window=%s\n",
			stringValue(data["name"]), stringValue(data["cluster"]), boolValue(data["ready"]), stringValue(data["window"]))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "STAGE\tTYPE\tSERVICE\tWORKLOAD\tCONTAINER\tTOPIC\tDATASOURCE\tEVIDENCE\tSTATUS")
		for _, item := range mapsFromItems(data["stages"]) {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				stringValue(item["name"]),
				stringValue(item["type"]),
				stringValue(item["service"]),
				stringValue(item["workload"]),
				stringValue(item["default_container"]),
				stringValue(item["topic"]),
				stringValue(item["datasource"]),
				strings.Join(stringList(item["evidence_sources"]), ","),
				stringValue(item["status"]),
			)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		writeMessages(w, "Missing evidence", stringList(data["missing_evidence"]))
		writeMessages(w, "Warnings", append(warnings, stringList(data["warnings"])...))
		return nil
	}
}
