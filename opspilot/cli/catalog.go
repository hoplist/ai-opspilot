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

func runCredentialsCatalog(opts globalOptions, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "plan" {
		return runCredentialPlan(opts, args[1:], out)
	}
	if len(args) > 0 && args[0] == "access" {
		return runCredentialLifecyclePlan(opts, "/api/credentials/access", args[1:], out, true)
	}
	if len(args) > 0 && args[0] == "revoke" {
		return runCredentialLifecyclePlan(opts, "/api/credentials/revoke", args[1:], out, false)
	}
	if len(args) > 0 && args[0] == "rotate" {
		return runCredentialLifecyclePlan(opts, "/api/credentials/rotate", args[1:], out, false)
	}
	if len(args) > 0 && args[0] != "catalog" && args[0] != "list" {
		return fmt.Errorf("expected credentials subcommand: catalog, plan, access, revoke, or rotate")
	}
	body, err := get(opts.backendURL, "/api/credentials/catalog", url.Values{})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return fmt.Errorf("credentials catalog response missing data")
	}
	return writeOutput(out, opts.output, data, writeCredentialsCatalogHuman(data, stringList(payload["warnings"])))
}

func runCredentialLifecyclePlan(opts globalOptions, endpoint string, args []string, out io.Writer, temporary bool) error {
	fs := flag.NewFlagSet("credentials lifecycle", flag.ExitOnError)
	kind := fs.String("kind", "", "credential kind, for example mysql, redis, s3")
	service := fs.String("service", "", "service name")
	name := fs.String("name", "", "credential/catalog name")
	cluster := fs.String("cluster", "", "cluster name")
	environment := fs.String("environment", "test", "environment name")
	scope := fs.String("scope", "", "credential scope")
	mode := fs.String("mode", "", "access mode, for example readonly")
	ttl := fs.String("ttl", "", "temporary access TTL, for example 2h")
	_ = fs.Parse(args)
	if *kind == "" && fs.NArg() > 0 {
		*kind = fs.Arg(0)
	}
	if temporary {
		if *kind == "" {
			*kind = "mysql"
		}
		if *mode == "" {
			*mode = "readonly"
		}
		if *ttl == "" {
			*ttl = "2h"
		}
	}
	body, err := get(opts.backendURL, endpoint, url.Values{
		"kind":        {*kind},
		"service":     {*service},
		"name":        {*name},
		"cluster":     {*cluster},
		"environment": {*environment},
		"scope":       {*scope},
		"mode":        {*mode},
		"ttl":         {*ttl},
	})
	if err != nil {
		return err
	}
	return writeRegistrationPlanOutput(out, opts.output, body)
}

func runCredentialPlan(opts globalOptions, args []string, out io.Writer) error {
	planType := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		planType = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("credentials plan", flag.ExitOnError)
	kind := fs.String("kind", "", "credential kind, for example mysql, redis, s3")
	service := fs.String("service", "", "service name")
	name := fs.String("name", "", "planned secret/catalog name")
	cluster := fs.String("cluster", "", "cluster name")
	environment := fs.String("environment", "test", "environment name")
	scope := fs.String("scope", "", "credential scope")
	mode := fs.String("mode", "", "access mode, for example readonly")
	ttl := fs.String("ttl", "", "temporary access TTL, for example 2h")
	_ = fs.Parse(args)
	if *kind == "" && fs.NArg() > 0 {
		*kind = fs.Arg(0)
	}
	switch planType {
	case "app-db", "database", "db":
		if *kind == "" {
			*kind = "mysql"
		}
	case "debug-access", "debug":
		if *kind == "" {
			*kind = "mysql"
		}
		if *mode == "" {
			*mode = "readonly"
		}
		if *ttl == "" {
			*ttl = "2h"
		}
	case "":
	default:
		if *kind == "" {
			*kind = planType
		}
	}
	if *kind == "" {
		return fmt.Errorf("credentials plan requires --kind")
	}
	body, err := get(opts.backendURL, "/api/credentials/plan", url.Values{
		"kind":        {*kind},
		"service":     {*service},
		"name":        {*name},
		"cluster":     {*cluster},
		"environment": {*environment},
		"scope":       {*scope},
		"mode":        {*mode},
		"ttl":         {*ttl},
	})
	if err != nil {
		return err
	}
	return writeRegistrationPlanOutput(out, opts.output, body)
}

func runClustersCatalog(opts globalOptions, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "plan" {
		return runClusterPlan(opts, args[1:], out)
	}
	if len(args) > 0 && args[0] != "catalog" && args[0] != "list" {
		return fmt.Errorf("expected clusters subcommand: catalog or plan")
	}
	body, err := get(opts.backendURL, "/api/clusters/catalog", url.Values{})
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return fmt.Errorf("clusters catalog response missing data")
	}
	return writeOutput(out, opts.output, data, writeClustersCatalogHuman(data, stringList(payload["warnings"])))
}

func runClusterPlan(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("clusters plan", flag.ExitOnError)
	name := fs.String("name", "", "cluster catalog name")
	cluster := fs.String("cluster", "", "cluster catalog name alias")
	kind := fs.String("kind", "remote", "cluster access kind")
	mode := fs.String("mode", "remote", "cluster access mode")
	environment := fs.String("environment", "test", "environment name")
	scope := fs.String("scope", "", "cluster scope")
	_ = fs.Parse(args)
	if *name == "" && fs.NArg() > 0 {
		*name = fs.Arg(0)
	}
	if *cluster == "" {
		*cluster = *name
	}
	body, err := get(opts.backendURL, "/api/clusters/plan", url.Values{
		"name":        {*name},
		"cluster":     {*cluster},
		"kind":        {*kind},
		"mode":        {*mode},
		"environment": {*environment},
		"scope":       {*scope},
	})
	if err != nil {
		return err
	}
	return writeRegistrationPlanOutput(out, opts.output, body)
}

func runDatasourcePlan(opts globalOptions, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "plan" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("datasources plan", flag.ExitOnError)
	kind := fs.String("kind", "", "datasource kind, for example prometheus, elk, apisix")
	name := fs.String("name", "", "datasource catalog name")
	cluster := fs.String("cluster", "node200-test", "cluster name")
	environment := fs.String("environment", "test", "environment name")
	scope := fs.String("scope", "", "datasource scope")
	_ = fs.Parse(args)
	if *kind == "" && fs.NArg() > 0 {
		*kind = fs.Arg(0)
	}
	if *kind == "" {
		return fmt.Errorf("datasources plan requires --kind")
	}
	body, err := get(opts.backendURL, "/api/datasources/plan", url.Values{
		"kind":        {*kind},
		"name":        {*name},
		"cluster":     {*cluster},
		"environment": {*environment},
		"scope":       {*scope},
	})
	if err != nil {
		return err
	}
	return writeRegistrationPlanOutput(out, opts.output, body)
}

func writeCredentialsCatalogHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Credentials catalog: source=%s count=%d\n", stringValue(data["source"]), intValue(data["count"]))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tCLASS\tSCOPE\tSTORAGE\tNAMESPACE\tUSED BY")
		for _, item := range mapsFromItems(data["items"]) {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				stringValue(item["name"]), stringValue(item["class"]), stringValue(item["scope"]),
				stringValue(item["storage"]), stringValue(item["namespace"]), strings.Join(stringList(item["used_by"]), ","))
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

func writeClustersCatalogHuman(data map[string]any, warnings []string) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Clusters catalog: source=%s count=%d\n", stringValue(data["source"]), intValue(data["count"]))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tENV\tK8S\tPROMETHEUS\tLOGS\tGITOPS\tARGOCD\tREGISTRY")
		for _, item := range mapsFromItems(data["items"]) {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				stringValue(item["name"]), stringValue(item["environment"]), stringValue(item["kubernetes_mode"]),
				stringValue(item["prometheus"]), stringValue(item["logs"]), stringValue(item["gitops_path"]),
				stringValue(item["argocd_namespace"]), stringValue(item["registry"]))
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

func writeRegistrationPlanOutput(out io.Writer, output string, body []byte) error {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return fmt.Errorf("registration plan response missing data")
	}
	return writeOutput(out, output, data, writeRegistrationPlanHuman(data))
}

func writeRegistrationPlanHuman(data map[string]any) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Plan: type=%s kind=%s name=%s risk=%s automation=%s\n",
			stringValue(data["type"]), stringValue(data["kind"]), stringValue(data["name"]),
			stringValue(data["risk"]), stringValue(data["automation"]))
		if summary := stringValue(data["summary"]); summary != "" {
			fmt.Fprintf(w, "Summary: %s\n", summary)
		}
		if service := stringValue(data["service"]); service != "" {
			fmt.Fprintf(w, "Service: %s\n", service)
		}
		if cluster := stringValue(data["cluster"]); cluster != "" {
			fmt.Fprintf(w, "Cluster: %s\n", cluster)
		}
		if mode := stringValue(data["mode"]); mode != "" {
			fmt.Fprintf(w, "Mode: %s\n", mode)
		}
		if ttl := stringValue(data["ttl"]); ttl != "" {
			fmt.Fprintf(w, "TTL: %s\n", ttl)
		}
		if keys := stringList(data["required_keys"]); len(keys) > 0 {
			fmt.Fprintf(w, "Required keys: %s\n", strings.Join(keys, ", "))
		}
		if steps := stringList(data["steps"]); len(steps) > 0 {
			fmt.Fprintln(w, "Steps:")
			for _, step := range steps {
				fmt.Fprintf(w, "- %s\n", step)
			}
		}
		if validation := stringList(data["validation"]); len(validation) > 0 {
			fmt.Fprintf(w, "Validation: %s\n", strings.Join(validation, "; "))
		}
		if warnings := stringList(data["warnings"]); len(warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(warnings, "; "))
		}
		return nil
	}
}
