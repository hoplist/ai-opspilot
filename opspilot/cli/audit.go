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

func runAudit(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected audit subcommand: recent or policy")
	}
	switch args[0] {
	case "recent", "list":
		return runAuditRecent(opts, args[1:], out)
	case "policy":
		return runAuditPolicy(opts, args[1:], out)
	default:
		return fmt.Errorf("unknown audit command: %s", args[0])
	}
}

func runAuditRecent(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("audit recent", flag.ExitOnError)
	actor := fs.String("actor", "", "actor filter")
	action := fs.String("action", "", "action substring filter")
	risk := fs.String("risk", "", "risk filter")
	outcome := fs.String("outcome", "", "outcome filter")
	limit := fs.Int("limit", 50, "result limit")
	_ = fs.Parse(args)
	body, err := get(opts.backendURL, "/api/audit/recent", url.Values{
		"actor":   {*actor},
		"action":  {*action},
		"risk":    {*risk},
		"outcome": {*outcome},
		"limit":   {fmt.Sprint(*limit)},
	})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return fmt.Errorf("audit recent response missing data")
	}
	return writeOutput(out, opts.output, data, writeAuditRecentHuman(data))
}

func runAuditPolicy(opts globalOptions, args []string, out io.Writer) error {
	body, err := get(opts.backendURL, "/api/audit/policy", url.Values{})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return fmt.Errorf("audit policy response missing data")
	}
	return writeOutput(out, opts.output, data, writeAuditPolicyHuman(data))
}

func writeAuditRecentHuman(data map[string]any) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Audit: enabled=%t source=%s count=%d\n", boolValue(data["enabled"]), stringValue(data["source"]), intValue(data["count"]))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "TIME\tACTOR\tRISK\tOUTCOME\tACTION\tTARGET")
		for _, item := range mapsFromItems(data["items"]) {
			target := firstNonEmptyString(stringValue(item["target"]), stringValue(item["namespace"]))
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				shortTime(stringValue(item["time"])),
				stringValue(item["actor"]),
				stringValue(item["risk"]),
				stringValue(item["outcome"]),
				oneLine(stringValue(item["action"]), 40),
				target,
			)
		}
		return tw.Flush()
	}
}

func writeAuditPolicyHuman(data map[string]any) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Audit policy: version=%s\n", stringValue(data["version"]))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "RISK\tAUTOMATION\tEXAMPLES")
		for _, item := range mapsFromItems(data["levels"]) {
			fmt.Fprintf(tw, "%s\t%s\t%s\n",
				stringValue(item["risk"]),
				stringValue(item["automation"]),
				strings.Join(stringList(item["examples"]), ", "),
			)
		}
		return tw.Flush()
	}
}
