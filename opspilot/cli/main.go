package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/contracts"
	intentpkg "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/intent"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/quality"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/version"
)

const defaultBackend = "http://127.0.0.1:18080"
const realFilesystemFilter = `fstype!~"tmpfs|overlay|squashfs|ramfs|cgroup2?|proc|sysfs|devtmpfs|devpts|securityfs|pstore|bpf|tracefs|debugfs|configfs|fusectl|mqueue|hugetlbfs"`

type globalOptions struct {
	backendURL string
	output     string
	cluster    string
}

const defaultPodLogSinceSeconds = 36000

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, `{"ok":false,"error":`+strconv.Quote(err.Error())+`}`)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	opts := globalOptions{
		backendURL: env("OPSPILOT_BACKEND_URL", defaultBackend),
		output:     env("OPSPILOT_OUTPUT", "json"),
		cluster:    env("OPSPILOT_CLUSTER", ""),
	}
	args = consumeGlobalFlags(args, &opts)
	if len(args) == 0 {
		return fmt.Errorf("missing command")
	}
	var endpoint string
	var values url.Values
	switch args[0] {
	case "--version", "-version", "version":
		fmt.Fprintln(out, version.Version)
		return nil
	case "schema":
		_, err := out.Write(contracts.CLISchema)
		if err == nil {
			_, err = fmt.Fprintln(out)
		}
		return err
	case "capabilities", "capability":
		return runCapabilities(opts, args[1:], out)
	case "doctor":
		return runDoctor(opts, args[1:], out)
	case "skills":
		return runSkillsRegistry(opts, args[1:], out)
	case "credentials", "credential":
		return runCredentialsCatalog(opts, args[1:], out)
	case "datasources", "datasource":
		return runDatasourcePlan(opts, args[1:], out)
	case "clusters", "cluster":
		return runClustersCatalog(opts, args[1:], out)
	case "inventory":
		endpoint, values = inventoryCommand(args[1:])
	case "metrics":
		if len(args) > 1 && args[1] == "filesystems" {
			return runMetricsFilesystems(opts, args[2:], out)
		}
		endpoint, values = metricsCommand(args[1:])
	case "k8s":
		endpoint, values = k8sCommand(args[1:])
	case "docker":
		endpoint, values = dockerCommand(args[1:])
	case "logs":
		endpoint, values = logsCommand(args[1:])
	case "evidence":
		endpoint, values = evidenceCommand(args[1:])
	case "errors":
		endpoint, values = errorsCommand(args[1:])
	case "ask", "nl":
		return runNaturalLanguage(opts, args[1:], out)
	case "release":
		if len(args) > 1 && args[1] == "service" {
			return runReleaseService(opts, args[2:], out)
		}
		if len(args) > 1 && args[1] == "trigger" {
			return runReleaseService(opts, append([]string{"--trigger"}, args[2:]...), out)
		}
		if len(args) > 1 && !knownReleaseCommand(args[1]) {
			return runReleaseService(opts, args[1:], out)
		}
		if len(args) > 1 && args[1] == "status" {
			return runReleaseStatus(opts, args[2:], out)
		}
		if len(args) > 1 && args[1] == "jobs" {
			return runReleaseJobs(opts, args[2:], out)
		}
		if len(args) > 1 && args[1] == "logs" {
			return runReleaseLogs(opts, args[2:], out)
		}
		if len(args) > 1 && args[1] == "history" {
			return runReleaseHistory(opts, args[2:], out)
		}
		if len(args) > 1 && args[1] == "rollback" {
			return runReleaseRollback(opts, args[2:], out)
		}
		endpoint, values = releaseCommand(args[1:])
	case "quality":
		return qualityCommand(opts, args[1:], out)
	case "context":
		endpoint, values = podRefCommand(args[1:], "/api/context/pod")
	case "diagnose":
		endpoint, values = diagnoseCommand(args[1:])
	case "inspect", "check":
		return inspectCommand(opts, args[1:], out)
	case "fix":
		return fixCommand(opts, args[1:], out)
	case "janitor":
		return janitorCommand(opts, args[1:], out)
	case "healer":
		return healerCommand(opts, args[1:], out)
	case "app":
		return appCommand(opts, args[1:], out)
	case "onboard":
		return onboardCommand(opts, args[1:], out)
	case "repo":
		return repoCommand(opts, args[1:], out)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
	body, err := get(opts.backendURL, endpoint, addCluster(values, opts.cluster))
	if err != nil {
		return err
	}
	_, err = out.Write(body)
	if err == nil {
		_, err = fmt.Fprintln(out)
	}
	return err
}

func knownReleaseCommand(command string) bool {
	switch command {
	case "status", "evidence", "diagnose", "jobs", "logs", "history", "rollback", "service", "trigger":
		return true
	default:
		return false
	}
}

type naturalLanguageResult struct {
	Query    string   `json:"query"`
	Action   string   `json:"action"`
	Service  string   `json:"service,omitempty"`
	Command  []string `json:"command"`
	Executed bool     `json:"executed"`
	DryRun   bool     `json:"dry_run"`
	Message  string   `json:"message,omitempty"`
	Result   any      `json:"result,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type naturalLanguageIntent = intentpkg.Intent

type capabilityItem struct {
	Name              string         `json:"name"`
	Label             string         `json:"label"`
	Category          string         `json:"category"`
	Configured        bool           `json:"configured"`
	Ready             bool           `json:"ready"`
	Available         bool           `json:"available"`
	Status            string         `json:"status"`
	AvailableEvidence []string       `json:"available_evidence,omitempty"`
	MissingEvidence   []string       `json:"missing_evidence,omitempty"`
	Message           string         `json:"message,omitempty"`
	Details           map[string]any `json:"details,omitempty"`
}

type capabilityResult struct {
	Ready             bool             `json:"ready"`
	Capabilities      []capabilityItem `json:"capabilities"`
	AvailableEvidence []string         `json:"available_evidence,omitempty"`
	MissingEvidence   []string         `json:"missing_evidence,omitempty"`
	Warnings          []string         `json:"warnings,omitempty"`
	Summary           map[string]any   `json:"summary,omitempty"`
	Raw               map[string]any   `json:"raw,omitempty"`
}

type doctorResult struct {
	Ready             bool           `json:"ready"`
	BackendURL        string         `json:"backend_url"`
	BackendReachable  bool           `json:"backend_reachable"`
	BackendVersion    string         `json:"backend_version,omitempty"`
	CapabilitiesReady bool           `json:"capabilities_ready"`
	AvailableEvidence []string       `json:"available_evidence,omitempty"`
	MissingEvidence   []string       `json:"missing_evidence,omitempty"`
	Warnings          []string       `json:"warnings,omitempty"`
	Findings          []string       `json:"findings"`
	Next              []string       `json:"next"`
	Raw               map[string]any `json:"raw,omitempty"`
}

func runDoctor(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	cluster := fs.String("cluster", "", "cluster name")
	_ = fs.Parse(args)
	if *cluster == "" {
		*cluster = opts.cluster
	}
	result := doctorResult{BackendURL: opts.backendURL, Raw: map[string]any{}}
	health, err := getJSONMap(opts.backendURL, "/api/health", url.Values{})
	if err != nil {
		result.Findings = append(result.Findings, "Backend is not reachable: "+err.Error())
		result.Next = append(result.Next, "Check OPSPILOT_BACKEND_URL or --backend-url.")
		result.MissingEvidence = append(result.MissingEvidence, "backend_health")
		return writeOutput(out, opts.output, result, writeDoctorHuman(result))
	}
	result.BackendReachable = true
	result.Raw["health"] = health
	if data := mapValue(health, "data"); data != nil {
		result.BackendVersion = stringValue(data["version"])
	}
	capabilities, err := fetchCapabilities(opts.backendURL, *cluster)
	if err != nil {
		result.Findings = append(result.Findings, "Capabilities endpoint failed: "+err.Error())
		result.Next = append(result.Next, "Check opspilot-core logs and Kubernetes API permissions.")
		result.MissingEvidence = append(result.MissingEvidence, "capabilities")
	} else {
		result.CapabilitiesReady = capabilities.Ready
		result.AvailableEvidence = capabilities.AvailableEvidence
		result.MissingEvidence = capabilities.MissingEvidence
		result.Warnings = append(result.Warnings, capabilities.Warnings...)
		result.Raw["capabilities"] = capabilities.Raw
		if capabilities.Ready {
			result.Findings = append(result.Findings, "Backend and core inspection capabilities are reachable.")
		} else {
			result.Findings = append(result.Findings, "Backend is reachable, but some evidence sources are missing.")
		}
	}
	if len(result.MissingEvidence) > 0 {
		result.Next = append(result.Next, "Continue with available evidence; report missing integrations explicitly.")
	}
	if len(result.Next) == 0 {
		result.Next = append(result.Next, "Run check cluster, check pod, or check service based on the user request.")
	}
	result.Ready = result.BackendReachable && result.CapabilitiesReady
	return writeOutput(out, opts.output, result, writeDoctorHuman(result))
}

func writeDoctorHuman(result doctorResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Doctor: ready=%t backend=%s reachable=%t version=%s\n", result.Ready, result.BackendURL, result.BackendReachable, result.BackendVersion)
		if len(result.Findings) > 0 {
			fmt.Fprintf(w, "Findings: %s\n", strings.Join(result.Findings, "; "))
		}
		if len(result.AvailableEvidence) > 0 {
			fmt.Fprintf(w, "Available evidence: %s\n", strings.Join(result.AvailableEvidence, "; "))
		}
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, "; "))
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(result.Next, "; "))
		}
		return nil
	}
}

func runCapabilities(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("capabilities", flag.ExitOnError)
	cluster := fs.String("cluster", "", "cluster name")
	_ = fs.Parse(args)
	if *cluster == "" {
		*cluster = opts.cluster
	}
	result, err := fetchCapabilities(opts.backendURL, *cluster)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, writeCapabilitiesHuman(result))
}

func fetchCapabilities(backendURL, cluster string) (capabilityResult, error) {
	body, err := get(backendURL, "/api/capabilities", addCluster(url.Values{}, cluster))
	if err != nil {
		return capabilityResult{}, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return capabilityResult{}, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return capabilityResult{}, fmt.Errorf("capabilities response missing data")
	}
	raw, _ := json.Marshal(data)
	var result capabilityResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return capabilityResult{}, err
	}
	result.Warnings = append(result.Warnings, stringList(payload["warnings"])...)
	result.Raw = data
	return result, nil
}

func writeCapabilitiesHuman(result capabilityResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Capabilities: ready=%t available=%d missing=%d\n", result.Ready, availableCapabilityCount(result.Capabilities), len(result.Capabilities)-availableCapabilityCount(result.Capabilities))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "CAPABILITY\tSTATUS\tEVIDENCE OR GAP")
		for _, item := range result.Capabilities {
			evidence := strings.Join(item.AvailableEvidence, ", ")
			if !item.Available {
				evidence = strings.Join(item.MissingEvidence, ", ")
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", item.Name, item.Status, oneLine(evidence, 120))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if len(result.AvailableEvidence) > 0 {
			fmt.Fprintf(w, "Available evidence: %s\n", strings.Join(result.AvailableEvidence, "; "))
		}
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, "; "))
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	}
}

type skillsRegistryResult struct {
	Version         string                `json:"version"`
	Source          string                `json:"source"`
	SourcePath      string                `json:"source_path,omitempty"`
	SourceVersion   string                `json:"source_version,omitempty"`
	ItemCount       int                   `json:"item_count"`
	IntegratedCount int                   `json:"integrated_count"`
	DynamicCount    int                   `json:"dynamic_count,omitempty"`
	Items           []skillregistry.Skill `json:"items"`
	Warnings        []string              `json:"warnings,omitempty"`
}

func runSkillsRegistry(opts globalOptions, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] != "registry" && args[0] != "list" {
		return fmt.Errorf("expected skills subcommand: registry")
	}
	if len(args) > 0 {
		args = args[1:]
	}
	fs := flag.NewFlagSet("skills registry", flag.ExitOnError)
	category := fs.String("category", "", "skill category filter")
	integratedOnly := fs.Bool("integrated-only", false, "show only integrated skills")
	_ = fs.Parse(args)

	result, err := fetchSkillsRegistry(opts.backendURL, *category, *integratedOnly)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, writeSkillsRegistryHuman(result))
}

func fetchSkillsRegistry(backendURL, category string, integratedOnly bool) (skillsRegistryResult, error) {
	body, err := get(backendURL, "/api/skills/registry", url.Values{
		"category":        {category},
		"integrated_only": {strconv.FormatBool(integratedOnly)},
	})
	if err != nil {
		return skillsRegistryResult{}, fmt.Errorf("backend skills registry unavailable: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return skillsRegistryResult{}, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return skillsRegistryResult{}, fmt.Errorf("skills registry response missing data")
	}
	raw, _ := json.Marshal(data)
	var catalog skillregistry.Catalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return skillsRegistryResult{}, err
	}
	return skillsResultFromCatalog(catalog, stringList(payload["warnings"])), nil
}

func skillsResultFromCatalog(catalog skillregistry.Catalog, warnings []string) skillsRegistryResult {
	return skillsRegistryResult{
		Version:         catalog.Version,
		Source:          catalog.Source,
		SourcePath:      catalog.SourcePath,
		SourceVersion:   catalog.SourceVersion,
		ItemCount:       catalog.ItemCount,
		IntegratedCount: catalog.IntegratedCount,
		DynamicCount:    catalog.DynamicCount,
		Items:           catalog.Items,
		Warnings:        warnings,
	}
}

func writeSkillsRegistryHuman(result skillsRegistryResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Skills registry: version=%s source=%s items=%d integrated=%d dynamic=%d\n",
			result.Version, result.Source, result.ItemCount, result.IntegratedCount, result.DynamicCount)
		if result.SourcePath != "" {
			fmt.Fprintf(w, "Source path: %s\n", result.SourcePath)
		}
		if result.SourceVersion != "" {
			fmt.Fprintf(w, "Source version: %s\n", result.SourceVersion)
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SKILL\tCATEGORY\tTIER\tINTEGRATED\tCOMMANDS")
		for _, item := range result.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\n",
				item.Name, item.Category, item.IntegrationTier, item.Integrated, oneLine(strings.Join(item.Commands, ", "), 100))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	}
}

func runCredentialsCatalog(opts globalOptions, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "plan" {
		return runCredentialPlan(opts, args[1:], out)
	}
	if len(args) > 0 && args[0] != "catalog" && args[0] != "list" {
		return fmt.Errorf("expected credentials subcommand: catalog or plan")
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

func runCredentialPlan(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("credentials plan", flag.ExitOnError)
	kind := fs.String("kind", "", "credential kind, for example mysql, redis, s3")
	service := fs.String("service", "", "service name")
	name := fs.String("name", "", "planned secret/catalog name")
	cluster := fs.String("cluster", "", "cluster name")
	environment := fs.String("environment", "test", "environment name")
	scope := fs.String("scope", "", "credential scope")
	_ = fs.Parse(args)
	if *kind == "" && fs.NArg() > 0 {
		*kind = fs.Arg(0)
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
	})
	if err != nil {
		return err
	}
	return writeRegistrationPlanOutput(out, opts.output, body)
}

func runClustersCatalog(opts globalOptions, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] != "catalog" && args[0] != "list" {
		return fmt.Errorf("expected clusters subcommand: catalog")
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
		return nil
	}
}

func runNaturalLanguage(opts globalOptions, args []string, out io.Writer) error {
	service := ""
	ref := "main"
	dryRun := false
	queryParts := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--dry-run":
			dryRun = true
		case arg == "--service" && i+1 < len(args):
			service = args[i+1]
			i++
		case strings.HasPrefix(arg, "--service="):
			service = strings.TrimPrefix(arg, "--service=")
		case arg == "--ref" && i+1 < len(args):
			ref = args[i+1]
			i++
		case strings.HasPrefix(arg, "--ref="):
			ref = strings.TrimPrefix(arg, "--ref=")
		default:
			queryParts = append(queryParts, arg)
		}
	}
	query := strings.TrimSpace(strings.Join(queryParts, " "))
	if query == "" {
		return fmt.Errorf("ask requires natural language text")
	}
	intent, warnings := fetchNaturalLanguageIntent(opts.backendURL, query, service)
	if intent.Service == "" {
		result := naturalLanguageResult{
			Query:    query,
			Action:   intent.Action,
			Command:  intent.Command,
			Executed: false,
			DryRun:   dryRun,
			Message:  "service could not be identified from the request",
			Warnings: warnings,
		}
		_ = writeOutput(out, opts.output, result, writeNaturalLanguageHuman(result))
		return fmt.Errorf("service could not be identified from natural language")
	}
	result := naturalLanguageResult{
		Query:    query,
		Action:   intent.Action,
		Service:  intent.Service,
		Command:  intent.Command,
		Executed: false,
		DryRun:   dryRun,
		Warnings: warnings,
	}
	if dryRun {
		result.Message = "dry run only; no action executed"
		return writeOutput(out, opts.output, result, writeNaturalLanguageHuman(result))
	}
	switch intent.Action {
	case "inspect_service":
		payload, err := fetchInspectService(opts.backendURL, intent.Service, "test", "", opts.cluster, 300, defaultPodLogSinceSeconds)
		if err != nil {
			return err
		}
		result.Executed = true
		result.Result = payload
	case "release_service":
		payload, err := triggerReleaseService(opts.backendURL, intent.Service, ref, opts.cluster, nil)
		if err != nil {
			return err
		}
		result.Executed = true
		result.Result = payload
	case "release_history":
		payload, err := fetchReleaseHistoryData(opts.backendURL, intent.Service, opts.cluster, 10)
		if err != nil {
			return err
		}
		result.Executed = true
		result.Result = payload
	case "rollback_service":
		if intent.Target == "" {
			return fmt.Errorf("rollback target could not be identified from natural language")
		}
		payload, err := rollbackReleaseService(opts.backendURL, intent.Service, intent.Target, opts.cluster)
		if err != nil {
			return err
		}
		result.Executed = true
		result.Result = payload
	default:
		return fmt.Errorf("unsupported natural language action: %s", intent.Action)
	}
	return writeOutput(out, opts.output, result, writeNaturalLanguageHuman(result))
}

func writeNaturalLanguageHuman(result naturalLanguageResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Ask: %s\n", result.Query)
		fmt.Fprintf(w, "Intent: %s service=%s executed=%t\n", result.Action, result.Service, result.Executed)
		if len(result.Command) > 0 {
			fmt.Fprintf(w, "Command: opspilot %s\n", strings.Join(result.Command, " "))
		}
		if result.Message != "" {
			fmt.Fprintf(w, "Message: %s\n", result.Message)
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		if result.Result != nil {
			switch payload := result.Result.(type) {
			case inspectServiceResult:
				fmt.Fprintf(w, "Status: %s stage=%s namespace=%s deployment=%s\n", payload.Status, payload.Stage, payload.Namespace, payload.Deployment)
				fmt.Fprintf(w, "Usage: pods=%d restarts=%d CPU %.3f cores memory %.1f MiB\n", payload.PodCount, payload.RestartCount, payload.TotalCPUCore, payload.TotalMemoryMiB)
				if len(payload.Findings) > 0 {
					fmt.Fprintf(w, "Findings: %s\n", strings.Join(payload.Findings, "; "))
				}
				if len(payload.EvidenceGaps) > 0 {
					fmt.Fprintf(w, "Evidence gaps: %s\n", strings.Join(payload.EvidenceGaps, ", "))
				}
				if len(payload.AvailableEvidence) > 0 {
					fmt.Fprintf(w, "Available evidence: %s\n", strings.Join(payload.AvailableEvidence, "; "))
				}
				if len(payload.MissingEvidence) > 0 {
					fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(payload.MissingEvidence, "; "))
				}
				return nil
			case map[string]any:
				if pipeline := mapValue(payload, "pipeline"); pipeline != nil {
					fmt.Fprintf(w, "Pipeline: id=%d status=%s ref=%s sha=%s\n",
						intValue(pipeline["id"]), stringValue(pipeline["status"]), stringValue(pipeline["ref"]), stringValue(pipeline["sha"]))
					if checks := stringList(payload["next_checks"]); len(checks) > 0 {
						fmt.Fprintf(w, "Next: %s\n", strings.Join(checks, "; "))
					}
					return nil
				}
			}
			body, err := json.MarshalIndent(result.Result, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(w, string(body))
		}
		return nil
	}
}

func fetchNaturalLanguageIntent(backendURL, query, service string) (intentpkg.Intent, []string) {
	body, err := get(backendURL, "/api/intent/parse", url.Values{
		"query":   {query},
		"service": {service},
	})
	if err == nil {
		var payload map[string]any
		if jsonErr := json.Unmarshal(body, &payload); jsonErr != nil {
			return intentpkg.Intent{}, []string{"intent parse: " + jsonErr.Error()}
		}
		data := mapValue(payload, "data")
		if data == nil {
			return intentpkg.Intent{}, []string{"intent parse: response missing data"}
		}
		raw, _ := json.Marshal(data)
		var parsed intentpkg.Intent
		if jsonErr := json.Unmarshal(raw, &parsed); jsonErr != nil {
			return intentpkg.Intent{}, []string{"intent parse: " + jsonErr.Error()}
		}
		warnings := append(stringList(payload["warnings"]), parsed.Warnings...)
		return parsed, warnings
	}
	services, warnings := fetchConfiguredServices(backendURL)
	warnings = append(warnings, "backend intent parser unavailable; used CLI compatibility parser: "+err.Error())
	parsed := intentpkg.Interpret(intentpkg.Request{
		Query:           query,
		ServiceOverride: service,
		Services:        services,
	})
	return parsed, append(warnings, parsed.Warnings...)
}

func interpretNaturalLanguage(query, serviceOverride string, services []string) naturalLanguageIntent {
	lower := strings.ToLower(query)
	service := firstNonEmptyString(serviceOverride, serviceFromText(lower, services))
	action := "inspect_service"
	command := []string{"inspect", "service", service}
	if containsAny(lower, []string{"回退", "rollback", "退回"}) {
		target := rollbackTargetFromText(query)
		action = "rollback_service"
		command = []string{"release", "rollback", service, target, "--confirm"}
		return naturalLanguageIntent{Action: action, Service: service, Target: target, Command: command}
	}
	if containsAny(lower, []string{"历史", "history", "版本记录", "发布记录"}) {
		action = "release_history"
		command = []string{"release", "history", service}
		return naturalLanguageIntent{Action: action, Service: service, Command: command}
	}
	if containsAny(lower, []string{"发布", "上线", "release", "deploy", "发版"}) {
		action = "release_service"
		command = []string{"release", "service", service, "--trigger"}
		return naturalLanguageIntent{Action: action, Service: service, Command: command}
	}
	return naturalLanguageIntent{Action: action, Service: service, Command: command}
}

func fetchConfiguredServices(backendURL string) ([]string, []string) {
	body, err := get(backendURL, "/api/health", url.Values{})
	if err != nil {
		return nil, []string{"health: " + err.Error()}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, []string{"health: " + err.Error()}
	}
	data := mapValue(payload, "data")
	release := mapValue(data, "release")
	out := []string{}
	for _, item := range stringList(release["services"]) {
		if item != "" {
			out = append(out, item)
		}
	}
	return out, nil
}

func serviceFromText(text string, services []string) string {
	for _, service := range services {
		if service != "" && strings.Contains(text, strings.ToLower(service)) {
			return service
		}
	}
	matches := regexp.MustCompile(`[a-zA-Z0-9][a-zA-Z0-9._/]*-[a-zA-Z0-9][a-zA-Z0-9._/-]*`).FindAllString(text, -1)
	if len(matches) > 0 {
		return strings.Trim(matches[0], `"'.,，。;；:：`)
	}
	return ""
}

func rollbackTargetFromText(query string) string {
	fields := strings.Fields(query)
	for i, field := range fields {
		lower := strings.ToLower(strings.Trim(field, `"'.,，。;；:：`))
		if (lower == "到" || lower == "to" || lower == "target") && i+1 < len(fields) {
			return strings.Trim(fields[i+1], `"'.,，。;；:：`)
		}
	}
	return ""
}

func errorsCommand(args []string) (string, url.Values) {
	if len(args) == 0 || args[0] != "recent" {
		fail("expected: errors recent")
	}
	fs := flag.NewFlagSet("errors recent", flag.ExitOnError)
	source := fs.String("source", "", "error source: kubernetes, argocd, release, middleware")
	service := fs.String("service", "", "service name")
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	limit := fs.Int("limit", 20, "result limit")
	_ = fs.Parse(args[1:])
	return "/api/errors/recent", url.Values{
		"source":    []string{*source},
		"service":   []string{*service},
		"namespace": []string{*namespace},
		"limit":     []string{strconv.Itoa(*limit)},
	}
}

func evidenceCommand(args []string) (string, url.Values) {
	if len(args) == 0 || args[0] != "request" {
		fail("expected: evidence request")
	}
	fs := flag.NewFlagSet("evidence request", flag.ExitOnError)
	host := fs.String("host", "", "gateway host/domain")
	uri := fs.String("uri", "", "request uri")
	at := fs.String("at", "", "center time, RFC3339 or yyyy-mm-dd HH:MM[:SS]")
	since := fs.Int("since", 900, "look back seconds when --at is empty")
	window := fs.Int("window", 300, "seconds before and after --at")
	limit := fs.Int("limit", 20, "result limit per evidence section")
	includeOptions := fs.Bool("include-options", false, "include CORS OPTIONS requests")
	skipAPISIX := fs.Bool("skip-apisix", false, "skip APISIX lookup and run service-log-only investigation")
	serviceOnly := fs.Bool("service-only", false, "alias for --skip-apisix")
	apisixIndex := fs.String("apisix-index", "", "APISIX Elasticsearch index pattern")
	serviceIndex := fs.String("service-index", "", "service log Elasticsearch index pattern")
	serviceURIField := fs.String("service-uri-field", "", "service log field containing URI text")
	_ = fs.Parse(args[1:])
	return "/api/evidence/request", url.Values{
		"host":              []string{*host},
		"uri":               []string{*uri},
		"at":                []string{*at},
		"since_seconds":     []string{strconv.Itoa(*since)},
		"window_seconds":    []string{strconv.Itoa(*window)},
		"limit":             []string{strconv.Itoa(*limit)},
		"include_options":   []string{strconv.FormatBool(*includeOptions)},
		"skip_apisix":       []string{strconv.FormatBool(*skipAPISIX || *serviceOnly)},
		"apisix_index":      []string{*apisixIndex},
		"service_index":     []string{*serviceIndex},
		"service_uri_field": []string{*serviceURIField},
	}
}

func logsCommand(args []string) (string, url.Values) {
	if len(args) == 0 || args[0] != "search" {
		fail("expected: logs search")
	}
	fs := flag.NewFlagSet("logs search", flag.ExitOnError)
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	pod := fs.String("pod", "", "pod")
	container := fs.String("container", "", "container")
	query := fs.String("q", "", "query string against log field")
	fs.StringVar(query, "query", "", "query string against log field")
	limit := fs.Int("limit", 20, "result limit")
	_ = fs.Parse(args[1:])
	return "/api/logs/search", url.Values{
		"namespace": []string{*namespace},
		"pod":       []string{*pod},
		"container": []string{*container},
		"q":         []string{*query},
		"limit":     []string{strconv.Itoa(*limit)},
	}
}

func dockerCommand(args []string) (string, url.Values) {
	if len(args) == 0 {
		fail("expected docker command")
	}
	switch args[0] {
	case "agents":
		return "/api/node-agents", url.Values{}
	case "containers":
		fs := flag.NewFlagSet("docker containers", flag.ExitOnError)
		host := fs.String("host", "", "node agent host, or all")
		_ = fs.Parse(args[1:])
		return "/api/docker/containers", url.Values{"host": []string{*host}}
	case "inspect":
		fs := flag.NewFlagSet("docker inspect", flag.ExitOnError)
		host := fs.String("host", "", "node agent host")
		container := fs.String("container", "", "container name or id")
		_ = fs.Parse(args[1:])
		return "/api/docker/inspect", url.Values{"host": []string{*host}, "container": []string{*container}}
	case "logs":
		fs := flag.NewFlagSet("docker logs", flag.ExitOnError)
		host := fs.String("host", "", "node agent host")
		container := fs.String("container", "", "container name or id")
		tail := fs.Int("tail", 300, "tail lines")
		since := fs.Int("since", 1800, "since seconds")
		limitBytes := fs.Int("limit-bytes", 1024*1024, "limit bytes")
		timestamps := fs.Bool("timestamps", false, "timestamps")
		_ = fs.Parse(args[1:])
		return "/api/docker/logs", url.Values{
			"host":          []string{*host},
			"container":     []string{*container},
			"tail_lines":    []string{strconv.Itoa(*tail)},
			"since_seconds": []string{strconv.Itoa(*since)},
			"limit_bytes":   []string{strconv.Itoa(*limitBytes)},
			"timestamps":    []string{strconv.FormatBool(*timestamps)},
		}
	case "stats":
		fs := flag.NewFlagSet("docker stats", flag.ExitOnError)
		host := fs.String("host", "", "node agent host")
		container := fs.String("container", "", "container name or id")
		_ = fs.Parse(args[1:])
		return "/api/docker/stats", url.Values{"host": []string{*host}, "container": []string{*container}}
	default:
		fail("unknown docker command: " + args[0])
	}
	return "", nil
}

func diagnoseCommand(args []string) (string, url.Values) {
	if len(args) == 0 {
		fail("expected diagnose subcommand")
	}
	switch args[0] {
	case "pod":
		return podRefCommand(args, "/api/diagnose/pod")
	case "docker":
		fs := flag.NewFlagSet("diagnose docker", flag.ExitOnError)
		host := fs.String("host", "", "node agent host")
		container := fs.String("container", "", "container name or id")
		tail := fs.Int("tail", 300, "tail lines")
		since := fs.Int("since", 1800, "since seconds")
		limitBytes := fs.Int("limit-bytes", 1024*1024, "limit bytes")
		timestamps := fs.Bool("timestamps", false, "timestamps")
		_ = fs.Parse(args[1:])
		return "/api/diagnose/docker", url.Values{
			"host":          []string{*host},
			"container":     []string{*container},
			"tail_lines":    []string{strconv.Itoa(*tail)},
			"since_seconds": []string{strconv.Itoa(*since)},
			"limit_bytes":   []string{strconv.Itoa(*limitBytes)},
			"timestamps":    []string{strconv.FormatBool(*timestamps)},
		}
	default:
		fail("unknown diagnose subcommand: " + args[0])
	}
	return "", nil
}

func releaseCommand(args []string) (string, url.Values) {
	if len(args) == 0 {
		fail("expected release command")
	}
	switch args[0] {
	case "status", "evidence", "diagnose":
		fs := flag.NewFlagSet("release "+args[0], flag.ExitOnError)
		service := fs.String("service", "", "release service name")
		_ = fs.Parse(args[1:])
		return "/api/release/status", url.Values{"service": []string{*service}}
	case "jobs":
		fs := flag.NewFlagSet("release jobs", flag.ExitOnError)
		service := fs.String("service", "", "release service name")
		_ = fs.Parse(args[1:])
		return "/api/release/jobs", url.Values{"service": []string{*service}}
	case "logs":
		fs := flag.NewFlagSet("release logs", flag.ExitOnError)
		service := fs.String("service", "", "release service name")
		job := fs.String("job", "", "GitLab job name")
		jobID := fs.String("job-id", "", "GitLab job id")
		tail := fs.Int("tail", 200, "tail lines")
		limitBytes := fs.Int("limit-bytes", 128*1024, "limit bytes")
		_ = fs.Parse(args[1:])
		return "/api/release/logs", url.Values{
			"service":     []string{*service},
			"job":         []string{*job},
			"job_id":      []string{*jobID},
			"tail_lines":  []string{strconv.Itoa(*tail)},
			"limit_bytes": []string{strconv.Itoa(*limitBytes)},
		}
	case "history":
		fs := flag.NewFlagSet("release history", flag.ExitOnError)
		service := fs.String("service", "", "release service name")
		limit := fs.Int("limit", 10, "history item limit")
		_ = fs.Parse(args[1:])
		return "/api/release/history", url.Values{"service": []string{*service}, "limit": []string{strconv.Itoa(*limit)}}
	default:
		fail("unknown release command: " + args[0])
	}
	return "", nil
}

func consumeGlobalFlags(args []string, opts *globalOptions) []string {
	out := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--backend-url" && i+1 < len(args) {
			opts.backendURL = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--backend-url=") {
			opts.backendURL = strings.TrimPrefix(arg, "--backend-url=")
			continue
		}
		if arg == "--output" && i+1 < len(args) {
			opts.output = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--output=") {
			opts.output = strings.TrimPrefix(arg, "--output=")
			continue
		}
		if arg == "--cluster" && i+1 < len(args) {
			opts.cluster = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--cluster=") {
			opts.cluster = strings.TrimPrefix(arg, "--cluster=")
			continue
		}
		out = append(out, arg)
	}
	return out
}

func addCluster(values url.Values, cluster string) url.Values {
	if values == nil {
		values = url.Values{}
	}
	if cluster != "" && values.Get("cluster") == "" {
		values.Set("cluster", cluster)
	}
	return values
}

func inventoryCommand(args []string) (string, url.Values) {
	if len(args) == 0 || args[0] != "overview" {
		fail("expected: inventory overview")
	}
	fs := flag.NewFlagSet("inventory overview", flag.ExitOnError)
	limit := fs.Int("limit", 10, "result limit")
	_ = fs.Parse(args[1:])
	return "/api/inventory/overview", url.Values{"limit": []string{strconv.Itoa(*limit)}}
}

func metricsCommand(args []string) (string, url.Values) {
	if len(args) == 0 {
		fail("expected metrics command")
	}
	switch args[0] {
	case "health":
		return "/api/metrics/health", url.Values{}
	case "datasources":
		return "/api/metrics/datasources", url.Values{}
	case "query":
		fs := flag.NewFlagSet("metrics query", flag.ExitOnError)
		query := fs.String("query", "", "promql query")
		fs.StringVar(query, "q", "", "promql query")
		source := fs.String("source", "", "prometheus datasource")
		_ = fs.Parse(args[1:])
		return "/api/metrics/query", url.Values{"query": []string{*query}, "source": []string{*source}}
	case "filesystems":
		fs := flag.NewFlagSet("metrics filesystems", flag.ExitOnError)
		source := fs.String("source", "", "prometheus datasource, or all")
		_ = fs.Parse(args[1:])
		return "/api/metrics/query", url.Values{"query": []string{"node_filesystem_avail_bytes{" + realFilesystemFilter + "}"}, "source": []string{*source}}
	case "nodes":
		fs := flag.NewFlagSet("metrics nodes", flag.ExitOnError)
		limit := fs.Int("limit", 20, "result limit")
		source := fs.String("source", "", "prometheus datasource, or all")
		_ = fs.Parse(args[1:])
		return "/api/metrics/nodes", url.Values{"limit": []string{strconv.Itoa(*limit)}, "source": []string{*source}}
	case "pods":
		fs := flag.NewFlagSet("metrics pods", flag.ExitOnError)
		namespace := fs.String("namespace", "", "namespace")
		fs.StringVar(namespace, "n", "", "namespace")
		sortBy := fs.String("sort", "cpu", "sort by cpu or memory")
		limit := fs.Int("limit", 20, "result limit")
		source := fs.String("source", "", "prometheus datasource, or all")
		_ = fs.Parse(args[1:])
		return "/api/metrics/pods", url.Values{
			"namespace": []string{*namespace},
			"sort":      []string{*sortBy},
			"limit":     []string{strconv.Itoa(*limit)},
			"source":    []string{*source},
		}
	case "containers":
		fs := flag.NewFlagSet("metrics containers", flag.ExitOnError)
		sortBy := fs.String("sort", "cpu", "sort by cpu or memory")
		limit := fs.Int("limit", 20, "result limit")
		source := fs.String("source", "", "prometheus datasource, or all")
		_ = fs.Parse(args[1:])
		return "/api/metrics/containers", url.Values{
			"sort":   []string{*sortBy},
			"limit":  []string{strconv.Itoa(*limit)},
			"source": []string{*source},
		}
	case "pod":
		fs := flag.NewFlagSet("metrics pod", flag.ExitOnError)
		namespace := fs.String("namespace", "", "namespace")
		fs.StringVar(namespace, "n", "", "namespace")
		pod := fs.String("pod", "", "pod")
		source := fs.String("source", "", "prometheus datasource")
		_ = fs.Parse(args[1:])
		return "/api/metrics/pod", url.Values{"namespace": []string{*namespace}, "pod": []string{*pod}, "source": []string{*source}}
	default:
		fail("unknown metrics command: " + args[0])
	}
	return "", nil
}

func k8sCommand(args []string) (string, url.Values) {
	if len(args) == 0 {
		fail("expected k8s command")
	}
	switch args[0] {
	case "pods":
		fs := flag.NewFlagSet("k8s pods", flag.ExitOnError)
		namespace := fs.String("namespace", "", "namespace")
		fs.StringVar(namespace, "n", "", "namespace")
		status := fs.String("status", "", "status")
		query := fs.String("q", "", "query")
		limit := fs.Int("limit", 100, "result limit")
		_ = fs.Parse(args[1:])
		return "/api/k8s/pods", url.Values{
			"namespace": []string{*namespace},
			"status":    []string{*status},
			"q":         []string{*query},
			"limit":     []string{strconv.Itoa(*limit)},
		}
	case "logs":
		if len(args) < 2 || args[1] != "pod" {
			fail("expected: k8s logs pod")
		}
		fs := flag.NewFlagSet("k8s logs pod", flag.ExitOnError)
		namespace := fs.String("namespace", "", "namespace")
		fs.StringVar(namespace, "n", "", "namespace")
		pod := fs.String("pod", "", "pod")
		container := fs.String("container", "", "container")
		fs.StringVar(container, "c", "", "container")
		tail := fs.Int("tail", 300, "tail lines")
		since := fs.Int("since", defaultPodLogSinceSeconds, "since seconds")
		limitBytes := fs.Int("limit-bytes", 1024*1024, "limit bytes")
		previous := fs.Bool("previous", false, "previous logs")
		timestamps := fs.Bool("timestamps", false, "timestamps")
		_ = fs.Parse(args[2:])
		return "/api/k8s/logs/pod", url.Values{
			"namespace":     []string{*namespace},
			"pod":           []string{*pod},
			"container":     []string{*container},
			"tail_lines":    []string{strconv.Itoa(*tail)},
			"since_seconds": []string{strconv.Itoa(*since)},
			"limit_bytes":   []string{strconv.Itoa(*limitBytes)},
			"previous":      []string{strconv.FormatBool(*previous)},
			"timestamps":    []string{strconv.FormatBool(*timestamps)},
		}
	default:
		fail("unknown k8s command: " + args[0])
	}
	return "", nil
}

func podRefCommand(args []string, endpoint string) (string, url.Values) {
	if len(args) == 0 || args[0] != "pod" {
		fail("expected pod subcommand")
	}
	fs := flag.NewFlagSet(strings.TrimPrefix(endpoint, "/api/"), flag.ExitOnError)
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	pod := fs.String("pod", "", "pod")
	source := fs.String("source", "", "prometheus datasource for metric evidence")
	_ = fs.Parse(args[1:])
	return endpoint, url.Values{"namespace": []string{*namespace}, "pod": []string{*pod}, "source": []string{*source}}
}

type apiEnvelope struct {
	OK   bool            `json:"ok"`
	Data json.RawMessage `json:"data"`
}

type metricItem struct {
	Metric map[string]string `json:"metric"`
	Source string            `json:"source"`
	Value  float64           `json:"value"`
}

type metricItemsData struct {
	Items []metricItem `json:"items"`
}

type filesystemRow struct {
	Source   string  `json:"source"`
	Node     string  `json:"node"`
	Mount    string  `json:"mount"`
	Device   string  `json:"device"`
	FSType   string  `json:"fstype"`
	FreeGiB  float64 `json:"free_gib"`
	TotalGiB float64 `json:"total_gib"`
	UsedPct  float64 `json:"used_percent"`
}

type filesystemsResult struct {
	Items []filesystemRow `json:"items"`
	Count int             `json:"item_count"`
}

func inspectCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected inspect subcommand: service, pod, cluster, or release")
	}
	switch args[0] {
	case "service":
		return runInspectService(opts, args[1:], out)
	case "pod":
		return runInspectPod(opts, args[1:], out)
	case "cluster":
		return runInspectCluster(opts, args[1:], out)
	case "release":
		return runReleaseStatus(opts, args[1:], out)
	default:
		return fmt.Errorf("unknown inspect command: %s", args[0])
	}
}

type fixPlanResult struct {
	TargetType           string                         `json:"target_type"`
	Target               string                         `json:"target"`
	Namespace            string                         `json:"namespace,omitempty"`
	DryRun               bool                           `json:"dry_run"`
	Status               string                         `json:"status"`
	Summary              string                         `json:"summary"`
	Evidence             []evidenceItem                 `json:"evidence"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	LikelyCauses         []likelyCause                  `json:"likely_causes,omitempty"`
	RecommendedActions   []recommendedAction            `json:"recommended_actions"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Warnings             []string                       `json:"warnings,omitempty"`
	Raw                  any                            `json:"raw,omitempty"`
}

func fixCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected fix subcommand: service or pod")
	}
	switch args[0] {
	case "service":
		return runFixService(opts, args[1:], out)
	case "pod":
		return runFixPod(opts, args[1:], out)
	default:
		return fmt.Errorf("unknown fix command: %s", args[0])
	}
}

func runFixService(opts globalOptions, args []string, out io.Writer) error {
	positionalService := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("fix service", flag.ExitOnError)
	service := fs.String("service", "", "service name")
	envName := fs.String("env", "test", "target environment")
	source := fs.String("source", "", "prometheus datasource")
	cluster := fs.String("cluster", "", "cluster name")
	tail := fs.Int("tail", 300, "tail lines")
	since := fs.Int("since", defaultPodLogSinceSeconds, "since seconds")
	dryRun := fs.Bool("dry-run", false, "plan only; do not mutate repositories or clusters")
	_ = fs.Parse(args)
	if *service == "" {
		*service = positionalService
	}
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("fix service requires --service")
	}
	if !*dryRun {
		return fmt.Errorf("fix service currently requires --dry-run")
	}
	inspection, err := fetchInspectService(opts.backendURL, *service, *envName, *source, firstNonEmptyString(*cluster, opts.cluster), *tail, *since)
	if err != nil {
		return err
	}
	pack := buildEvidencePack(inspection)
	result := fixPlanResult{
		TargetType:         "service",
		Target:             inspection.Service,
		Namespace:          inspection.Namespace,
		DryRun:             true,
		Status:             pack.Status,
		Summary:            firstNonEmptyString(pack.Summary, "Generated a dry-run service fix plan from OpsPilot evidence."),
		Evidence:           pack.Evidence,
		MissingEvidence:    pack.MissingEvidence,
		LikelyCauses:       pack.LikelyCauses,
		RecommendedActions: fixActionsFromEvidence("service", inspection.Service, pack),
		Warnings:           inspection.Warnings,
		Raw:                inspection,
	}
	recommendations, warning := fetchSkillRecommendations(opts.backendURL, "service", pack.Status, pack.MissingEvidence, append([]string{pack.Summary}, evidenceItemMessages(pack.Evidence)...))
	result.SkillRecommendations = recommendations
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	return writeOutput(out, opts.output, result, writeFixPlanHuman(result))
}

func runFixPod(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("fix pod", flag.ExitOnError)
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	pod := fs.String("pod", "", "pod")
	source := fs.String("source", "", "prometheus datasource")
	cluster := fs.String("cluster", "", "cluster name")
	tail := fs.Int("tail", 300, "tail lines")
	since := fs.Int("since", defaultPodLogSinceSeconds, "since seconds")
	dryRun := fs.Bool("dry-run", false, "plan only; do not mutate repositories or clusters")
	_ = fs.Parse(args)
	if *pod == "" && fs.NArg() > 0 {
		*pod = fs.Arg(0)
	}
	if *namespace == "" || *pod == "" {
		return fmt.Errorf("fix pod requires --namespace and --pod")
	}
	if !*dryRun {
		return fmt.Errorf("fix pod currently requires --dry-run")
	}
	inspection, err := fetchInspectPod(opts.backendURL, *namespace, *pod, *source, firstNonEmptyString(*cluster, opts.cluster), *tail, *since)
	if err != nil {
		return err
	}
	pack := buildEvidencePack(inspection)
	result := fixPlanResult{
		TargetType:         "pod",
		Target:             inspection.Pod,
		Namespace:          inspection.Namespace,
		DryRun:             true,
		Status:             pack.Status,
		Summary:            firstNonEmptyString(pack.Summary, "Generated a dry-run Pod fix plan from OpsPilot evidence."),
		Evidence:           pack.Evidence,
		MissingEvidence:    pack.MissingEvidence,
		LikelyCauses:       pack.LikelyCauses,
		RecommendedActions: fixActionsFromEvidence("pod", inspection.Pod, pack),
		Raw:                inspection,
	}
	recommendations, warning := fetchSkillRecommendations(opts.backendURL, "pod", pack.Status, pack.MissingEvidence, append([]string{pack.Summary}, evidenceItemMessages(pack.Evidence)...))
	result.SkillRecommendations = recommendations
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	return writeOutput(out, opts.output, result, writeFixPlanHuman(result))
}

func fixActionsFromEvidence(targetType, target string, pack evidencePack) []recommendedAction {
	actions := []recommendedAction{
		{Type: "ai_review", Target: "evidence_pack", Instruction: "Feed this evidence pack to AI before making code or configuration changes."},
	}
	if pack.Status != "healthy" {
		actions = append(actions,
			recommendedAction{Type: "code_or_config_review", Target: "repository", Instruction: "Inspect startup code, configuration loading, Dockerfile, probes, and deployment YAML for " + target + "."},
			recommendedAction{Type: "release_validation", Target: "pipeline", Instruction: "After a fix, publish through GitLab Runner -> BuildKit -> Registry -> GitOps -> Argo CD, then run check " + targetType + " again."},
		)
	} else {
		actions = append(actions, recommendedAction{Type: "no_code_change", Target: targetType, Instruction: "No direct code change is suggested from current evidence; fill missing evidence before changing code."})
	}
	if len(pack.MissingEvidence) > 0 {
		actions = append(actions, recommendedAction{Type: "missing_evidence", Target: "opspilot", Instruction: "The diagnosis is partial because evidence is missing: " + strings.Join(pack.MissingEvidence, ", ")})
	}
	return actions
}

func writeFixPlanHuman(result fixPlanResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Fix plan: %s %s dry_run=%t status=%s\n", result.TargetType, result.Target, result.DryRun, result.Status)
		if result.Namespace != "" {
			fmt.Fprintf(w, "Namespace: %s\n", result.Namespace)
		}
		if result.Summary != "" {
			fmt.Fprintf(w, "Summary: %s\n", result.Summary)
		}
		if len(result.Evidence) > 0 {
			fmt.Fprintln(w, "Evidence:")
			for _, item := range result.Evidence {
				fmt.Fprintf(w, "- %s: %s\n", item.Source, item.Message)
			}
		}
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, ", "))
		}
		if len(result.LikelyCauses) > 0 {
			fmt.Fprintln(w, "Likely causes:")
			for _, cause := range result.LikelyCauses {
				fmt.Fprintf(w, "- %s confidence=%.2f: %s\n", cause.Type, cause.Confidence, cause.Reason)
			}
		}
		if len(result.RecommendedActions) > 0 {
			fmt.Fprintln(w, "Recommended actions:")
			for _, action := range result.RecommendedActions {
				fmt.Fprintf(w, "- %s %s: %s\n", action.Type, action.Target, action.Instruction)
			}
		}
		writeSkillRecommendationsHuman(w, result.SkillRecommendations)
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	}
}

func writeSkillRecommendationsHuman(w io.Writer, recommendations []skillregistry.Recommendation) {
	if len(recommendations) == 0 {
		return
	}
	fmt.Fprintln(w, "Recommended skills:")
	for _, item := range recommendations {
		fmt.Fprintf(w, "- %s: %s\n", item.Name, item.Reason)
	}
}

func fetchSkillRecommendations(backendURL, targetType, status string, missingEvidence, findings []string) ([]skillregistry.Recommendation, string) {
	values := url.Values{
		"target_type": {targetType},
		"status":      {status},
	}
	for _, item := range missingEvidence {
		if strings.TrimSpace(item) != "" {
			values.Add("missing_evidence", item)
		}
	}
	for _, item := range findings {
		if strings.TrimSpace(item) != "" {
			values.Add("finding", item)
		}
	}
	body, err := get(backendURL, "/api/skills/recommend", values)
	if err != nil {
		return nil, "skills recommend: " + err.Error()
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "skills recommend: " + err.Error()
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, "skills recommend: response missing data"
	}
	raw, _ := json.Marshal(data["items"])
	var result []skillregistry.Recommendation
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, "skills recommend: " + err.Error()
	}
	return result, ""
}

func runMetricsFilesystems(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("metrics filesystems", flag.ExitOnError)
	source := fs.String("source", "", "prometheus datasource, or all")
	_ = fs.Parse(args)
	result, err := fetchFilesystems(opts.backendURL, *source)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SOURCE\tNODE\tMOUNT\tDEVICE\tFSTYPE\tFREE\tTOTAL\tUSED")
		for _, row := range result.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%.1fGiB\t%.1fGiB\t%.1f%%\n",
				row.Source, row.Node, row.Mount, row.Device, row.FSType, row.FreeGiB, row.TotalGiB, row.UsedPct)
		}
		return tw.Flush()
	})
}

func fetchFilesystems(backendURL, source string) (filesystemsResult, error) {
	avail, err := fetchMetricItems(backendURL, "node_filesystem_avail_bytes{"+realFilesystemFilter+"}", source)
	if err != nil {
		return filesystemsResult{}, err
	}
	size, err := fetchMetricItems(backendURL, "node_filesystem_size_bytes{"+realFilesystemFilter+"}", source)
	if err != nil {
		return filesystemsResult{}, err
	}
	sizes := map[string]float64{}
	for _, item := range size {
		sizes[filesystemKey(item)] = item.Value
	}
	rows := make([]filesystemRow, 0, len(avail))
	for _, item := range avail {
		total := sizes[filesystemKey(item)]
		if total <= 0 {
			continue
		}
		usedPct := (1 - item.Value/total) * 100
		rows = append(rows, filesystemRow{
			Source:   item.Source,
			Node:     metricNode(item.Metric),
			Mount:    item.Metric["mountpoint"],
			Device:   item.Metric["device"],
			FSType:   item.Metric["fstype"],
			FreeGiB:  round1(item.Value / (1024 * 1024 * 1024)),
			TotalGiB: round1(total / (1024 * 1024 * 1024)),
			UsedPct:  round1(usedPct),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Node == rows[j].Node {
			return rows[i].Mount < rows[j].Mount
		}
		return rows[i].Node < rows[j].Node
	})
	return filesystemsResult{Items: rows, Count: len(rows)}, nil
}

type inspectPodResult struct {
	Cluster              string                         `json:"cluster,omitempty"`
	Namespace            string                         `json:"namespace"`
	Pod                  string                         `json:"pod"`
	Node                 string                         `json:"node,omitempty"`
	Status               string                         `json:"status,omitempty"`
	Ready                bool                           `json:"ready"`
	RestartCount         int                            `json:"restart_count"`
	Container            string                         `json:"container,omitempty"`
	SpecImage            string                         `json:"spec_image,omitempty"`
	StatusImage          string                         `json:"status_image,omitempty"`
	ImageID              string                         `json:"image_id,omitempty"`
	CPUCore              float64                        `json:"cpu_cores"`
	MemoryMiB            float64                        `json:"memory_mib"`
	KubernetesLogBytes   int                            `json:"kubernetes_log_bytes"`
	ElasticsearchLogHits int                            `json:"elasticsearch_log_hits"`
	EvidenceGaps         []string                       `json:"evidence_gaps"`
	AvailableEvidence    []string                       `json:"available_evidence,omitempty"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	CapabilityWarnings   []string                       `json:"capability_warnings,omitempty"`
	Findings             []string                       `json:"findings"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Raw                  map[string]any                 `json:"raw,omitempty"`
}

type inspectServiceResult struct {
	Cluster              string                         `json:"cluster,omitempty"`
	Service              string                         `json:"service"`
	Environment          string                         `json:"environment,omitempty"`
	Namespace            string                         `json:"namespace,omitempty"`
	Deployment           string                         `json:"deployment,omitempty"`
	Status               string                         `json:"status,omitempty"`
	Stage                string                         `json:"stage,omitempty"`
	Image                string                         `json:"image,omitempty"`
	PodCount             int                            `json:"pod_count"`
	TotalCPUCore         float64                        `json:"total_cpu_cores"`
	TotalMemoryMiB       float64                        `json:"total_memory_mib"`
	RestartCount         int                            `json:"restart_count"`
	Pods                 []inspectPodResult             `json:"pods,omitempty"`
	ReleaseGaps          []string                       `json:"release_gaps,omitempty"`
	EvidenceGaps         []string                       `json:"evidence_gaps,omitempty"`
	AvailableEvidence    []string                       `json:"available_evidence,omitempty"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	CapabilityWarnings   []string                       `json:"capability_warnings,omitempty"`
	Findings             []string                       `json:"findings"`
	Next                 []string                       `json:"next,omitempty"`
	Warnings             []string                       `json:"warnings,omitempty"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Raw                  map[string]any                 `json:"raw,omitempty"`
}

func runInspectService(opts globalOptions, args []string, out io.Writer) error {
	positionalService := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("inspect service", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	envName := fs.String("env", "test", "target environment")
	source := fs.String("source", "", "prometheus datasource")
	cluster := fs.String("cluster", "", "cluster name")
	tail := fs.Int("tail", 300, "tail lines")
	since := fs.Int("since", defaultPodLogSinceSeconds, "since seconds")
	_ = fs.Parse(args)
	if *service == "" {
		*service = positionalService
	}
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("inspect service requires --service")
	}
	result, err := fetchInspectService(opts.backendURL, *service, *envName, *source, firstNonEmptyString(*cluster, opts.cluster), *tail, *since)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Service: %s env=%s\n", result.Service, result.Environment)
		fmt.Fprintf(w, "Status: %s stage=%s namespace=%s deployment=%s\n", result.Status, result.Stage, result.Namespace, result.Deployment)
		if result.Image != "" {
			fmt.Fprintf(w, "Image: %s\n", result.Image)
		}
		fmt.Fprintf(w, "Usage: pods=%d restarts=%d CPU %.3f cores memory %.1f MiB\n",
			result.PodCount, result.RestartCount, result.TotalCPUCore, result.TotalMemoryMiB)
		if len(result.Findings) > 0 {
			fmt.Fprintf(w, "Findings: %s\n", strings.Join(result.Findings, "; "))
		}
		if len(result.ReleaseGaps) > 0 {
			fmt.Fprintf(w, "Release gaps: %s\n", strings.Join(result.ReleaseGaps, ", "))
		}
		if len(result.EvidenceGaps) > 0 {
			fmt.Fprintf(w, "Evidence gaps: %s\n", strings.Join(result.EvidenceGaps, ", "))
		}
		if len(result.AvailableEvidence) > 0 {
			fmt.Fprintf(w, "Available evidence: %s\n", strings.Join(result.AvailableEvidence, "; "))
		}
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, "; "))
		}
		writeSkillRecommendationsHuman(w, result.SkillRecommendations)
		if len(result.Pods) > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "POD\tSTATUS\tREADY\tRESTARTS\tIMAGE\tCPU\tMEMORY\tK8S LOG\tELK")
			for _, pod := range result.Pods {
				fmt.Fprintf(tw, "%s\t%s\t%t\t%d\t%s\t%.3f\t%.1fMiB\t%dB\t%d\n",
					pod.Pod, pod.Status, pod.Ready, pod.RestartCount, imageTagHint(pod), pod.CPUCore, pod.MemoryMiB, pod.KubernetesLogBytes, pod.ElasticsearchLogHits)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(result.Next, "; "))
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		if len(result.CapabilityWarnings) > 0 {
			fmt.Fprintf(w, "Capability warnings: %s\n", strings.Join(result.CapabilityWarnings, "; "))
		}
		return nil
	})
}

func fetchInspectService(backendURL, service, envName, source, cluster string, tail, since int) (inspectServiceResult, error) {
	data, err := fetchReleaseStatusData(backendURL, service, cluster)
	if err != nil {
		return inspectServiceResult{}, err
	}
	result := inspectServiceResult{
		Service:     firstNonEmptyString(stringValue(data["service"]), service),
		Environment: firstNonEmptyString(stringValue(data["environment"]), envName),
		Namespace:   stringValue(data["namespace"]),
		Deployment:  stringValue(data["deployment"]),
		Status:      stringValue(data["status"]),
		Stage:       stringValue(data["stage"]),
		Image:       stringValue(data["image"]),
		ReleaseGaps: stringList(data["gaps"]),
		Next:        stringList(data["next_checks"]),
		Cluster:     cluster,
		Raw:         map[string]any{"release_status": data},
	}
	if capabilities, err := fetchCapabilities(backendURL, cluster); err == nil {
		result.AvailableEvidence = capabilities.AvailableEvidence
		result.MissingEvidence = capabilities.MissingEvidence
		result.CapabilityWarnings = capabilities.Warnings
		result.Raw["capabilities"] = capabilities.Raw
	} else {
		if strings.Contains(err.Error(), "does not have a Kubernetes datasource") {
			return result, err
		}
		result.CapabilityWarnings = append(result.CapabilityWarnings, "capabilities: "+err.Error())
	}
	evidence := mapValue(data, "evidence")
	pods := mapValue(evidence, "pods")
	podItems := mapsFromItems(pods["items"])
	result.PodCount = intValue(pods["item_count"])
	if result.PodCount == 0 {
		result.PodCount = len(podItems)
	}
	if len(podItems) == 0 {
		result.EvidenceGaps = append(result.EvidenceGaps, "service_pods_missing")
		result.Findings = append(result.Findings, "No matching Pods were found from release evidence.")
	} else {
		for _, item := range podItems {
			podName := stringValue(item["name"])
			namespace := firstNonEmptyString(stringValue(item["namespace"]), result.Namespace)
			if podName == "" || namespace == "" {
				continue
			}
			pod, err := fetchInspectPod(backendURL, namespace, podName, source, cluster, tail, since)
			if err != nil {
				result.Warnings = append(result.Warnings, podName+": "+err.Error())
				result.EvidenceGaps = append(result.EvidenceGaps, "pod_inspection_failed")
				continue
			}
			result.Pods = append(result.Pods, pod)
			result.TotalCPUCore += pod.CPUCore
			result.TotalMemoryMiB += pod.MemoryMiB
			result.RestartCount += pod.RestartCount
			result.EvidenceGaps = append(result.EvidenceGaps, pod.EvidenceGaps...)
		}
	}
	result.TotalCPUCore = round3(result.TotalCPUCore)
	result.TotalMemoryMiB = round1(result.TotalMemoryMiB)
	result.ReleaseGaps = uniqueStrings(result.ReleaseGaps)
	result.EvidenceGaps = uniqueStrings(result.EvidenceGaps)
	result.Next = uniqueStrings(result.Next)
	result.Findings = append(result.Findings, serviceLogEvidenceFindings(result.EvidenceGaps)...)
	switch {
	case result.Status == "healthy" && result.RestartCount == 0:
		result.Findings = append(result.Findings, "Service rollout is healthy and no Pod restarts were found.")
	case result.Status != "" && result.Status != "healthy":
		result.Findings = append(result.Findings, "Service release status is "+result.Status+".")
	}
	if result.TotalCPUCore < 0.1 && result.TotalMemoryMiB < 256 && len(result.Pods) > 0 {
		result.Findings = append(result.Findings, "Current Pod resource usage is low.")
	}
	if len(result.ReleaseGaps) > 0 || len(result.EvidenceGaps) > 0 {
		result.Findings = append(result.Findings, "Some evidence is missing; treat the healthy checks as partial.")
	}
	recommendations, warning := fetchSkillRecommendations(backendURL, "service", serviceEvidenceStatus(result),
		uniqueStrings(append(append(result.MissingEvidence, result.EvidenceGaps...), result.ReleaseGaps...)),
		append(result.Findings, result.Next...))
	result.SkillRecommendations = recommendations
	if warning != "" {
		result.CapabilityWarnings = append(result.CapabilityWarnings, warning)
	}
	return result, nil
}

func runInspectPod(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("inspect pod", flag.ExitOnError)
	namespace := fs.String("namespace", "", "namespace")
	fs.StringVar(namespace, "n", "", "namespace")
	pod := fs.String("pod", "", "pod")
	source := fs.String("source", "", "prometheus datasource")
	cluster := fs.String("cluster", "", "cluster name")
	tail := fs.Int("tail", 300, "tail lines")
	since := fs.Int("since", defaultPodLogSinceSeconds, "since seconds")
	_ = fs.Parse(args)
	if *pod == "" && fs.NArg() > 0 {
		*pod = fs.Arg(0)
	}
	if *namespace == "" || *pod == "" {
		return fmt.Errorf("inspect pod requires --namespace and --pod")
	}
	result, err := fetchInspectPod(opts.backendURL, *namespace, *pod, *source, firstNonEmptyString(*cluster, opts.cluster), *tail, *since)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Pod: %s/%s\n", result.Namespace, result.Pod)
		fmt.Fprintf(w, "Status: %s ready=%t restarts=%d node=%s\n", result.Status, result.Ready, result.RestartCount, result.Node)
		writeImageEvidenceHuman(w, result)
		fmt.Fprintf(w, "Usage: CPU %.3f cores, memory %.1f MiB\n", result.CPUCore, result.MemoryMiB)
		fmt.Fprintf(w, "Logs: Kubernetes %d bytes, ELK hits %d\n", result.KubernetesLogBytes, result.ElasticsearchLogHits)
		if len(result.EvidenceGaps) > 0 {
			fmt.Fprintf(w, "Evidence gaps: %s\n", strings.Join(result.EvidenceGaps, ", "))
		}
		if len(result.AvailableEvidence) > 0 {
			fmt.Fprintf(w, "Available evidence: %s\n", strings.Join(result.AvailableEvidence, "; "))
		}
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, "; "))
		}
		if len(result.Findings) > 0 {
			fmt.Fprintf(w, "Findings: %s\n", strings.Join(result.Findings, "; "))
		}
		writeSkillRecommendationsHuman(w, result.SkillRecommendations)
		if len(result.CapabilityWarnings) > 0 {
			fmt.Fprintf(w, "Capability warnings: %s\n", strings.Join(result.CapabilityWarnings, "; "))
		}
		return nil
	})
}

func fetchInspectPod(backendURL, namespace, pod, source, cluster string, tail, since int) (inspectPodResult, error) {
	result := inspectPodResult{Cluster: cluster, Namespace: namespace, Pod: pod, Raw: map[string]any{}}
	if capabilities, err := fetchCapabilities(backendURL, cluster); err == nil {
		result.AvailableEvidence = capabilities.AvailableEvidence
		result.MissingEvidence = capabilities.MissingEvidence
		result.CapabilityWarnings = capabilities.Warnings
		result.Raw["capabilities"] = capabilities.Raw
	} else {
		if strings.Contains(err.Error(), "does not have a Kubernetes datasource") {
			return result, err
		}
		result.CapabilityWarnings = append(result.CapabilityWarnings, "capabilities: "+err.Error())
	}
	contextBody, err := get(backendURL, "/api/context/pod", addCluster(url.Values{"namespace": {namespace}, "pod": {pod}, "source": {source}}, cluster))
	if err != nil {
		return result, err
	}
	var contextPayload map[string]any
	_ = json.Unmarshal(contextBody, &contextPayload)
	result.Raw["context"] = contextPayload
	if data := mapValue(contextPayload, "data"); data != nil {
		if summary := mapValue(data, "summary"); summary != nil {
			result.Node = stringValue(summary["node"])
			result.Status = stringValue(summary["status"])
			result.Ready = boolValue(summary["ready"])
			result.RestartCount = intValue(summary["restart_count"])
			applyPrimaryContainerEvidence(&result, summary)
		}
	}
	metricsBody, err := get(backendURL, "/api/metrics/pod", url.Values{"namespace": {namespace}, "pod": {pod}, "source": {source}})
	if err == nil {
		var metricsPayload map[string]any
		_ = json.Unmarshal(metricsBody, &metricsPayload)
		result.Raw["metrics"] = metricsPayload
		if data := mapValue(metricsPayload, "data"); data != nil {
			result.CPUCore = floatValue(data["cpu_cores"])
			result.MemoryMiB = round1(floatValue(data["memory_working_set_bytes"]) / (1024 * 1024))
			if result.RestartCount == 0 {
				result.RestartCount = intValue(data["restart_count"])
			}
		}
	}
	k8sLogAvailable := false
	elkLogAvailable := false
	logBody, err := get(backendURL, "/api/k8s/logs/pod", addCluster(url.Values{
		"namespace":     {namespace},
		"pod":           {pod},
		"tail_lines":    {strconv.Itoa(tail)},
		"since_seconds": {strconv.Itoa(since)},
	}, cluster))
	if err == nil {
		k8sLogAvailable = true
		var logPayload map[string]any
		_ = json.Unmarshal(logBody, &logPayload)
		result.Raw["kubernetes_logs"] = logPayload
		if data := mapValue(logPayload, "data"); data != nil {
			result.KubernetesLogBytes = len(stringValue(data["text"]))
		}
	} else {
		result.Raw["kubernetes_logs_error"] = err.Error()
		result.EvidenceGaps = append(result.EvidenceGaps, "kubernetes_logs_unavailable")
	}
	elkBody, err := get(backendURL, "/api/logs/search", url.Values{"namespace": {namespace}, "pod": {pod}, "limit": {"1"}})
	if err == nil {
		elkLogAvailable = true
		var elkPayload map[string]any
		_ = json.Unmarshal(elkBody, &elkPayload)
		result.Raw["elk_logs"] = elkPayload
		if data := mapValue(elkPayload, "data"); data != nil {
			result.ElasticsearchLogHits = intValue(data["total"])
			if result.ElasticsearchLogHits == 0 {
				result.ElasticsearchLogHits = intValue(data["item_count"])
			}
		}
	} else {
		result.Raw["elk_logs_error"] = err.Error()
		result.EvidenceGaps = append(result.EvidenceGaps, "elk_logs_unavailable")
	}
	if k8sLogAvailable && result.KubernetesLogBytes == 0 {
		result.EvidenceGaps = append(result.EvidenceGaps, "kubernetes_logs_empty")
	}
	if elkLogAvailable && result.ElasticsearchLogHits == 0 {
		result.EvidenceGaps = append(result.EvidenceGaps, "elk_logs_missing_or_empty")
	}
	if result.Ready {
		result.Findings = append(result.Findings, "Pod is currently ready.")
	}
	result.Findings = append(result.Findings, logEvidenceFindings(result, k8sLogAvailable, elkLogAvailable)...)
	if result.RestartCount > 0 {
		result.Findings = append(result.Findings, fmt.Sprintf("Pod has historical restarts: %d.", result.RestartCount))
	}
	recommendations, warning := fetchSkillRecommendations(backendURL, "pod", podEvidenceStatus(result),
		uniqueStrings(append(result.MissingEvidence, result.EvidenceGaps...)), result.Findings)
	result.SkillRecommendations = recommendations
	if warning != "" {
		result.CapabilityWarnings = append(result.CapabilityWarnings, warning)
	}
	return result, nil
}

type inspectClusterResult struct {
	Cluster              string                         `json:"cluster,omitempty"`
	AbnormalPods         map[string]any                 `json:"abnormal_pods"`
	Nodes                []map[string]any               `json:"nodes"`
	TopCPU               []map[string]any               `json:"top_cpu_pods"`
	TopMemory            []map[string]any               `json:"top_memory_pods"`
	Restarts24h          []metricItem                   `json:"restarts_24h"`
	Filesystems          []filesystemRow                `json:"filesystems"`
	AvailableEvidence    []string                       `json:"available_evidence,omitempty"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	CapabilityWarnings   []string                       `json:"capability_warnings,omitempty"`
	Findings             []string                       `json:"findings"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Raw                  map[string]any                 `json:"raw,omitempty"`
}

func runInspectCluster(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("inspect cluster", flag.ExitOnError)
	source := fs.String("source", "all", "prometheus datasource, or all")
	cluster := fs.String("cluster", "", "cluster name")
	limit := fs.Int("limit", 10, "top result limit")
	_ = fs.Parse(args)
	result, err := fetchInspectCluster(opts.backendURL, *source, firstNonEmptyString(*cluster, opts.cluster), *limit)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintln(w, "Cluster inspection")
		if len(result.Findings) > 0 {
			fmt.Fprintf(w, "Findings: %s\n", strings.Join(result.Findings, "; "))
		}
		if len(result.AvailableEvidence) > 0 {
			fmt.Fprintf(w, "Available evidence: %s\n", strings.Join(result.AvailableEvidence, "; "))
		}
		if len(result.MissingEvidence) > 0 {
			fmt.Fprintf(w, "Missing evidence: %s\n", strings.Join(result.MissingEvidence, "; "))
		}
		if len(result.CapabilityWarnings) > 0 {
			fmt.Fprintf(w, "Capability warnings: %s\n", strings.Join(result.CapabilityWarnings, "; "))
		}
		writeSkillRecommendationsHuman(w, result.SkillRecommendations)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "\nNODES\tCPU\tMEMORY\tROOTFS")
		for _, node := range result.Nodes {
			fmt.Fprintf(tw, "%s\t%.1f%%\t%.1f%%\t%.1f%%\n",
				stringValue(node["node"]), floatValue(node["cpu_used_percent"]), floatValue(node["memory_used_percent"]), floatValue(node["rootfs_used_percent"]))
		}
		fmt.Fprintln(tw, "\nTOP CPU PODS\tNAMESPACE\tCPU")
		for _, pod := range result.TopCPU {
			fmt.Fprintf(tw, "%s\t%s\t%.3f cores\n", stringValue(pod["pod"]), stringValue(pod["namespace"]), floatValue(pod["cpu_cores"]))
		}
		fmt.Fprintln(tw, "\nTOP MEMORY PODS\tNAMESPACE\tMEMORY")
		for _, pod := range result.TopMemory {
			fmt.Fprintf(tw, "%s\t%s\t%.1fMiB\n", stringValue(pod["pod"]), stringValue(pod["namespace"]), floatValue(pod["memory_working_set_bytes"])/(1024*1024))
		}
		fmt.Fprintln(tw, "\nRESTARTS 24H\tNAMESPACE\tCONTAINER\tCOUNT")
		if len(result.Restarts24h) == 0 {
			fmt.Fprintln(tw, "-\t-\t-\t0")
		}
		for _, item := range result.Restarts24h {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%.1f\n", item.Metric["pod"], item.Metric["namespace"], item.Metric["container"], item.Value)
		}
		fmt.Fprintln(tw, "\nFILESYSTEMS\tMOUNT\tFREE\tTOTAL\tUSED")
		for _, row := range result.Filesystems {
			fmt.Fprintf(tw, "%s\t%s\t%.1fGiB\t%.1fGiB\t%.1f%%\n", row.Node, row.Mount, row.FreeGiB, row.TotalGiB, row.UsedPct)
		}
		return tw.Flush()
	})
}

type releaseServiceResult struct {
	Cluster          string           `json:"cluster,omitempty"`
	Service          string           `json:"service"`
	Environment      string           `json:"environment"`
	Status           string           `json:"status,omitempty"`
	Stage            string           `json:"stage,omitempty"`
	Namespace        string           `json:"namespace,omitempty"`
	Deployment       string           `json:"deployment,omitempty"`
	Image            string           `json:"image,omitempty"`
	TriggerSupported bool             `json:"trigger_supported"`
	TriggerHint      string           `json:"trigger_hint"`
	Gaps             []string         `json:"gaps,omitempty"`
	Next             []string         `json:"next,omitempty"`
	Pipeline         map[string]any   `json:"pipeline,omitempty"`
	BuildKit         map[string]any   `json:"buildkit,omitempty"`
	Registry         map[string]any   `json:"registry,omitempty"`
	GitOps           map[string]any   `json:"gitops,omitempty"`
	ArgoCD           map[string]any   `json:"argocd,omitempty"`
	Quality          map[string]any   `json:"quality,omitempty"`
	Jobs             []map[string]any `json:"jobs,omitempty"`
	JobCount         int              `json:"job_count"`
	History          []map[string]any `json:"history,omitempty"`
	HistoryCount     int              `json:"history_count"`
	Triggered        bool             `json:"triggered"`
	Trigger          map[string]any   `json:"trigger,omitempty"`
	Warnings         []string         `json:"warnings,omitempty"`
	Raw              map[string]any   `json:"raw,omitempty"`
}

func runReleaseService(opts globalOptions, args []string, out io.Writer) error {
	positionalService := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("release service", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	envName := fs.String("env", "test", "target environment")
	cluster := fs.String("cluster", "", "cluster name")
	historyLimit := fs.Int("history", 5, "release history item limit")
	trigger := fs.Bool("trigger", false, "trigger a new release pipeline")
	ref := fs.String("ref", "main", "GitLab ref to trigger")
	_ = fs.Parse(args)
	if *service == "" {
		*service = positionalService
	}
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release service requires --service")
	}
	activeCluster := firstNonEmptyString(*cluster, opts.cluster)
	result, err := fetchReleaseService(opts.backendURL, *service, *envName, activeCluster, *historyLimit)
	if err != nil {
		return err
	}
	if *trigger {
		triggerResult, err := triggerReleaseService(opts.backendURL, *service, *ref, activeCluster, nil)
		if err != nil {
			return err
		}
		result.Triggered = true
		result.TriggerSupported = true
		result.TriggerHint = "submitted GitLab pipeline through OpsPilot"
		result.Trigger = triggerResult
		if result.Raw == nil {
			result.Raw = map[string]any{}
		}
		result.Raw["trigger"] = triggerResult
		if checks := stringList(triggerResult["next_checks"]); len(checks) > 0 {
			result.Next = uniqueStrings(append(result.Next, checks...))
		}
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Release service: %s env=%s\n", result.Service, result.Environment)
		fmt.Fprintf(w, "Status: %s stage=%s namespace=%s deployment=%s\n", result.Status, result.Stage, result.Namespace, result.Deployment)
		if result.Image != "" {
			fmt.Fprintf(w, "Image: %s\n", result.Image)
		}
		if result.Trigger != nil {
			if pipeline := mapValue(result.Trigger, "pipeline"); pipeline != nil {
				fmt.Fprintf(w, "Triggered: pipeline id=%d status=%s ref=%s sha=%s\n",
					intValue(pipeline["id"]), stringValue(pipeline["status"]), stringValue(pipeline["ref"]), stringValue(pipeline["sha"]))
			} else {
				fmt.Fprintf(w, "Triggered: %s\n", stringValue(result.Trigger["status"]))
			}
		}
		if result.Pipeline != nil {
			fmt.Fprintf(w, "GitLab pipeline: %s id=%d ref=%s sha=%s\n",
				stringValue(result.Pipeline["status"]), intValue(result.Pipeline["id"]), stringValue(result.Pipeline["ref"]), stringValue(result.Pipeline["sha"]))
		}
		if result.GitOps != nil {
			fmt.Fprintf(w, "GitOps: %s image=%s\n", stringValue(result.GitOps["status"]), stringValue(result.GitOps["desired_image"]))
		}
		if result.ArgoCD != nil {
			fmt.Fprintf(w, "Argo CD: sync=%s health=%s\n", stringValue(result.ArgoCD["sync_status"]), stringValue(result.ArgoCD["health_status"]))
		}
		if result.Quality != nil {
			fmt.Fprintf(w, "Quality: %s reason=%s optional=%t\n",
				stringValue(result.Quality["status"]), stringValue(result.Quality["reason"]), boolValue(result.Quality["optional"]))
		}
		if len(result.Jobs) > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "JOB\tSTAGE\tSTATUS\tDURATION\tFAILURE")
			for _, job := range result.Jobs {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%.1fs\t%s\n",
					stringValue(job["name"]), stringValue(job["stage"]), stringValue(job["status"]), floatValue(job["duration"]), stringValue(job["failure_reason"]))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if len(result.History) > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "HISTORY\tREVISION\tDATE\tTAG\tMESSAGE")
			for _, item := range result.History {
				current := ""
				if boolValue(item["current"]) {
					current = "*"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					current, stringValue(item["short_revision"]), shortTime(stringValue(item["committed_at"])), stringValue(item["tag"]), oneLine(stringValue(item["message"]), 80))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if len(result.Gaps) > 0 {
			fmt.Fprintf(w, "Gaps: %s\n", strings.Join(result.Gaps, ", "))
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(result.Next, "; "))
		}
		fmt.Fprintf(w, "Trigger: %s\n", result.TriggerHint)
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	})
}

func fetchReleaseService(backendURL, service, envName, cluster string, historyLimit int) (releaseServiceResult, error) {
	status, err := fetchReleaseStatusData(backendURL, service, cluster)
	if err != nil {
		return releaseServiceResult{}, err
	}
	result := releaseServiceResult{
		Service:          firstNonEmptyString(stringValue(status["service"]), service),
		Cluster:          cluster,
		Environment:      firstNonEmptyString(stringValue(status["environment"]), envName),
		Status:           stringValue(status["status"]),
		Stage:            stringValue(status["stage"]),
		Namespace:        stringValue(status["namespace"]),
		Deployment:       stringValue(status["deployment"]),
		Image:            stringValue(status["image"]),
		TriggerSupported: true,
		TriggerHint:      "use release service --trigger to submit a GitLab pipeline through OpsPilot",
		Gaps:             stringList(status["gaps"]),
		Next:             stringList(status["next_checks"]),
		Raw:              map[string]any{"status": status},
	}
	if evidence := mapValue(status, "evidence"); evidence != nil {
		result.Pipeline = mapValue(evidence, "gitlab_pipeline")
		result.BuildKit = mapValue(evidence, "buildkit")
		result.Registry = mapValue(evidence, "registry")
		result.GitOps = mapValue(evidence, "gitops")
		result.ArgoCD = mapValue(evidence, "argocd")
		result.Quality = mapValue(evidence, "quality")
	}
	if jobs, err := fetchReleaseJobsData(backendURL, service, cluster); err != nil {
		result.Warnings = append(result.Warnings, "release jobs: "+err.Error())
	} else {
		result.Raw["jobs"] = jobs
		result.Jobs = mapsFromItems(jobs["items"])
		result.JobCount = intValue(jobs["item_count"])
	}
	if historyLimit > 0 {
		if history, err := fetchReleaseHistoryData(backendURL, service, cluster, historyLimit); err != nil {
			result.Warnings = append(result.Warnings, "release history: "+err.Error())
		} else {
			result.Raw["history"] = history
			result.History = mapsFromItems(history["items"])
			result.HistoryCount = intValue(history["item_count"])
		}
	}
	return result, nil
}

func triggerReleaseService(backendURL, service, ref, cluster string, variables map[string]string) (map[string]any, error) {
	values := addCluster(url.Values{"service": {service}, "ref": {ref}}, cluster)
	for key, value := range variables {
		values.Set("var."+key, value)
	}
	body, err := post(backendURL, "/api/release/trigger", values)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("release trigger response missing data")
	}
	return data, nil
}

func rollbackReleaseService(backendURL, service, target, cluster string) (map[string]any, error) {
	body, err := post(backendURL, "/api/release/rollback", addCluster(url.Values{
		"service": {service},
		"to":      {target},
		"confirm": {"true"},
	}, cluster))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("release rollback response missing data")
	}
	return data, nil
}

func qualityCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected quality command: run, status, or runner")
	}
	switch args[0] {
	case "run":
		return runQualityRun(opts, args[1:], out)
	case "status":
		return runQualityStatus(opts, args[1:], out)
	case "runner":
		return runQualityRunner(args[1:], out)
	default:
		return fmt.Errorf("unknown quality command: %s", args[0])
	}
}

func runQualityRun(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "service" {
		return fmt.Errorf("expected: quality run service")
	}
	positionalService := ""
	args = args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("quality run service", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	baseURL := fs.String("base-url", "", "override quality base URL")
	cluster := fs.String("cluster", "", "cluster name")
	_ = fs.Parse(args)
	if *service == "" {
		*service = positionalService
	}
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("quality run service requires --service")
	}
	body, err := post(opts.backendURL, "/api/quality/run", addCluster(url.Values{"service": {*service}, "base_url": {*baseURL}}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	data, err := unwrapData(body, "quality run")
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, data, writeQualityHuman("Quality run", data))
}

func runQualityStatus(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "service" {
		return fmt.Errorf("expected: quality status service")
	}
	positionalService := ""
	args = args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalService = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("quality status service", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	_ = fs.Parse(args)
	if *service == "" {
		*service = positionalService
	}
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("quality status service requires --service")
	}
	body, err := get(opts.backendURL, "/api/quality/status", addCluster(url.Values{"service": {*service}}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	data, err := unwrapData(body, "quality status")
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, data, writeQualityHuman("Quality status", data))
}

func runQualityRunner(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("quality runner", flag.ExitOnError)
	configPath := fs.String("config", "", "quality config path, YAML or JSON")
	baseURL := fs.String("base-url", env("OPSPILOT_QUALITY_BASE_URL", ""), "quality base URL override")
	_ = fs.Parse(args)
	cfg, err := readQualityRunnerConfig(*configPath)
	if err != nil {
		return err
	}
	report := quality.Run(context.Background(), cfg, *baseURL, nil)
	return quality.WriteReport(out, report)
}

func readQualityRunnerConfig(path string) (quality.Config, error) {
	if raw := env("OPSPILOT_QUALITY_CONFIG_JSON", ""); raw != "" {
		return quality.ParseJSON(raw)
	}
	if path == "" {
		return quality.DefaultConfig(), nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return quality.Config{}, err
	}
	text := string(body)
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		return quality.ParseJSON(text)
	}
	return quality.ParseYAML(text)
}

func writeQualityHuman(title string, data map[string]any) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "%s: service=%s status=%s optional=%t\n",
			title, stringValue(data["service"]), stringValue(data["status"]), boolValue(data["optional"]))
		if reason := stringValue(data["reason"]); reason != "" {
			fmt.Fprintf(w, "Reason: %s\n", reason)
		}
		if namespace := stringValue(data["namespace"]); namespace != "" {
			fmt.Fprintf(w, "Namespace: %s\n", namespace)
		}
		if jobName := firstNonEmptyString(stringValue(data["job_name"]), stringValue(mapValue(data, "job")["name"])); jobName != "" {
			fmt.Fprintf(w, "Job: %s\n", jobName)
		}
		if report := mapValue(data, "report"); report != nil {
			fmt.Fprintf(w, "Report: status=%s checks=%d passed=%d failed=%d duration=%dms\n",
				stringValue(report["status"]), intValue(report["check_count"]), intValue(report["passed_count"]), intValue(report["failed_count"]), intValue(report["duration_ms"]))
			if summary := stringValue(report["summary"]); summary != "" {
				fmt.Fprintf(w, "Summary: %s\n", summary)
			}
		}
		if checks := stringList(data["next_checks"]); len(checks) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(checks, "; "))
		}
		if logsTail := stringValue(data["logs_tail"]); logsTail != "" {
			fmt.Fprintf(w, "Logs tail:\n%s\n", logsTail)
		}
		return nil
	}
}

func unwrapData(body []byte, label string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("%s response missing data", label)
	}
	return data, nil
}

func fetchReleaseStatusData(backendURL, service, cluster string) (map[string]any, error) {
	body, err := get(backendURL, "/api/release/status", addCluster(url.Values{"service": {service}}, cluster))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("release status response missing data")
	}
	return data, nil
}

func fetchReleaseJobsData(backendURL, service, cluster string) (map[string]any, error) {
	body, err := get(backendURL, "/api/release/jobs", addCluster(url.Values{"service": {service}}, cluster))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("release jobs response missing data")
	}
	return data, nil
}

func fetchReleaseHistoryData(backendURL, service, cluster string, limit int) (map[string]any, error) {
	body, err := get(backendURL, "/api/release/history", addCluster(url.Values{"service": {service}, "limit": {strconv.Itoa(limit)}}, cluster))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return nil, fmt.Errorf("release history response missing data")
	}
	return data, nil
}

func runReleaseStatus(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release status", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release status requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/status", addCluster(url.Values{"service": []string{*service}}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release: %s\n", stringValue(data["service"]))
		fmt.Fprintf(w, "Status: %s stage=%s namespace=%s deployment=%s\n",
			stringValue(data["status"]), stringValue(data["stage"]), stringValue(data["namespace"]), stringValue(data["deployment"]))
		if image := stringValue(data["image"]); image != "" {
			fmt.Fprintf(w, "Image: %s\n", image)
		}
		if evidence := mapValue(data, "evidence"); evidence != nil {
			if k8s := mapValue(evidence, "kubernetes"); k8s != nil {
				fmt.Fprintf(w, "Kubernetes: ready=%d desired=%d updated=%d available=%d\n",
					intValue(k8s["ready_replicas"]), intValue(k8s["desired_replicas"]), intValue(k8s["updated_replicas"]), intValue(k8s["available_replicas"]))
			}
			if pods := mapValue(evidence, "pods"); pods != nil {
				fmt.Fprintf(w, "Pods: %d/%d listed\n", intValue(pods["item_count"]), intValue(pods["total_count"]))
			}
			if registry := mapValue(evidence, "registry"); registry != nil {
				fmt.Fprintf(w, "Registry: %s tag=%s\n", stringValue(registry["status"]), stringValue(registry["tag"]))
			}
			if pipeline := mapValue(evidence, "gitlab_pipeline"); pipeline != nil {
				fmt.Fprintf(w, "GitLab: %s ref=%s sha=%s\n", stringValue(pipeline["status"]), stringValue(pipeline["ref"]), stringValue(pipeline["sha"]))
			}
			if gitops := mapValue(evidence, "gitops"); gitops != nil {
				fmt.Fprintf(w, "GitOps: %s image=%s\n", stringValue(gitops["status"]), stringValue(gitops["desired_image"]))
			}
			if argocd := mapValue(evidence, "argocd"); argocd != nil {
				fmt.Fprintf(w, "Argo CD: sync=%s health=%s\n", stringValue(argocd["sync_status"]), stringValue(argocd["health_status"]))
			}
			if quality := mapValue(evidence, "quality"); quality != nil {
				fmt.Fprintf(w, "Quality: %s reason=%s optional=%t\n",
					stringValue(quality["status"]), stringValue(quality["reason"]), boolValue(quality["optional"]))
			}
		}
		if gaps := stringList(data["gaps"]); len(gaps) > 0 {
			fmt.Fprintf(w, "Gaps: %s\n", strings.Join(gaps, ", "))
		}
		if checks := stringList(data["next_checks"]); len(checks) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(checks, "; "))
		}
		return nil
	})
}

func runReleaseJobs(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release jobs", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release jobs requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/jobs", addCluster(url.Values{"service": []string{*service}}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release jobs: %s\n", stringValue(data["service"]))
		if pipeline := mapValue(data, "pipeline"); pipeline != nil {
			fmt.Fprintf(w, "Pipeline: %s id=%d ref=%s sha=%s\n",
				stringValue(pipeline["status"]), intValue(pipeline["id"]), stringValue(pipeline["ref"]), stringValue(pipeline["sha"]))
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tSTAGE\tNAME\tSTATUS\tDURATION\tFAILURE")
		for _, job := range mapsFromItems(data["items"]) {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%.1fs\t%s\n",
				intValue(job["id"]), stringValue(job["stage"]), stringValue(job["name"]), stringValue(job["status"]), floatValue(job["duration"]), stringValue(job["failure_reason"]))
		}
		return tw.Flush()
	})
}

func runReleaseLogs(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release logs", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	job := fs.String("job", "", "GitLab job name")
	jobID := fs.String("job-id", "", "GitLab job id")
	tail := fs.Int("tail", 200, "tail lines")
	limitBytes := fs.Int("limit-bytes", 128*1024, "limit bytes")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release logs requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/logs", addCluster(url.Values{
		"service":     []string{*service},
		"job":         []string{*job},
		"job_id":      []string{*jobID},
		"tail_lines":  []string{strconv.Itoa(*tail)},
		"limit_bytes": []string{strconv.Itoa(*limitBytes)},
	}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release log: %s job=%s id=%d truncated=%t\n",
			stringValue(data["service"]), stringValue(data["job_name"]), intValue(data["job_id"]), boolValue(data["truncated"]))
		fmt.Fprintln(w, stringValue(data["text"]))
		return nil
	})
}

func runReleaseHistory(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release history", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	limit := fs.Int("limit", 10, "history item limit")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *service == "" {
		return fmt.Errorf("release history requires --service")
	}
	body, err := get(opts.backendURL, "/api/release/history", addCluster(url.Values{"service": []string{*service}, "limit": []string{strconv.Itoa(*limit)}}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Release history: %s\n", stringValue(data["service"]))
		if image := stringValue(data["current_image"]); image != "" {
			fmt.Fprintf(w, "Current image: %s\n", image)
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "CURRENT\tREVISION\tDATE\tTAG\tMESSAGE")
		for _, item := range mapsFromItems(data["items"]) {
			current := ""
			if boolValue(item["current"]) {
				current = "*"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				current,
				stringValue(item["short_revision"]),
				shortTime(stringValue(item["committed_at"])),
				stringValue(item["tag"]),
				oneLine(stringValue(item["message"]), 80),
			)
		}
		return tw.Flush()
	})
}

func runReleaseRollback(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release rollback", flag.ExitOnError)
	service := fs.String("service", "", "release service name")
	cluster := fs.String("cluster", "", "cluster name")
	target := fs.String("to", "", "target tag, full image, or GitOps revision")
	fs.StringVar(target, "target", "", "target tag, full image, or GitOps revision")
	confirm := fs.Bool("confirm", false, "confirm GitOps rollback commit")
	_ = fs.Parse(args)
	if *service == "" && fs.NArg() > 0 {
		*service = fs.Arg(0)
	}
	if *target == "" && fs.NArg() > 1 {
		*target = fs.Arg(1)
	}
	if *service == "" {
		return fmt.Errorf("release rollback requires --service")
	}
	if *target == "" {
		return fmt.Errorf("release rollback requires --to")
	}
	if !*confirm {
		return fmt.Errorf("release rollback requires --confirm")
	}
	body, err := post(opts.backendURL, "/api/release/rollback", addCluster(url.Values{
		"service": []string{*service},
		"to":      []string{*target},
		"confirm": []string{strconv.FormatBool(*confirm)},
	}, firstNonEmptyString(*cluster, opts.cluster)))
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	data := mapValue(payload, "data")
	return writeOutput(out, opts.output, data, func(w io.Writer) error {
		fmt.Fprintf(w, "Rollback: %s status=%s\n", stringValue(data["service"]), stringValue(data["status"]))
		fmt.Fprintf(w, "Previous: %s\n", stringValue(data["previous_image"]))
		fmt.Fprintf(w, "Target: %s\n", stringValue(data["target_image"]))
		fmt.Fprintf(w, "GitOps: %s %s branch=%s\n",
			stringValue(data["gitops_project"]), stringValue(data["gitops_path"]), stringValue(data["branch"]))
		if commit := stringValue(data["commit_short_id"]); commit != "" {
			fmt.Fprintf(w, "Commit: %s %s\n", commit, stringValue(data["commit_message"]))
		}
		if reason := stringValue(data["reason"]); reason != "" {
			fmt.Fprintf(w, "Reason: %s\n", reason)
		}
		if checks := stringList(data["next_checks"]); len(checks) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(checks, "; "))
		}
		return nil
	})
}

func fetchInspectCluster(backendURL, source, cluster string, limit int) (inspectClusterResult, error) {
	result := inspectClusterResult{Cluster: cluster, Raw: map[string]any{}}
	if capabilities, err := fetchCapabilities(backendURL, cluster); err == nil {
		result.AvailableEvidence = capabilities.AvailableEvidence
		result.MissingEvidence = capabilities.MissingEvidence
		result.CapabilityWarnings = capabilities.Warnings
		result.Raw["capabilities"] = capabilities.Raw
	} else {
		result.CapabilityWarnings = append(result.CapabilityWarnings, "capabilities: "+err.Error())
	}
	abnormal, _ := getJSONMap(backendURL, "/api/k8s/pods", addCluster(url.Values{"status": {"abnormal"}, "limit": {strconv.Itoa(limit)}}, cluster))
	nodes, _ := getJSONMap(backendURL, "/api/metrics/nodes", url.Values{"source": {source}, "limit": {"100"}})
	topCPU, _ := getJSONMap(backendURL, "/api/metrics/pods", url.Values{"source": {source}, "sort": {"cpu"}, "limit": {strconv.Itoa(limit)}})
	topMemory, _ := getJSONMap(backendURL, "/api/metrics/pods", url.Values{"source": {source}, "sort": {"memory"}, "limit": {strconv.Itoa(limit)}})
	result.Raw["abnormal_pods"] = abnormal
	result.Raw["nodes"] = nodes
	result.Raw["top_cpu_pods"] = topCPU
	result.Raw["top_memory_pods"] = topMemory
	if data := mapValue(abnormal, "data"); data != nil {
		result.AbnormalPods = data
		if intValue(data["total_count"]) == 0 && intValue(data["item_count"]) == 0 {
			result.Findings = append(result.Findings, "No abnormal Pods found.")
		}
	}
	if data := mapValue(nodes, "data"); data != nil {
		result.Nodes = mapsFromItems(data["items"])
		for _, node := range result.Nodes {
			if floatValue(node["memory_used_percent"]) >= 80 {
				result.Findings = append(result.Findings, "High node memory: "+stringValue(node["node"]))
			}
			if floatValue(node["rootfs_used_percent"]) >= 80 {
				result.Findings = append(result.Findings, "High root filesystem usage: "+stringValue(node["node"]))
			}
		}
	}
	if data := mapValue(topCPU, "data"); data != nil {
		result.TopCPU = mapsFromItems(data["items"])
	}
	if data := mapValue(topMemory, "data"); data != nil {
		result.TopMemory = mapsFromItems(data["items"])
	}
	restarts, err := fetchMetricItems(backendURL, "topk(20, sum by (namespace,pod,container) (increase(kube_pod_container_status_restarts_total[24h])))", "node200-k8s")
	if err == nil {
		for _, item := range restarts {
			if item.Value > 0 {
				result.Restarts24h = append(result.Restarts24h, item)
			}
		}
	}
	filesystems, err := fetchFilesystems(backendURL, source)
	if err == nil {
		result.Filesystems = filesystems.Items
		for _, row := range result.Filesystems {
			if row.UsedPct >= 80 {
				result.Findings = append(result.Findings, "High filesystem usage: "+row.Node+" "+row.Mount)
			}
		}
	}
	if len(result.Restarts24h) > 0 {
		result.Findings = append(result.Findings, fmt.Sprintf("%d containers have restarts in the last 24h.", len(result.Restarts24h)))
	}
	recommendations, warning := fetchSkillRecommendations(backendURL, "cluster", clusterEvidenceStatus(result), result.MissingEvidence, result.Findings)
	result.SkillRecommendations = recommendations
	if warning != "" {
		result.CapabilityWarnings = append(result.CapabilityWarnings, warning)
	}
	return result, nil
}

func fetchMetricItems(backendURL, query, source string) ([]metricItem, error) {
	body, err := get(backendURL, "/api/metrics/query", url.Values{"query": {query}, "source": {source}})
	if err != nil {
		return nil, err
	}
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	var data metricItemsData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, err
	}
	return data.Items, nil
}

func getJSONMap(backendURL, endpoint string, values url.Values) (map[string]any, error) {
	body, err := get(backendURL, endpoint, values)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

type evidenceSubject struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type evidenceItem struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

type likelyCause struct {
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type recommendedAction struct {
	Type        string `json:"type"`
	Target      string `json:"target,omitempty"`
	Instruction string `json:"instruction"`
}

type evidencePack struct {
	Subject              evidenceSubject                `json:"subject"`
	Status               string                         `json:"status"`
	Summary              string                         `json:"summary"`
	Evidence             []evidenceItem                 `json:"evidence"`
	MissingEvidence      []string                       `json:"missing_evidence,omitempty"`
	LikelyCauses         []likelyCause                  `json:"likely_causes,omitempty"`
	RecommendedActions   []recommendedAction            `json:"recommended_actions,omitempty"`
	SkillRecommendations []skillregistry.Recommendation `json:"skill_recommendations,omitempty"`
	Raw                  any                            `json:"raw,omitempty"`
}

func buildEvidencePack(payload any) evidencePack {
	switch v := payload.(type) {
	case doctorResult:
		return evidencePack{
			Subject:         evidenceSubject{Type: "opspilot", Name: v.BackendURL},
			Status:          statusFromBool(v.Ready),
			Summary:         strings.Join(v.Findings, "; "),
			Evidence:        evidenceItems("doctor", append([]string{fmt.Sprintf("backend_reachable=%t", v.BackendReachable)}, v.AvailableEvidence...)),
			MissingEvidence: v.MissingEvidence,
			LikelyCauses:    causesFromMissing(v.MissingEvidence),
			RecommendedActions: []recommendedAction{
				{Type: "next_check", Target: "cli", Instruction: strings.Join(v.Next, "; ")},
			},
			SkillRecommendations: skillregistry.Recommend("opspilot", statusFromBool(v.Ready), v.MissingEvidence, v.Findings),
		}
	case inspectPodResult:
		status := podEvidenceStatus(v)
		missing := uniqueStrings(append(v.MissingEvidence, v.EvidenceGaps...))
		return evidencePack{
			Subject:         evidenceSubject{Type: "pod", Name: v.Pod, Namespace: v.Namespace},
			Status:          status,
			Summary:         strings.Join(v.Findings, "; "),
			Evidence:        podEvidenceItems(v),
			MissingEvidence: missing,
			LikelyCauses:    podLikelyCauses(v),
			RecommendedActions: []recommendedAction{
				{Type: "next_check", Target: "pod", Instruction: "Review events, recent logs, resource usage, and missing evidence before changing code."},
			},
			SkillRecommendations: skillregistry.Recommend("pod", status, missing, v.Findings),
		}
	case inspectServiceResult:
		status := serviceEvidenceStatus(v)
		missing := uniqueStrings(append(append(v.MissingEvidence, v.EvidenceGaps...), v.ReleaseGaps...))
		actions := []recommendedAction{
			{Type: "code_or_config_review", Target: "repo", Instruction: "If logs or events point to application errors, inspect the service repository and generate a small fix."},
		}
		if next := strings.Join(v.Next, "; "); next != "" {
			actions = append([]recommendedAction{{Type: "next_check", Target: "service", Instruction: next}}, actions...)
		}
		return evidencePack{
			Subject:            evidenceSubject{Type: "service", Name: v.Service, Namespace: v.Namespace},
			Status:             status,
			Summary:            strings.Join(v.Findings, "; "),
			Evidence:           serviceEvidenceItems(v),
			MissingEvidence:    missing,
			LikelyCauses:       serviceLikelyCauses(v),
			RecommendedActions: actions,
			SkillRecommendations: skillregistry.Recommend("service", status, missing,
				append(v.Findings, v.Next...)),
		}
	case inspectClusterResult:
		status := clusterEvidenceStatus(v)
		return evidencePack{
			Subject:         evidenceSubject{Type: "cluster"},
			Status:          status,
			Summary:         strings.Join(v.Findings, "; "),
			Evidence:        clusterEvidenceItems(v),
			MissingEvidence: v.MissingEvidence,
			LikelyCauses:    causesFromMissing(v.MissingEvidence),
			RecommendedActions: []recommendedAction{
				{Type: "next_check", Target: "cluster", Instruction: "Inspect abnormal Pods, high restart containers, and high filesystem or memory usage first."},
			},
			SkillRecommendations: skillregistry.Recommend("cluster", status, v.MissingEvidence, v.Findings),
		}
	case fixPlanResult:
		return evidencePack{
			Subject:              evidenceSubject{Type: v.TargetType, Name: v.Target, Namespace: v.Namespace},
			Status:               v.Status,
			Summary:              v.Summary,
			Evidence:             v.Evidence,
			MissingEvidence:      v.MissingEvidence,
			LikelyCauses:         v.LikelyCauses,
			RecommendedActions:   v.RecommendedActions,
			SkillRecommendations: v.SkillRecommendations,
		}
	case map[string]any:
		if report := mapValue(v, "report"); report != nil || (boolValue(v["optional"]) && strings.HasPrefix(stringValue(v["reason"]), "quality_")) || strings.Contains(stringValue(v["job_name"]), "quality") || mapValue(v, "job") != nil {
			status := firstNonEmptyString(stringValue(v["status"]), stringValue(report["status"]), "unknown")
			summary := firstNonEmptyString(stringValue(report["summary"]), stringValue(v["reason"]), "Optional API quality evidence.")
			evidence := []evidenceItem{
				{Source: "quality", Message: fmt.Sprintf("status=%s optional=%t", status, boolValue(v["optional"]))},
			}
			if report != nil {
				evidence = append(evidence, evidenceItem{Source: "quality_report", Message: fmt.Sprintf("checks=%d passed=%d failed=%d duration=%dms",
					intValue(report["check_count"]), intValue(report["passed_count"]), intValue(report["failed_count"]), intValue(report["duration_ms"]))})
			}
			actions := []recommendedAction{
				{Type: "next_check", Target: "service", Instruction: "Use quality report together with release status, Pod logs, metrics, and events before changing code."},
			}
			if status == "failed" {
				actions = append(actions, recommendedAction{Type: "code_or_config_review", Target: "api", Instruction: "Inspect the failing endpoint, route, health check, service port, and application startup path."})
			}
			return evidencePack{
				Subject:            evidenceSubject{Type: "quality", Name: stringValue(v["service"]), Namespace: stringValue(v["namespace"])},
				Status:             status,
				Summary:            summary,
				Evidence:           evidence,
				RecommendedActions: actions,
				Raw:                v,
			}
		}
		return evidencePack{
			Subject: evidenceSubject{Type: "api_response", Name: firstNonEmptyString(stringValue(v["service"]), stringValue(v["name"]))},
			Status:  firstNonEmptyString(stringValue(v["status"]), "unknown"),
			Summary: firstNonEmptyString(stringValue(v["summary"]), "Raw API response evidence."),
			Evidence: []evidenceItem{
				{Source: "api", Message: "Raw response is available in raw."},
			},
			MissingEvidence: stringList(v["gaps"]),
			Raw:             v,
		}
	default:
		return evidencePack{
			Subject: evidenceSubject{Type: "unknown"},
			Status:  "unknown",
			Summary: "Raw payload evidence.",
			Evidence: []evidenceItem{
				{Source: "payload", Message: "Raw payload is available in raw."},
			},
			Raw: payload,
		}
	}
}

func statusFromBool(ok bool) string {
	if ok {
		return "healthy"
	}
	return "degraded"
}

func evidenceItems(source string, messages []string) []evidenceItem {
	out := []evidenceItem{}
	for _, message := range messages {
		if strings.TrimSpace(message) != "" {
			out = append(out, evidenceItem{Source: source, Message: message})
		}
	}
	return out
}

func evidenceItemMessages(items []evidenceItem) []string {
	messages := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Message) != "" {
			messages = append(messages, item.Message)
		}
	}
	return messages
}

func podEvidenceStatus(v inspectPodResult) string {
	if v.Ready && v.RestartCount == 0 {
		return "healthy"
	}
	if v.Ready {
		return "degraded"
	}
	return "unhealthy"
}

func serviceEvidenceStatus(v inspectServiceResult) string {
	if v.Status == "healthy" && v.RestartCount == 0 {
		return "healthy"
	}
	if v.Status == "" {
		return "unknown"
	}
	return v.Status
}

func clusterEvidenceStatus(v inspectClusterResult) string {
	if len(v.Findings) == 0 || (len(v.Findings) == 1 && strings.Contains(v.Findings[0], "No abnormal Pods")) {
		return "healthy"
	}
	return "degraded"
}

func podEvidenceItems(v inspectPodResult) []evidenceItem {
	items := []evidenceItem{
		{Source: "kubernetes_pod", Message: fmt.Sprintf("status=%s ready=%t restarts=%d node=%s", v.Status, v.Ready, v.RestartCount, v.Node)},
		{Source: "metrics", Message: fmt.Sprintf("cpu=%.3f cores memory=%.1f MiB", v.CPUCore, v.MemoryMiB)},
		{Source: "logs", Message: fmt.Sprintf("kubernetes_log_bytes=%d elk_hits=%d", v.KubernetesLogBytes, v.ElasticsearchLogHits)},
	}
	for _, finding := range v.Findings {
		items = append(items, evidenceItem{Source: "finding", Message: finding})
	}
	return items
}

func serviceEvidenceItems(v inspectServiceResult) []evidenceItem {
	items := []evidenceItem{
		{Source: "release", Message: fmt.Sprintf("status=%s stage=%s namespace=%s deployment=%s", v.Status, v.Stage, v.Namespace, v.Deployment)},
		{Source: "workload", Message: fmt.Sprintf("pods=%d restarts=%d cpu=%.3f cores memory=%.1f MiB", v.PodCount, v.RestartCount, v.TotalCPUCore, v.TotalMemoryMiB)},
	}
	if v.Image != "" {
		items = append(items, evidenceItem{Source: "image", Message: v.Image})
	}
	for _, finding := range v.Findings {
		items = append(items, evidenceItem{Source: "finding", Message: finding})
	}
	for _, pod := range v.Pods {
		items = append(items, evidenceItem{Source: "pod", Message: fmt.Sprintf("%s/%s status=%s ready=%t restarts=%d", pod.Namespace, pod.Pod, pod.Status, pod.Ready, pod.RestartCount)})
	}
	return items
}

func clusterEvidenceItems(v inspectClusterResult) []evidenceItem {
	items := []evidenceItem{
		{Source: "cluster", Message: fmt.Sprintf("nodes=%d top_cpu_pods=%d top_memory_pods=%d filesystems=%d", len(v.Nodes), len(v.TopCPU), len(v.TopMemory), len(v.Filesystems))},
	}
	for _, finding := range v.Findings {
		items = append(items, evidenceItem{Source: "finding", Message: finding})
	}
	return items
}

func podLikelyCauses(v inspectPodResult) []likelyCause {
	causes := []likelyCause{}
	if !v.Ready {
		causes = append(causes, likelyCause{Type: "runtime_or_configuration", Confidence: 0.7, Reason: "Pod is not ready."})
	}
	if v.RestartCount > 0 {
		causes = append(causes, likelyCause{Type: "application_crash_or_probe_failure", Confidence: 0.75, Reason: "Pod has restarts."})
	}
	return append(causes, causesFromMissing(v.EvidenceGaps)...)
}

func serviceLikelyCauses(v inspectServiceResult) []likelyCause {
	causes := []likelyCause{}
	if v.Status != "" && v.Status != "healthy" {
		causes = append(causes, likelyCause{Type: "release_or_rollout", Confidence: 0.75, Reason: "Release status is " + v.Status + "."})
	}
	if v.RestartCount > 0 {
		causes = append(causes, likelyCause{Type: "application_crash_or_probe_failure", Confidence: 0.75, Reason: "One or more Pods restarted."})
	}
	if v.PodCount == 0 {
		causes = append(causes, likelyCause{Type: "deployment_or_selector", Confidence: 0.65, Reason: "No Pods were found for the service."})
	}
	return append(causes, causesFromMissing(append(v.EvidenceGaps, v.ReleaseGaps...))...)
}

func causesFromMissing(missing []string) []likelyCause {
	if len(missing) == 0 {
		return nil
	}
	return []likelyCause{
		{Type: "missing_evidence", Confidence: 0.4, Reason: "Some integrations or evidence sources are missing: " + strings.Join(uniqueStrings(missing), ", ")},
	}
}

func writeOutput(out io.Writer, output string, payload any, table func(io.Writer) error) error {
	switch strings.ToLower(output) {
	case "", "json":
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(body))
		return err
	case "pretty":
		body, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(body))
		return err
	case "evidence":
		body, err := json.MarshalIndent(buildEvidencePack(payload), "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(body))
		return err
	case "table", "human":
		return table(out)
	default:
		return fmt.Errorf("unknown output: %s", output)
	}
}

func filesystemKey(item metricItem) string {
	return item.Source + "|" + metricNode(item.Metric) + "|" + item.Metric["device"] + "|" + item.Metric["mountpoint"]
}

func metricNode(metric map[string]string) string {
	if metric["node"] != "" {
		return metric["node"]
	}
	return metric["host"]
}

func mapValue(m map[string]any, key string) map[string]any {
	if value, ok := m[key].(map[string]any); ok {
		return value
	}
	return nil
}

func mapsFromItems(value any) []map[string]any {
	items, _ := value.([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func stringList(value any) []string {
	out := []string{}
	items, _ := value.([]any)
	for _, item := range items {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func boolValue(value any) bool {
	v, _ := value.(bool)
	return v
}

func intValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

func floatValue(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}

func round1(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func round3(value float64) float64 {
	return float64(int(value*1000+0.5)) / 1000
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func applyPrimaryContainerEvidence(result *inspectPodResult, summary map[string]any) {
	containers, _ := summary["containers"].([]any)
	if len(containers) == 0 {
		return
	}
	first, _ := containers[0].(map[string]any)
	if first == nil {
		return
	}
	result.Container = stringValue(first["name"])
	result.SpecImage = firstNonEmptyString(stringValue(first["spec_image"]), stringValue(first["image"]))
	result.StatusImage = stringValue(first["status_image"])
	result.ImageID = stringValue(first["image_id"])
}

func imageTagHint(pod inspectPodResult) string {
	image := firstNonEmptyString(pod.SpecImage, pod.StatusImage)
	if image == "" {
		return "-"
	}
	if idx := strings.LastIndex(image, ":"); idx >= 0 && idx < len(image)-1 {
		return image[idx+1:]
	}
	if idx := strings.LastIndex(image, "@"); idx >= 0 && idx < len(image)-1 {
		return image[idx+1:]
	}
	return image
}

func writeImageEvidenceHuman(w io.Writer, pod inspectPodResult) {
	if pod.SpecImage == "" && pod.StatusImage == "" && pod.ImageID == "" {
		return
	}
	if pod.Container != "" {
		fmt.Fprintf(w, "Container: %s\n", pod.Container)
	}
	if pod.SpecImage != "" {
		fmt.Fprintf(w, "Spec image: %s\n", pod.SpecImage)
	}
	if pod.StatusImage != "" {
		fmt.Fprintf(w, "Status image: %s\n", pod.StatusImage)
	}
	if pod.ImageID != "" {
		fmt.Fprintf(w, "Image ID: %s\n", pod.ImageID)
	}
	if pod.SpecImage != "" && pod.StatusImage != "" && pod.SpecImage != pod.StatusImage {
		fmt.Fprintln(w, "Image note: Kubernetes status may show an older tag when both tags point to the same image digest; use spec image and image ID for rollout evidence.")
	}
}

func logEvidenceFindings(result inspectPodResult, k8sLogAvailable, elkLogAvailable bool) []string {
	findings := []string{}
	switch {
	case k8sLogAvailable && result.KubernetesLogBytes > 0:
		findings = append(findings, "Kubernetes short-window logs are available.")
	case k8sLogAvailable:
		findings = append(findings, "Kubernetes short-window logs are empty; continue with Pod status, events, metrics, and release evidence.")
	default:
		findings = append(findings, "Kubernetes short-window logs could not be read; continue with Pod status, events, metrics, and release evidence.")
	}
	switch {
	case elkLogAvailable && result.ElasticsearchLogHits > 0:
		findings = append(findings, "ELK/OpenSearch log evidence is available.")
	case elkLogAvailable:
		findings = append(findings, "ELK/OpenSearch returned no matching logs for this Pod; this does not block Pod-level checks.")
	default:
		findings = append(findings, "ELK/OpenSearch is unavailable or not connected for this service; historical or rotated logs are missing, but Pod-level checks remain usable.")
	}
	return findings
}

func serviceLogEvidenceFindings(gaps []string) []string {
	findings := []string{}
	gapSet := map[string]bool{}
	for _, gap := range gaps {
		gapSet[gap] = true
	}
	switch {
	case gapSet["kubernetes_logs_unavailable"]:
		findings = append(findings, "Kubernetes short-window logs could not be read for at least one Pod; Pod status, events, metrics, and release evidence remain usable.")
	case gapSet["kubernetes_logs_empty"]:
		findings = append(findings, "Kubernetes short-window logs are empty for at least one Pod; this does not block status, event, metric, or release checks.")
	}
	switch {
	case gapSet["elk_logs_unavailable"] || gapSet["elk_logs_missing_or_empty"] || gapSet["elk_logs_empty"] || gapSet["elk_logs_missing"]:
		findings = append(findings, "ELK/OpenSearch log evidence is missing or unavailable; historical logs are incomplete, but current Pod-level checks remain usable.")
	}
	return findings
}

func availableCapabilityCount(items []capabilityItem) int {
	count := 0
	for _, item := range items {
		if item.Available {
			count++
		}
	}
	return count
}

func get(baseURL, endpoint string, values url.Values) ([]byte, error) {
	clean := url.Values{}
	for key, vals := range values {
		if len(vals) > 0 && vals[0] != "" {
			clean.Set(key, vals[0])
		}
	}
	target := strings.TrimRight(baseURL, "/") + endpoint
	if encoded := clean.Encode(); encoded != "" {
		target += "?" + encoded
	}
	resp, err := http.Get(target)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backend returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if json.Valid(body) {
		return body, nil
	}
	return nil, fmt.Errorf("backend returned non-json response")
}

func post(baseURL, endpoint string, values url.Values) ([]byte, error) {
	clean := url.Values{}
	for key, vals := range values {
		if len(vals) > 0 && vals[0] != "" {
			clean.Set(key, vals[0])
		}
	}
	target := strings.TrimRight(baseURL, "/") + endpoint
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(clean.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backend returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if json.Valid(body) {
		return body, nil
	}
	return nil, fmt.Errorf("backend returned non-json response")
}

func shortTime(value string) string {
	if len(value) >= len("2006-01-02T15:04:05") {
		return strings.ReplaceAll(value[:16], "T", " ")
	}
	return value
}

func oneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}
