package release

import (
	"bytes"
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

func (c *gitLabClient) fileCommits(ctx context.Context, project, filePath, ref string, limit int) ([]map[string]any, error) {
	if !c.configured() {
		return nil, errors.New("gitlab url or token is not configured")
	}
	if project == "" {
		return nil, errors.New("gitops project is not configured")
	}
	if filePath == "" {
		return nil, errors.New("gitops file path is not configured")
	}
	if ref == "" {
		ref = "main"
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	query := url.Values{}
	query.Set("path", filePath)
	query.Set("ref_name", ref)
	query.Set("per_page", fmt.Sprint(limit))
	endpoint := c.apiPath("projects", project, "repository/commits") + "?" + query.Encode()
	var commits []map[string]any
	if err := c.getJSON(ctx, endpoint, &commits); err != nil {
		return nil, err
	}
	return commits, nil
}

func (c *gitLabClient) commitFileUpdate(ctx context.Context, project, branch, filePath, content, commitMessage string) (map[string]any, error) {
	if !c.configured() {
		return nil, errors.New("gitlab url or token is not configured")
	}
	if project == "" {
		return nil, errors.New("gitops project is not configured")
	}
	if branch == "" {
		branch = "main"
	}
	if filePath == "" {
		return nil, errors.New("gitops file path is not configured")
	}
	if commitMessage == "" {
		return nil, errors.New("commit message is not configured")
	}
	payload := map[string]any{
		"branch":         branch,
		"commit_message": commitMessage,
		"actions": []map[string]string{
			{
				"action":    "update",
				"file_path": filePath,
				"content":   content,
			},
		},
	}
	body, err := c.doJSON(ctx, http.MethodPost, c.apiPath("projects", project, "repository/commits"), payload)
	if err != nil {
		return nil, err
	}
	var commit map[string]any
	if err := json.Unmarshal(body, &commit); err != nil {
		return nil, err
	}
	return commit, nil
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

func (c *gitLabClient) doJSON(ctx context.Context, method, endpoint string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitlab api %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return responseBody, nil
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
	if container == "" {
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "image:") {
				return imageFromLine(line)
			}
		}
		return ""
	}
	start := -1
	startIndent := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		afterDash := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		if !strings.HasPrefix(afterDash, "name:") {
			continue
		}
		name := strings.Trim(strings.TrimSpace(strings.TrimPrefix(afterDash, "name:")), `"'`)
		if name == container {
			start = i
			startIndent = leadingSpaceCount(line)
			break
		}
	}
	if start < 0 {
		return ""
	}
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if leadingSpaceCount(line) <= startIndent && strings.HasPrefix(trimmed, "- ") {
			break
		}
		if leadingSpaceCount(line) < startIndent {
			break
		}
		if strings.HasPrefix(trimmed, "image:") {
			return imageFromLine(line)
		}
	}
	return ""
}

func replaceImageInManifest(text, container, targetImage string) (string, string, error) {
	if targetImage == "" {
		return "", "", errors.New("target image is empty")
	}
	lines := strings.Split(text, "\n")
	if container == "" {
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "image:") {
				oldImage := imageFromLine(line)
				lines[i] = leadingWhitespace(line) + "image: " + targetImage
				return strings.Join(lines, "\n"), oldImage, nil
			}
		}
		return "", "", errors.New("image field not found in manifest")
	}
	start := -1
	startIndent := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		afterDash := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		if !strings.HasPrefix(afterDash, "name:") {
			continue
		}
		name := strings.Trim(strings.TrimSpace(strings.TrimPrefix(afterDash, "name:")), `"'`)
		if name != container {
			continue
		}
		start = i
		startIndent = leadingSpaceCount(line)
		break
	}
	if start < 0 {
		return "", "", fmt.Errorf("container %q not found in manifest", container)
	}
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if leadingSpaceCount(line) <= startIndent && strings.HasPrefix(trimmed, "- ") {
			break
		}
		if leadingSpaceCount(line) < startIndent {
			break
		}
		if strings.HasPrefix(trimmed, "image:") {
			oldImage := imageFromLine(line)
			lines[i] = leadingWhitespace(line) + "image: " + targetImage
			return strings.Join(lines, "\n"), oldImage, nil
		}
	}
	return "", "", fmt.Errorf("image field for container %q not found in manifest", container)
}

func imageWithTag(currentImage, tag string) (string, error) {
	if currentImage == "" {
		return "", errors.New("current image is empty")
	}
	if tag == "" {
		return "", errors.New("target tag is empty")
	}
	if strings.Contains(currentImage, "@") {
		base, _, _ := strings.Cut(currentImage, "@")
		return base + ":" + tag, nil
	}
	lastSlash := strings.LastIndex(currentImage, "/")
	lastColon := strings.LastIndex(currentImage, ":")
	if lastColon <= lastSlash {
		return currentImage + ":" + tag, nil
	}
	return currentImage[:lastColon+1] + tag, nil
}

func looksLikeImage(value string) bool {
	return strings.Contains(value, "@") || (strings.Contains(value, "/") && strings.LastIndex(value, ":") > strings.LastIndex(value, "/"))
}

func imageFromLine(line string) string {
	trimmed := strings.TrimSpace(line)
	return strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "image:")), `"'`)
}

func leadingWhitespace(line string) string {
	return line[:leadingSpaceCount(line)]
}

func leadingSpaceCount(line string) int {
	for i, r := range line {
		if r != ' ' && r != '\t' {
			return i
		}
	}
	return len(line)
}

func shortRevision(revision string) string {
	if len(revision) <= 8 {
		return revision
	}
	return revision[:8]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}
