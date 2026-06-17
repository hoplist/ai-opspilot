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

func runProfilesCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		args = []string{"status"}
	}
	switch args[0] {
	case "datasources", "status":
		return runProfilesEndpoint(opts, "/api/profiles/status", url.Values{}, out, writeProfilesStatusHuman)
	case "link":
		fs := flag.NewFlagSet("profiles link", flag.ExitOnError)
		source := fs.String("source", "", "Parca datasource name")
		service := fs.String("service", "", "service name")
		namespace := fs.String("namespace", "", "Kubernetes namespace")
		pod := fs.String("pod", "", "Kubernetes pod")
		since := fs.String("since", "10m", "profile time window")
		_ = fs.Parse(args[1:])
		return runProfilesEndpoint(opts, "/api/profiles/link", url.Values{
			"source":    {*source},
			"service":   {*service},
			"namespace": {*namespace},
			"pod":       {*pod},
			"since":     {*since},
		}, out, writeProfilesLinkHuman)
	default:
		return fmt.Errorf("expected profiles subcommand: datasources, status, or link")
	}
}

func runProfilesEndpoint(opts globalOptions, endpoint string, values url.Values, out io.Writer, table func(map[string]any, []string) func(io.Writer) error) error {
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
		return fmt.Errorf("profiles response missing data")
	}
	return writeOutput(out, opts.output, data, table(data, stringList(payload["warnings"])))
}

func writeProfilesStatusHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Profiles: configured=%t ready=%t datasources=%d\n",
			boolValue(data["configured"]), boolValue(data["ready"]), intValue(data["datasource_count"]))
		writeProfileDatasourceRows(w, mapsFromItems(data["datasources"]))
		writeMessages(w, "Missing evidence", stringList(data["missing_evidence"]))
		writeMessages(w, "Warnings", warnings)
		return nil
	}
}

func writeProfilesLinkHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Profile link: configured=%t ready=%t source=%s since=%s\n",
			boolValue(data["configured"]), boolValue(data["ready"]), stringValue(data["source"]), stringValue(data["since"]))
		if query := stringValue(data["query"]); query != "" {
			fmt.Fprintf(w, "Query: %s\n", query)
		}
		if link := stringValue(data["url"]); link != "" {
			fmt.Fprintf(w, "URL: %s\n", link)
		}
		writeMessages(w, "Missing evidence", stringList(data["missing_evidence"]))
		if sources := mapsFromItems(data["datasources"]); len(sources) > 0 && stringValue(data["url"]) == "" {
			writeProfileDatasourceRows(w, sources)
		}
		writeMessages(w, "Warnings", warnings)
		return nil
	}
}

func writeProfileDatasourceRows(w io.Writer, rows []map[string]any) {
	if len(rows) == 0 {
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tCLUSTER\tREGION\tURL\tSTATUS\tERROR")
	for _, item := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\t%s\n",
			stringValue(item["name"]), stringValue(item["cluster"]), stringValue(item["region"]), boolValue(item["url_set"]),
			stringValue(item["status"]), oneLine(strings.TrimSpace(stringValue(item["error"])), 80))
	}
	_ = tw.Flush()
}
