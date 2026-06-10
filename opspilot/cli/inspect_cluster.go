package main

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
)

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

func fetchInspectCluster(backendURL, source, cluster string, limit int) (inspectClusterResult, error) {
	result := inspectClusterResult{Cluster: cluster, Raw: map[string]any{}}
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
