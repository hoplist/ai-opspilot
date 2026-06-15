package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
)

type networkCounter struct {
	rx uint64
	tx uint64
}

func collectHostNetwork(ctx context.Context, docker *dockerClient, cfg config, req nodeagent.HostNetworkRequest) (nodeagent.HostNetworkResponse, error) {
	req = nodeagent.BoundedHostNetworkRequest(req)
	warnings := []string{}
	before, err := readNetDev(cfg.hostRoot)
	if err != nil {
		warnings = append(warnings, "netdev before: "+err.Error())
	}
	containerBefore, containerWarnings := collectContainerNetworkSnapshot(ctx, docker, cfg, req.Limit)
	warnings = append(warnings, containerWarnings...)
	if err := waitForSample(ctx, time.Duration(req.DurationSeconds)*time.Second); err != nil {
		return nodeagent.HostNetworkResponse{}, err
	}
	after, err := readNetDev(cfg.hostRoot)
	if err != nil {
		warnings = append(warnings, "netdev after: "+err.Error())
	}
	containerAfter, containerWarnings := collectContainerNetworkSnapshot(ctx, docker, cfg, req.Limit)
	warnings = append(warnings, containerWarnings...)
	tcpStates, err := readTCPStates(cfg.hostRoot)
	if err != nil {
		warnings = append(warnings, "tcp states: "+err.Error())
	}
	return nodeagent.HostNetworkResponse{
		Duration:   req.DurationSeconds,
		Interfaces: interfaceDeltas(before, after, req.DurationSeconds, req.Limit),
		Containers: containerDeltas(containerBefore, containerAfter, req.DurationSeconds, req.Limit),
		TCPStates:  tcpStates,
		Limits: nodeagent.HostNetworkLimits{
			DurationSeconds: req.DurationSeconds,
			TopLimit:        req.Limit,
			ReadOnly:        true,
			EBPFEnabled:     false,
		},
		Warnings: warnings,
	}, nil
}

func waitForSample(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func readNetDev(hostRoot string) (map[string]networkCounter, error) {
	path := "/proc/net/dev"
	if strings.TrimSpace(hostRoot) != "" {
		path = filepath.Join(hostRoot, "proc", "net", "dev")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := map[string]networkCounter{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 16 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		out[strings.TrimSpace(name)] = networkCounter{rx: rx, tx: tx}
	}
	return out, scanner.Err()
}

func collectContainerNetworkSnapshot(ctx context.Context, docker *dockerClient, cfg config, limit int) (map[string]nodeagent.HostContainerNetwork, []string) {
	containers, err := docker.containers(ctx)
	if err != nil {
		return nil, []string{"container network: " + err.Error()}
	}
	out := map[string]nodeagent.HostContainerNetwork{}
	warnings := []string{}
	count := 0
	for _, raw := range containers {
		item := containerSummary(raw)
		if !allowedContainer(cfg.allowedContainers, item) {
			continue
		}
		id := fmt.Sprint(item["id"])
		stats, err := docker.stats(ctx, id)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("container network %s: %v", firstContainerName(item), err))
			continue
		}
		network := networkStats(stats)
		out[id] = nodeagent.HostContainerNetwork{
			Container: firstContainerName(item),
			ID:        shortID(id),
			RXBytes:   uint64(floatValue(network["rx_bytes"])),
			TXBytes:   uint64(floatValue(network["tx_bytes"])),
		}
		count++
		if count >= limit {
			break
		}
	}
	return out, warnings
}

func interfaceDeltas(before, after map[string]networkCounter, durationSeconds, limit int) []nodeagent.HostNetworkInterface {
	items := []nodeagent.HostNetworkInterface{}
	duration := float64(durationSeconds)
	for name, current := range after {
		previous := before[name]
		rxDelta := counterDelta(previous.rx, current.rx)
		txDelta := counterDelta(previous.tx, current.tx)
		if current.rx == 0 && current.tx == 0 && rxDelta == 0 && txDelta == 0 {
			continue
		}
		items = append(items, nodeagent.HostNetworkInterface{
			Name:    name,
			RXBytes: current.rx,
			TXBytes: current.tx,
			RXBps:   float64(rxDelta) / duration,
			TXBps:   float64(txDelta) / duration,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		left := items[i].RXBps + items[i].TXBps
		right := items[j].RXBps + items[j].TXBps
		if left == right {
			return items[i].Name < items[j].Name
		}
		return left > right
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func containerDeltas(before, after map[string]nodeagent.HostContainerNetwork, durationSeconds, limit int) []nodeagent.HostContainerNetwork {
	items := []nodeagent.HostContainerNetwork{}
	duration := float64(durationSeconds)
	for id, current := range after {
		previous := before[id]
		rxDelta := counterDelta(previous.RXBytes, current.RXBytes)
		txDelta := counterDelta(previous.TXBytes, current.TXBytes)
		current.RXBps = float64(rxDelta) / duration
		current.TXBps = float64(txDelta) / duration
		items = append(items, current)
	}
	sort.Slice(items, func(i, j int) bool {
		left := items[i].RXBps + items[i].TXBps
		right := items[j].RXBps + items[j].TXBps
		if left == right {
			return items[i].Container < items[j].Container
		}
		return left > right
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func counterDelta(before, after uint64) uint64 {
	if after < before {
		return 0
	}
	return after - before
}

func readTCPStates(hostRoot string) (map[string]int, error) {
	files := []string{"/proc/net/tcp", "/proc/net/tcp6"}
	if strings.TrimSpace(hostRoot) != "" {
		files = []string{
			filepath.Join(hostRoot, "proc", "net", "tcp"),
			filepath.Join(hostRoot, "proc", "net", "tcp6"),
		}
	}
	out := map[string]int{}
	for _, file := range files {
		if err := addTCPStates(file, out); err != nil && len(out) == 0 {
			return out, err
		}
	}
	return out, nil
}

func addTCPStates(path string, out map[string]int) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		out[tcpStateName(fields[3])]++
	}
	return scanner.Err()
}

func tcpStateName(hexState string) string {
	switch strings.ToUpper(hexState) {
	case "01":
		return "ESTABLISHED"
	case "02":
		return "SYN_SENT"
	case "03":
		return "SYN_RECV"
	case "04":
		return "FIN_WAIT1"
	case "05":
		return "FIN_WAIT2"
	case "06":
		return "TIME_WAIT"
	case "07":
		return "CLOSE"
	case "08":
		return "CLOSE_WAIT"
	case "09":
		return "LAST_ACK"
	case "0A":
		return "LISTEN"
	case "0B":
		return "CLOSING"
	default:
		return hexState
	}
}
