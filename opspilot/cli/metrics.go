package main

import (
	"flag"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
)

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
