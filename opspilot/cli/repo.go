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
		return fmt.Errorf("expected repo command: preflight, precheck, autofix, or upload-plan")
	}
	switch args[0] {
	case "preflight":
		return repoPreflightCommand(opts, args[1:], out)
	case "precheck":
		return repoPrecheckCommand(opts, args[1:], out)
	case "autofix":
		return repoAutofixCommand(opts, args[1:], out)
	case "upload-plan":
		return repoUploadPlanCommand(opts, args[1:], out)
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
			CredentialPlans: append(middlewareCredentialPlans(cfg), configSourceCredentialPlans(cfg)...),
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

func repoUploadPlanCommand(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("repo upload-plan", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	name := fs.String("name", "", "repository name override")
	targetBase := fs.String("target-base", "tpo/sandbox/devex", "default GitLab group for identity-less test uploads")
	targetProject := fs.String("target-project", "", "full GitLab project path override")
	namespace := fs.String("namespace", "sandbox", "test namespace for identity-less uploads")
	environment := fs.String("env", "test", "target environment")
	owner := fs.String("owner", "sandbox", "owner metadata for identity-less uploads")
	group := fs.String("group", "devex", "group metadata for identity-less uploads")
	projectName := fs.String("project-name", "sandbox", "project metadata for identity-less uploads")
	gitopsRoot := fs.String("gitops-root", "clusters/test/apps/sandbox", "GitOps app root for identity-less uploads")
	_ = fs.Parse(args)
	result, err := buildRepoUploadPlan(*repo, *name, repoUploadPlanOptions{
		TargetBase:    *targetBase,
		TargetProject: *targetProject,
		Namespace:     *namespace,
		Environment:   *environment,
		Owner:         *owner,
		Group:         *group,
		Project:       *projectName,
		GitOpsRoot:    *gitopsRoot,
	})
	if err != nil {
		return err
	}
	return writeRepoUploadPlan(out, opts.output, result)
}

type repoUploadPlanOptions struct {
	TargetBase    string
	TargetProject string
	Namespace     string
	Environment   string
	Owner         string
	Group         string
	Project       string
	GitOpsRoot    string
}

func buildRepoUploadPlan(repo, name string, opts repoUploadPlanOptions) (repoUploadPlanResult, error) {
	repoPath, err := filepath.Abs(repo)
	if err != nil {
		return repoUploadPlanResult{}, err
	}
	repoName := sanitizeDNSLabel(firstNonEmpty(name, filepath.Base(filepath.Clean(repoPath))))
	if repoName == "" {
		return repoUploadPlanResult{}, fmt.Errorf("repository name could not be inferred")
	}
	opts.TargetBase = strings.Trim(strings.TrimSpace(opts.TargetBase), "/")
	opts.TargetProject = strings.Trim(strings.TrimSpace(opts.TargetProject), "/")
	opts.Namespace = firstNonEmpty(opts.Namespace, "sandbox")
	opts.Environment = firstNonEmpty(opts.Environment, "test")
	opts.Owner = firstNonEmpty(opts.Owner, "sandbox")
	opts.Group = firstNonEmpty(opts.Group, defaultGroup)
	opts.Project = firstNonEmpty(opts.Project, "sandbox")
	opts.GitOpsRoot = strings.Trim(strings.TrimSpace(firstNonEmpty(opts.GitOpsRoot, "clusters/test/apps/sandbox")), "/")
	targetProject := opts.TargetProject
	if targetProject == "" {
		targetProject = strings.Trim(opts.TargetBase+"/"+repoName, "/")
	}
	language, err := withRepo(repoPath, func() (string, error) {
		return detectLanguage(), nil
	})
	if err != nil {
		return repoUploadPlanResult{}, err
	}
	return repoUploadPlanResult{
		Mode:     "plan",
		Ready:    true,
		Repo:     repoPath,
		RepoName: repoName,
		Language: language,
		Target: repoUploadTarget{
			GitLabProject: targetProject,
			Base:          opts.TargetBase,
			Owner:         opts.Owner,
			Group:         opts.Group,
			Project:       opts.Project,
			Environment:   opts.Environment,
		},
		Runtime: repoUploadRuntime{
			Namespace:    opts.Namespace,
			GitOpsPath:   opts.GitOpsRoot + "/" + repoName,
			ReleaseScope: "test-only",
		},
		Boundaries: []string{
			"plan-only: does not create GitLab projects, push code, or mutate Kubernetes",
			"identity-less uploads are test-only and should stay in the sandbox target",
		},
		Next: []string{
			"create or reuse GitLab project " + targetProject,
			"push the repository to the target project",
			"run repo autofix or onboard repo to generate platform files",
			"release through GitLab Runner -> BuildKit -> Registry -> GitOps -> Argo CD",
		},
	}, nil
}
