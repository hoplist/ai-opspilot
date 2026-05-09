#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import json
import os

from auto_inspection.paths import PROJECT_ROOT


BASE_DIR = str(PROJECT_ROOT)


def _resolve_config_path(path):
    if not path:
        return ""
    if os.path.isabs(path):
        return path
    return os.path.join(BASE_DIR, path)


def get_config_file_path():
    path = (os.getenv("CONFIG_FILE", "") or "").strip()
    if not path:
        profile = (os.getenv("CONFIG_PROFILE", "") or "").strip()
        if profile:
            path = f"config.{profile}.json"
        else:
            path = "config.json"
    return _resolve_config_path(path)


def _load_config_file():
    path = get_config_file_path()
    if not path or not os.path.isfile(path):
        return {}
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
        if isinstance(data, dict):
            return data
    except (OSError, json.JSONDecodeError):
        return {}
    return {}


_CONFIG_DATA = _load_config_file()


def _raw_value(name, default):
    env_value = os.getenv(name)
    if env_value is not None and env_value.strip() != "":
        return env_value
    if name in _CONFIG_DATA:
        return _CONFIG_DATA[name]
    return default


def _env_int(name, default):
    value = _raw_value(name, default)
    if value is None or value == "":
        return default
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def _env_float(name, default):
    value = _raw_value(name, default)
    if value is None or value == "":
        return default
    try:
        return float(value)
    except (TypeError, ValueError):
        return default


def _env_bool(name, default):
    value = _raw_value(name, default)
    if value is None or value == "":
        return default
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return value != 0
    return str(value).strip().lower() in {"1", "true", "yes", "y", "on"}


def _normalize_list(value):
    if value is None:
        return []
    if isinstance(value, str):
        return [item.strip() for item in value.split(",") if item.strip()]
    if isinstance(value, (list, tuple, set)):
        return [str(item).strip() for item in value if str(item).strip()]
    value = str(value).strip()
    return [value] if value else []


def _env_list(name, default):
    value = _raw_value(name, default)
    if isinstance(value, str) and value.strip() == "":
        value = default
    return _normalize_list(value)


def _env_str(name, default):
    value = _raw_value(name, default)
    if value is None:
        return default
    if isinstance(value, str):
        value = value.strip()
        return value if value != "" else default
    return str(value)


def _env_dict(name, default):
    value = _raw_value(name, default)
    if value is None:
        return default
    if isinstance(value, dict):
        return value
    if isinstance(value, str):
        try:
            parsed = json.loads(value)
        except json.JSONDecodeError:
            return default
        if isinstance(parsed, dict):
            return parsed
    return default


PROMETHEUS_URLS = _env_list("PROMETHEUS_URLS", "http://10.234.4.233:9090,http://10.234.4.220:20090,http://10.234.4.124:20090")
PROMETHEUS_URL = PROMETHEUS_URLS[0] if PROMETHEUS_URLS else ""
PROMETHEUS_CLUSTERS = _env_list("PROMETHEUS_CLUSTERS", "")
SOURCE_ID = _env_str("SOURCE_ID", "")

RANGE_DAYS = _env_int("RANGE_DAYS", 7)

OLLAMA_URL = _env_str("OLLAMA_URL", "http://127.0.0.1:11434/api/generate")
OLLAMA_MODEL = _env_str("OLLAMA_MODEL", "gemma3:12b")
OLLAMA_TIMEOUT = _env_int("OLLAMA_TIMEOUT", 180)
AI_SUMMARY_MODE = _env_str("AI_SUMMARY_MODE", "strict")
AI_INVESTIGATION_ENABLED = _env_bool("AI_INVESTIGATION_ENABLED", False)
AI_INVESTIGATION_MAX_LOGS = _env_int("AI_INVESTIGATION_MAX_LOGS", 200)
AI_INVESTIGATION_MAX_EVENTS = _env_int("AI_INVESTIGATION_MAX_EVENTS", 100)
AI_INVESTIGATION_MAX_METRICS_SERIES = _env_int("AI_INVESTIGATION_MAX_METRICS_SERIES", 32)
AI_INVESTIGATION_TIMEOUT = _env_int("AI_INVESTIGATION_TIMEOUT", 180)
AI_INVESTIGATION_MAX_PODS = _env_int("AI_INVESTIGATION_MAX_PODS", 3)
AI_INVESTIGATION_LOG_TAIL_LINES = _env_int("AI_INVESTIGATION_LOG_TAIL_LINES", 120)
POD_RESTART_NOTIFY_ENABLED = _env_bool("POD_RESTART_NOTIFY_ENABLED", False)
POD_RESTART_NOTIFY_WEBHOOK_URL = _env_str("POD_RESTART_NOTIFY_WEBHOOK_URL", "")
POD_RESTART_NOTIFY_WEBHOOK_TYPE = _env_str("POD_RESTART_NOTIFY_WEBHOOK_TYPE", "generic")
POD_RESTART_NOTIFY_TARGETS = _env_list("POD_RESTART_NOTIFY_TARGETS", "")
POD_RESTART_NOTIFY_STATE_FILE = _env_str(
    "POD_RESTART_NOTIFY_STATE_FILE",
    "data/pod_restart_notify_state.json",
)
ALERT_NOTIFY_ENABLED = _env_bool("ALERT_NOTIFY_ENABLED", False)
ALERT_NOTIFY_WEBHOOK_URL = _env_str("ALERT_NOTIFY_WEBHOOK_URL", "")
ALERT_NOTIFY_WEBHOOK_TYPE = _env_str("ALERT_NOTIFY_WEBHOOK_TYPE", "generic")
ALERT_NOTIFY_STATE_FILE = _env_str("ALERT_NOTIFY_STATE_FILE", "data/alert_notify_state.json")
ALERT_NOTIFY_RANGE_HOURS = _env_int("ALERT_NOTIFY_RANGE_HOURS", 1)
ALERT_NOTIFY_COOLDOWN_SECONDS = _env_int("ALERT_NOTIFY_COOLDOWN_SECONDS", 1800)
ALERT_NOTIFY_MIN_HOURS = _env_float("ALERT_NOTIFY_MIN_HOURS", 0.0)
DASHBOARD_LINK_TEMPLATES = _env_dict(
    "DASHBOARD_LINK_TEMPLATES",
    {
        "logs": "",
        "events": "",
        "yaml": "",
        "shell": "",
        "metrics": "",
    },
)
RESOURCE_ONLY_DISK = _env_bool("RESOURCE_ONLY_DISK", False)
RESOURCE_GROUP_LABELS = _env_list("RESOURCE_GROUP_LABELS", "job")
RESOURCE_REMAINING_CURRENT = _env_float("RESOURCE_REMAINING_CURRENT", 0.30)
RESOURCE_REMAINING_MAX = _env_float("RESOURCE_REMAINING_MAX", 0.50)
RESOURCE_REMAINING_ABUNDANT_CURRENT = _env_float("RESOURCE_REMAINING_ABUNDANT_CURRENT", 0.20)
RESOURCE_REMAINING_ABUNDANT_MAX = _env_float("RESOURCE_REMAINING_ABUNDANT_MAX", 0.30)
RESOURCE_TREND_ENABLED = _env_bool("RESOURCE_TREND_ENABLED", True)
RESOURCE_TREND_DAYS = _env_int("RESOURCE_TREND_DAYS", 14)
RESOURCE_FORECAST_WINDOW_HOURS = _env_int("RESOURCE_FORECAST_WINDOW_HOURS", 72)
RESOURCE_TREND_FLAT_DELTA = _env_float("RESOURCE_TREND_FLAT_DELTA", 0.02)
RESOURCE_OUTPUT_DIR = _env_str("RESOURCE_OUTPUT_DIR", "outputs")
RESOURCE_USAGE_ALERT = _env_float("RESOURCE_USAGE_ALERT", 0.80)
RESOURCE_USAGE_WATCH = _env_float("RESOURCE_USAGE_WATCH", 0.70)
RESOURCE_DISK_MOUNT_EXCLUDE_RE = _env_str(
    "RESOURCE_DISK_MOUNT_EXCLUDE_RE",
    r"^/(proc|sys|dev|run|boot)(/|$)|^/etc/(hosts|hostname|resolv[.]conf)$|^/var/lib/(kubelet|containerd|docker|containers|cri-o)(/|$)|^/var/lib/kubelet/(pods|plugins|plugins_registry)(/|$)",
)
RESOURCE_DISK_MOUNT_INCLUDE_RE = _env_str("RESOURCE_DISK_MOUNT_INCLUDE_RE", "")
RESOURCE_POD_ENABLED = _env_bool("RESOURCE_POD_ENABLED", True)
RESOURCE_POD_REQUIRE_KSM = _env_bool("RESOURCE_POD_REQUIRE_KSM", True)
RESOURCE_POD_BASELINE = _env_str("RESOURCE_POD_BASELINE", "auto")  # auto|limits|requests|allocatable
RESOURCE_POD_TREND_DAYS = _env_int("RESOURCE_POD_TREND_DAYS", 1)
RESOURCE_POD_SHORT_WINDOW_MINUTES = _env_int("RESOURCE_POD_SHORT_WINDOW_MINUTES", 30)
RESOURCE_POD_NAMESPACE_INCLUDE_RE = _env_str("RESOURCE_POD_NAMESPACE_INCLUDE_RE", "")
RESOURCE_POD_NAMESPACE_EXCLUDE_RE = _env_str("RESOURCE_POD_NAMESPACE_EXCLUDE_RE", "")
RESOURCE_POD_NAME_INCLUDE_RE = _env_str("RESOURCE_POD_NAME_INCLUDE_RE", "")
RESOURCE_POD_NAME_EXCLUDE_RE = _env_str("RESOURCE_POD_NAME_EXCLUDE_RE", "")
RESOURCE_POD_GROUP_LABELS = _env_list("RESOURCE_POD_GROUP_LABELS", "namespace")
RESOURCE_POD_RESTART_HOURS = _env_int("RESOURCE_POD_RESTART_HOURS", 24)
RESOURCE_POD_TREND_ANOMALY_ENABLED = _env_bool("RESOURCE_POD_TREND_ANOMALY_ENABLED", True)
RESOURCE_POD_TREND_WATCH_RATIO = _env_float("RESOURCE_POD_TREND_WATCH_RATIO", 1.5)
RESOURCE_POD_TREND_ALERT_RATIO = _env_float("RESOURCE_POD_TREND_ALERT_RATIO", 2.0)
RESOURCE_POD_TREND_WATCH_DELTA = _env_float("RESOURCE_POD_TREND_WATCH_DELTA", 0.10)
RESOURCE_POD_TREND_ALERT_DELTA = _env_float("RESOURCE_POD_TREND_ALERT_DELTA", 0.20)
RESOURCE_POD_TREND_MIN_CURRENT = _env_float("RESOURCE_POD_TREND_MIN_CURRENT", 0.30)
RESOURCE_POD_TREND_BASELINE_FLOOR = _env_float("RESOURCE_POD_TREND_BASELINE_FLOOR", 0.05)

BASELINE_HISTORY_DAYS = _env_int("BASELINE_HISTORY_DAYS", 28)
BASELINE_STEP = _env_int("BASELINE_STEP", 300)
BASELINE_MAX_POINTS = _env_int("BASELINE_MAX_POINTS", 11000)

PROM_STEP = _env_int("PROM_STEP", 60)
PROM_MAX_POINTS = _env_int("PROM_MAX_POINTS", 10000)

ANOMALY_DEVIATION_RATIO = _env_float("ANOMALY_DEVIATION_RATIO", 0.2)

JENKINS_OFFLINE_EXCLUDE_NODES = _env_list(
    "JENKINS_OFFLINE_EXCLUDE_NODES",
    "Built-In Node,master,mac18876,mac96",
)
JENKINS_OFFLINE_ALERT_NAME = _env_str(
    "JENKINS_OFFLINE_ALERT_NAME",
    "Jenkins\u8282\u70b9\u79bb\u7ebf\u544a\u8b66",
)
JENKINS_OFFLINE_ALERT_LABELS = _env_list(
    "JENKINS_OFFLINE_ALERT_LABELS",
    "alertname,rulename",
)
JENKINS_OFFLINE_ALERT_PROMQL = _env_str("JENKINS_OFFLINE_ALERT_PROMQL", "")
JENKINS_OFFLINE_METRIC_PROMQL = _env_str(
    "JENKINS_OFFLINE_METRIC_PROMQL",
    "jenkins_node_online_status == bool 1",
)

REQUEST_RETRIES = _env_int("REQUEST_RETRIES", 3)
REQUEST_BACKOFF_SECONDS = _env_float("REQUEST_BACKOFF_SECONDS", 0.5)
OPENSEARCH_URL = _env_str("OPENSEARCH_URL", "")
OPENSEARCH_USERNAME = _env_str("OPENSEARCH_USERNAME", "")
OPENSEARCH_PASSWORD = _env_str("OPENSEARCH_PASSWORD", "")
OPENSEARCH_VERIFY_SSL = _env_bool("OPENSEARCH_VERIFY_SSL", True)
OPENSEARCH_TIMEOUT = _env_int("OPENSEARCH_TIMEOUT", 30)
OPENSEARCH_DASHBOARDS_URL = _env_str("OPENSEARCH_DASHBOARDS_URL", "")
OPENSEARCH_INDEX_LOGS = _env_str("OPENSEARCH_INDEX_LOGS", "logs-k8s-*")
OPENSEARCH_INDEX_EVENTS = _env_str("OPENSEARCH_INDEX_EVENTS", "events-k8s-*")
OPENSEARCH_INDEX_TRACES = _env_str("OPENSEARCH_INDEX_TRACES", "otel-traces-*")
OPENSEARCH_INDEX_INCIDENTS = _env_str("OPENSEARCH_INDEX_INCIDENTS", "inspection-incidents-*")
OPENSEARCH_INDEX_INVESTIGATIONS = _env_str(
    "OPENSEARCH_INDEX_INVESTIGATIONS",
    "inspection-investigations-*",
)
OPENSEARCH_SNAPSHOT_REPOSITORY = _env_str("OPENSEARCH_SNAPSHOT_REPOSITORY", "auto-inspection-local-fs")
OPENSEARCH_SNAPSHOT_PATH = _env_str("OPENSEARCH_SNAPSHOT_PATH", "/usr/share/opensearch/data/snapshots")
OPENSEARCH_RETENTION_LOGS_DAYS = _env_int("OPENSEARCH_RETENTION_LOGS_DAYS", 14)
OPENSEARCH_RETENTION_EVENTS_DAYS = _env_int("OPENSEARCH_RETENTION_EVENTS_DAYS", 30)
OPENSEARCH_RETENTION_INCIDENTS_DAYS = _env_int("OPENSEARCH_RETENTION_INCIDENTS_DAYS", 60)
OPENSEARCH_RETENTION_INVESTIGATIONS_DAYS = _env_int("OPENSEARCH_RETENTION_INVESTIGATIONS_DAYS", 90)
PYROSCOPE_URL = _env_str("PYROSCOPE_URL", "http://pyroscope.observability.svc.cluster.local:4040")
INVESTIGATION_HOT_STORE_DRIVER = _env_str("INVESTIGATION_HOT_STORE_DRIVER", "sqlite")
INVESTIGATION_SQLITE_PATH = _env_str("INVESTIGATION_SQLITE_PATH", "data/investigation_hot.db")
INVESTIGATION_MYSQL_HOST = _env_str("INVESTIGATION_MYSQL_HOST", "")
INVESTIGATION_MYSQL_PORT = _env_int("INVESTIGATION_MYSQL_PORT", 3306)
INVESTIGATION_MYSQL_USER = _env_str("INVESTIGATION_MYSQL_USER", "")
INVESTIGATION_MYSQL_PASSWORD = _env_str("INVESTIGATION_MYSQL_PASSWORD", "")
INVESTIGATION_MYSQL_DATABASE = _env_str("INVESTIGATION_MYSQL_DATABASE", "auto_inspection")
INVESTIGATION_COLD_STORE_DRIVER = _env_str("INVESTIGATION_COLD_STORE_DRIVER", "")
MINIO_ENDPOINT = _env_str("MINIO_ENDPOINT", "")
MINIO_ACCESS_KEY = _env_str("MINIO_ACCESS_KEY", "")
MINIO_SECRET_KEY = _env_str("MINIO_SECRET_KEY", "")
MINIO_BUCKET = _env_str("MINIO_BUCKET", "auto-inspection-archive")
MINIO_SECURE = _env_bool("MINIO_SECURE", False)
MINIO_PREFIX = _env_str("MINIO_PREFIX", "investigations")
K8S_DIRECT_ENABLED = _env_bool("K8S_DIRECT_ENABLED", True)
K8S_IN_CLUSTER_ENABLED = _env_bool("K8S_IN_CLUSTER_ENABLED", False)
KUBECTL_BIN = _env_str("KUBECTL_BIN", "kubectl")
KUBECONFIG_PATH = _env_str("KUBECONFIG_PATH", ".ssh/config")
SOURCE_STATE_FILE = _env_str("SOURCE_STATE_FILE", "data/source_state.json")
BUSINESS_DOMAIN_SUFFIXES = _env_list("BUSINESS_DOMAIN_SUFFIXES", "tpo.xzoa.com")
BUSINESS_BACKEND_SUFFIX = _env_str("BUSINESS_BACKEND_SUFFIX", "-server")
BUSINESS_FRONTEND_SUFFIX = _env_str("BUSINESS_FRONTEND_SUFFIX", "-web")
BUSINESS_SERVICE_MAP = _env_dict("BUSINESS_SERVICE_MAP", {})

HISTORY_RETENTION_DAYS = _env_int("HISTORY_RETENTION_DAYS", 90)

RISK_WEIGHT = _env_dict(
    "RISK_WEIGHT",
    {
        "cpu": 2,
        "mem": 3,
        "disk": 4,
    },
)

RISK_ORDER = _env_list("RISK_ORDER", "medium,high,critical")

_ESCALATION_POLICY_OVERRIDES = _env_dict("ESCALATION_POLICY", {})
ESCALATION_POLICY = {
    "ongoing_weeks_critical": _env_int(
        "ONGOING_WEEKS_CRITICAL",
        _ESCALATION_POLICY_OVERRIDES.get("ongoing_weeks_critical", 3),
    ),
    "regression_boost": _env_bool(
        "REGRESSION_BOOST",
        _ESCALATION_POLICY_OVERRIDES.get("regression_boost", True),
    ),
    "multi_signal_threshold": _env_int(
        "MULTI_SIGNAL_THRESHOLD",
        _ESCALATION_POLICY_OVERRIDES.get("multi_signal_threshold", 3),
    ),
}

PROMQL_CPU = _env_str(
    "PROMQL_CPU",
    '1 - avg by (instance)(rate(node_cpu_seconds_total{mode="idle"}[5m]))',
)
PROMQL_MEM = _env_str(
    "PROMQL_MEM",
    "1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)",
)
PROMQL_DISK = _env_str(
    "PROMQL_DISK",
    "max by (instance)(1 - node_filesystem_avail_bytes / node_filesystem_size_bytes)",
)
PROMQL_SWAP_ACTIVE = _env_str("PROMQL_SWAP_ACTIVE", "node_memory_SwapTotal_bytes > 0")

RUNBOOK_FILE = _env_str("RUNBOOK_FILE", "runbooks/runbooks.json")
