package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/repoupload"
)

const defaultRepoUploadAllowedBase = "tpo/sandbox/devex"

func registerRepoRoutes(mux *http.ServeMux, state *runtimeState) {
	handleAPI(mux, "/api/repo/upload-target", wrapPost(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if err := r.ParseForm(); err != nil {
			return nil, nil, err
		}
		targetProject := strings.Trim(r.Form.Get("target_project"), "/ ")
		if targetProject == "" {
			targetProject = strings.Trim(r.Form.Get("gitlab_project"), "/ ")
		}
		if targetProject == "" {
			return nil, nil, requestError{message: "target_project is required"}
		}
		allowedBases := repoupload.ParseAllowedBases(env("OPSPILOT_REPO_UPLOAD_ALLOWED_BASES", defaultRepoUploadAllowedBase))
		if !repoupload.TargetAllowed(targetProject, allowedBases) {
			return nil, nil, requestError{message: "target_project is outside allowed upload bases"}
		}
		snap := state.snapshot()
		gitlabURL := configValue(snap.config.Settings.GitLabURL, env("OPSPILOT_GITLAB_URL", ""))
		client := repoupload.NewClient(gitlabURL, env("OPSPILOT_GITLAB_TOKEN", ""), nil)
		project, action, err := client.EnsureProject(ctx, targetProject, !boolForm(r, "no_reuse"))
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{
			"status":            "ready",
			"action":            action,
			"project_id":        project.ID,
			"project_path":      firstNonEmpty(project.PathWithNamespace, targetProject),
			"http_url_to_repo":  project.HTTPURLToRepo,
			"web_url":           project.WebURL,
			"allowed_bases":     allowedBases,
			"server_owned":      true,
			"local_push_needed": true,
		}, nil, nil
	}))
}
