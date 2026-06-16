package repoupload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const HTTPTimeout = 30 * time.Second

type Project struct {
	ID                int    `json:"id"`
	Path              string `json:"path,omitempty"`
	PathWithNamespace string `json:"path_with_namespace"`
	HTTPURLToRepo     string `json:"http_url_to_repo"`
	WebURL            string `json:"web_url,omitempty"`
}

type Group struct {
	ID       int    `json:"id"`
	FullPath string `json:"full_path"`
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: HTTPTimeout}
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		http:    client,
	}
}

func (c *Client) Configured() bool {
	return c != nil && c.baseURL != "" && c.token != ""
}

func (c *Client) EnsureProject(ctx context.Context, projectPath string, reuseExisting bool) (Project, string, error) {
	if !c.Configured() {
		return Project{}, "", fmt.Errorf("gitlab url or token is not configured")
	}
	projectPath = strings.Trim(projectPath, "/")
	if projectPath == "" {
		return Project{}, "", fmt.Errorf("target project is required")
	}
	if project, ok, err := c.project(ctx, projectPath); err != nil {
		return Project{}, "", err
	} else if ok {
		if !reuseExisting {
			return Project{}, "", fmt.Errorf("GitLab project already exists: %s", projectPath)
		}
		return project, "reused", nil
	}
	namespacePath, projectName := path.Split(projectPath)
	namespacePath = strings.Trim(namespacePath, "/")
	projectName = strings.Trim(projectName, "/")
	if namespacePath == "" || projectName == "" {
		return Project{}, "", fmt.Errorf("target project must include namespace and project name")
	}
	group, err := c.group(ctx, namespacePath)
	if err != nil {
		return Project{}, "", err
	}
	project, err := c.createProject(ctx, group.ID, projectName)
	if err != nil {
		return Project{}, "", err
	}
	return project, "created", nil
}

func (c *Client) project(ctx context.Context, projectPath string) (Project, bool, error) {
	var project Project
	status, body, err := c.do(ctx, http.MethodGet, "/api/v4/projects/"+url.PathEscape(projectPath), nil)
	if err != nil {
		return project, false, err
	}
	if status == http.StatusNotFound {
		return project, false, nil
	}
	if status < 200 || status >= 300 {
		return project, false, fmt.Errorf("GitLab project lookup failed: %s", string(body))
	}
	if err := json.Unmarshal(body, &project); err != nil {
		return project, false, err
	}
	return project, true, nil
}

func (c *Client) group(ctx context.Context, groupPath string) (Group, error) {
	var group Group
	status, body, err := c.do(ctx, http.MethodGet, "/api/v4/groups/"+url.PathEscape(groupPath), nil)
	if err != nil {
		return group, err
	}
	if status == http.StatusNotFound {
		return group, fmt.Errorf("GitLab namespace not found: %s", groupPath)
	}
	if status < 200 || status >= 300 {
		return group, fmt.Errorf("GitLab namespace lookup failed: %s", string(body))
	}
	if err := json.Unmarshal(body, &group); err != nil {
		return group, err
	}
	return group, nil
}

func (c *Client) createProject(ctx context.Context, namespaceID int, projectName string) (Project, error) {
	var project Project
	payload := map[string]any{
		"name":                   projectName,
		"path":                   projectName,
		"namespace_id":           namespaceID,
		"visibility":             "private",
		"initialize_with_readme": false,
		"description":            "[SANDBOX] Test-only repository uploaded by OpsPilot.",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return project, err
	}
	status, response, err := c.do(ctx, http.MethodPost, "/api/v4/projects", body)
	if err != nil {
		return project, err
	}
	if status < 200 || status >= 300 {
		return project, fmt.Errorf("GitLab project create failed: %s", string(response))
	}
	if err := json.Unmarshal(response, &project); err != nil {
		return project, err
	}
	return project, nil
}

func (c *Client) do(ctx context.Context, method, endpoint string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	response, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, response, nil
}

func ParseAllowedBases(raw string) []string {
	out := []string{}
	for _, item := range strings.FieldsFunc(raw, func(ch rune) bool {
		return ch == ',' || ch == ';' || ch == '|'
	}) {
		item = strings.Trim(item, "/ ")
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func TargetAllowed(projectPath string, allowedBases []string) bool {
	projectPath = strings.Trim(projectPath, "/")
	if projectPath == "" {
		return false
	}
	for _, base := range allowedBases {
		base = strings.Trim(base, "/")
		if base == "" {
			continue
		}
		if projectPath == base || strings.HasPrefix(projectPath, base+"/") {
			return true
		}
	}
	return false
}
