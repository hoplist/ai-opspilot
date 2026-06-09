package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
)

type skillsRegistryResult struct {
	Version         string                `json:"version"`
	Source          string                `json:"source"`
	SourcePath      string                `json:"source_path,omitempty"`
	SourceVersion   string                `json:"source_version,omitempty"`
	ItemCount       int                   `json:"item_count"`
	IntegratedCount int                   `json:"integrated_count"`
	DynamicCount    int                   `json:"dynamic_count,omitempty"`
	Items           []skillregistry.Skill `json:"items"`
	Warnings        []string              `json:"warnings,omitempty"`
}

func runSkillsRegistry(opts globalOptions, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "validate" {
		return runSkillsValidate(opts, args[1:], out)
	}
	if len(args) > 0 && args[0] == "sources" {
		return runSkillsSources(opts, args[1:], out)
	}
	if len(args) > 0 && args[0] == "candidates" {
		return runSkillsCandidates(opts, args[1:], out)
	}
	if len(args) > 0 && args[0] == "import-plan" {
		return runSkillsImportPlan(opts, args[1:], out)
	}
	if len(args) > 0 && args[0] == "promote" {
		return runSkillsPromote(opts, args[1:], out)
	}
	if len(args) > 0 && args[0] != "registry" && args[0] != "list" {
		return fmt.Errorf("expected skills subcommand: registry, validate, sources, candidates, import-plan, or promote")
	}
	if len(args) > 0 {
		args = args[1:]
	}
	fs := flag.NewFlagSet("skills registry", flag.ExitOnError)
	category := fs.String("category", "", "skill category filter")
	integratedOnly := fs.Bool("integrated-only", false, "show only integrated skills")
	_ = fs.Parse(args)

	result, err := fetchSkillsRegistry(opts.backendURL, *category, *integratedOnly)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, writeSkillsRegistryHuman(result))
}

func runSkillsValidate(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("skills validate", flag.ExitOnError)
	dir := fs.String("dir", "", "local skills directory to validate; omit to validate the backend runtime registry")
	_ = fs.Parse(args)
	var result skillregistry.ValidationResult
	var err error
	if strings.TrimSpace(*dir) != "" {
		result = skillregistry.ValidateDirectory(*dir)
	} else {
		result, err = fetchSkillsValidation(opts.backendURL)
		if err != nil {
			return err
		}
	}
	return writeOutput(out, opts.output, result, writeSkillsValidationHuman(result))
}

func runSkillsSources(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("skills sources", flag.ExitOnError)
	_ = fs.Parse(args)
	result, err := fetchSkillsMirror(opts.backendURL, "/api/skills/sources")
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, writeSkillsSourcesHuman(result))
}

func runSkillsCandidates(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("skills candidates", flag.ExitOnError)
	_ = fs.Parse(args)
	result, err := fetchSkillsMirror(opts.backendURL, "/api/skills/candidates")
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, writeSkillsCandidatesHuman(result))
}

func runSkillsImportPlan(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("skills import-plan", flag.ExitOnError)
	name := fs.String("name", "", "candidate skill name")
	_ = fs.Parse(args)
	result, err := fetchSkillsImportPlan(opts.backendURL, *name)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, writeSkillsImportPlanHuman(result))
}

func runSkillsPromote(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("skills promote", flag.ExitOnError)
	name := fs.String("name", "", "candidate skill name")
	dryRun := fs.Bool("dry-run", true, "generate a review plan without writing files")
	_ = fs.Parse(args)
	if !*dryRun {
		return fmt.Errorf("skills promote is dry-run only; commit reviewed files through the GitLab skills repository")
	}
	result, err := fetchSkillsImportPlan(opts.backendURL, *name)
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, writeSkillsImportPlanHuman(result))
}

func fetchSkillsRegistry(backendURL, category string, integratedOnly bool) (skillsRegistryResult, error) {
	body, err := get(backendURL, "/api/skills/registry", url.Values{
		"category":        {category},
		"integrated_only": {strconv.FormatBool(integratedOnly)},
	})
	if err != nil {
		return skillsRegistryResult{}, fmt.Errorf("backend skills registry unavailable: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return skillsRegistryResult{}, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return skillsRegistryResult{}, fmt.Errorf("skills registry response missing data")
	}
	raw, _ := json.Marshal(data)
	var catalog skillregistry.Catalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return skillsRegistryResult{}, err
	}
	return skillsResultFromCatalog(catalog, stringList(payload["warnings"])), nil
}

func fetchSkillsImportPlan(backendURL, name string) (skillregistry.ImportPlan, error) {
	body, err := get(backendURL, "/api/skills/import-plan", url.Values{"name": {name}})
	if err != nil {
		return skillregistry.ImportPlan{}, fmt.Errorf("backend skills import plan unavailable: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return skillregistry.ImportPlan{}, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return skillregistry.ImportPlan{}, fmt.Errorf("skills import plan response missing data")
	}
	raw, _ := json.Marshal(data)
	var result skillregistry.ImportPlan
	if err := json.Unmarshal(raw, &result); err != nil {
		return skillregistry.ImportPlan{}, err
	}
	return result, nil
}

func fetchSkillsValidation(backendURL string) (skillregistry.ValidationResult, error) {
	body, err := get(backendURL, "/api/skills/validate", url.Values{})
	if err != nil {
		return skillregistry.ValidationResult{}, fmt.Errorf("backend skills validation unavailable: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return skillregistry.ValidationResult{}, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return skillregistry.ValidationResult{}, fmt.Errorf("skills validation response missing data")
	}
	raw, _ := json.Marshal(data)
	var result skillregistry.ValidationResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return skillregistry.ValidationResult{}, err
	}
	return result, nil
}

func fetchSkillsMirror(backendURL, endpoint string) (skillregistry.MirrorIndex, error) {
	body, err := get(backendURL, endpoint, url.Values{})
	if err != nil {
		return skillregistry.MirrorIndex{}, fmt.Errorf("backend skills mirror unavailable: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return skillregistry.MirrorIndex{}, err
	}
	data := mapValue(payload, "data")
	if data == nil {
		return skillregistry.MirrorIndex{}, fmt.Errorf("skills mirror response missing data")
	}
	raw, _ := json.Marshal(data)
	var result skillregistry.MirrorIndex
	if err := json.Unmarshal(raw, &result); err != nil {
		return skillregistry.MirrorIndex{}, err
	}
	return result, nil
}

func skillsResultFromCatalog(catalog skillregistry.Catalog, warnings []string) skillsRegistryResult {
	return skillsRegistryResult{
		Version:         catalog.Version,
		Source:          catalog.Source,
		SourcePath:      catalog.SourcePath,
		SourceVersion:   catalog.SourceVersion,
		ItemCount:       catalog.ItemCount,
		IntegratedCount: catalog.IntegratedCount,
		DynamicCount:    catalog.DynamicCount,
		Items:           catalog.Items,
		Warnings:        warnings,
	}
}

func writeSkillsRegistryHuman(result skillsRegistryResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Skills registry: version=%s source=%s items=%d integrated=%d dynamic=%d\n",
			result.Version, result.Source, result.ItemCount, result.IntegratedCount, result.DynamicCount)
		if result.SourcePath != "" {
			fmt.Fprintf(w, "Source path: %s\n", result.SourcePath)
		}
		if result.SourceVersion != "" {
			fmt.Fprintf(w, "Source version: %s\n", result.SourceVersion)
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SKILL\tCATEGORY\tTIER\tINTEGRATED\tCOMMANDS")
		for _, item := range result.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\n",
				item.Name, item.Category, item.IntegrationTier, item.Integrated, oneLine(strings.Join(item.Commands, ", "), 100))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	}
}

func writeSkillsValidationHuman(result skillregistry.ValidationResult) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Skills validation: ready=%t root=%s skills=%d errors=%d warnings=%d\n",
			result.Ready, result.Root, result.SkillCount, result.ErrorCount, result.WarnCount)
		if len(result.SkillNames) > 0 {
			fmt.Fprintf(w, "Skills: %s\n", strings.Join(result.SkillNames, ", "))
		}
		if len(result.ExampleGaps) > 0 {
			fmt.Fprintf(w, "Example gaps: %s\n", strings.Join(result.ExampleGaps, ", "))
		}
		if len(result.Issues) == 0 {
			return nil
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "LEVEL\tSKILL\tFIELD\tMESSAGE")
		for _, issue := range result.Issues {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", issue.Level, issue.Skill, issue.Field, oneLine(issue.Message, 120))
		}
		return tw.Flush()
	}
}

func writeSkillsSourcesHuman(result skillregistry.MirrorIndex) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Skills mirror: ready=%t root=%s skills=%d candidates=%d unsupported=%d upstream=%d\n",
			result.Ready, result.Root, result.SkillsCount, result.CandidateCount, result.UnsupportedCount, result.UpstreamCount)
		if result.RegistryPath != "" {
			fmt.Fprintf(w, "Registry: %s\n", result.RegistryPath)
		}
		if len(result.Sources) > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "SOURCE\tSTATUS\tREASON")
			for _, source := range result.Sources {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", source.Name, source.Status, oneLine(source.Reason, 120))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	}
}

func writeSkillsCandidatesHuman(result skillregistry.MirrorIndex) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Skills candidates: ready=%t root=%s candidates=%d unsupported=%d\n",
			result.Ready, result.Root, result.CandidateCount, result.UnsupportedCount)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SKILL\tSTATUS\tCATEGORY\tSOURCE\tREASON")
		for _, item := range result.Candidates {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", item.Name, firstNonEmpty(item.Status, "candidate"), item.Category, item.Source, oneLine(item.Reason, 100))
		}
		for _, item := range result.Unsupported {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", item.Name, firstNonEmpty(item.Status, "unsupported"), item.Category, item.Source, oneLine(item.Reason, 100))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		return nil
	}
}

func writeSkillsImportPlanHuman(result skillregistry.ImportPlan) func(io.Writer) error {
	return func(w io.Writer) error {
		fmt.Fprintf(w, "Skills import plan: name=%s status=%s ready=%t dry_run=%t\n", result.Name, result.Status, result.Ready, result.DryRun)
		if result.Source != "" || result.Category != "" || result.RuntimePath != "" {
			fmt.Fprintf(w, "Source: %s category=%s runtime_path=%s\n", result.Source, result.Category, result.RuntimePath)
		}
		if result.Reason != "" {
			fmt.Fprintf(w, "Reason: %s\n", result.Reason)
		}
		if len(result.Files) > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "PATH\tEXISTS\tPREVIEW")
			for _, file := range result.Files {
				fmt.Fprintf(tw, "%s\t%t\t%s\n", file.Path, file.Exists, oneLine(file.Body, 100))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(w, "Warnings: %s\n", strings.Join(result.Warnings, "; "))
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(result.Next, " | "))
		}
		return nil
	}
}
