package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/contracts"
)

const defaultBackend = "http://127.0.0.1:18080"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, `{"ok":false,"error":`+strconv.Quote(err.Error())+`}`)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	backendURL := env("OPSPILOT_BACKEND_URL", defaultBackend)
	args = consumeGlobalFlags(args, &backendURL)
	if len(args) == 0 {
		return fmt.Errorf("missing command")
	}
	var endpoint string
	var values url.Values
	switch args[0] {
	case "schema":
		_, err := out.Write(contracts.CLISchema)
		if err == nil {
			_, err = fmt.Fprintln(out)
		}
		return err
	case "inventory":
		endpoint, values = inventoryCommand(args[1:])
	case "k8s":
		endpoint, values = k8sCommand(args[1:])
	case "context":
		endpoint, values = podRefCommand(args[1:], "/api/context/pod")
	case "diagnose":
		endpoint, values = podRefCommand(args[1:], "/api/diagnose/pod")
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
	body, err := get(backendURL, endpoint, values)
	if err != nil {
		return err
	}
	_, err = out.Write(body)
	if err == nil {
		_, err = fmt.Fprintln(out)
	}
	return err
}

func consumeGlobalFlags(args []string, backendURL *string) []string {
	out := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--backend-url" && i+1 < len(args) {
			*backendURL = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--backend-url=") {
			*backendURL = strings.TrimPrefix(arg, "--backend-url=")
			continue
		}
		out = append(out, arg)
	}
	return out
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
		since := fs.Int("since", 1800, "since seconds")
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
	_ = fs.Parse(args[1:])
	return endpoint, url.Values{"namespace": []string{*namespace}, "pod": []string{*pod}}
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
