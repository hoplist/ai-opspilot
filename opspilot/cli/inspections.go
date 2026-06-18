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

func runInspectionsCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		args = []string{"catalog"}
	}
	switch args[0] {
	case "catalog", "list":
		return runInspectionsEndpoint(opts, "/api/inspections/catalog", url.Values{}, out, writeInspectionsCatalogHuman)
	case "run", "check":
		fs := flag.NewFlagSet("inspection run", flag.ExitOnError)
		name := fs.String("name", "", "inspection policy name")
		cluster := fs.String("cluster", opts.cluster, "optional cluster filter")
		_ = fs.Parse(args[1:])
		if *name == "" && fs.NArg() > 0 {
			*name = fs.Arg(0)
		}
		return runInspectionsEndpoint(opts, "/api/inspections/run", url.Values{
			"name":    {*name},
			"cluster": {*cluster},
		}, out, writeInspectionRunHuman)
	case "generate", "draft":
		fs := flag.NewFlagSet("inspection generate", flag.ExitOnError)
		cluster := fs.String("cluster", opts.cluster, "cluster name")
		service := fs.String("service", "", "optional service name")
		_ = fs.Parse(args[1:])
		return runInspectionsEndpoint(opts, "/api/inspections/generate", url.Values{
			"cluster": {*cluster},
			"service": {*service},
		}, out, writeInspectionGenerateHuman)
	default:
		return fmt.Errorf("expected inspection subcommand: catalog, run, or generate")
	}
}

func runInspectionsEndpoint(opts globalOptions, endpoint string, values url.Values, out io.Writer, table func(map[string]any, []string) func(io.Writer) error) error {
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
		return fmt.Errorf("inspections response missing data")
	}
	return writeOutput(out, opts.output, data, table(data, stringList(payload["warnings"])))
}

func writeInspectionsCatalogHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Inspections: source=%s count=%d\n", stringValue(data["source"]), intValue(data["count"]))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tCLUSTER\tREGION\tSCHEDULE\tSERVICES\tFLOWS\tCHECKS")
		for _, item := range mapsFromItems(data["items"]) {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
				stringValue(item["name"]),
				stringValue(item["cluster"]),
				stringValue(item["region"]),
				stringValue(item["schedule"]),
				strings.Join(stringList(item["services"]), ","),
				strings.Join(stringList(item["flows"]), ","),
				intValue(item["check_count"]),
			)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		writeMessages(w, "Warnings", warnings)
		return nil
	}
}

func writeInspectionRunHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Inspection: name=%s cluster=%s ready=%t schedule=%s\n",
			stringValue(data["name"]), stringValue(data["cluster"]), boolValue(data["ready"]), stringValue(data["schedule"]))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "CHECK\tTYPE\tENABLED\tDATASOURCE\tSTATUS\tGAPS")
		for _, item := range mapsFromItems(data["checks"]) {
			fmt.Fprintf(tw, "%s\t%s\t%t\t%s\t%s\t%s\n",
				stringValue(item["name"]),
				stringValue(item["type"]),
				boolValue(item["enabled"]),
				stringValue(item["datasource"]),
				stringValue(item["status"]),
				strings.Join(stringList(item["missing_evidence"]), ","),
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

func writeInspectionGenerateHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Inspection draft: cluster=%s service=%s ready=%t\n",
			stringValue(data["cluster"]), stringValue(data["service"]), boolValue(data["ready"]))
		if yaml := stringValue(data["yaml"]); yaml != "" {
			fmt.Fprintln(w, yaml)
		}
		writeMessages(w, "Missing evidence", stringList(data["missing_evidence"]))
		writeMessages(w, "Warnings", append(warnings, stringList(data["warnings"])...))
		return nil
	}
}
