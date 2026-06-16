package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/contracts"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/version"
)

const defaultBackend = "http://127.0.0.1:18080"
const realFilesystemFilter = `fstype!~"tmpfs|overlay|squashfs|ramfs|cgroup2?|proc|sysfs|devtmpfs|devpts|securityfs|pstore|bpf|tracefs|debugfs|configfs|fusectl|mqueue|hugetlbfs"`
const cliHTTPTimeout = 30 * time.Second

var cliHTTPClient = &http.Client{Timeout: cliHTTPTimeout}

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
	case "config":
		return runConfigCommand(opts, args[1:], out)
	case "credentials", "credential":
		return runCredentialsCatalog(opts, args[1:], out)
	case "audit":
		return runAudit(opts, args[1:], out)
	case "services", "service":
		return runServicesCatalog(opts, args[1:], out)
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
	case "host":
		return runHostCommand(opts, args[1:], out)
	case "logs":
		if handled, err := runLogsCommand(opts, args[1:], out); handled {
			return err
		}
		endpoint, values = logsCommand(args[1:])
	case "evidence":
		if handled, err := runEvidenceCommand(opts, args[1:], out); handled {
			return err
		}
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
