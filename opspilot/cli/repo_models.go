package main

type repoPolicyItem struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Status  string `json:"status"`
	Level   string `json:"level"`
	Message string `json:"message,omitempty"`
	Fixable bool   `json:"fixable"`
	Action  string `json:"action,omitempty"`
}

type repoPreflightResult struct {
	Service     string               `json:"service"`
	Project     string               `json:"project"`
	Language    string               `json:"language"`
	Namespace   string               `json:"namespace"`
	Ready       bool                 `json:"ready"`
	Autofixable bool                 `json:"autofixable"`
	Items       []repoPolicyItem     `json:"items"`
	Gaps        []string             `json:"gaps"`
	Next        []string             `json:"next"`
	Config      onboardServiceConfig `json:"config"`
}

type repoAutofixResult struct {
	Service         string               `json:"service"`
	Project         string               `json:"project"`
	Mode            string               `json:"mode"`
	Files           []onboardWriteResult `json:"files"`
	ReleaseMapping  string               `json:"release_mapping"`
	GitOpsPlan      onboardGitOpsPlan    `json:"gitops_plan"`
	CredentialPlans []string             `json:"credential_plans,omitempty"`
	Preflight       repoPreflightResult  `json:"preflight"`
}

type repoUploadPlanResult struct {
	Mode       string            `json:"mode"`
	Ready      bool              `json:"ready"`
	Repo       string            `json:"repo"`
	RepoName   string            `json:"repo_name"`
	Language   string            `json:"language"`
	Target     repoUploadTarget  `json:"target"`
	Runtime    repoUploadRuntime `json:"runtime"`
	Boundaries []string          `json:"boundaries"`
	Next       []string          `json:"next"`
}

type repoUploadResult struct {
	Status   string                 `json:"status"`
	Ready    bool                   `json:"ready"`
	Plan     repoUploadPlanResult   `json:"plan"`
	Precheck repoUploadPrecheck     `json:"precheck"`
	GitLab   repoUploadGitLabResult `json:"gitlab"`
	Git      repoUploadGitResult    `json:"git"`
	Warnings []string               `json:"warnings,omitempty"`
	Next     []string               `json:"next,omitempty"`
}

type repoUploadPrecheck struct {
	Status   string              `json:"status"`
	Ready    bool                `json:"ready"`
	Summary  codePrecheckSummary `json:"summary"`
	Blockers []codePrecheckItem  `json:"blockers,omitempty"`
}

type repoUploadGitLabResult struct {
	Action        string `json:"action"`
	ProjectID     int    `json:"project_id,omitempty"`
	ProjectPath   string `json:"project_path"`
	HTTPURLToRepo string `json:"http_url_to_repo,omitempty"`
	WebURL        string `json:"web_url,omitempty"`
}

type repoUploadGitResult struct {
	Commit    string `json:"commit,omitempty"`
	Dirty     bool   `json:"dirty"`
	Ref       string `json:"ref"`
	Push      string `json:"push,omitempty"`
	RemoteURL string `json:"remote_url,omitempty"`
}

type repoUploadTarget struct {
	GitLabProject string `json:"gitlab_project"`
	Base          string `json:"base"`
	Owner         string `json:"owner"`
	Group         string `json:"group"`
	Project       string `json:"project"`
	Environment   string `json:"environment"`
}

type repoUploadRuntime struct {
	Namespace    string `json:"namespace"`
	GitOpsPath   string `json:"gitops_path"`
	ReleaseScope string `json:"release_scope"`
}

type codePrecheckSummary struct {
	Blockers int `json:"blockers"`
	Warnings int `json:"warnings"`
	Passed   int `json:"passed"`
}

type codePrecheckItem struct {
	ID             string   `json:"id"`
	Severity       string   `json:"severity"`
	Category       string   `json:"category"`
	Gate           string   `json:"gate,omitempty"`
	Decision       string   `json:"decision,omitempty"`
	Audience       string   `json:"audience,omitempty"`
	Path           string   `json:"path"`
	Line           int      `json:"line"`
	Message        string   `json:"message"`
	Snippet        string   `json:"snippet,omitempty"`
	Skill          string   `json:"skill"`
	Recommendation string   `json:"recommendation"`
	FixOptions     []string `json:"fix_options,omitempty"`
}

type codePrecheckPolicy struct {
	Owner                 string `json:"owner"`
	Audience              string `json:"audience"`
	Mode                  string `json:"mode"`
	HumanApprovalRequired bool   `json:"human_approval_required"`
	BlockerRule           string `json:"blocker_rule"`
	WarningRule           string `json:"warning_rule"`
}

type codePrecheckResult struct {
	Service      string              `json:"service"`
	Project      string              `json:"project"`
	Status       string              `json:"status"`
	Ready        bool                `json:"ready"`
	Summary      codePrecheckSummary `json:"summary"`
	Items        []codePrecheckItem  `json:"items"`
	Policy       codePrecheckPolicy  `json:"policy"`
	EvidencePath string              `json:"evidence_path,omitempty"`
	Skills       []string            `json:"skills"`
	Next         []string            `json:"next,omitempty"`
}

type repoLayoutOptions struct {
	CIPath             string
	DeployPath         string
	Namespace          string
	NamespacePath      string
	LimitRangePath     string
	ResourceQuotaPath  string
	ServiceAccountPath string
	QualityPath        string
}
