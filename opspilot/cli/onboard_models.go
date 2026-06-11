package main

type onboardServiceConfig struct {
	Name           string                      `json:"name"`
	GitLabProject  string                      `json:"gitlab_project"`
	Organization   string                      `json:"organization,omitempty"`
	Group          string                      `json:"group,omitempty"`
	Project        string                      `json:"project,omitempty"`
	Language       string                      `json:"language"`
	BuildEntry     string                      `json:"build_entry"`
	BuildOutput    string                      `json:"build_output"`
	Port           int                         `json:"port"`
	HealthPath     string                      `json:"health_path"`
	Namespace      string                      `json:"namespace"`
	NamespaceSrc   string                      `json:"namespace_source,omitempty"`
	Replicas       int                         `json:"replicas"`
	Container      string                      `json:"container"`
	DockerMode     string                      `json:"dockerfile_mode"`
	DockerPath     string                      `json:"dockerfile_path"`
	CIMode         string                      `json:"ci_mode"`
	PromSource     string                      `json:"prometheus_source"`
	Resources      onboardResourcesConfig      `json:"resources"`
	NamespaceGuard onboardNamespaceGuardConfig `json:"namespace_guard"`
	Middleware     []onboardMiddlewareConfig   `json:"middleware,omitempty"`
	Storage        []onboardStorageConfig      `json:"storage,omitempty"`
	ConfigSources  []onboardConfigSourceConfig `json:"config_sources,omitempty"`
}

type onboardResourcesConfig struct {
	Profile       string `json:"profile"`
	RequestCPU    string `json:"request_cpu"`
	RequestMemory string `json:"request_memory"`
	LimitCPU      string `json:"limit_cpu"`
	LimitMemory   string `json:"limit_memory"`
}

type onboardNamespaceGuardConfig struct {
	LimitRange     bool   `json:"limit_range"`
	ResourceQuota  bool   `json:"resource_quota"`
	RequestsCPU    string `json:"requests_cpu"`
	RequestsMemory string `json:"requests_memory"`
	LimitsCPU      string `json:"limits_cpu"`
	LimitsMemory   string `json:"limits_memory"`
	Pods           string `json:"pods"`
}

type onboardMiddlewareConfig struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Display    string   `json:"display"`
	Mode       string   `json:"mode"`
	Allocation string   `json:"allocation"`
	Provision  string   `json:"provision,omitempty"`
	Resource   string   `json:"resource"`
	Secret     string   `json:"secret"`
	Env        []string `json:"env"`
	Reason     string   `json:"reason,omitempty"`
	Evidence   []string `json:"evidence,omitempty"`
}

type onboardStorageConfig struct {
	Name          string   `json:"name"`
	Purpose       string   `json:"purpose"`
	Mode          string   `json:"mode"`
	MountPath     string   `json:"mount_path"`
	HostPath      string   `json:"host_path,omitempty"`
	SizeHint      string   `json:"size_hint,omitempty"`
	SizeLimit     string   `json:"size_limit,omitempty"`
	RetentionDays int      `json:"retention_days,omitempty"`
	ReadOnly      bool     `json:"read_only,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
}

type onboardConfigSourceConfig struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required,omitempty"`
	AppID       string   `json:"app_id,omitempty"`
	Env         string   `json:"env,omitempty"`
	Cluster     string   `json:"cluster,omitempty"`
	Namespaces  []string `json:"namespaces,omitempty"`
	Meta        string   `json:"meta,omitempty"`
	ConfigMap   string   `json:"config_map,omitempty"`
	TokenSecret string   `json:"token_secret,omitempty"`
	InjectMode  string   `json:"inject_mode,omitempty"`
	EnvFlag     string   `json:"env_flag,omitempty"`
	MetaFlag    string   `json:"meta_flag,omitempty"`
	MountPath   string   `json:"mount_path,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
}

type middlewareCatalogEntry struct {
	Kind       string
	Display    string
	Mode       string
	Allocation string
	Env        []string
	Tokens     []string
}

type onboardWriteResult struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

type onboardResult struct {
	Service        string               `json:"service"`
	Mode           string               `json:"mode"`
	Files          []onboardWriteResult `json:"files"`
	ReleaseMapping string               `json:"release_mapping"`
	GitOpsPlan     onboardGitOpsPlan    `json:"gitops_plan"`
}

type onboardRepoResult struct {
	Service         string               `json:"service"`
	Environment     string               `json:"environment"`
	Repo            string               `json:"repo"`
	Project         string               `json:"project"`
	Mode            string               `json:"mode"`
	Ready           bool                 `json:"ready"`
	Language        string               `json:"language"`
	Namespace       string               `json:"namespace"`
	Port            int                  `json:"port"`
	Config          onboardServiceConfig `json:"config"`
	Files           []onboardWriteResult `json:"files"`
	Preflight       *onboardCheckResult  `json:"preflight,omitempty"`
	Gaps            []string             `json:"gaps,omitempty"`
	Next            []string             `json:"next,omitempty"`
	ReleaseMapping  string               `json:"release_mapping"`
	GitOpsPlan      onboardGitOpsPlan    `json:"gitops_plan"`
	CredentialPlans []string             `json:"credential_plans,omitempty"`
}

type onboardGitOpsPlan struct {
	Cluster         string   `json:"cluster"`
	Path            string   `json:"path"`
	DeploymentPath  string   `json:"deployment_path"`
	ApplicationName string   `json:"application_name"`
	Namespace       string   `json:"namespace"`
	Image           string   `json:"image"`
	StandardFlow    []string `json:"standard_flow"`
}

type onboardDetectResult struct {
	Service string               `json:"service"`
	Ready   bool                 `json:"ready"`
	Config  onboardServiceConfig `json:"config"`
	Files   map[string]bool      `json:"files"`
	Gaps    []string             `json:"gaps"`
	Next    []string             `json:"next"`
}

type namespaceResolution struct {
	Namespace    string
	Source       string
	Organization string
	Group        string
	Project      string
	Service      string
}

const (
	defaultOrganization      = "tpo"
	defaultGroup             = "devex"
	defaultNamespacePrefix   = "cicd"
	defaultResourceProfile   = "small"
	defaultCITemplateProject = "platform/opspilot"
	defaultHostPathRoot      = "/data/opspilot/hostpath"
)

var resourceProfiles = map[string]onboardResourcesConfig{
	"small": {
		Profile:       "small",
		RequestCPU:    "50m",
		RequestMemory: "64Mi",
		LimitCPU:      "500m",
		LimitMemory:   "256Mi",
	},
	"medium": {
		Profile:       "medium",
		RequestCPU:    "100m",
		RequestMemory: "128Mi",
		LimitCPU:      "1",
		LimitMemory:   "512Mi",
	},
	"large": {
		Profile:       "large",
		RequestCPU:    "500m",
		RequestMemory: "512Mi",
		LimitCPU:      "2",
		LimitMemory:   "2Gi",
	},
}

var defaultNamespaceGuard = onboardNamespaceGuardConfig{
	LimitRange:     true,
	ResourceQuota:  true,
	RequestsCPU:    "4",
	RequestsMemory: "8Gi",
	LimitsCPU:      "8",
	LimitsMemory:   "16Gi",
	Pods:           "50",
}

var projectSuffixes = map[string]bool{
	"admin":   true,
	"api":     true,
	"core":    true,
	"job":     true,
	"service": true,
	"web":     true,
	"worker":  true,
}

var middlewareCatalog = []middlewareCatalogEntry{
	{
		Kind:       "mysql",
		Display:    "MySQL database",
		Mode:       "shared-database",
		Allocation: "database-user",
		Env:        []string{"DATABASE_URL"},
		Tokens: []string{
			"go-sql-driver/mysql", "mysql2", "mysqlclient", "pymysql", "mysql-connector",
			"jdbc:mysql", "mysql_", "mysql://",
		},
	},
	{
		Kind:       "postgres",
		Display:    "PostgreSQL database",
		Mode:       "shared-database",
		Allocation: "database-user-schema",
		Env:        []string{"DATABASE_URL"},
		Tokens: []string{
			"lib/pq", "pgx", "psycopg", "asyncpg", "node-postgres", "postgresql",
			"jdbc:postgresql", "postgres://", "postgres_", "pghost",
		},
	},
	{
		Kind:       "redis",
		Display:    "Redis cache",
		Mode:       "shared-cache",
		Allocation: "key-prefix",
		Env:        []string{"REDIS_URL"},
		Tokens: []string{
			"go-redis", "ioredis", "redis-py", "redis_url", "redis.host",
			"redis_host", "redis://",
		},
	},
	{
		Kind:       "rabbitmq",
		Display:    "RabbitMQ message queue",
		Mode:       "shared-broker",
		Allocation: "vhost-user",
		Env:        []string{"AMQP_URL"},
		Tokens: []string{
			"rabbitmq", "amqplib", "pika", "spring.rabbitmq", "amqp_url", "rabbitmq_url",
			"amqp://",
		},
	},
	{
		Kind:       "s3",
		Display:    "S3 compatible object storage",
		Mode:       "shared-object-storage",
		Allocation: "bucket-access-key",
		Env:        []string{"S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY"},
		Tokens: []string{
			"minio", "boto3", "@aws-sdk/client-s3", "aws-sdk", "s3_endpoint", "s3_bucket",
			"aws_access_key_id",
		},
	},
	{
		Kind:       "opensearch",
		Display:    "OpenSearch/Elasticsearch search",
		Mode:       "shared-search",
		Allocation: "index-prefix",
		Env:        []string{"OPENSEARCH_URL"},
		Tokens: []string{
			"opensearch", "elasticsearch", "elastic_client", "elasticsearch_url",
			"opensearch_url",
		},
	},
	{
		Kind:       "kafka",
		Display:    "Kafka streaming",
		Mode:       "shared-streaming",
		Allocation: "topic-prefix-acl",
		Env:        []string{"KAFKA_BROKERS"},
		Tokens: []string{
			"kafka", "sarama", "confluent-kafka", "kafka_brokers", "spring.kafka",
		},
	},
}
