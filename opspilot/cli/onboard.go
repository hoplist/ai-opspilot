package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

func onboardCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected: onboard repo, service, check, detect, or generate")
	}
	switch args[0] {
	case "repo":
		return onboardRepoCommand(opts, args[1:], out)
	case "service":
		return onboardServiceCommand(args[1:], out)
	case "check":
		return onboardCheckCommand(args[1:], out)
	case "detect":
		return onboardDetectCommand(args[1:], out)
	case "generate":
		return onboardGenerateCommand(args[1:], out)
	default:
		return fmt.Errorf("expected: onboard repo, service, check, detect, or generate")
	}
}

func onboardRepoCommand(opts globalOptions, args []string, out io.Writer) error {
	positionalProject := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalProject = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("onboard repo", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	project := fs.String("project", "", "GitLab project path, for example tpo/devex/skillshub/skillshub-api")
	catalog := fs.String("namespace-catalog", "opspilot.namespaces.yaml", "namespace catalog path")
	envName := fs.String("env", "test", "target environment")
	write := fs.Bool("write", false, "write generated files")
	force := fs.Bool("force", false, "overwrite existing generated files")
	_ = fs.Parse(args)
	if *project == "" {
		*project = positionalProject
	}
	if *project == "" && fs.NArg() > 0 {
		*project = fs.Arg(0)
	}
	if *project == "" {
		return fmt.Errorf("onboard repo requires a GitLab project path")
	}
	result, err := withRepo(*repo, func() (onboardRepoResult, error) {
		detected, err := detectOnboardRepository(*project, *catalog)
		if err != nil {
			return onboardRepoResult{}, err
		}
		cfg := detected.Config
		if err := cfg.defaults(); err != nil {
			return onboardRepoResult{}, err
		}
		files := append([]generatedFile{{path: "opspilot.service.yaml", body: serviceConfigTemplate(cfg)}}, onboardFiles(cfg)...)
		results := make([]onboardWriteResult, 0, len(files))
		for _, file := range files {
			action := "planned"
			if *write {
				action, err = writeGeneratedFile(file.path, file.body, *force)
				if err != nil {
					return onboardRepoResult{}, err
				}
			}
			results = append(results, onboardWriteResult{Path: file.path, Action: action})
		}
		ready := detected.Ready
		gaps := append([]string{}, detected.Gaps...)
		next := append([]string{}, detected.Next...)
		var preflight *onboardCheckResult
		if *write {
			check := checkOnboardRepository(cfg)
			preflight = &check
			ready = check.Ready
			gaps = append([]string{}, check.Missing...)
			next = append([]string{}, check.Next...)
		} else {
			next = append([]string{"rerun with --write to generate platform files"}, next...)
		}
		if ready {
			next = append(next,
				"push the repository to GitLab",
				"wait for GitLab Runner -> BuildKit -> GitOps -> Argo CD",
				"run opspilot inspect service "+cfg.Name+" --output human",
			)
		}
		return onboardRepoResult{
			Service:         cfg.Name,
			Environment:     *envName,
			Repo:            *repo,
			Project:         cfg.GitLabProject,
			Mode:            writeMode(*write),
			Ready:           ready,
			Language:        cfg.Language,
			Namespace:       cfg.Namespace,
			Port:            cfg.Port,
			Config:          cfg,
			Files:           results,
			Preflight:       preflight,
			Gaps:            uniqueStrings(gaps),
			Next:            uniqueStrings(next),
			ReleaseMapping:  releaseMapping(cfg),
			GitOpsPlan:      gitOpsPlan(cfg),
			CredentialPlans: append(middlewareCredentialPlans(cfg), configSourceCredentialPlans(cfg)...),
		}, nil
	})
	if err != nil {
		return err
	}
	writeErr := writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Onboard: %s env=%s ready=%t mode=%s\n", result.Service, result.Environment, result.Ready, result.Mode)
		fmt.Fprintf(w, "Repo: %s\n", result.Repo)
		fmt.Fprintf(w, "Project: %s\n", result.Project)
		fmt.Fprintf(w, "Detected: language=%s port=%d namespace=%s\n", result.Language, result.Port, result.Namespace)
		if len(result.Gaps) > 0 {
			fmt.Fprintf(w, "Gaps: %s\n", strings.Join(result.Gaps, ", "))
		}
		if len(result.Files) > 0 {
			fmt.Fprintln(w, "Files:")
			for _, file := range result.Files {
				fmt.Fprintf(w, "  %s\t%s\n", file.Action, file.Path)
			}
		}
		if result.ReleaseMapping != "" {
			fmt.Fprintf(w, "Release mapping: %s\n", result.ReleaseMapping)
		}
		fmt.Fprintf(w, "GitOps: path=%s app=%s image=%s\n", result.GitOpsPlan.Path, result.GitOpsPlan.ApplicationName, result.GitOpsPlan.Image)
		if len(result.CredentialPlans) > 0 {
			fmt.Fprintf(w, "Credential plans: %s\n", strings.Join(result.CredentialPlans, "; "))
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(result.Next, "; "))
		}
		return nil
	})
	if writeErr != nil {
		return writeErr
	}
	if result.Mode == "write" && !result.Ready {
		return fmt.Errorf("onboard repo generated files but repository is not release-ready")
	}
	return nil
}
