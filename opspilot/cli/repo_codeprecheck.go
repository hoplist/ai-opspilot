package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func codePrecheck(project string) (codePrecheckResult, error) {
	detected, err := detectOnboardRepository(project, "opspilot.namespaces.yaml")
	if err != nil {
		return codePrecheckResult{}, err
	}
	cfg := detected.Config
	if err := cfg.defaults(); err != nil {
		return codePrecheckResult{}, err
	}
	items, err := scanCodePrecheckItems()
	if err != nil {
		return codePrecheckResult{}, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		if severityRank(items[i].Severity) != severityRank(items[j].Severity) {
			return severityRank(items[i].Severity) < severityRank(items[j].Severity)
		}
		if items[i].Path != items[j].Path {
			return items[i].Path < items[j].Path
		}
		return items[i].Line < items[j].Line
	})
	result := codePrecheckResult{
		Service: cfg.Name,
		Project: cfg.GitLabProject,
		Status:  "pass",
		Ready:   true,
		Items:   items,
		Policy: codePrecheckPolicy{
			Owner:                 "opspilot",
			Audience:              "vibecoding",
			Mode:                  "automatic_quality_gate",
			HumanApprovalRequired: false,
			BlockerRule:           "block only high-confidence failures that are likely to break runtime, expose secrets, corrupt data, or endanger nodes",
			WarningRule:           "do not block uncertain findings; provide AI-readable repair guidance",
		},
		Skills: []string{
			"code-reviewer",
			"security-reviewer",
			"secure-code-guardian",
			"database-optimizer",
			"debugging-wizard",
		},
	}
	for _, item := range items {
		switch item.Severity {
		case "blocker":
			result.Summary.Blockers++
		case "warning":
			result.Summary.Warnings++
		default:
			result.Summary.Passed++
		}
	}
	switch {
	case result.Summary.Blockers > 0:
		result.Ready = false
		result.Status = "blocker"
		result.Next = []string{
			"ask OpsPilot to explain code precheck blockers",
			"fix blocker findings before BuildKit packaging",
		}
	case result.Summary.Warnings > 0:
		result.Status = "warn"
		result.Next = []string{"review warning findings after release or ask OpsPilot for a fix plan"}
	default:
		result.Next = []string{"continue to language tests and BuildKit packaging"}
	}
	return result, nil
}

func scanCodePrecheckItems() ([]codePrecheckItem, error) {
	items := []codePrecheckItem{}
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if shouldSkipCodePrecheckDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldScanCodePrecheckFile(path) {
			return nil
		}
		body, ok := readSmallTextFile(path)
		if !ok {
			return nil
		}
		items = append(items, scanCodePrecheckText(filepath.ToSlash(path), string(body))...)
		return nil
	})
	items = append(items, scanVueRuntimeTemplateFindings()...)
	return dedupeCodePrecheckItems(items), err
}

func shouldSkipCodePrecheckDir(name string) bool {
	switch name {
	case ".git", ".opspilot", "node_modules", "vendor", "dist", "build", "target", ".next", ".venv", "venv", "__pycache__", "coverage", ".pytest_cache", "ci", "docs", "gitops-manifests-work":
		return true
	default:
		return false
	}
}

func shouldScanCodePrecheckFile(path string) bool {
	slashPath := filepath.ToSlash(path)
	if strings.HasPrefix(slashPath, "deploy/") ||
		strings.HasPrefix(slashPath, "ci/") ||
		strings.HasPrefix(slashPath, "docs/") ||
		strings.Contains(slashPath, "/test/") ||
		strings.Contains(slashPath, "/tests/") ||
		strings.Contains(slashPath, "/fixtures/") {
		return false
	}
	base := strings.ToLower(filepath.Base(path))
	if base == ".gitlab-ci.yml" ||
		base == "opspilot.service.yaml" ||
		base == "opspilot.namespaces.yaml" ||
		base == "opspilot.release-service.txt" ||
		strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, ".test.js") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".spec.js") ||
		strings.HasSuffix(base, ".spec.ts") {
		return false
	}
	switch base {
	case "dockerfile", ".env", ".env.example", "application.yml", "application.yaml", "application.properties":
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".java", ".kt", ".php", ".cs", ".sql", ".yml", ".yaml", ".properties", ".toml":
		return true
	default:
		return false
	}
}

func scanCodePrecheckText(path, text string) []codePrecheckItem {
	items := []codePrecheckItem{}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		items = append(items, codePrecheckLineFindings(path, lineNo, trimmed)...)
		if n1 := codePrecheckNPlusOne(path, lineNo, lines, i); n1 != nil {
			items = append(items, *n1)
		}
	}
	return items
}

func codePrecheckLineFindings(path string, lineNo int, line string) []codePrecheckItem {
	items := []codePrecheckItem{}
	lower := strings.ToLower(line)
	normalizedSQL := normalizeSQLLine(line)
	if isCodePrecheckRuleDefinitionLine(lower) {
		return items
	}
	switch {
	case looksLikeSecretLeak(line):
		items = append(items, codePrecheckFinding("secret_leak", "blocker", "security", path, lineNo, "possible hardcoded secret or token", line, "security-reviewer", "Move the value to GitLab/Kubernetes Secret and rotate it if it is real."))
	case containsDangerousShellLine(lower):
		items = append(items, codePrecheckFinding("dangerous_shell", "blocker", "security", path, lineNo, "dangerous shell execution detected", line, "security-reviewer", "Remove shell execution or replace it with a bounded, reviewed command without user input."))
	case containsDestructiveSQL(normalizedSQL):
		items = append(items, codePrecheckFinding("db_destructive_sql", "blocker", "database", path, lineNo, "destructive SQL detected", line, "database-optimizer", "Move destructive operations to an explicit migration/admin workflow with safeguards."))
	case containsUnguardedWriteSQL(normalizedSQL):
		items = append(items, codePrecheckFinding("db_unguarded_write", "blocker", "database", path, lineNo, "UPDATE/DELETE without a visible WHERE guard", line, "database-optimizer", "Add a guarded WHERE condition and verify affected rows before committing."))
	case containsQueryHandlerWrite(lower):
		items = append(items, codePrecheckFinding("api_query_writes_data", "blocker", "code", path, lineNo, "query-style handler appears to write data", line, "code-reviewer", "Separate read and write paths or change the route/handler semantics."))
	case containsUnboundedFileWrite(lower):
		items = append(items, codePrecheckFinding("unbounded_file_write", "blocker", "storage", path, lineNo, "possible unbounded write to logs/uploads/runtime path", line, "code-reviewer", "Add file size, type, retention, and path validation before writing."))
	case containsFullTableRead(normalizedSQL):
		items = append(items, codePrecheckFinding("db_full_table_read", "warning", "database", path, lineNo, "possible full-table read without pagination or filtering", line, "database-optimizer", "Add WHERE, LIMIT, or pagination and verify indexes."))
	case containsSelectStar(normalizedSQL):
		items = append(items, codePrecheckFinding("db_select_star", "warning", "database", path, lineNo, "SELECT * needs review", line, "database-optimizer", "Select only needed columns and confirm the query is bounded."))
	case containsRawSQLConstruction(line):
		items = append(items, codePrecheckFinding("raw_sql_construction", "warning", "security", path, lineNo, "raw SQL string construction needs review", line, "secure-code-guardian", "Use parameterized queries or ORM placeholders."))
	case containsMissingTimeoutHint(lower):
		items = append(items, codePrecheckFinding("missing_timeout_hint", "warning", "reliability", path, lineNo, "outbound client call may need an explicit timeout", line, "code-reviewer", "Configure request/database/client timeout to avoid stuck workers."))
	}
	return items
}

func isCodePrecheckRuleDefinitionLine(lower string) bool {
	return strings.Contains(lower, "codeprecheckfinding(") ||
		strings.Contains(lower, "regexp.mustcompile(") ||
		strings.Contains(lower, "containsany(") ||
		strings.Contains(lower, "strings.contains(lower,") ||
		strings.Contains(lower, "containsdangerousshellline(") ||
		strings.Contains(lower, "add('") ||
		strings.Contains(lower, "add(\"")
}

func codePrecheckFinding(id, severity, category, path string, line int, message, snippet, skill, recommendation string) codePrecheckItem {
	decision := "suggest_only"
	if severity == "blocker" {
		decision = "block_release"
	} else if severity == "warning" {
		decision = "warn_only"
	}
	return codePrecheckItem{
		ID:             id,
		Severity:       severity,
		Category:       category,
		Gate:           "auto_quality",
		Decision:       decision,
		Audience:       "vibecoding",
		Path:           path,
		Line:           line,
		Message:        message,
		Snippet:        truncateSnippet(snippet),
		Skill:          skill,
		Recommendation: recommendation,
		FixOptions:     codePrecheckFixOptions(id),
	}
}

func scanVueRuntimeTemplateFindings() []codePrecheckItem {
	body, ok := readSmallTextFile("package.json")
	if !ok {
		return nil
	}
	packageJSON := string(body)
	if !strings.Contains(packageJSON, `"vue"`) || strings.Contains(packageJSON, "@vitejs/plugin-vue") {
		return nil
	}
	items := []codePrecheckItem{}
	templatePattern := regexp.MustCompile(`(^|[,{]\s*)template\s*:`)
	_ = filepath.WalkDir("src", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipCodePrecheckDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".js" && ext != ".jsx" && ext != ".mjs" && ext != ".ts" && ext != ".tsx" {
			return nil
		}
		body, ok := readSmallTextFile(path)
		if !ok {
			return nil
		}
		text := string(body)
		lines := strings.Split(text, "\n")
		for _, match := range templatePattern.FindAllStringIndex(text, -1) {
			lineNo := strings.Count(text[:match[0]], "\n") + 1
			line := "template:"
			if lineNo > 0 && lineNo <= len(lines) {
				line = strings.TrimSpace(lines[lineNo-1])
			}
			items = append(items, codePrecheckFinding(
				"vue_runtime_template_without_compiler",
				"blocker",
				"frontend",
				filepath.ToSlash(path),
				lineNo,
				"Vue runtime-only build uses an inline template without compiler support",
				line,
				"vue-expert-js",
				"Use a .vue SFC with @vitejs/plugin-vue, configure the full Vue compiler build explicitly, or render with h()/render functions.",
			))
		}
		return nil
	})
	return items
}

func codePrecheckFixOptions(id string) []string {
	switch id {
	case "vue_runtime_template_without_compiler":
		return []string{
			"Recommended: convert the component to a .vue single-file component and add @vitejs/plugin-vue to Vite.",
			"Alternative: keep JavaScript-only code and replace template: with an h()/render function.",
			"Alternative: explicitly configure a Vue build that includes the runtime compiler.",
		}
	case "secret_leak":
		return []string{
			"Move the value to a GitLab CI variable or Kubernetes Secret.",
			"Rotate the token/password if it is real.",
			"Replace committed examples with placeholders such as YOUR_TOKEN_HERE.",
		}
	case "db_unguarded_write":
		return []string{
			"Add a WHERE condition and verify affected rows.",
			"Move bulk writes to an explicit migration/admin task.",
		}
	case "db_full_table_read":
		return []string{
			"Add pagination or LIMIT.",
			"Add filtering and confirm an index exists for the filter.",
		}
	default:
		return nil
	}
}

func codePrecheckNPlusOne(path string, lineNo int, lines []string, index int) *codePrecheckItem {
	lower := strings.ToLower(strings.TrimSpace(lines[index]))
	if !strings.HasPrefix(lower, "for ") && !strings.HasPrefix(lower, "for(") && !strings.HasPrefix(lower, "while ") {
		return nil
	}
	end := index + 8
	if end > len(lines) {
		end = len(lines)
	}
	window := strings.ToLower(strings.Join(lines[index:end], "\n"))
	if isHTTPQueryHelperWindow(window) {
		return nil
	}
	if containsAny(window, []string{".query(", ".find(", ".findall(", ".filter(", "select ", "jdbc", "gorm.", "db."}) {
		item := codePrecheckFinding("possible_n_plus_one", "warning", "database", path, lineNo, "loop contains database-like access", lines[index], "database-optimizer", "Batch the query, prefetch relations, or move database access outside the loop.")
		return &item
	}
	return nil
}

func isHTTPQueryHelperWindow(window string) bool {
	return strings.Contains(window, "r.url.query()") ||
		strings.Contains(window, ".url.query()") ||
		strings.Contains(window, "url.values") ||
		strings.Contains(window, "strings.fieldsfunc")
}

func normalizeSQLLine(line string) string {
	return strings.Join(strings.Fields(strings.ToLower(line)), " ")
}

func looksLikeSecretLeak(line string) bool {
	lower := strings.ToLower(line)
	if !containsAny(lower, []string{"password", "passwd", "secret", "token", "access_key", "accesskey", "api_key", "apikey", "private_key"}) {
		return false
	}
	if !strings.Contains(line, "=") && !strings.Contains(line, ":") {
		return false
	}
	if containsAny(lower, []string{"${", "$env", "env.", "env(", "getenv", "os.getenv", "secretref", "valuefrom", "example", "placeholder", "changeme", "your_", "<", "xxx", "token_or_api", "missing", "source_skill_path", "image_pull_secret", "pull_secret", "settings.", "qualitysettings."}) {
		return false
	}
	value := valueAfterAssignment(line)
	if value == "" || strings.ContainsAny(value, "()[]{}") {
		return false
	}
	return looksLikeSecretLiteral(value) || strings.Contains(lower, "private_key")
}

func valueAfterAssignment(line string) string {
	idx := strings.IndexAny(line, "=:")
	if idx < 0 {
		return ""
	}
	value := strings.TrimSpace(line[idx+1:])
	value = strings.Trim(value, `"', `)
	return value
}

func looksLikeSecretLiteral(value string) bool {
	value = strings.Trim(value, `"'`)
	if len(value) < 16 || strings.Contains(value, " ") {
		return false
	}
	if strings.HasPrefix(value, "glpat-") || strings.HasPrefix(value, "sk-") {
		return true
	}
	return regexp.MustCompile(`^[A-Za-z0-9_./+=:-]{16,}$`).MatchString(value)
}

func containsDangerousShellLine(lower string) bool {
	return strings.Contains(lower, "rm -rf /") ||
		((strings.Contains(lower, "curl ") || strings.Contains(lower, "wget ")) && (strings.Contains(lower, "| sh") || strings.Contains(lower, "| bash"))) ||
		strings.Contains(lower, "mkfs.") ||
		strings.Contains(lower, ":(){ :|:& };:")
}

func containsDestructiveSQL(sql string) bool {
	return regexp.MustCompile(`\b(drop|truncate)\s+(table|database|schema)\b`).MatchString(sql)
}

func containsUnguardedWriteSQL(sql string) bool {
	if !regexp.MustCompile(`\b(update\s+[a-zA-Z0-9_."` + "`" + `]+\s+set|delete\s+from\s+[a-zA-Z0-9_."` + "`" + `]+)\b`).MatchString(sql) {
		return false
	}
	return !strings.Contains(sql, " where ")
}

func containsQueryHandlerWrite(lower string) bool {
	return containsAny(lower, []string{"query", "search", "list", "get"}) &&
		containsAny(lower, []string{".save(", ".create(", ".insert(", ".update(", ".delete(", "delete from", "update "})
}

func containsUnboundedFileWrite(lower string) bool {
	return containsAny(lower, []string{"writefile", "create(", "openfile", "write("}) &&
		containsAny(lower, []string{"/logs", "/upload", "/uploads", "/runtime", "log_dir", "upload_dir", "runtime_dir"})
}

func containsFullTableRead(sql string) bool {
	if !regexp.MustCompile(`\b(select|findall|find_all|all\(\))\b`).MatchString(sql) {
		return false
	}
	if containsAny(sql, []string{" where ", " limit ", " offset ", " page", "paginate", "take(", "skip("}) {
		return false
	}
	return regexp.MustCompile(`\bfrom\s+[a-zA-Z0-9_]+`).MatchString(sql) || containsAny(sql, []string{"findall(", "find_all(", ".all()"})
}

func containsSelectStar(sql string) bool {
	return strings.Contains(sql, "select *") && !containsAny(sql, []string{" limit ", " where "})
}

func containsRawSQLConstruction(line string) bool {
	lower := strings.ToLower(line)
	if !containsAny(lower, []string{"select ", "update ", "delete ", "insert "}) {
		return false
	}
	return strings.Contains(line, "fmt.Sprintf") ||
		strings.Contains(line, "f\"") ||
		strings.Contains(line, "${") ||
		strings.Contains(line, "+")
}

func containsMissingTimeoutHint(lower string) bool {
	if containsAny(lower, []string{"timeout", "withtimeout", "context.withtimeout"}) {
		return false
	}
	return containsAny(lower, []string{"http.get(", "http.post(", "requests.get(", "requests.post(", "new resttemplate", "webclient.create("})
}

func truncateSnippet(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 160 {
		return value
	}
	return value[:160]
}

func dedupeCodePrecheckItems(items []codePrecheckItem) []codePrecheckItem {
	seen := map[string]bool{}
	out := []codePrecheckItem{}
	for _, item := range items {
		key := fmt.Sprintf("%s:%s:%d", item.ID, item.Path, item.Line)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func severityRank(value string) int {
	switch value {
	case "blocker":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}
