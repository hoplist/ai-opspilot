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
	Mode        string `json:"mode,omitempty"`
	TTL         string `json:"ttl,omitempty"`
}

type RegistrationPlan struct {
	Type            string     `json:"type"`
	Kind            string     `json:"kind"`
	Name            string     `json:"name"`
	Service         string     `json:"service,omitempty"`
	Cluster         string     `json:"cluster,omitempty"`
	Environment     string     `json:"environment,omitempty"`
	Scope           string     `json:"scope,omitempty"`
	Mode            string     `json:"mode,omitempty"`
	TTL             string     `json:"ttl,omitempty"`
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
	if req.TTL != "" || req.Kind == "debug-access" {
		return DebugAccessPlan(req)
	}
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
		Mode:         req.Mode,
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

func DebugAccessPlan(req RegistrationPlanRequest) RegistrationPlan {
	req = normalizePlanRequest(req)
	kind := firstNonEmpty(req.Kind, "mysql")
	if kind == "debug-access" {
		kind = "mysql"
	}
	ttl := firstNonEmpty(req.TTL, "2h")
	mode := firstNonEmpty(req.Mode, "readonly")
	name := firstNonEmpty(req.Name, "debug-"+firstNonEmpty(req.Service, "service")+"-"+kind)
	scope := firstNonEmpty(req.Scope, req.Environment+"/"+firstNonEmpty(req.Service, "service")+"/"+kind+"/debug")
	credential := Credential{
		Name:        name,
		Class:       "debug-temporary",
		Environment: req.Environment,
		Scope:       scope,
		Storage:     "temporary-account-ledger",
		Namespace:   serviceNamespace(req.Service),
		UsedBy:      nonEmptyList(req.Service),
		Permissions: []string{mode + " " + kind + " access", "time-limited access"},
		Owner:       "platform",
		Rotation:    "ttl:" + ttl,
		Source:      "plan",
	}
	return RegistrationPlan{
		Type:        "credential",
		Kind:        kind,
		Name:        name,
		Service:     req.Service,
		Cluster:     req.Cluster,
		Environment: req.Environment,
		Scope:       scope,
		Mode:        mode,
		TTL:         ttl,
		Risk:        "controlled_mutate",
		Automation:  "plan_first",
		Summary:     fmt.Sprintf("Plan temporary %s debug access for %s without exposing the password in persistent logs.", mode, firstNonEmpty(req.Service, "a service")),
		Credential:  credential,
		Steps: []string{
			"Resolve the service-owned datasource and database or namespace from the credential ledger.",
			"Create a temporary account with the requested mode and TTL.",
			"Return the secret through an ephemeral channel; do not store it in Git, GitOps, or persistent logs.",
			"Record created, expires, revoked, and last-used metadata in the credential ledger.",
			"Automatically revoke the temporary account after TTL.",
		},
		Validation: []string{
			"opspilot credentials catalog --output human",
			"opspilot errors recent --source middleware --service " + req.Service,
			"opspilot credentials plan debug-access --service " + req.Service + " --kind " + kind + " --ttl " + ttl + " --output human",
		},
		Warnings: []string{
			"This command only produces a plan in this phase; it does not create real accounts.",
			"Use readonly mode by default. Write-capable debug access needs a separate explicit plan.",
		},
	}
}

func CredentialRevokePlan(req RegistrationPlanRequest) RegistrationPlan {
	req = normalizePlanRequest(req)
	name := firstNonEmpty(req.Name, plannedCredentialName(req))
	scope := firstNonEmpty(req.Scope, plannedCredentialScope(req))
	return RegistrationPlan{
		Type:        "credential_revoke",
		Kind:        firstNonEmpty(req.Kind, "generic"),
		Name:        name,
		Service:     req.Service,
		Cluster:     req.Cluster,
		Environment: req.Environment,
		Scope:       scope,
		Risk:        "controlled_mutate",
		Automation:  "plan_first",
		Summary:     fmt.Sprintf("Plan revocation for credential %s without deleting or disabling it yet.", name),
		Credential: Credential{
			Name:        name,
			Class:       "planned-revoke",
			Environment: req.Environment,
			Scope:       scope,
			Storage:     "credential-ledger",
			Namespace:   serviceNamespace(req.Service),
			UsedBy:      nonEmptyList(req.Service),
			Permissions: []string{"revoke credential after dependency check"},
			Owner:       "platform",
			Source:      "plan",
		},
		Steps: []string{
			"Find all workloads, CI jobs, GitOps manifests, and debug sessions that reference the credential.",
			"Confirm no active release or workload still requires the credential.",
			"Create a replacement credential first if the service still needs access.",
			"Revoke or disable the old account in the upstream system.",
			"Remove only metadata references after rollout evidence confirms the replacement works.",
		},
		Validation: []string{
			"opspilot credentials catalog --output human",
			"opspilot release status --service " + req.Service + " --output human",
			"opspilot inspect service " + req.Service + " --output human",
		},
		Warnings: []string{
			"This command only produces a revocation plan in this phase.",
			"Never revoke shared or unknown-scope credentials without dependency evidence.",
		},
	}
}

func CredentialRotatePlan(req RegistrationPlanRequest) RegistrationPlan {
	req = normalizePlanRequest(req)
	name := firstNonEmpty(req.Name, plannedCredentialName(req))
	scope := firstNonEmpty(req.Scope, plannedCredentialScope(req))
	return RegistrationPlan{
		Type:        "credential_rotate",
		Kind:        firstNonEmpty(req.Kind, "generic"),
		Name:        name,
		Service:     req.Service,
		Cluster:     req.Cluster,
		Environment: req.Environment,
		Scope:       scope,
		Risk:        "controlled_mutate",
		Automation:  "plan_first",
		Summary:     fmt.Sprintf("Plan rotation for credential %s without exposing the new secret value.", name),
		Credential: Credential{
			Name:        name,
			Class:       "planned-rotate",
			Environment: req.Environment,
			Scope:       scope,
			Storage:     "credential-ledger",
			Namespace:   serviceNamespace(req.Service),
			UsedBy:      nonEmptyList(req.Service),
			Permissions: credentialPermissions(firstNonEmpty(req.Kind, "generic")),
			Owner:       "platform",
			Rotation:    "planned",
			Source:      "plan",
		},
		Steps: []string{
			"Generate a replacement secret in the upstream system or external secret manager.",
			"Update the Kubernetes Secret or external secret reference without committing the secret value.",
			"Roll out dependent workloads through GitOps or a controlled restart.",
			"Verify readiness, logs, metrics, and release status after the new credential is used.",
			"Revoke the old credential only after the replacement is confirmed healthy.",
		},
		Validation: []string{
			"opspilot credentials catalog --output human",
			"opspilot release status --service " + req.Service + " --output human",
			"opspilot fix service " + req.Service + " --dry-run --output evidence",
		},
		Warnings: []string{
			"This command only produces a rotation plan in this phase.",
			"Do not print or store the new secret value in OpsPilot evidence output.",
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

func ClusterRegistrationPlan(req RegistrationPlanRequest) RegistrationPlan {
	req = normalizePlanRequest(req)
	name := firstNonEmpty(req.Name, req.Cluster)
	if name == "" || name == "node200-test" {
		name = "new-cluster"
	}
	mode := firstNonEmpty(req.Mode, "remote")
	cluster := Cluster{
		Name:           name,
		Environment:    req.Environment,
		KubernetesMode: mode,
		KubernetesRef:  name + "-kubeconfig",
		KubeconfigPath: "/var/run/opspilot/clusters/" + name + "-kubeconfig/kubeconfig",
		KubeContext:    name,
		Source:         "plan",
	}
	return RegistrationPlan{
		Type:            "cluster",
		Kind:            mode,
		Name:            name,
		Cluster:         name,
		Environment:     req.Environment,
		Scope:           firstNonEmpty(req.Scope, req.Environment+"/"+name),
		Mode:            mode,
		Risk:            "controlled_mutate",
		Automation:      "plan_first",
		Summary:         "Plan server-side cluster registration without giving kubeconfig to CLI clients.",
		ClusterMetadata: cluster,
		RequiredKeys:    []string{"kubeconfig"},
		Steps: []string{
			"Create an OpsPilot-owned Kubernetes Secret or external secret containing the target kubeconfig.",
			"Mount the secret into opspilot-core at the planned kubeconfig path.",
			"Add metadata to OPSPILOT_CLUSTER_CATALOG without embedding kubeconfig contents.",
			"Map optional Prometheus, logs, GitOps, Argo CD, and registry names only after each datasource is verified.",
			"Publish through the standard OpsPilot release flow and verify clusters catalog.",
		},
		GitOpsPaths: []string{
			"clusters/<management-cluster>/apps/opspilot-core/deployment.yaml",
			"clusters/<management-cluster>/apps/opspilot-core/configmap.yaml",
			"clusters/<management-cluster>/apps/opspilot-core/external-secret.yaml or sealed-secret.yaml",
		},
		Validation: []string{
			"opspilot clusters catalog --output human",
			"opspilot inspect cluster --cluster " + name + " --output human",
			"opspilot capabilities --cluster " + name + " --output human",
		},
		Warnings: []string{
			"This phase does not register the remote cluster; it only records the plan.",
			"CLI clients should pass --cluster only. They should not store kubeconfig files.",
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
