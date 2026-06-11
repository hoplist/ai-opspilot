package evidence

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Target struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Cluster   string `json:"cluster,omitempty"`
}

type Source struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type Gap struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Impact  string `json:"impact,omitempty"`
}

type Action struct {
	Type              string   `json:"type"`
	Risk              string   `json:"risk"`
	Command           string   `json:"command,omitempty"`
	Instruction       string   `json:"instruction"`
	MinimumValidation []string `json:"minimum_validation,omitempty"`
}

type Pack struct {
	ID                 string         `json:"id"`
	Version            string         `json:"version"`
	GeneratedAt        string         `json:"generated_at"`
	Trigger            string         `json:"trigger"`
	Target             Target         `json:"target"`
	Status             string         `json:"status"`
	Summary            string         `json:"summary"`
	Sources            []Source       `json:"sources"`
	Evidence           map[string]any `json:"evidence"`
	MissingEvidence    []Gap          `json:"missing_evidence,omitempty"`
	RecommendedActions []Action       `json:"recommended_actions,omitempty"`
	Warnings           []string       `json:"warnings,omitempty"`
}

type Store struct {
	dir string
}

func NewStore(dir string) *Store {
	return &Store{dir: strings.TrimSpace(dir)}
}

func (s *Store) Enabled() bool {
	return s != nil && s.dir != ""
}

func (s *Store) Write(pack Pack) (string, error) {
	if !s.Enabled() {
		return "", nil
	}
	pack = Normalize(pack)
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(s.dir, pack.ID+".json")
	body, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) Recent(limit int) (map[string]any, error) {
	result := map[string]any{
		"enabled": s.Enabled(),
		"source":  sourceName(s),
		"items":   []Pack{},
		"count":   0,
	}
	if !s.Enabled() {
		return result, nil
	}
	if limit <= 0 {
		limit = 20
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}
	items := []Pack{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return result, err
		}
		var pack Pack
		if err := json.Unmarshal(body, &pack); err != nil {
			return result, err
		}
		items = append(items, pack)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].GeneratedAt > items[j].GeneratedAt
	})
	truncated := len(items) > limit
	if truncated {
		items = items[:limit]
	}
	result["items"] = items
	result["count"] = len(items)
	result["truncated"] = truncated
	return result, nil
}

func Normalize(pack Pack) Pack {
	if pack.Version == "" {
		pack.Version = "v1"
	}
	if pack.GeneratedAt == "" {
		pack.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if pack.Trigger == "" {
		pack.Trigger = "manual"
	}
	if pack.Status == "" {
		pack.Status = "unknown"
	}
	if pack.Evidence == nil {
		pack.Evidence = map[string]any{}
	}
	if pack.ID == "" {
		pack.ID = stableID(pack.GeneratedAt, pack.Trigger, pack.Target.Type, pack.Target.Namespace, pack.Target.Name, pack.Status)
	}
	return pack
}

func GapFromCode(code string) Gap {
	code = strings.TrimSpace(code)
	return Gap{
		Code:    code,
		Message: messageForGap(code),
		Impact:  impactForGap(code),
	}
}

func GapsFromCodes(codes []string) []Gap {
	seen := map[string]bool{}
	out := []Gap{}
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, GapFromCode(code))
	}
	return out
}

func ReadOnlyNextCheck(command, instruction string) Action {
	return Action{
		Type:        "next_check",
		Risk:        "read_only",
		Command:     command,
		Instruction: instruction,
	}
}

func PlanOnlyAction(instruction string, validation []string) Action {
	return Action{
		Type:              "plan",
		Risk:              "high_risk",
		Instruction:       instruction,
		MinimumValidation: validation,
	}
}

func messageForGap(code string) string {
	switch code {
	case "elk_logs_missing", "logs.unavailable":
		return "ELK/OpenSearch logs are unavailable; current Kubernetes logs may still be usable."
	case "apisix.not_configured", "apisix_logs_missing":
		return "APISIX access logs are not configured; gateway-to-service correlation is unavailable."
	case "service_logs.missing", "service_logs_missing":
		return "Service log index is not configured; URI-level service log correlation is unavailable."
	case "release_mapping_missing":
		return "Service is not registered in the release/service catalog; Kubernetes evidence can still be used."
	default:
		return "Evidence source is missing or unavailable."
	}
}

func impactForGap(code string) string {
	switch code {
	case "elk_logs_missing", "logs.unavailable":
		return "Cannot inspect historical or rotated logs."
	case "apisix.not_configured", "apisix_logs_missing":
		return "Cannot prove external request path through gateway logs."
	case "service_logs.missing", "service_logs_missing":
		return "Cannot correlate by business URI inside service logs."
	default:
		return ""
	}
}

func stableID(parts ...string) string {
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])[:16]
}

func sourceName(s *Store) string {
	if s == nil || s.dir == "" {
		return "disabled"
	}
	return fmt.Sprintf("dir:%s", s.dir)
}
