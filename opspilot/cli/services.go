package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"text/tabwriter"
)

func runServicesCatalog(opts globalOptions, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] != "catalog" && args[0] != "list" {
		return fmt.Errorf("expected services subcommand: catalog")
	}
	body, err := get(opts.backendURL, "/api/services/catalog", url.Values{})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return fmt.Errorf("services catalog response missing data")
	}
	return writeOutput(out, opts.output, data, writeServicesCatalogHuman(data, stringList(payload["warnings"])))
}

func writeServicesCatalogHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Services catalog: source=%s count=%d\n", stringValue(data["source"]), intValue(data["count"]))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tNAMESPACE\tDEPLOYMENT\tREPO\tOWNER\tMIDDLEWARE\tCONFIG\tRELEASE")
		for _, item := range mapsFromItems(data["items"]) {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%t\n",
				stringValue(item["name"]),
				stringValue(item["namespace"]),
				stringValue(item["deployment"]),
				stringValue(item["repo"]),
				stringValue(item["owner"]),
				strings.Join(stringList(item["middleware"]), ","),
				strings.Join(stringList(item["config_sources"]), ","),
				boolValue(item["release_mapped"]),
			)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if len(warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(warnings, "; "))
		}
		return nil
	}
}
