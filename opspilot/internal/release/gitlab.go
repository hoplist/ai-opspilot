package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

type Datasources struct {
	GitLabURL     string
	GitLabToken   string
	GitOpsProject string
	GitOpsRef     string
}

type gitLabClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newGitLabClient(baseURL, token string) *gitLabClient {
	return &gitLabClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{},
	}
}

func (c *gitLabClient) configured() bool {
	return c.baseURL != "" && c.token != ""
}

func (c *gitLabClient) latestPipeline(ctx context.Context, project string) (map[string]any, error) {
	if !c.configured() {
		return nil, errors.New("gitlab url or token is not configured")
	}
	if project == "" {
		return nil, errors.New("gitlab project is not configured")
	}
	endpoint := c.apiPath("projects", project, "pipelines") + "?per_page=1"
	var pipelines []map[string]any
	if err := c.getJSON(ctx, endpoint, &pipelines); err != nil {
		return nil, err
	}
	if len(pipelines) == 0 {
		return map[string]any{"status": "missing", "project": project}, nil
	}
	pipeline := pipelines[0]
	return map[string]any{
		"status":      pipeline["status"],
		"project":     project,
		"id":          pipeline["id"],
		"ref":         pipeline["ref"],
		"sha":         pipeline["sha"],
		"web_url":     pipeline["web_url"],
		"created_at":  pipeline["created_at"],
		"updated_at":  pipeline["updated_at"],
		"finished_at": pipeline["finished_at"],
	}, nil
}

func (c *gitLabClient) latestPipelineJobs(ctx context.Context, project string) (map[string]any, error) {
	pipeline, err := c.latestPipeline(ctx, project)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprint(pipeline["id"])
	if id == "" || id == "<nil>" {
		return map[string]any{"project": project, "pipeline": pipeline, "items": []any{}, "item_count": 0}, nil
	}
	endpoint := c.apiPath("projects", project, "pipelines", id, "jobs") + "?per_page=100"
	var rawJobs []map[string]any
	if err := c.getJSON(ctx, endpoint, &rawJobs); err != nil {
		return nil, err
	}
	jobs := make([]map[string]any, 0, len(rawJobs))
	for _, job := range rawJobs {
		jobs = append(jobs, map[string]any{
			"id":             job["id"],
			"name":           job["name"],
			"stage":          job["stage"],
			"status":         job["status"],
			"ref":            job["ref"],
			"tag":            job["tag"],
			"web_url":        job["web_url"],
			"created_at":     job["created_at"],
			"started_at":     job["started_at"],
			"finished_at":    job["finished_at"],
			"duration":       job["duration"],
			"failure_reason": job["failure_reason"],
		})
	}
	return map[string]any{
		"project":    project,
		"pipeline":   pipeline,
		"items":      jobs,
		"item_count": len(jobs),
	}, nil
}

func (c *gitLabClient) jobTrace(ctx context.Context, project string, jobID int64) (string, error) {
	if !c.configured() {
		return "", errors.New("gitlab url or token is not configured")
	}
	if project == "" {
		return "", errors.New("gitlab project is not configured")
	}
	if jobID == 0 {
		return "", errors.New("gitlab job id is not configured")
	}
	endpoint := c.apiPath("projects", project, "jobs", fmt.Sprint(jobID), "trace")
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *gitLabClient) registryTag(ctx context.Context, project, image string) (map[string]any, error) {
	if !c.configured() {
		return nil, errors.New("gitlab url or token is not configured")
	}
	if project == "" {
		return nil, errors.New("gitlab project is not configured")
	}
	repoName, tag := splitImageNameTag(image)
	if tag == "" {
		return map[string]any{"status": "unknown", "image": image, "reason": "image tag is empty"}, nil
	}
	endpoint := c.apiPath("projects", project, "registry/repositories") + "?tags=true&per_page=100"
	var repos []map[string]any
	if err := c.getJSON(ctx, endpoint, &repos); err != nil {
		return nil, err
	}
	for _, repo := range repos {
		repoPath := fmt.Sprint(repo["path"])
		location := fmt.Sprint(repo["location"])
		if !strings.HasSuffix(repoPath, repoName) && !strings.Contains(location, repoName) {
			continue
		}
		id := fmt.Sprint(repo["id"])
		tagEndpoint := c.apiPath("projects", project, "registry/repositories", id, "tags", tag)
		var tagInfo map[string]any
		if err := c.getJSON(ctx, tagEndpoint, &tagInfo); err != nil {
			return map[string]any{"status": "missing", "image": image, "tag": tag, "repository_id": id}, nil
		}
		return map[string]any{
			"status":        "exists",
			"image":         image,
			"tag":           tag,
			"repository_id": id,
			"digest":        tagInfo["digest"],
			"created_at":    tagInfo["created_at"],
			"revision":      tagInfo["revision"],
		}, nil
	}
	return map[string]any{"status": "missing", "image": image, "tag": tag}, nil
}

func (c *gitLabClient) rawFile(ctx context.Context, project, filePath, ref string) (string, error) {
	if !c.configured() {
		return "", errors.New("gitlab url or token is not configured")
	}
	if project == "" {
		return "", errors.New("gitops project is not configured")
	}
	if filePath == "" {
		return "", errors.New("gitops file path is not configured")
	}
	if ref == "" {
		ref = "main"
	}
	endpoint := c.apiPath("projects", project, "repository/files", filePath, "raw") + "?ref=" + url.QueryEscape(ref)
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *gitLabClient) apiPath(parts ...string) string {
	escaped := []string{"/api/v4"}
	for i, part := range parts {
		if i == 1 && parts[0] == "projects" {
			escaped = append(escaped, url.PathEscape(part))
			continue
		}
		if i == 2 && parts[0] == "projects" && parts[2] == "repository/files" {
			escaped = append(escaped, part)
			continue
		}
		if i == 3 && parts[0] == "projects" && parts[2] == "repository/files" {
			escaped = append(escaped, url.PathEscape(part))
			continue
		}
		escaped = append(escaped, path.Clean(part))
	}
	return strings.Join(escaped, "/")
}

func (c *gitLabClient) getJSON(ctx context.Context, endpoint string, target any) error {
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func (c *gitLabClient) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitlab api %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func splitImageNameTag(image string) (string, string) {
	lastSlash := strings.LastIndex(image, "/")
	name := image
	if lastSlash >= 0 {
		name = image[lastSlash+1:]
	}
	colon := strings.LastIndex(name, ":")
	if colon < 0 {
		return name, ""
	}
	return name[:colon], name[colon+1:]
}

func desiredImageFromManifest(text, container string) string {
	lines := strings.Split(text, "\n")
	inTargetContainer := container == ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		nameLine := trimmed
		if strings.HasPrefix(nameLine, "- ") {
			nameLine = strings.TrimSpace(strings.TrimPrefix(nameLine, "- "))
		}
		if strings.HasPrefix(nameLine, "name:") {
			name := strings.Trim(strings.TrimSpace(strings.TrimPrefix(nameLine, "name:")), `"'`)
			inTargetContainer = container == "" || name == container
			continue
		}
		if inTargetContainer && strings.HasPrefix(trimmed, "image:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "image:")), `"'`)
		}
	}
	return ""
}
