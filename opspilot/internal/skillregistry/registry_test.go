package skillregistry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryIntegratedCoreSkills(t *testing.T) {
	catalog := Registry("", true)
	if catalog.IntegratedCount < 6 {
		t.Fatalf("integrated count = %d", catalog.IntegratedCount)
	}
	required := []string{
		"opspilot-ops",
		"auto-inspection-rca",
		"kubernetes-specialist",
		"monitoring-expert",
		"devops-engineer",
		"code-reviewer",
		"security-reviewer",
		"secure-code-guardian",
		"database-optimizer",
		"debugging-wizard",
	}
	for _, name := range required {
		if !hasSkill(catalog.Items, name) {
			t.Fatalf("missing skill %s", name)
		}
	}
}

func TestRecommendPodIncludesKubernetesAndDebugging(t *testing.T) {
	recommendations := Recommend("pod", "unhealthy", []string{"elk_logs_missing"}, []string{"Pod is not ready", "restart count is high"})
	for _, name := range []string{"opspilot-ops", "kubernetes-specialist", "monitoring-expert", "debugging-wizard", "auto-inspection-rca"} {
		if !hasRecommendation(recommendations, name) {
			t.Fatalf("missing recommendation %s: %#v", name, recommendations)
		}
	}
}

func TestRecommendCodePrecheckIncludesReviewSkills(t *testing.T) {
	recommendations := Recommend("code-precheck", "blocker", []string{}, []string{"db_unguarded_write", "secret_leak"})
	for _, name := range []string{"code-reviewer", "security-reviewer", "secure-code-guardian", "database-optimizer", "devops-engineer"} {
		if !hasRecommendation(recommendations, name) {
			t.Fatalf("missing recommendation %s: %#v", name, recommendations)
		}
	}
}

func TestRegistryWithOptionsLoadsDynamicSkills(t *testing.T) {
	dir := t.TempDir()
	writeTestSkillRepo(t, dir)
	catalog, warnings := RegistryWithOptions("", true, Options{DynamicEnabled: true, SkillsDir: dir})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if catalog.Source != "gitlab" || catalog.SourceVersion != "abc123" || catalog.DynamicCount != 1 || catalog.ItemCount != 1 {
		t.Fatalf("catalog source = %#v", catalog)
	}
	found := false
	for _, item := range catalog.Items {
		if item.Name == "opspilot-ops" {
			found = true
			if item.Label != "OpsPilot Ops Dynamic" || !hasString(item.Commands, "ask") {
				t.Fatalf("dynamic skill not applied: %#v", item)
			}
		}
	}
	if !found {
		t.Fatal("dynamic opspilot-ops not found")
	}
}

func TestRegistryWithOptionsLoadsDynamicSkillsFromSymlinkRoot(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "releases", "abc123")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkillRepo(t, releaseDir)
	current := filepath.Join(dir, "current")
	if err := os.Symlink(filepath.Join("releases", "abc123"), current); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	catalog, warnings := RegistryWithOptions("", true, Options{DynamicEnabled: true, SkillsDir: current})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if catalog.Source != "gitlab" || catalog.DynamicCount != 1 || !hasSkill(catalog.Items, "opspilot-ops") {
		t.Fatalf("catalog = %#v", catalog)
	}
}

func TestRegistryWithOptionsLoadsGitSyncStyleSkillsSubdir(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "root", "abc123")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkillRepo(t, filepath.Join(releaseDir, "skills"))
	_ = os.Remove(filepath.Join(releaseDir, "skills", ".opspilot-skills-version"))
	current := filepath.Join(dir, "current")
	if err := os.Symlink(filepath.Join("root", "abc123"), current); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	catalog, warnings := RegistryWithOptions("", true, Options{DynamicEnabled: true, SkillsDir: filepath.Join(current, "skills")})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if catalog.SourceVersion != "abc123" || catalog.DynamicCount != 1 || !hasSkill(catalog.Items, "opspilot-ops") {
		t.Fatalf("catalog = %#v", catalog)
	}
}

func TestValidateDirectoryReportsReadySkillRepo(t *testing.T) {
	dir := t.TempDir()
	writeTestSkillRepo(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "opspilot-ops", "SKILL.md"), []byte("# OpsPilot Ops\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "opspilot-ops", "examples"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := ValidateDirectory(dir)
	if !result.Ready || result.ErrorCount != 0 || result.SkillCount != 1 {
		t.Fatalf("validation result = %#v", result)
	}
}

func TestValidateDirectoryBlocksArbitraryShellCommand(t *testing.T) {
	dir := t.TempDir()
	writeTestSkillRepo(t, dir)
	yaml := `name: risky
label: Risky
category: security
integrated: true
commands:
  - kubectl delete pod demo
`
	skillDir := filepath.Join(dir, "risky")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	result := ValidateDirectory(dir)
	if result.Ready || result.ErrorCount == 0 {
		t.Fatalf("expected validation error: %#v", result)
	}
}

func TestMirrorWithSkillsDirReadsRegistryAndCandidates(t *testing.T) {
	root := t.TempDir()
	writeTestSkillRepo(t, filepath.Join(root, "skills"))
	if err := os.MkdirAll(filepath.Join(root, "upstream", "garrytan-gstack"), 0o755); err != nil {
		t.Fatal(err)
	}
	registry := `version: test
sources:
  - name: garrytan/gstack
    status: mirrored
    reason: local mirror for OpsPilot adaptation
skills:
  - name: gstack-ship
    status: integrated
    source: garrytan/gstack
    runtime_path: skills/gstack-ship
    category: release
    priority: 90
  - name: gstack-health
    status: candidate
    source: garrytan/gstack
    upstream_path: skills/health
    category: platform
    reason: useful but not mapped yet
  - name: gstack-browse
    status: unsupported
    source: garrytan/gstack
    reason: requires client browser runtime
`
	if err := os.WriteFile(filepath.Join(root, "registry.yaml"), []byte(registry), 0o600); err != nil {
		t.Fatal(err)
	}
	index := MirrorWithSkillsDir(filepath.Join(root, "skills"))
	if !index.Ready || index.SkillsCount != 1 || index.CandidateCount != 1 || index.UnsupportedCount != 1 || index.UpstreamCount != 1 {
		t.Fatalf("index = %#v", index)
	}
	if len(index.Sources) != 1 || index.Sources[0].Name != "garrytan/gstack" {
		t.Fatalf("sources = %#v", index.Sources)
	}
}

func TestImportPlanWithSkillsDirGeneratesDraftForCandidate(t *testing.T) {
	root := t.TempDir()
	writeTestSkillRepo(t, filepath.Join(root, "skills"))
	registry := `version: test
sources:
  - name: garrytan/gstack
    status: mirrored
skills:
  - name: gstack-health
    status: candidate
    source: garrytan/gstack
    upstream_path: skills/health
    category: platform
    reason: useful server-side health review
`
	if err := os.WriteFile(filepath.Join(root, "registry.yaml"), []byte(registry), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := ImportPlanWithSkillsDir(filepath.Join(root, "skills"), "gstack-health")
	if !plan.Ready || !plan.DryRun || plan.Status != "candidate_plan" || plan.RuntimePath != "skills/gstack-health" {
		t.Fatalf("plan = %#v", plan)
	}
	if len(plan.Files) != 3 || plan.Files[0].Path != "skills/gstack-health/skill.yaml" {
		t.Fatalf("files = %#v", plan.Files)
	}
	if !strings.Contains(plan.Files[0].Body, "integrated: false") || !strings.Contains(plan.Files[0].Body, "doctor") {
		t.Fatalf("skill draft = %s", plan.Files[0].Body)
	}
}

func TestImportPlanWithSkillsDirDoesNotPromoteUnsupported(t *testing.T) {
	root := t.TempDir()
	writeTestSkillRepo(t, filepath.Join(root, "skills"))
	registry := `version: test
skills:
  - name: gstack-browse
    status: unsupported
    source: garrytan/gstack
    reason: requires browser runtime
`
	if err := os.WriteFile(filepath.Join(root, "registry.yaml"), []byte(registry), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := ImportPlanWithSkillsDir(filepath.Join(root, "skills"), "gstack-browse")
	if plan.Ready || plan.Status != "unsupported" || len(plan.Files) != 0 {
		t.Fatalf("plan = %#v", plan)
	}
	if !strings.Contains(strings.Join(plan.Warnings, " "), "browser runtime") {
		t.Fatalf("warnings = %#v", plan.Warnings)
	}
}

func writeTestSkillRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".opspilot-skills-version"), []byte("abc123"), 0o600); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(dir, "opspilot-ops")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `name: opspilot-ops
label: OpsPilot Ops Dynamic
category: platform
integration_tier: core
integrated: true
priority: 110
summary: Dynamic server-side OpsPilot skill.
commands:
  - ask
  - inspect service
boundaries:
  - no arbitrary shell
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasSkill(items []Skill, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func hasString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func hasRecommendation(items []Recommendation, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
