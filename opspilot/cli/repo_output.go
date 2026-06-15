package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

func writeRepoPreflight(out io.Writer, output string, result repoPreflightResult) error {
	return writeOutput(out, output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Repo preflight: %s ready=%t autofixable=%t\n", result.Service, result.Ready, result.Autofixable)
		fmt.Fprintf(w, "Project: %s namespace=%s language=%s\n", result.Project, result.Namespace, result.Language)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "STATUS\tLEVEL\tCHECK\tFIX\tMESSAGE")
		for _, item := range result.Items {
			fix := ""
			if item.Fixable {
				fix = "auto"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", item.Status, item.Level, item.Name, fix, item.Message)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(uniqueStrings(result.Next), "; "))
		}
		return nil
	})
}

func writeRepoUploadPlan(out io.Writer, output string, result repoUploadPlanResult) error {
	return writeOutput(out, output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Repo upload plan: %s mode=%s ready=%t\n", result.RepoName, result.Mode, result.Ready)
		fmt.Fprintf(w, "Repo: %s\n", result.Repo)
		fmt.Fprintf(w, "Target: gitlab=%s env=%s owner=%s\n", result.Target.GitLabProject, result.Target.Environment, result.Target.Owner)
		fmt.Fprintf(w, "Runtime: namespace=%s gitops=%s scope=%s language=%s\n",
			result.Runtime.Namespace, result.Runtime.GitOpsPath, result.Runtime.ReleaseScope, result.Language)
		if len(result.Boundaries) > 0 {
			fmt.Fprintf(w, "Boundaries: %s\n", strings.Join(result.Boundaries, "; "))
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(result.Next, "; "))
		}
		return nil
	})
}

func writeCodePrecheck(out io.Writer, output string, result codePrecheckResult) error {
	return writeOutput(out, output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Code precheck: %s status=%s ready=%t blockers=%d warnings=%d\n",
			result.Service, result.Status, result.Ready, result.Summary.Blockers, result.Summary.Warnings)
		if result.Policy.Mode != "" {
			fmt.Fprintf(w, "Policy: %s audience=%s human_approval_required=%t\n",
				result.Policy.Mode, result.Policy.Audience, result.Policy.HumanApprovalRequired)
		}
		if result.EvidencePath != "" {
			fmt.Fprintf(w, "Evidence: %s\n", result.EvidencePath)
		}
		if len(result.Items) > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "SEVERITY\tCATEGORY\tFILE\tLINE\tSKILL\tMESSAGE")
			for _, item := range result.Items {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", item.Severity, item.Category, item.Path, item.Line, item.Skill, item.Message)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(result.Next, "; "))
		}
		return nil
	})
}

func writeCodePrecheckEvidence(result codePrecheckResult) error {
	path := codePrecheckEvidencePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	result.EvidencePath = path
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func codePrecheckEvidencePath() string {
	return filepath.Join(".opspilot", "evidence", "code-precheck.json")
}
