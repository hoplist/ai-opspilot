package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	defaultSandboxGitLabBase = "tpo/sandbox/devex"
	defaultSandboxNamespace  = "sandbox"
	defaultSandboxGitOpsRoot = "clusters/test/apps/sandbox"
	defaultGitLabURL         = "http://192.168.48.206:8929"
)

func repoUploadPlanCommand(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("repo upload-plan", flag.ExitOnError)
	repo, name, planOpts := addRepoUploadPlanFlags(fs)
	_ = fs.Parse(args)
	result, err := buildRepoUploadPlan(*repo, *name, planOpts.values())
	if err != nil {
		return err
	}
	return writeRepoUploadPlan(out, opts.output, result)
}

func repoUploadCommand(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("repo upload", flag.ExitOnError)
	repo, name, planOpts := addRepoUploadPlanFlags(fs)
	_ = fs.String("gitlab-url", env("OPSPILOT_GITLAB_URL", defaultGitLabURL), "deprecated: GitLab project creation is handled by opspilot-core")
	pushToken := fs.String("push-token", firstNonEmpty(env("OPSPILOT_GITLAB_PUSH_TOKEN", ""), env("OPSPILOT_GITLAB_TOKEN", ""), env("GITLAB_TOKEN", "")), "GitLab token used only for local git push")
	fs.StringVar(pushToken, "token", *pushToken, "alias for --push-token")
	ref := fs.String("ref", "main", "remote branch to push")
	confirm := fs.Bool("confirm", false, "confirm GitLab project creation/reuse and git push")
	reuseExisting := fs.Bool("reuse-existing", true, "reuse existing GitLab project at the target path")
	_ = fs.Parse(args)

	plan, err := buildRepoUploadPlan(*repo, *name, planOpts.values())
	if err != nil {
		return err
	}
	result := repoUploadResult{
		Status: "planned",
		Ready:  false,
		Plan:   plan,
		Git:    repoUploadGitResult{Ref: firstNonEmpty(*ref, "main")},
		Next:   append([]string{"rerun with --confirm to create/reuse the GitLab project and push current HEAD"}, plan.Next...),
	}
	if !*confirm {
		if writeErr := writeRepoUpload(out, opts.output, result); writeErr != nil {
			return writeErr
		}
		return fmt.Errorf("repo upload requires --confirm")
	}
	precheck, err := runRepoUploadPrecheck(plan)
	result.Precheck = precheck
	if err != nil {
		result.Status = "blocked"
		result.Next = []string{"fix code-precheck blocker findings before uploading"}
		_ = writeRepoUpload(out, opts.output, result)
		return err
	}

	git, err := repoUploadGitState(plan.Repo, result.Git.Ref)
	result.Git = git
	if err != nil {
		result.Status = "blocked"
		result.Next = []string{"commit local changes first; repo upload pushes the current committed HEAD only"}
		_ = writeRepoUpload(out, opts.output, result)
		return err
	}

	gitlab, warnings, err := repoUploadEnsureTarget(opts.backendURL, plan.Target.GitLabProject, *reuseExisting)
	result.GitLab = gitlab
	result.Warnings = append(result.Warnings, warnings...)
	if err != nil {
		result.Status = "blocked"
		result.Next = []string{"check opspilot-core GitLab configuration and allowed repo upload bases"}
		_ = writeRepoUpload(out, opts.output, result)
		return err
	}
	if result.GitLab.HTTPURLToRepo == "" {
		result.Status = "blocked"
		_ = writeRepoUpload(out, opts.output, result)
		return fmt.Errorf("GitLab project did not return http_url_to_repo")
	}
	if strings.TrimSpace(*pushToken) == "" {
		result.Status = "blocked"
		result.Next = []string{"set OPSPILOT_GITLAB_PUSH_TOKEN or pass --push-token for local git push; core already owns project create/reuse"}
		_ = writeRepoUpload(out, opts.output, result)
		return fmt.Errorf("GitLab push token is required")
	}

	pushedURL, err := gitPushWithToken(plan.Repo, result.GitLab.HTTPURLToRepo, *pushToken, result.Git.Ref)
	result.Git.RemoteURL = stripURLCredentials(pushedURL)
	if err != nil {
		result.Status = "blocked"
		result.Next = []string{"check GitLab token write_repository permission and repository branch protection"}
		_ = writeRepoUpload(out, opts.output, result)
		return err
	}
	result.Status = "uploaded"
	result.Ready = true
	result.Git.Push = "success"
	result.Next = []string{
		"watch the GitLab pipeline for " + result.GitLab.ProjectPath,
		"run repo autofix/onboard if the uploaded repository still lacks platform files",
		"release through GitLab Runner -> BuildKit -> Registry -> GitOps -> Argo CD",
	}
	return writeRepoUpload(out, opts.output, result)
}

func repoUploadEnsureTarget(backendURL, targetProject string, reuseExisting bool) (repoUploadGitLabResult, []string, error) {
	values := url.Values{"target_project": {targetProject}}
	if !reuseExisting {
		values.Set("no_reuse", "true")
	}
	body, err := post(backendURL, "/api/repo/upload-target", values)
	if err != nil {
		return repoUploadGitLabResult{}, nil, err
	}
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return repoUploadGitLabResult{}, nil, err
	}
	if !env.OK {
		return repoUploadGitLabResult{}, env.Warnings, fmt.Errorf("repo upload target API returned ok=false")
	}
	var data repoUploadGitLabResult
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return repoUploadGitLabResult{}, env.Warnings, err
	}
	return data, env.Warnings, nil
}

type repoUploadFlagValues struct {
	targetBase    *string
	targetProject *string
	namespace     *string
	environment   *string
	owner         *string
	group         *string
	project       *string
	gitopsRoot    *string
}

func addRepoUploadPlanFlags(fs *flag.FlagSet) (*string, *string, repoUploadFlagValues) {
	repo := fs.String("repo", ".", "repository path")
	name := fs.String("name", "", "repository name override")
	return repo, name, repoUploadFlagValues{
		targetBase:    fs.String("target-base", defaultSandboxGitLabBase, "default GitLab group for identity-less test uploads"),
		targetProject: fs.String("target-project", "", "full GitLab project path override"),
		namespace:     fs.String("namespace", defaultSandboxNamespace, "test namespace for identity-less uploads"),
		environment:   fs.String("env", "test", "target environment"),
		owner:         fs.String("owner", "sandbox", "owner metadata for identity-less uploads"),
		group:         fs.String("group", defaultGroup, "group metadata for identity-less uploads"),
		project:       fs.String("project-name", "sandbox", "project metadata for identity-less uploads"),
		gitopsRoot:    fs.String("gitops-root", defaultSandboxGitOpsRoot, "GitOps app root for identity-less uploads"),
	}
}

func (v repoUploadFlagValues) values() repoUploadPlanOptions {
	return repoUploadPlanOptions{
		TargetBase:    *v.targetBase,
		TargetProject: *v.targetProject,
		Namespace:     *v.namespace,
		Environment:   *v.environment,
		Owner:         *v.owner,
		Group:         *v.group,
		Project:       *v.project,
		GitOpsRoot:    *v.gitopsRoot,
	}
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
	opts.Namespace = firstNonEmpty(opts.Namespace, defaultSandboxNamespace)
	opts.Environment = firstNonEmpty(opts.Environment, "test")
	opts.Owner = firstNonEmpty(opts.Owner, "sandbox")
	opts.Group = firstNonEmpty(opts.Group, defaultGroup)
	opts.Project = firstNonEmpty(opts.Project, "sandbox")
	opts.GitOpsRoot = strings.Trim(strings.TrimSpace(firstNonEmpty(opts.GitOpsRoot, defaultSandboxGitOpsRoot)), "/")
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

func runRepoUploadPrecheck(plan repoUploadPlanResult) (repoUploadPrecheck, error) {
	result, err := withRepo(plan.Repo, func() (codePrecheckResult, error) {
		return codePrecheck(plan.Target.GitLabProject)
	})
	if err != nil {
		return repoUploadPrecheck{}, err
	}
	out := repoUploadPrecheck{
		Status:  result.Status,
		Ready:   result.Ready,
		Summary: result.Summary,
	}
	for _, item := range result.Items {
		if item.Severity == "blocker" {
			out.Blockers = append(out.Blockers, item)
		}
	}
	if !result.Ready {
		return out, fmt.Errorf("repository failed OpsPilot code precheck")
	}
	return out, nil
}

func repoUploadGitState(repo, ref string) (repoUploadGitResult, error) {
	ref = firstNonEmpty(ref, "main")
	commit, err := gitOutput(repo, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return repoUploadGitResult{Ref: ref}, fmt.Errorf("repository has no committed HEAD")
	}
	status, err := gitOutput(repo, "status", "--porcelain")
	if err != nil {
		return repoUploadGitResult{Ref: ref}, err
	}
	dirty := strings.TrimSpace(status) != ""
	out := repoUploadGitResult{Commit: strings.TrimSpace(commit), Dirty: dirty, Ref: ref}
	if dirty {
		return out, fmt.Errorf("repository has uncommitted changes")
	}
	return out, nil
}

func gitOutput(repo string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

func gitPushWithToken(repo, httpURLToRepo, token, ref string) (string, error) {
	pushURL, err := gitPushURL(httpURLToRepo)
	if err != nil {
		return "", err
	}
	script, err := writeGitAskpass()
	if err != nil {
		return "", err
	}
	defer os.Remove(script)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "push", pushURL, "HEAD:"+firstNonEmpty(ref, "main"))
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS="+script,
		"OPSPILOT_GIT_ASKPASS_TOKEN="+token,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := sanitizeSecret(strings.TrimSpace(stderr.String()), token)
		if msg == "" {
			msg = err.Error()
		}
		return pushURL, fmt.Errorf("git push failed: %s", msg)
	}
	return pushURL, nil
}

func gitPushURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	parsed.User = url.User("oauth2")
	return parsed.String(), nil
}

func writeGitAskpass() (string, error) {
	ext := ".sh"
	body := "#!/bin/sh\nprintf '%s\\n' \"$OPSPILOT_GIT_ASKPASS_TOKEN\"\n"
	if runtime.GOOS == "windows" {
		ext = ".bat"
		body = "@echo off\r\necho %OPSPILOT_GIT_ASKPASS_TOKEN%\r\n"
	}
	file, err := os.CreateTemp("", "opspilot-git-askpass-*"+ext)
	if err != nil {
		return "", err
	}
	if _, err := file.WriteString(body); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(file.Name(), 0o700); err != nil {
			_ = os.Remove(file.Name())
			return "", err
		}
	}
	return file.Name(), nil
}

func stripURLCredentials(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.User = nil
	return parsed.String()
}

func sanitizeSecret(text, secret string) string {
	if secret == "" {
		return text
	}
	return strings.ReplaceAll(text, secret, "<redacted>")
}
