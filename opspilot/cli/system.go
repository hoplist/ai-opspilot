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
