package audit

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Record struct {
	ID          string            `json:"id"`
	Time        string            `json:"time"`
	Actor       string            `json:"actor"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Action      string            `json:"action"`
	TargetType  string            `json:"target_type,omitempty"`
	Target      string            `json:"target,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Cluster     string            `json:"cluster,omitempty"`
	Risk        string            `json:"risk"`
	Outcome     string            `json:"outcome"`
	StatusCode  int               `json:"status_code,omitempty"`
	Error       string            `json:"error,omitempty"`
	Warnings    []string          `json:"warnings,omitempty"`
	EvidenceRef string            `json:"evidence_ref,omitempty"`
	Query       map[string]string `json:"query,omitempty"`
}

type Query struct {
	Limit   int
	Actor   string
	Action  string
	Risk    string
	Outcome string
}

type Result struct {
	Enabled   bool     `json:"enabled"`
	Source    string   `json:"source"`
	Count     int      `json:"count"`
	Items     []Record `json:"items"`
	Truncated bool     `json:"truncated"`
}

type Recorder struct {
	path string
	mu   sync.Mutex
}

func NewRecorder(path string) *Recorder {
	return &Recorder{path: strings.TrimSpace(path)}
}

func (r *Recorder) Enabled() bool {
	return r != nil && r.path != ""
}

func (r *Recorder) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

func (r *Recorder) Record(record Record) error {
	if !r.Enabled() {
		return nil
	}
	record = normalizeRecord(record)
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func (r *Recorder) Recent(query Query) (Result, error) {
	result := Result{Enabled: r.Enabled(), Source: sourceName(r)}
	if !r.Enabled() {
		return result, nil
	}
	if query.Limit <= 0 {
		query.Limit = 50
	}
	file, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}
	defer file.Close()

	items := []Record{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return result, err
		}
		if !match(record, query) {
			continue
		}
		items = append(items, record)
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Time > items[j].Time
	})
	total := len(items)
	if len(items) > query.Limit {
		items = items[:query.Limit]
	}
	result.Count = len(items)
	result.Items = items
	result.Truncated = total > len(items)
	return result, nil
}

func Policy() map[string]any {
	return map[string]any{
		"version": "v1",
		"levels": []map[string]any{
			{
				"risk":       "read_only",
				"automation": "auto_execute",
				"examples":   []string{"inspect", "release status", "metrics", "logs", "audit recent"},
			},
			{
				"risk":       "controlled_mutate",
				"automation": "plan_first",
				"examples":   []string{"release trigger", "release rollback --confirm", "quality run"},
			},
			{
				"risk":       "high_risk",
				"automation": "plan_only",
				"examples":   []string{"delete namespace", "delete data", "hostPath cleanup", "credential rotation"},
			},
			{
				"risk":       "forbidden",
				"automation": "blocked",
				"examples":   []string{"secret value dump", "unbounded shell execution", "unaudited destructive action"},
			},
		},
		"minimum_validation": []string{
			"Every controlled mutation must have an audit record.",
			"Every high-risk request must return a plan and minimum validation steps.",
			"Missing evidence must be reported explicitly instead of treated as success.",
		},
	}
}

func normalizeRecord(record Record) Record {
	if record.Time == "" {
		record.Time = time.Now().UTC().Format(time.RFC3339)
	}
	if record.Actor == "" {
		record.Actor = "anonymous"
	}
	if record.Action == "" {
		record.Action = strings.Trim(record.Method+" "+record.Path, " ")
	}
	if record.Risk == "" {
		record.Risk = RiskFor(record.Method, record.Path)
	}
	if record.Outcome == "" {
		record.Outcome = "unknown"
	}
	if record.ID == "" {
		record.ID = stableID(record.Time, record.Actor, record.Method, record.Path, record.Target, record.Outcome)
	}
	return record
}

func RiskFor(method, path string) string {
	method = strings.ToUpper(method)
	path = strings.ToLower(path)
	if method == "GET" {
		return "read_only"
	}
	if strings.Contains(path, "rollback") || strings.Contains(path, "trigger") || strings.Contains(path, "quality/run") {
		return "controlled_mutate"
	}
	if strings.Contains(path, "delete") || strings.Contains(path, "decommission") {
		return "high_risk"
	}
	return "controlled_mutate"
}

func match(record Record, query Query) bool {
	if query.Actor != "" && record.Actor != query.Actor {
		return false
	}
	if query.Action != "" && !strings.Contains(record.Action, query.Action) {
		return false
	}
	if query.Risk != "" && record.Risk != query.Risk {
		return false
	}
	if query.Outcome != "" && record.Outcome != query.Outcome {
		return false
	}
	return true
}

func stableID(parts ...string) string {
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])[:16]
}

func sourceName(r *Recorder) string {
	if r == nil || r.path == "" {
		return "disabled"
	}
	return fmt.Sprintf("jsonl:%s", r.path)
}
