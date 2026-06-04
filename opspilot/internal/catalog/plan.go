package catalog

import "fmt"

type RegistrationPlanRequest struct {
	Type        string `json:"type"`
	Kind        string `json:"kind"`
	Name        string `json:"name,omitempty"`
	Service     string `json:"service,omitempty"`
	Cluster     string `json:"cluster,omitempty"`
	Environment string `json:"environment,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

type RegistrationPlan struct {
	Type            string     `json:"type"`
	Kind            string     `json:"kind"`
	Name            string     `json:"name"`
	Service         string     `json:"service,omitempty"`
	Cluster         string     `json:"cluster,omitempty"`
	Environment     string     `json:"environment,omitempty"`
	Scope           string     `json:"scope,omitempty"`
	Risk            string     `json:"risk"`
	Automation      string     `json:"automation"`
	Summary         string     `json:"summary"`
	Credential      Credential `json:"credential"`
	RequiredKeys    []string   `json:"required_keys,omitempty"`
	Steps           []string   `json:"steps"`
	GitOpsPaths     []string   `json:"gitops_paths,omitempty"`
	Validation      []string   `json:"validation,omitempty"`
	Warnings        []string   `json:"warnings,omitempty"`
	ClusterMetadata Cluster    `json:"cluster_metadata,omitempty"`
}

func CredentialRegistrationPlan(req RegistrationPlanRequest) RegistrationPlan {
	req = normalizePlanRequest(req)
	keys := credentialKeys(req.Kind)
	name := firstNonEmpty(req.Name, plannedCredentialName(req))
	scope := firstNonEmpty(req.Scope, plannedCredentialScope(req))
	credential := Credential{
		Name:        name,
		Class:       "application-runtime",
		Environment: req.Environment,
		Scope:       scope,
		Storage:     "kubernetes-credential-ref",
		Namespace:   serviceNamespace(req.Service),
		UsedBy:      nonEmptyList(req.Service),
		Permissions: credentialPermissions(req.Kind),
		Owner:       "platform",
		Rotation:    "90d",
		Source:      "plan",
	}
	return RegistrationPlan{
		Type:         "credential",
		Kind:         req.Kind,
		Name:         name,
		Service:      req.Service,
		Environment:  req.Environment,
		Scope:        scope,
		Risk:         "controlled_mutate",
		Automation:   "plan_first",
		Summary:      fmt.Sprintf("Plan service-scoped %s credentials without exposing secret values.", req.Kind),
		Credential:   credential,
		RequiredKeys: keys,
		Steps: []string{
			"Allocate service-scoped account, database, schema, bucket, topic, or prefix through the platform owner.",
			"Create or update a Kubernetes Secret in the service namespace with only the required keys.",
			"Reference the Secret from generated Deployment envFrom or explicit env mappings.",
			"Record metadata in OPSPILOT_CREDENTIAL_CATALOG without storing secret values.",
			"Run repo preflight and release through GitLab Runner -> BuildKit -> Registry -> GitOps -> Argo CD.",
		},
		GitOpsPaths: []string{
			"clusters/<cluster>/apps/<group>/<project>/<service>/deployment.yaml",
			"clusters/<cluster>/apps/<group>/<project>/<service>/secret-ref.yaml or external-secret.yaml",
		},
		Validation: []string{
			"opspilot credentials catalog --output human",
			"opspilot inspect service " + req.Service + " --output human",
			"opspilot fix service " + req.Service + " --dry-run --output evidence",
		},
	}
}

func DatasourceRegistrationPlan(req RegistrationPlanRequest) RegistrationPlan {
	req = normalizePlanRequest(req)
	name := firstNonEmpty(req.Name, req.Kind+"-"+req.Cluster)
	scope := firstNonEmpty(req.Scope, req.Cluster+"/"+req.Kind)
	keys := datasourceKeys(req.Kind)
	credential := Credential{
		Name:        name,
		Class:       "observability-datasource",
		Environment: req.Environment,
		Scope:       scope,
		Storage:     "kubernetes-config-or-credential-ref",
		Namespace:   "opspilot",
		UsedBy:      []string{"opspilot-core"},
		Permissions: datasourcePermissions(req.Kind),
		Owner:       "platform",
		Rotation:    "90d",
		Source:      "plan",
	}
	cluster := Cluster{
		Name:        req.Cluster,
		Environment: req.Environment,
		Source:      "plan",
	}
	switch req.Kind {
	case "prometheus":
		cluster.Prometheus = name
	case "elk", "opensearch", "openobserve", "apisix":
		cluster.Logs = name
	}
	return RegistrationPlan{
		Type:            "datasource",
		Kind:            req.Kind,
		Name:            name,
		Cluster:         req.Cluster,
		Environment:     req.Environment,
		Scope:           scope,
		Risk:            "controlled_mutate",
		Automation:      "plan_first",
		Summary:         fmt.Sprintf("Plan %s datasource metadata for OpsPilot without exposing secret values.", req.Kind),
		Credential:      credential,
		ClusterMetadata: cluster,
		RequiredKeys:    keys,
		Steps: []string{
			"Confirm datasource network path from opspilot-core to the target endpoint.",
			"Create a Kubernetes Secret only when authentication is required; otherwise store endpoint metadata in ConfigMap.",
			"Update OPSPILOT_CLUSTER_CATALOG and the relevant datasource env vars.",
			"Publish through the standard OpsPilot GitLab/GitOps pipeline.",
			"Verify capability gaps explicitly; missing datasource must not block Pod-first investigation.",
		},
		GitOpsPaths: []string{
			"clusters/<cluster>/apps/opspilot-core/configmap.yaml",
			"clusters/<cluster>/apps/opspilot-core/external-secret.yaml or sealed-secret.yaml",
		},
		Validation: []string{
			"opspilot clusters catalog --output human",
			"opspilot capabilities --output human",
			"opspilot doctor --output human",
		},
	}
}

func normalizePlanRequest(req RegistrationPlanRequest) RegistrationPlanRequest {
	req.Type = firstNonEmpty(req.Type, "credential")
	req.Kind = firstNonEmpty(req.Kind, "generic")
	req.Environment = firstNonEmpty(req.Environment, "test")
	req.Cluster = firstNonEmpty(req.Cluster, "node200-test")
	return req
}

func plannedCredentialName(req RegistrationPlanRequest) string {
	service := firstNonEmpty(req.Service, "service")
	return service + "-" + req.Kind + "-credentials"
}

func plannedCredentialScope(req RegistrationPlanRequest) string {
	if req.Service == "" {
		return req.Environment + "/" + req.Kind
	}
	return req.Environment + "/" + req.Service + "/" + req.Kind
}

func serviceNamespace(service string) string {
	if service == "" {
		return ""
	}
	return "cicd-" + service
}

func credentialKeys(kind string) []string {
	switch kind {
	case "mysql", "postgres":
		return []string{"DATABASE_URL"}
	case "redis":
		return []string{"REDIS_URL"}
	case "rabbitmq":
		return []string{"AMQP_URL"}
	case "s3", "minio":
		return []string{"S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY"}
	case "opensearch", "elasticsearch":
		return []string{"OPENSEARCH_URL", "OPENSEARCH_USERNAME", "OPENSEARCH_PASSWORD"}
	case "kafka":
		return []string{"KAFKA_BROKERS", "KAFKA_USERNAME", "KAFKA_PASSWORD"}
	default:
		return []string{"APP_SECRET"}
	}
}

func datasourceKeys(kind string) []string {
	switch kind {
	case "prometheus":
		return []string{"PROMETHEUS_URL"}
	case "elk", "opensearch":
		return []string{"LOGSEARCH_URL", "LOGSEARCH_INDEX", "LOGSEARCH_USERNAME", "LOGSEARCH_PASSWORD"}
	case "openobserve":
		return []string{"OPENOBSERVE_URL", "OPENOBSERVE_ORG", "OPENOBSERVE_TOKEN"}
	case "apisix":
		return []string{"OPSPILOT_APISIX_INDEX", "OPSPILOT_LOG_CORRELATION_ROUTES"}
	default:
		return []string{"DATASOURCE_URL"}
	}
}

func credentialPermissions(kind string) []string {
	switch kind {
	case "mysql", "postgres":
		return []string{"service-scoped database/schema access"}
	case "redis":
		return []string{"service-scoped key prefix access"}
	case "s3", "minio":
		return []string{"service-scoped bucket or prefix access"}
	case "rabbitmq":
		return []string{"service-scoped vhost access"}
	case "kafka":
		return []string{"service-scoped topic prefix access"}
	default:
		return []string{"service-scoped runtime access"}
	}
}

func datasourcePermissions(kind string) []string {
	switch kind {
	case "prometheus":
		return []string{"read metrics"}
	case "elk", "opensearch", "openobserve", "apisix":
		return []string{"read logs"}
	default:
		return []string{"read datasource"}
	}
}

func nonEmptyList(values ...string) []string {
	out := []string{}
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
