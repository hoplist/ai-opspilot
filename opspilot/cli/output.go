package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

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
