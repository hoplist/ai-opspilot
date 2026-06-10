package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

func repoCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected repo command: preflight, precheck, or autofix")
	}
	switch args[0] {
	case "preflight":
		return repoPreflightCommand(opts, args[1:], out)
	case "precheck":
		return repoPrecheckCommand(opts, args[1:], out)
	case "autofix":
		return repoAutofixCommand(opts, args[1:], out)
	default:
		return fmt.Errorf("unknown repo command: %s", args[0])
	}
}

func repoPreflightCommand(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("repo preflight", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	project := fs.String("project", "", "GitLab project path, for example tpo/devex/skillshub/skillshub-api")
	catalog := fs.String("namespace-catalog", "opspilot.namespaces.yaml", "namespace catalog path")
	ciPath := fs.String("ci-path", ".gitlab-ci.yml", "GitLab CI file path relative to repository path")
	deployPath := fs.String("deploy-path", filepath.Join("deploy", "k8s"), "Kubernetes manifest directory relative to repository path")
	namespace := fs.String("namespace", "", "override expected Kubernetes namespace for preflight")
	namespacePath := fs.String("namespace-path", "", "namespace manifest path relative to repository path")
	limitrangePath := fs.String("limitrange-path", "", "LimitRange manifest path relative to repository path")
	resourcequotaPath := fs.String("resourcequota-path", "", "ResourceQuota manifest path relative to repository path")
	serviceaccountPath := fs.String("serviceaccount-path", "", "ServiceAccount manifest path relative to repository path")
	qualityPath := fs.String("quality-path", "", "optional quality config path relative to repository path")
	_ = fs.Parse(args)
	result, err := withRepo(*repo, func() (repoPreflightResult, error) {
		return repoPreflight(*project, *catalog, repoLayoutOptions{
			CIPath:             *ciPath,
			DeployPath:         *deployPath,
			Namespace:          *namespace,
			NamespacePath:      *namespacePath,
			LimitRangePath:     *limitrangePath,
			ResourceQuotaPath:  *resourcequotaPath,
			ServiceAccountPath: *serviceaccountPath,
			QualityPath:        *qualityPath,
		})
	})
	if err != nil {
		return err
	}
	writeErr := writeRepoPreflight(out, opts.output, result)
	if writeErr != nil {
		return writeErr
	}
	if !result.Ready {
		return fmt.Errorf("repository failed OpsPilot governance preflight")
	}
	return nil
}

func repoPrecheckCommand(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("repo precheck", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	project := fs.String("project", "", "GitLab project path, for example tpo/devex/skillshub/skillshub-api")
	write := fs.Bool("write", false, "write .opspilot/evidence/code-precheck.json")
	warnOnly := fs.Bool("warn-only", false, "do not fail when blocker findings exist")
	_ = fs.Parse(args)
	result, err := withRepo(*repo, func() (codePrecheckResult, error) {
		result, err := codePrecheck(*project)
		if err != nil {
			return codePrecheckResult{}, err
		}
		if *write {
			if err := writeCodePrecheckEvidence(result); err != nil {
				return codePrecheckResult{}, err
			}
			result.EvidencePath = codePrecheckEvidencePath()
		}
		return result, nil
	})
	if err != nil {
		return err
	}
	writeErr := writeCodePrecheck(out, opts.output, result)
	if writeErr != nil {
		return writeErr
	}
	if !*warnOnly && !result.Ready {
		return fmt.Errorf("repository failed OpsPilot code precheck")
	}
	return nil
}

func repoAutofixCommand(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("repo autofix", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	project := fs.String("project", "", "GitLab project path, for example tpo/devex/skillshub/skillshub-api")
	catalog := fs.String("namespace-catalog", "opspilot.namespaces.yaml", "namespace catalog path")
	write := fs.Bool("write", false, "write generated files")
	force := fs.Bool("force", false, "overwrite existing generated files")
	_ = fs.Parse(args)
	result, err := withRepo(*repo, func() (repoAutofixResult, error) {
		preflight, err := repoPreflight(*project, *catalog, repoLayoutOptions{})
		if err != nil {
			return repoAutofixResult{}, err
		}
		cfg := preflight.Config
		if shouldGenerateDockerfile(preflight) {
			cfg.DockerMode = "generate"
		}
		files := append([]generatedFile{{path: "opspilot.service.yaml", body: serviceConfigTemplate(cfg)}}, onboardFiles(cfg)...)
		results := make([]onboardWriteResult, 0, len(files))
		for _, file := range files {
			action := "planned"
			if *write {
				action, err = writeGeneratedFile(file.path, file.body, *force)
				if err != nil {
					return repoAutofixResult{}, err
				}
			}
			results = append(results, onboardWriteResult{Path: file.path, Action: action})
		}
		return repoAutofixResult{
			Service:         cfg.Name,
			Project:         cfg.GitLabProject,
			Mode:            writeMode(*write),
			Files:           results,
			ReleaseMapping:  releaseMapping(cfg),
			GitOpsPlan:      gitOpsPlan(cfg),
			CredentialPlans: middlewareCredentialPlans(cfg),
			Preflight:       preflight,
		}, nil
	})
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Repo autofix: %s mode=%s\n", result.Service, result.Mode)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ACTION\tPATH")
		for _, file := range result.Files {
			fmt.Fprintf(tw, "%s\t%s\n", file.Action, file.Path)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		fmt.Fprintf(w, "GitOps: path=%s app=%s image=%s\n", result.GitOpsPlan.Path, result.GitOpsPlan.ApplicationName, result.GitOpsPlan.Image)
		if len(result.CredentialPlans) > 0 {
			fmt.Fprintf(w, "Credential plans: %s\n", strings.Join(result.CredentialPlans, "; "))
		}
		return nil
	})
}
