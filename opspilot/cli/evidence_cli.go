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

func runEvidenceCommand(opts globalOptions, args []string, out io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "pack":
		return true, runEvidencePack(opts, args[1:], out)
	case "packs":
		return true, runEvidencePacksRecent(opts, args[1:], out)
	default:
		return false, nil
	}
}

func runEvidencePack(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("evidence pack", flag.ExitOnError)
	targetType := fs.String("target-type", "", "target type: service, pod, or cluster")
	service := fs.String("service", "", "service name")
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	pod := fs.String("pod", "", "pod name")
	trigger := fs.String("trigger", "manual", "trigger name")
	persist := fs.Bool("persist", false, "persist the generated evidence pack on the server")
	_ = fs.Parse(args)
	body, err := get(opts.backendURL, "/api/evidence/pack", addCluster(url.Values{
		"target_type": {*targetType},
		"service":     {*service},
		"namespace":   {*namespace},
		"pod":         {*pod},
		"trigger":     {*trigger},
		"persist":     {fmt.Sprint(*persist)},
	}, opts.cluster))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return fmt.Errorf("evidence pack response missing data")
	}
	return writeOutput(out, opts.output, data, writeEvidencePackHuman(data, stringList(payload["warnings"])))
}

func runEvidencePacksRecent(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("evidence packs", flag.ExitOnError)
	limit := fs.Int("limit", 20, "result limit")
	_ = fs.Parse(args)
	body, err := get(opts.backendURL, "/api/evidence/packs/recent", url.Values{"limit": {fmt.Sprint(*limit)}})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return fmt.Errorf("evidence packs response missing data")
	}
	return writeOutput(out, opts.output, data, writeEvidencePacksHuman(data))
}

func writeEvidencePackHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		target := mapValue(data, "target")
		fmt.Fprintf(w, "Evidence Pack: id=%s status=%s trigger=%s target=%s/%s\n",
			stringValue(data["id"]),
			stringValue(data["status"]),
			stringValue(data["trigger"]),
			stringValue(target["type"]),
			stringValue(target["name"]),
		)
		if summary := stringValue(data["summary"]); summary != "" {
			fmt.Fprintf(w, "Summary: %s\n", summary)
		}
		if gaps := mapsFromItems(data["missing_evidence"]); len(gaps) > 0 {
			fmt.Fprintln(w, "Missing evidence:")
			for _, gap := range gaps {
				fmt.Fprintf(w, "- %s: %s\n", stringValue(gap["code"]), stringValue(gap["message"]))
			}
		}
		if actions := mapsFromItems(data["recommended_actions"]); len(actions) > 0 {
			fmt.Fprintln(w, "Recommended actions:")
			for _, action := range actions {
				fmt.Fprintf(w, "- [%s] %s\n", stringValue(action["risk"]), stringValue(action["instruction"]))
			}
		}
		if len(warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(warnings, "; "))
		}
		return nil
	}
}

func writeEvidencePacksHuman(data map[string]any) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Evidence Packs: enabled=%t source=%s count=%d\n", boolValue(data["enabled"]), stringValue(data["source"]), intValue(data["count"]))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "TIME\tSTATUS\tTRIGGER\tTARGET\tSUMMARY")
		for _, item := range mapsFromItems(data["items"]) {
			target := mapValue(item, "target")
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s/%s\t%s\n",
				shortTime(stringValue(item["generated_at"])),
				stringValue(item["status"]),
				stringValue(item["trigger"]),
				stringValue(target["type"]),
				stringValue(target["name"]),
				oneLine(stringValue(item["summary"]), 80),
			)
		}
		return tw.Flush()
	}
}
