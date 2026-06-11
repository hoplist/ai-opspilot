package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"text/tabwriter"
)

func runHostCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected host command")
	}
	switch args[0] {
	case "disk":
		data, warnings, err := fetchHostDisk(opts.backendURL, args[1:])
		if err != nil {
			return err
		}
		return writeOutput(out, opts.output, data, writeHostDiskHuman(data, warnings, false))
	case "cleanup":
		if len(args) < 2 || args[1] != "plan" {
			return fmt.Errorf("expected: host cleanup plan")
		}
		data, warnings, err := fetchHostDisk(opts.backendURL, args[2:])
		if err != nil {
			return err
		}
		return writeOutput(out, opts.output, data, writeHostDiskHuman(data, warnings, true))
	default:
		return fmt.Errorf("unknown host command: %s", args[0])
	}
}

func fetchHostDisk(backendURL string, args []string) (map[string]any, []string, error) {
	fs := flag.NewFlagSet("host disk", flag.ExitOnError)
	host := fs.String("host", "", "node agent host, or all")
	limit := fs.Int("limit", 20, "result limit")
	depth := fs.Int("depth", 2, "directory scan depth")
	_ = fs.Parse(args)
	body, err := get(backendURL, "/api/host/disk", url.Values{
		"host":  []string{*host},
		"limit": []string{strconv.Itoa(*limit)},
		"depth": []string{strconv.Itoa(*depth)},
	})
	if err != nil {
		return nil, nil, err
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, nil, err
	}
	data := mapValue(envelope, "data")
	if data == nil {
		return nil, nil, fmt.Errorf("host disk response missing data")
	}
	warnings := stringList(envelope["warnings"])
	warnings = append(warnings, stringList(data["warnings"])...)
	return data, warnings, nil
}

func writeHostDiskHuman(data map[string]any, warnings []string, planOnly bool) func(io.Writer) error {
	return func(w io.Writer) error {
		if data["items"] != nil {
			fmt.Fprintf(w, "Host disk: host=all count=%d\n", intValue(data["item_count"]))
			for _, item := range mapsFromItems(data["items"]) {
				if err := writeSingleHostDiskHuman(w, item, warnings, planOnly); err != nil {
					return err
				}
			}
			return nil
		}
		return writeSingleHostDiskHuman(w, data, warnings, planOnly)
	}
}

func writeSingleHostDiskHuman(w io.Writer, data map[string]any, warnings []string, planOnly bool) error {
	fmt.Fprintf(w, "Host disk: host=%s read_only=true\n", firstNonEmptyString(stringValue(data["host"]), "default"))
	if !planOnly {
		filesystems := mapsFromItems(data["filesystems"])
		if len(filesystems) > 0 {
			fmt.Fprintln(w, "\nFilesystems:")
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "PATH\tMOUNT\tFSTYPE\tFREE\tTOTAL\tUSED")
			for _, fs := range filesystems {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%.1f%%\n",
					stringValue(fs["path"]),
					stringValue(fs["mountpoint"]),
					stringValue(fs["fstype"]),
					formatBytes(floatValue(fs["avail_bytes"])),
					formatBytes(floatValue(fs["total_bytes"])),
					floatValue(fs["used_percent"]),
				)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		topPaths := mapsFromItems(data["top_paths"])
		if len(topPaths) > 0 {
			fmt.Fprintln(w, "\nTop paths:")
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "PATH\tSIZE\tDEPTH\tNOTE")
			for _, item := range topPaths {
				fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n",
					stringValue(item["path"]),
					formatBytes(floatValue(item["size_bytes"])),
					intValue(item["depth"]),
					stringValue(item["error"]),
				)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		docker := mapValue(data, "docker")
		if docker != nil {
			fmt.Fprintf(w, "\nDocker: available=%t reclaimable=%s images=%s containers_rw=%s volumes=%s build_cache=%s\n",
				boolValue(docker["available"]),
				formatBytes(floatValue(docker["approx_reclaimable_bytes"])),
				formatBytes(floatValue(docker["images_size_bytes"])),
				formatBytes(floatValue(docker["containers_size_rw_bytes"])),
				formatBytes(floatValue(docker["volumes_size_bytes"])),
				formatBytes(floatValue(docker["build_cache_size_bytes"])),
			)
		}
		logs := mapsFromItems(data["container_logs"])
		if len(logs) > 0 {
			fmt.Fprintln(w, "\nContainer logs:")
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "CONTAINER\tDRIVER\tSIZE\tLOG_PATH\tWARNING")
			for _, log := range logs {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					stringValue(log["container"]),
					stringValue(log["log_driver"]),
					formatBytes(floatValue(log["size_bytes"])),
					stringValue(log["log_path"]),
					stringValue(log["warning"]),
				)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
	}
	plans := mapsFromItems(data["cleanup_plan"])
	fmt.Fprintln(w, "\nCleanup plan:")
	if len(plans) == 0 {
		fmt.Fprintln(w, "- No cleanup candidate found from current read-only evidence.")
	} else {
		for _, plan := range plans {
			fmt.Fprintf(w, "- [%s] %s\n  evidence: %s\n  recommendation: %s\n  min_validation: %s\n  boundary: %s\n",
				stringValue(plan["risk"]),
				stringValue(plan["summary"]),
				stringValue(plan["evidence"]),
				stringValue(plan["recommendation"]),
				stringValue(plan["min_validation"]),
				stringValue(plan["execution_boundary"]),
			)
		}
	}
	if len(warnings) > 0 {
		fmt.Fprintln(w, "\nWarnings:")
		for _, warning := range warnings {
			fmt.Fprintf(w, "- %s\n", warning)
		}
	}
	return nil
}

func formatBytes(value float64) string {
	if value <= 0 {
		return "0B"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	for _, unit := range units {
		if value < 1024 || unit == "PiB" {
			if unit == "B" {
				return fmt.Sprintf("%.0f%s", value, unit)
			}
			return fmt.Sprintf("%.1f%s", value, unit)
		}
		value = value / 1024
	}
	return "0B"
}
