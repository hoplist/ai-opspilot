#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import datetime
import json
import os
import re
import requests
import subprocess
import uuid
from pathlib import Path

from auto_inspection import config
from auto_inspection import dashboards_client
from auto_inspection import event_search
from auto_inspection import incident_store
from auto_inspection import investigation_storage
from auto_inspection import source_context
from auto_inspection import log_search
from auto_inspection import opensearch_client
from auto_inspection import prometheus_client
from auto_inspection.http_client import request_json
from auto_inspection.paths import project_path


DEFAULT_RANGE_HOURS = 6
INVESTIGATION_DIR = project_path("data", "investigations")
INVESTIGATION_TEMPLATE_NAME = "auto-inspection-investigations"
KUBECTL_LOG_KEYWORDS = (
    "error",
    "warn",
    "exception",
    "fatal",
    "panic",
    "oom",
    "killed",
    "crash",
    "back-off",
    "failed",
    "timeout",
    "address already in use",
)
_INSTANCE_TARGET_CACHE = None
_KUBE_POD_INFO_CACHE = None
_SERVICE_TARGET_CACHE = {}
_IN_CLUSTER_SESSION = requests.Session()
_IN_CLUSTER_SESSION.trust_env = False


def _now_ts():
    return int(datetime.datetime.now().timestamp())


def _now_str():
    return datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def _parse_time(value):
    if value in (None, ""):
        return None
    if isinstance(value, (int, float)):
        return int(value)
    text = str(value).strip()
    if not text:
        return None
    if text.isdigit():
        return int(text)
    try:
        dt = datetime.datetime.fromisoformat(text.replace("Z", "+00:00"))
        return int(dt.timestamp())
    except ValueError:
        return None


def _truthy(value):
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return value != 0
    return str(value or "").strip().lower() in {"1", "true", "yes", "y", "on"}


def _bounded_int(value, default, minimum, maximum):
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        parsed = default
    return max(minimum, min(parsed, maximum))


def _to_float(value):
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def _iso_utc(timestamp):
    return datetime.datetime.fromtimestamp(
        int(timestamp),
        datetime.timezone.utc,
    ).strftime("%Y-%m-%dT%H:%M:%SZ")


def _promql_escape(value):
    return str(value or "").replace("\\", "\\\\").replace('"', '\\"')


def _promql_regex_escape(value):
    return re.sub(r'([.^$*+?{}\[\]|()\\])', r'\\\1', str(value or ""))


def _duration_expr(seconds):
    seconds = max(60, int(seconds or 3600))
    if seconds % 86400 == 0:
        return f"{max(1, seconds // 86400)}d"
    if seconds % 3600 == 0:
        return f"{max(1, seconds // 3600)}h"
    return f"{max(1, seconds // 60)}m"


def _kubectl_available():
    return bool(getattr(config, "K8S_DIRECT_ENABLED", True))


def _in_cluster_token_path():
    return "/var/run/secrets/kubernetes.io/serviceaccount/token"


def _in_cluster_ca_path():
    return "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"


def _k8s_in_cluster_available():
    if not bool(getattr(config, "K8S_IN_CLUSTER_ENABLED", False)):
        return False
    return bool(
        os.environ.get("KUBERNETES_SERVICE_HOST")
        and os.path.exists(_in_cluster_token_path())
        and os.path.exists(_in_cluster_ca_path())
    )


def _k8s_api_base_url():
    host = os.environ.get("KUBERNETES_SERVICE_HOST", "kubernetes.default.svc")
    port = os.environ.get("KUBERNETES_SERVICE_PORT", "443")
    return f"https://{host}:{port}"


def _k8s_api_headers():
    with open(_in_cluster_token_path(), "r", encoding="utf-8") as f:
        token = f.read().strip()
    return {"Authorization": f"Bearer {token}"}


def _k8s_api_request(method, path, *, params=None, timeout=30, expect_json=True):
    if not _k8s_in_cluster_available():
        raise RuntimeError("In-cluster Kubernetes API is not available.")

    url = f"{_k8s_api_base_url()}{path}"
    response = _IN_CLUSTER_SESSION.request(
        method,
        url,
        params=params,
        timeout=timeout,
        headers=_k8s_api_headers(),
        verify=_in_cluster_ca_path(),
        proxies={"http": None, "https": None},
    )
    if response.status_code >= 400:
        text = response.text.strip()
        if len(text) > 500:
            text = text[:500] + "..."
        raise RuntimeError(f"Kubernetes API request failed: status={response.status_code} body={text}")
    if not expect_json:
        return response.text
    try:
        return response.json()
    except ValueError as exc:
        raise RuntimeError(f"Invalid JSON from Kubernetes API: {exc}") from exc


def _k8s_get_pod_json(namespace, pod_name):
    return _k8s_api_request("GET", f"/api/v1/namespaces/{namespace}/pods/{pod_name}", expect_json=True)


def _k8s_list_pods_json(namespace, *, label_selector=""):
    params = {}
    if label_selector:
        params["labelSelector"] = label_selector
    return _k8s_api_request("GET", f"/api/v1/namespaces/{namespace}/pods", params=params, expect_json=True)


def _k8s_get_service_json(namespace, service_name):
    return _k8s_api_request("GET", f"/api/v1/namespaces/{namespace}/services/{service_name}", expect_json=True)


def _k8s_list_events_json(namespace):
    return _k8s_api_request("GET", f"/api/v1/namespaces/{namespace}/events", expect_json=True)


def _k8s_get_pod_logs(namespace, pod_name, *, previous=False, tail_lines=100, since_time=None):
    params = {
        "allContainers": "true",
        "timestamps": "true",
        "tailLines": str(max(20, int(tail_lines or 100))),
    }
    if previous:
        params["previous"] = "true"
    if since_time:
        params["sinceTime"] = since_time
    return _k8s_api_request(
        "GET",
        f"/api/v1/namespaces/{namespace}/pods/{pod_name}/log",
        params=params,
        timeout=40,
        expect_json=False,
    )


def _resolved_kubeconfig_path():
    path = getattr(config, "KUBECONFIG_PATH", ".ssh/config") or ".ssh/config"
    if os.path.isabs(path):
        return path
    return project_path(path)


def _kubectl_env():
    env = os.environ.copy()
    env["KUBECONFIG"] = _resolved_kubeconfig_path()
    return env


def _run_kubectl(args, *, timeout=30, expect_json=False):
    if not _kubectl_available():
        raise RuntimeError("Direct Kubernetes access is disabled.")

    command = [getattr(config, "KUBECTL_BIN", "kubectl")] + list(args)
    try:
        completed = subprocess.run(
            command,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=timeout,
            env=_kubectl_env(),
            check=False,
        )
    except FileNotFoundError as exc:
        raise RuntimeError(f"kubectl not found: {exc}") from exc
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"kubectl timeout: {' '.join(command)}") from exc

    if completed.returncode != 0:
        stderr = (completed.stderr or completed.stdout or "").strip()
        if len(stderr) > 500:
            stderr = stderr[:500] + "..."
        raise RuntimeError(stderr or f"kubectl failed with exit code {completed.returncode}")

    if not expect_json:
        return completed.stdout
    try:
        return json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"Invalid JSON from kubectl: {exc}") from exc


def _container_resources(spec_containers):
    resources = {}
    for container in spec_containers or []:
        resources[container.get("name")] = container.get("resources") or {}
    return resources


def _container_status_state(status):
    state = status.get("state") or {}
    waiting = state.get("waiting") or {}
    running = state.get("running") or {}
    terminated = state.get("terminated") or {}
    if waiting:
        return {
            "kind": "waiting",
            "reason": waiting.get("reason"),
            "message": waiting.get("message"),
        }
    if running:
        return {
            "kind": "running",
            "started_at": running.get("startedAt"),
        }
    if terminated:
        return {
            "kind": "terminated",
            "reason": terminated.get("reason"),
            "message": terminated.get("message"),
            "exit_code": terminated.get("exitCode"),
            "started_at": terminated.get("startedAt"),
            "finished_at": terminated.get("finishedAt"),
        }
    return {}


def _summarize_pod(pod_json):
    metadata = pod_json.get("metadata") or {}
    spec = pod_json.get("spec") or {}
    status = pod_json.get("status") or {}
    resources_by_name = _container_resources(spec.get("containers"))
    container_statuses = []
    for item in status.get("containerStatuses") or []:
        name = item.get("name")
        last_terminated = ((item.get("lastState") or {}).get("terminated")) or {}
        container_statuses.append(
            {
                "name": name,
                "image": item.get("image"),
                "ready": bool(item.get("ready")),
                "restart_count": int(item.get("restartCount") or 0),
                "state": _container_status_state(item),
                "last_terminated": {
                    "reason": last_terminated.get("reason"),
                    "exit_code": last_terminated.get("exitCode"),
                    "started_at": last_terminated.get("startedAt"),
                    "finished_at": last_terminated.get("finishedAt"),
                },
                "resources": resources_by_name.get(name) or {},
            }
        )

    conditions = []
    for item in status.get("conditions") or []:
        conditions.append(
            {
                "type": item.get("type"),
                "status": item.get("status"),
                "reason": item.get("reason"),
                "message": item.get("message"),
                "last_transition_time": item.get("lastTransitionTime"),
            }
        )

    owner = ((metadata.get("ownerReferences") or [{}])[0]) or {}
    return {
        "name": metadata.get("name"),
        "namespace": metadata.get("namespace"),
        "node": spec.get("nodeName"),
        "pod_ip": status.get("podIP"),
        "phase": status.get("phase"),
        "qos_class": status.get("qosClass"),
        "owner_kind": owner.get("kind"),
        "owner_name": owner.get("name"),
        "labels": metadata.get("labels") or {},
        "created_at": metadata.get("creationTimestamp"),
        "start_time": status.get("startTime"),
        "conditions": conditions,
        "containers": container_statuses,
        "raw": pod_json,
    }


def _load_namespace_pods(namespace):
    if _k8s_in_cluster_available():
        data = _k8s_list_pods_json(namespace)
    else:
        data = _run_kubectl(["get", "pods", "-n", namespace, "-o", "json"], expect_json=True)
    return data.get("items") or []


def _match_pod_to_workload(pod_json, workload_name):
    metadata = pod_json.get("metadata") or {}
    name = metadata.get("name") or ""
    if name == workload_name or name.startswith(f"{workload_name}-") or name.startswith(workload_name):
        return True
    if (metadata.get("generateName") or "").startswith(workload_name):
        return True
    owner_refs = metadata.get("ownerReferences") or []
    return any((ref.get("name") or "") == workload_name for ref in owner_refs)


def _resolve_target_pods(namespace, pod=None, workload_name=None):
    if pod:
        if _k8s_in_cluster_available():
            pod_json = _k8s_get_pod_json(namespace, pod)
        else:
            pod_json = _run_kubectl(["get", "pod", pod, "-n", namespace, "-o", "json"], expect_json=True)
        return [_summarize_pod(pod_json)]

    items = []
    for pod_json in _load_namespace_pods(namespace):
        if _match_pod_to_workload(pod_json, workload_name):
            items.append(_summarize_pod(pod_json))

    items.sort(
        key=lambda item: (
            sum(container.get("restart_count", 0) for container in item.get("containers") or []),
            1 if item.get("phase") != "Running" else 0,
            item.get("name") or "",
        ),
        reverse=True,
    )
    limit = max(1, int(getattr(config, "AI_INVESTIGATION_MAX_PODS", 3) or 3))
    return items[:limit]


def _parse_event_timestamp(value):
    if not value:
        return 0
    try:
        return int(datetime.datetime.fromisoformat(str(value).replace("Z", "+00:00")).timestamp())
    except ValueError:
        return 0


def _normalize_event_item(item):
    involved = item.get("involvedObject") or {}
    source = item.get("source") or {}
    metadata = item.get("metadata") or {}
    return {
        "timestamp": item.get("lastTimestamp") or item.get("eventTime") or item.get("firstTimestamp") or metadata.get("creationTimestamp"),
        "type": item.get("type"),
        "reason": item.get("reason"),
        "message": item.get("message"),
        "namespace": metadata.get("namespace"),
        "name": metadata.get("name"),
        "count": item.get("count"),
        "first_timestamp": item.get("firstTimestamp"),
        "last_timestamp": item.get("lastTimestamp"),
        "object_kind": involved.get("kind"),
        "object_name": involved.get("name"),
        "source_component": source.get("component"),
        "source_host": source.get("host"),
    }


def _collect_kubernetes_events(request, pod_names):
    namespace = request["namespace"]
    if _k8s_in_cluster_available():
        data = _k8s_list_events_json(namespace)
    else:
        data = _run_kubectl(["get", "events", "-n", namespace, "-o", "json"], expect_json=True)
    pod_set = set(pod_names or [])
    workload_name = request.get("workload_name") or ""
    items = []
    for event in data.get("items") or []:
        involved = event.get("involvedObject") or {}
        involved_name = involved.get("name") or ""
        if pod_set:
            if involved_name not in pod_set and not any(involved_name.startswith(f"{name}-") for name in pod_set):
                if not workload_name or not involved_name.startswith(workload_name):
                    continue
        elif workload_name and not involved_name.startswith(workload_name):
            continue

        event_ts = _parse_event_timestamp(
            event.get("lastTimestamp")
            or event.get("eventTime")
            or event.get("firstTimestamp")
            or ((event.get("metadata") or {}).get("creationTimestamp"))
        )
        if event_ts and (event_ts < request["start_ts"] or event_ts > request["end_ts"]):
            continue
        items.append(_normalize_event_item(event))

    items.sort(key=lambda item: _parse_event_timestamp(item.get("timestamp")), reverse=True)
    return items[: request["max_events"]]


def _parse_log_output(text, pod_name, mode):
    items = []
    for raw_line in (text or "").splitlines():
        line = raw_line.strip()
        if not line:
            continue
        timestamp = ""
        message = line
        if " " in line and "T" in line[:32]:
            timestamp, message = line.split(" ", 1)
        items.append(
            {
                "timestamp": timestamp,
                "pod": pod_name,
                "mode": mode,
                "message": message.strip(),
            }
        )
    return items


def _log_score(message):
    lowered = str(message or "").lower()
    score = 0
    for keyword in KUBECTL_LOG_KEYWORDS:
        if keyword in lowered:
            score += 2
    if lowered.startswith("error") or lowered.startswith("warning"):
        score += 1
    return score


def _dedupe_log_items(items, limit):
    seen = set()
    ranked = []
    for item in items:
        key = (item.get("pod"), item.get("mode"), item.get("message"))
        if key in seen:
            continue
        seen.add(key)
        ranked.append(item)
    ranked.sort(
        key=lambda item: (
            _log_score(item.get("message")),
            item.get("timestamp") or "",
        ),
        reverse=True,
    )
    return ranked[:limit]


def _collect_kubernetes_logs(request, pod_names):
    namespace = request["namespace"]
    total_limit = request["max_logs"]
    tail_lines = max(20, int(getattr(config, "AI_INVESTIGATION_LOG_TAIL_LINES", 120) or 120))
    pod_count = max(1, len(pod_names))
    per_pod_tail = max(20, min(tail_lines, total_limit // pod_count if total_limit else tail_lines))
    since_time = _iso_utc(request["start_ts"])
    items = []

    for pod_name in pod_names:
        if _k8s_in_cluster_available():
            calls = [
                ("current", {"previous": False, "tail_lines": per_pod_tail, "since_time": since_time}),
                ("previous", {"previous": True, "tail_lines": max(20, per_pod_tail // 2), "since_time": None}),
            ]
            for mode, params in calls:
                try:
                    output = _k8s_get_pod_logs(namespace, pod_name, **params)
                except RuntimeError as exc:
                    text = str(exc).lower()
                    if mode == "previous" and ("previous terminated container" in text or "not found" in text):
                        continue
                    items.append(
                        {
                            "timestamp": "",
                            "pod": pod_name,
                            "mode": mode,
                            "message": f"[k8s api logs failed] {exc}",
                        }
                    )
                    continue
                items.extend(_parse_log_output(output, pod_name, mode))
        else:
            commands = [
                (
                    "current",
                    [
                        "logs",
                        pod_name,
                        "-n",
                        namespace,
                        "--all-containers=true",
                        "--timestamps",
                        f"--tail={per_pod_tail}",
                        f"--since-time={since_time}",
                    ],
                ),
                (
                    "previous",
                    [
                        "logs",
                        pod_name,
                        "-n",
                        namespace,
                        "--all-containers=true",
                        "--timestamps",
                        "--previous",
                        f"--tail={max(20, per_pod_tail // 2)}",
                    ],
                ),
            ]
            for mode, args in commands:
                try:
                    output = _run_kubectl(args, timeout=40, expect_json=False)
                except RuntimeError as exc:
                    text = str(exc).lower()
                    if mode == "previous" and ("previous terminated container" in text or "not found" in text):
                        continue
                    items.append(
                        {
                            "timestamp": "",
                            "pod": pod_name,
                            "mode": mode,
                            "message": f"[kubectl logs failed] {exc}",
                        }
                    )
                    continue
                items.extend(_parse_log_output(output, pod_name, mode))

    return _dedupe_log_items(items, total_limit)


def _search_logs_via_opensearch(request, pod_names):
    if not opensearch_client.is_configured():
        return []
    items = []
    per_pod = max(10, request["max_logs"] // max(1, len(pod_names)))
    for pod_name in pod_names or [request.get("pod")]:
        if not pod_name:
            continue
        try:
            result = log_search.search_logs(
                {
                    "cluster": request.get("cluster"),
                    "namespace": request["namespace"],
                    "pod": pod_name,
                    "q": request.get("query"),
                    "start_ts": request["start_ts"],
                    "end_ts": request["end_ts"],
                    "size": per_pod,
                }
            )
        except Exception:
            return []
        items.extend(result.get("items") or [])
    normalized = []
    for item in items:
        normalized.append(
            {
                "timestamp": item.get("timestamp") or "",
                "pod": item.get("pod"),
                "mode": "search",
                "message": item.get("message") or "",
                "severity": item.get("severity"),
                "logger": item.get("logger"),
                "source": "opensearch",
            }
        )
    return _dedupe_log_items(normalized, request["max_logs"])


def _search_events_via_opensearch(request, pod_names):
    if not opensearch_client.is_configured():
        return []
    items = []
    per_pod = max(10, request["max_events"] // max(1, len(pod_names)))
    for pod_name in pod_names or [request.get("pod")]:
        if not pod_name:
            continue
        try:
            result = event_search.search_events(
                {
                    "cluster": request.get("cluster"),
                    "namespace": request["namespace"],
                    "pod": pod_name,
                    "q": request.get("query"),
                    "start_ts": request["start_ts"],
                    "end_ts": request["end_ts"],
                    "size": per_pod,
                }
            )
        except Exception:
            return []
        items.extend(result.get("items") or [])
    deduped = []
    seen = set()
    for item in items:
        key = (
            item.get("timestamp"),
            item.get("reason"),
            item.get("message"),
            ((item.get("regarding") or {}).get("name")),
        )
        if key in seen:
            continue
        seen.add(key)
        deduped.append(item)
    deduped.sort(key=lambda item: _parse_event_timestamp(item.get("timestamp")), reverse=True)
    return deduped[: request["max_events"]]


def _cluster_url_pairs():
    urls = list(getattr(config, "PROMETHEUS_URLS", []) or [])
    clusters = list(getattr(config, "PROMETHEUS_CLUSTERS", []) or [])
    pairs = []
    for idx, url in enumerate(urls):
        cluster = clusters[idx] if idx < len(clusters) and clusters[idx] else f"cluster-{idx + 1}"
        pairs.append((cluster, url))
    return pairs


def _load_active_targets():
    global _INSTANCE_TARGET_CACHE
    if _INSTANCE_TARGET_CACHE is not None:
        return _INSTANCE_TARGET_CACHE
    items = []
    for cluster_name, url in _cluster_url_pairs():
        try:
            targets = prometheus_client.active_targets(url=url, timeout=15)
        except Exception:
            continue
        for target in targets:
            labels = target.get("labels") or {}
            discovered = target.get("discoveredLabels") or {}
            items.append(
                {
                    "cluster": cluster_name,
                    "instance": labels.get("instance") or discovered.get("__address__") or "",
                    "job": labels.get("job") or discovered.get("job") or "",
                    "namespace": labels.get("namespace") or discovered.get("__meta_kubernetes_namespace") or "",
                    "pod": labels.get("pod") or discovered.get("__meta_kubernetes_pod_name") or "",
                    "service": labels.get("service") or discovered.get("__meta_kubernetes_service_name") or "",
                    "node": labels.get("node") or discovered.get("__meta_kubernetes_pod_node_name") or "",
                    "container": labels.get("container") or discovered.get("__meta_kubernetes_pod_container_name") or "",
                    "aliases": _target_aliases(
                        labels.get("instance") or discovered.get("__address__") or "",
                        labels.get("namespace") or discovered.get("__meta_kubernetes_namespace") or "",
                        labels.get("pod") or discovered.get("__meta_kubernetes_pod_name") or "",
                        labels.get("service") or discovered.get("__meta_kubernetes_service_name") or "",
                        labels.get("node") or discovered.get("__meta_kubernetes_pod_node_name") or "",
                    ),
                }
            )
    _INSTANCE_TARGET_CACHE = items
    return items


def _normalize_host_alias(value):
    text = str(value or "").strip()
    if not text:
        return ""
    if "://" in text:
        text = text.split("://", 1)[1]
    return text.split("/", 1)[0]


def _value_aliases(value):
    text = str(value or "").strip()
    if not text:
        return []
    aliases = {text}
    host = _normalize_host_alias(text)
    if host:
        aliases.add(host)
        if ":" in host:
            aliases.add(host.rsplit(":", 1)[0])
    return [alias for alias in aliases if alias]


def _target_aliases(instance, namespace, pod, service, node):
    aliases = set(_value_aliases(instance))
    if namespace and pod:
        aliases.add(f"{namespace}/{pod}")
    if namespace and service:
        aliases.add(f"{namespace}/{service}")
    if pod:
        aliases.add(pod)
    if service:
        aliases.add(service)
    if node:
        aliases.add(node)
    return sorted(alias for alias in aliases if alias)


def _find_unique_alias_match(entries, value):
    matches = []
    for entry in entries or []:
        aliases = entry.get("aliases") or _target_aliases(
            entry.get("instance") or "",
            entry.get("namespace") or "",
            entry.get("pod") or "",
            entry.get("service") or "",
            entry.get("node") or "",
        )
        if value in aliases:
            matches.append(entry)
    if len(matches) != 1:
        return {}
    return dict(matches[0])


def _load_kube_pod_info_targets():
    global _KUBE_POD_INFO_CACHE
    if _KUBE_POD_INFO_CACHE is not None:
        return _KUBE_POD_INFO_CACHE
    items = []
    promql = (
        "max by (namespace,pod,node,host_ip,pod_ip,created_by_kind,created_by_name) "
        "(kube_pod_info)"
    )
    for cluster_name, url in _cluster_url_pairs():
        try:
            series = prometheus_client.query_instant(promql, url=url, timeout=15)
        except Exception:
            continue
        for item in series or []:
            metric = item.get("metric") or {}
            namespace = metric.get("namespace") or ""
            pod = metric.get("pod") or ""
            if not namespace or not pod:
                continue
            service = metric.get("service") or ""
            entry = {
                "cluster": cluster_name,
                "instance": metric.get("pod_ip") or metric.get("host_ip") or "",
                "job": "",
                "namespace": namespace,
                "pod": pod,
                "service": service,
                "node": metric.get("node") or "",
                "container": "",
                "workload_name": metric.get("created_by_name") or "",
                "workload_kind": metric.get("created_by_kind") or "",
                "host_ip": metric.get("host_ip") or "",
                "pod_ip": metric.get("pod_ip") or "",
                "aliases": _target_aliases(
                    metric.get("pod_ip") or metric.get("host_ip") or "",
                    namespace,
                    pod,
                    service,
                    metric.get("node") or "",
                ),
            }
            items.append(entry)
    _KUBE_POD_INFO_CACHE = items
    return items


def _service_selector(namespace, service_name):
    if not namespace or not service_name:
        return ""
    try:
        if _k8s_in_cluster_available():
            service_json = _k8s_get_service_json(namespace, service_name)
        else:
            service_json = _run_kubectl(["get", "service", service_name, "-n", namespace, "-o", "json"], expect_json=True)
    except Exception:
        return ""
    selector = (((service_json or {}).get("spec") or {}).get("selector")) or {}
    parts = []
    for key, value in selector.items():
        text_key = str(key or "").strip()
        text_value = str(value or "").strip()
        if text_key and text_value:
            parts.append(f"{text_key}={text_value}")
    return ",".join(parts)


def _resolve_service_mapping(namespace, service_name):
    cache_key = f"{namespace}/{service_name}"
    if cache_key in _SERVICE_TARGET_CACHE:
        return dict(_SERVICE_TARGET_CACHE[cache_key])

    mapping = {
        "namespace": namespace,
        "service": service_name,
        "pod": "",
        "node": "",
        "workload_name": "",
        "workload_kind": "",
    }
    selector = _service_selector(namespace, service_name)
    if selector:
        try:
            if _k8s_in_cluster_available():
                pods_json = _k8s_list_pods_json(namespace, label_selector=selector)
            else:
                pods_json = _run_kubectl(["get", "pods", "-n", namespace, "-l", selector, "-o", "json"], expect_json=True)
            pods = [_summarize_pod(item) for item in (pods_json.get("items") or [])]
            pods.sort(
                key=lambda item: (
                    sum(container.get("restart_count", 0) for container in item.get("containers") or []),
                    item.get("name") or "",
                ),
                reverse=True,
            )
            if pods:
                mapping["pod"] = pods[0].get("name") or ""
                mapping["node"] = pods[0].get("node") or ""
                mapping.update(_resolve_workload_from_pod(namespace, mapping["pod"]))
        except Exception:
            pass
    _SERVICE_TARGET_CACHE[cache_key] = mapping
    return dict(mapping)


def _resolve_workload_from_pod(namespace, pod_name):
    if not namespace or not pod_name:
        return {"workload_name": "", "workload_kind": ""}
    try:
        if _k8s_in_cluster_available():
            pod_json = _k8s_get_pod_json(namespace, pod_name)
        else:
            pod_json = _run_kubectl(["get", "pod", pod_name, "-n", namespace, "-o", "json"], expect_json=True)
    except Exception:
        return {"workload_name": "", "workload_kind": ""}
    metadata = pod_json.get("metadata") or {}
    owner = ((metadata.get("ownerReferences") or [{}])[0]) or {}
    workload_name = owner.get("name") or ""
    workload_kind = owner.get("kind") or ""
    return {"workload_name": workload_name, "workload_kind": workload_kind}


def _resolve_instance_mapping(instance):
    value = str(instance or "").strip()
    if not value:
        return {}

    parsed = _parse_target_text(value)
    if parsed.get("namespace") and parsed.get("pod"):
        mapped = {
            "cluster": parsed.get("cluster") or "",
            "instance": value,
            "job": "",
            "namespace": parsed["namespace"],
            "pod": parsed["pod"],
            "service": "",
            "node": "",
            "container": "",
            "mapped_by": "namespace-pod",
        }
        mapped.update(_resolve_workload_from_pod(parsed["namespace"], parsed["pod"]))
        mapped["investigation_supported"] = bool(mapped.get("namespace") and (mapped.get("pod") or mapped.get("workload_name")))
        return mapped

    for alias in _value_aliases(value):
        target = _find_unique_alias_match(_load_active_targets(), alias)
        if target:
            mapped = dict(target)
            if mapped.get("namespace") and mapped.get("service") and not mapped.get("pod"):
                mapped.update(_resolve_service_mapping(mapped.get("namespace"), mapped.get("service")))
            if mapped.get("namespace") and mapped.get("pod") and not mapped.get("workload_name"):
                mapped.update(_resolve_workload_from_pod(mapped.get("namespace"), mapped.get("pod")))
            mapped["mapped_by"] = "prometheus-targets"
            mapped["investigation_supported"] = bool(mapped.get("namespace") and (mapped.get("pod") or mapped.get("workload_name")))
            return mapped

    for alias in _value_aliases(value):
        target = _find_unique_alias_match(_load_kube_pod_info_targets(), alias)
        if target:
            mapped = dict(target)
            if mapped.get("namespace") and mapped.get("service") and not mapped.get("pod"):
                mapped.update(_resolve_service_mapping(mapped.get("namespace"), mapped.get("service")))
            if mapped.get("namespace") and mapped.get("pod") and not mapped.get("workload_name"):
                mapped.update(_resolve_workload_from_pod(mapped.get("namespace"), mapped.get("pod")))
            mapped["mapped_by"] = "kube-pod-info"
            mapped["investigation_supported"] = bool(mapped.get("namespace") and (mapped.get("pod") or mapped.get("workload_name")))
            return mapped
    return {}


def _select_prometheus_sources(cluster_name=None):
    pairs = _cluster_url_pairs()
    if cluster_name:
        exact = [item for item in pairs if item[0] == cluster_name]
        if exact:
            return exact
    return pairs


def _query_pod_metric(promql, source):
    cluster_name, url = source
    try:
        series = prometheus_client.query_instant(promql, url=url, timeout=10)
        return {"cluster": cluster_name, "url": url, "series": series}
    except Exception as exc:
        return {"cluster": cluster_name, "url": url, "error": str(exc), "series": []}


def _series_to_pod_map(series):
    result = {}
    for item in series or []:
        metric = item.get("metric") or {}
        pod = metric.get("pod")
        namespace = metric.get("namespace")
        if not pod or not namespace:
            continue
        key = f"{namespace}/{pod}"
        try:
            value = float((item.get("value") or [None, 0])[1])
        except (TypeError, ValueError):
            value = 0.0
        entry = result.setdefault(key, {"namespace": namespace, "pod": pod, "values": {}})
        reason = metric.get("reason")
        if reason:
            entry["values"].setdefault("reasons", {})[reason] = value
        else:
            entry["values"]["value"] = value
    return result


def _merge_pod_metric_maps(target_map, source_map, field):
    for key, entry in source_map.items():
        target = target_map.setdefault(
            key,
            {"namespace": entry["namespace"], "pod": entry["pod"]},
        )
        values = entry.get("values") or {}
        if "reasons" in values:
            target[field] = values["reasons"]
        else:
            target[field] = values.get("value")


def _build_prometheus_queries(namespace, pod_names, duration_expr):
    pod_regex = "|".join(_promql_regex_escape(name) for name in pod_names)
    pod_selector = (
        f'namespace="{_promql_escape(namespace)}",pod=~"{pod_regex}",container!="POD",container!=""'
    )
    status_selector = f'namespace="{_promql_escape(namespace)}",pod=~"{pod_regex}"'
    cpu_requests = (
        f'((kube_pod_container_resource_requests_cpu_cores{{{pod_selector}}}) '
        f'or (kube_pod_container_resource_requests{{{pod_selector},resource="cpu",unit="core"}}))'
    )
    cpu_limits = (
        f'((kube_pod_container_resource_limits_cpu_cores{{{pod_selector}}}) '
        f'or (kube_pod_container_resource_limits{{{pod_selector},resource="cpu",unit="core"}}))'
    )
    mem_requests = (
        f'((kube_pod_container_resource_requests_memory_bytes{{{pod_selector}}}) '
        f'or (kube_pod_container_resource_requests{{{pod_selector},resource="memory",unit="byte"}}))'
    )
    mem_limits = (
        f'((kube_pod_container_resource_limits_memory_bytes{{{pod_selector}}}) '
        f'or (kube_pod_container_resource_limits{{{pod_selector},resource="memory",unit="byte"}}))'
    )
    return {
        "cpu_cores": f"sum by (namespace,pod) (rate(container_cpu_usage_seconds_total{{{pod_selector}}}[5m]))",
        "memory_working_set_bytes": f"sum by (namespace,pod) (container_memory_working_set_bytes{{{pod_selector}}})",
        "cpu_request_cores": f"sum by (namespace,pod) {cpu_requests}",
        "cpu_limit_cores": f"sum by (namespace,pod) {cpu_limits}",
        "memory_request_bytes": f"sum by (namespace,pod) {mem_requests}",
        "memory_limit_bytes": f"sum by (namespace,pod) {mem_limits}",
        "restart_total": f"sum by (namespace,pod) (kube_pod_container_status_restarts_total{{{pod_selector}}})",
        "restart_increase": f"sum by (namespace,pod) (increase(kube_pod_container_status_restarts_total{{{pod_selector}}}[{duration_expr}]))",
        "ready_containers": f"sum by (namespace,pod) (kube_pod_container_status_ready{{{pod_selector}}})",
        "total_containers": f"count by (namespace,pod) (kube_pod_container_status_ready{{{pod_selector}}})",
        "waiting_reasons": f"max by (namespace,pod,reason) (kube_pod_container_status_waiting_reason{{{status_selector}}})",
        "last_terminated_reasons": f"max by (namespace,pod,reason) (kube_pod_container_status_last_terminated_reason{{{status_selector}}})",
    }


def _collect_prometheus_context(request, pod_names):
    sources = _select_prometheus_sources(request.get("cluster"))
    if not sources:
        return {"sources": [], "pods": [], "errors": ["No Prometheus URLs configured."]}

    duration_expr = _duration_expr(request["end_ts"] - request["start_ts"])
    queries = _build_prometheus_queries(request["namespace"], pod_names, duration_expr)
    merged = {}
    errors = []
    source_payloads = []

    for source in sources:
        cluster_name, url = source
        source_entry = {"cluster": cluster_name, "url": url, "queries": {}, "pods": {}}
        for name, promql in queries.items():
            response = _query_pod_metric(promql, source)
            if response.get("error"):
                errors.append(f"{cluster_name}:{name}:{response['error']}")
                source_entry["queries"][name] = {"error": response["error"]}
                continue
            pod_map = _series_to_pod_map(response.get("series"))
            source_entry["queries"][name] = {"series_count": len(response.get("series") or [])}
            for key, entry in pod_map.items():
                pod_entry = source_entry["pods"].setdefault(
                    key,
                    {"namespace": entry["namespace"], "pod": entry["pod"]},
                )
                values = entry.get("values") or {}
                if "reasons" in values:
                    pod_entry[name] = values["reasons"]
                else:
                    pod_entry[name] = values.get("value")
            _merge_pod_metric_maps(merged, pod_map, name)
        source_payloads.append(source_entry)

    pod_items = list(merged.values())
    pod_items.sort(key=lambda item: item.get("restart_total") or 0, reverse=True)
    limit = max(1, int(getattr(config, "AI_INVESTIGATION_MAX_METRICS_SERIES", 32) or 32))
    return {
        "sources": source_payloads,
        "pods": pod_items[:limit],
        "errors": errors,
    }


def _find_condition(pod_summary, condition_type):
    for item in pod_summary.get("conditions") or []:
        if item.get("type") == condition_type:
            return item
    return {}


def _flatten_prom_pod_context(prometheus_context, namespace, pod_name):
    key = f"{namespace}/{pod_name}"
    for item in prometheus_context.get("pods") or []:
        if f"{item.get('namespace')}/{item.get('pod')}" == key:
            return item
    return {}


def _build_analysis_input(request, target, logs, events, prometheus_context):
    lines = []
    lines.append("## Investigation Request")
    lines.append(f"- Cluster: {request.get('cluster') or '-'}")
    lines.append(f"- Namespace: {request.get('namespace')}")
    lines.append(f"- Pod: {request.get('pod') or '-'}")
    lines.append(f"- Workload: {request.get('workload_name') or '-'}")
    lines.append(f"- Window: {_iso_utc(request['start_ts'])} -> {_iso_utc(request['end_ts'])}")
    if request.get("question"):
        lines.append(f"- User question: {request['question']}")
    lines.append("")
    lines.append("## Pod Snapshots")
    for pod in target.get("pods") or []:
        ready = _find_condition(pod, "Ready")
        lines.append(
            f"- Pod {pod['name']} phase={pod.get('phase')} node={pod.get('node')} ready={ready.get('status')} reason={ready.get('reason') or '-'}"
        )
        for container in pod.get("containers") or []:
            state = container.get("state") or {}
            last_terminated = container.get("last_terminated") or {}
            lines.append(
                f"  - Container {container['name']} restart_count={container.get('restart_count')} state={state.get('kind')} "
                f"reason={state.get('reason') or '-'} last_terminated={last_terminated.get('reason') or '-'} "
                f"exit_code={last_terminated.get('exit_code')}"
            )
            resources = container.get("resources") or {}
            if resources:
                lines.append(
                    f"    requests={resources.get('requests') or {}} limits={resources.get('limits') or {}}"
                )
    lines.append("")
    lines.append("## Prometheus Context")
    for item in prometheus_context.get("pods") or []:
        lines.append(
            f"- {item.get('namespace')}/{item.get('pod')} cpu={item.get('cpu_cores')} mem={item.get('memory_working_set_bytes')} "
            f"restart_total={item.get('restart_total')} restart_increase={item.get('restart_increase')} "
            f"mem_limit={item.get('memory_limit_bytes')} waiting={item.get('waiting_reasons') or {}} "
            f"last_terminated={item.get('last_terminated_reasons') or {}}"
        )
    if prometheus_context.get("errors"):
        lines.append("- Metric errors:")
        for item in prometheus_context["errors"][:10]:
            lines.append(f"  - {item}")
    lines.append("")
    lines.append("## Kubernetes Events")
    for idx, item in enumerate(events[: request["max_events"]], start=1):
        lines.append(
            f"- E{idx}: {item.get('timestamp')} {item.get('type')}/{item.get('reason')} "
            f"object={item.get('object_kind')}/{item.get('object_name')} message={item.get('message')}"
        )
    lines.append("")
    lines.append("## Logs")
    for idx, item in enumerate(logs[: request["max_logs"]], start=1):
        lines.append(
            f"- L{idx}: {item.get('timestamp')} pod={item.get('pod')} mode={item.get('mode')} message={item.get('message')}"
        )
    return "\n".join(lines).strip()


def _maybe_float(value):
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def _bytes_to_mib(value):
    value = _maybe_float(value)
    if value is None:
        return None
    return round(value / (1024 * 1024), 2)


def _heuristic_analysis(request, target, logs, events, prometheus_context):
    pod = (target.get("pods") or [{}])[0]
    containers = pod.get("containers") or []
    log_messages = "\n".join(item.get("message") or "" for item in logs[:40]).lower()
    event_messages = "\n".join(item.get("message") or "" for item in events[:20]).lower()
    root_causes = []
    actions = []
    impact = []
    timeline = []
    need_human_check = []
    summary = "Insufficient evidence to determine a high-confidence root cause."

    for item in events[:5]:
        timeline.append(
            f"{item.get('timestamp') or '-'} {item.get('type')}/{item.get('reason')} {item.get('message')}"
        )

    for container in containers:
        state = container.get("state") or {}
        last_terminated = container.get("last_terminated") or {}
        restart_count = int(container.get("restart_count") or 0)
        prom_pod = _flatten_prom_pod_context(prometheus_context, pod.get("namespace"), pod.get("name"))
        mem_limit_mib = _bytes_to_mib(prom_pod.get("memory_limit_bytes"))
        mem_usage_mib = _bytes_to_mib(prom_pod.get("memory_working_set_bytes"))

        if (
            (
                state.get("reason") == "CrashLoopBackOff"
                or restart_count >= 3
                or "back-off" in event_messages
            )
            and (
                last_terminated.get("reason") == "OOMKilled"
                or "oomkilled" in event_messages
                or "oom" in log_messages
            )
        ):
            summary = (
                f"Pod {pod.get('namespace')}/{pod.get('name')} is repeatedly crashing; the strongest signal points "
                "to memory pressure or an undersized memory limit causing OOM during startup."
            )
            evidence = [
                f"Container {container.get('name')} waiting reason is CrashLoopBackOff.",
                f"Last terminated reason is {last_terminated.get('reason')} exit_code={last_terminated.get('exit_code')}.",
            ]
            if mem_limit_mib is not None:
                evidence.append(f"Prometheus reports memory limit about {mem_limit_mib} MiB.")
            if mem_usage_mib is not None:
                evidence.append(f"Prometheus reports current memory working set about {mem_usage_mib} MiB.")
            if "limited ram" in log_messages:
                evidence.append("Startup logs explicitly mention limited RAM and reduced cache sizes.")
            root_causes.append(
                {
                    "title": "Memory limit too low or workload memory spike causes OOM during startup",
                    "confidence": 0.92,
                    "evidence": evidence,
                    "counter_evidence": [],
                }
            )
            actions.extend(
                [
                    "Increase the pod memory limit and request for the ClickHouse container before restarting again.",
                    "Check whether the workload size or background merge behavior exceeds the current memory limit.",
                    "Inspect previous container logs around the OOM window to confirm whether startup or merge tasks trigger the spike.",
                ]
            )
            impact.append("The target pod is not Ready and keeps restarting, so the service behind this pod is unstable or unavailable.")
            if "address already in use" in log_messages:
                need_human_check.append("Confirm whether repeated rapid restarts leave sockets in use or multiple processes start inside the container.")
            break

        if state.get("reason") == "CrashLoopBackOff":
            summary = (
                f"Pod {pod.get('namespace')}/{pod.get('name')} is in CrashLoopBackOff; logs and events indicate a repeated startup failure."
            )
            root_causes.append(
                {
                    "title": "Application startup repeatedly fails and triggers CrashLoopBackOff",
                    "confidence": 0.74,
                    "evidence": [
                        f"Container {container.get('name')} waiting reason is CrashLoopBackOff.",
                        f"Restart count is {container.get('restart_count')}.",
                        "Kubernetes events contain BackOff / Unhealthy signals." if event_messages else "Repeated restarts are visible in pod status.",
                    ],
                    "counter_evidence": [],
                }
            )
            actions.extend(
                [
                    "Review previous container logs to isolate the first failing startup message.",
                    "Check readiness and liveness probe thresholds against actual startup time.",
                ]
            )
            impact.append("The target pod is unstable and not Ready.")
            break

    if not root_causes and ("back-off" in event_messages or "unhealthy" in event_messages):
        summary = (
            f"Pod {pod.get('namespace')}/{pod.get('name')} is unhealthy; Kubernetes events show repeated restart or readiness failures."
        )
        root_causes.append(
            {
                "title": "Repeated restart or readiness failure detected from Kubernetes events",
                "confidence": 0.61,
                "evidence": [item.get("message") for item in events[:3]],
                "counter_evidence": [],
            }
        )
        actions.append("Inspect pod status and the latest logs to identify the underlying application failure.")

    if not actions:
        actions.append("Collect a wider log window or enable AI mode to improve diagnosis detail.")

    return {
        "source": "heuristic",
        "summary": summary,
        "root_cause": root_causes,
        "impact": impact,
        "timeline": timeline[:10],
        "actions": actions[:6],
        "need_human_check": need_human_check[:5],
    }


def _parse_json_text(text):
    raw = (text or "").strip()
    if raw.startswith("```"):
        raw = raw.strip("`")
        if raw.lower().startswith("json"):
            raw = raw[4:].strip()
    start = raw.find("{")
    end = raw.rfind("}")
    if start >= 0 and end >= start:
        raw = raw[start : end + 1]
    return json.loads(raw)


def _generate_ai_analysis(request, target, logs, events, prometheus_context, heuristic):
    prompt = f"""
You are an SRE investigator. Use only the evidence below.
Return strict JSON with keys:
summary, root_cause, impact, timeline, actions, need_human_check.

Rules:
1. Do not invent facts.
2. Each root_cause item must contain title, confidence, evidence, counter_evidence.
3. If uncertain, say hypothesis explicitly in the title or evidence.
4. Keep the output concise and actionable.

Evidence:
{_build_analysis_input(request, target, logs, events, prometheus_context)}
""".strip()

    try:
        data = request_json(
            "POST",
            getattr(config, "OLLAMA_URL", ""),
            payload={
                "model": getattr(config, "OLLAMA_MODEL", ""),
                "prompt": prompt,
                "stream": False,
            },
            timeout=getattr(config, "AI_INVESTIGATION_TIMEOUT", 180),
            retries=getattr(config, "REQUEST_RETRIES", 3),
            backoff_seconds=getattr(config, "REQUEST_BACKOFF_SECONDS", 0.5),
        )
        text = (data.get("response") or "").strip()
        parsed = _parse_json_text(text)
        parsed["source"] = "ollama"
        parsed["raw_response"] = text
        return parsed, prompt, None
    except Exception as exc:
        fallback = dict(heuristic)
        fallback["source"] = "heuristic_fallback"
        fallback["llm_error"] = str(exc)
        return fallback, prompt, str(exc)


def _resolve_write_index(pattern):
    pattern = (pattern or "").strip()
    if not pattern:
        return ""
    if "*" not in pattern:
        return pattern
    return pattern.replace("*", datetime.datetime.now().strftime("%Y.%m.%d"))


def _iter_investigation_paths():
    directory = Path(INVESTIGATION_DIR)
    if not directory.exists():
        return []
    items = []
    for path in directory.glob("*.json"):
        if path.name == "latest.json":
            continue
        items.append(path)
    return sorted(items, key=lambda item: item.stat().st_mtime, reverse=True)


def _safe_read_json(path):
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except (OSError, json.JSONDecodeError):
        return None


def _summary_from_payload(payload):
    request = payload.get("request") or {}
    analysis = payload.get("analysis") or {}
    evidence = payload.get("evidence") or {}
    return {
        "investigation_id": payload.get("investigation_id"),
        "generated_at": payload.get("generated_at"),
        "namespace": request.get("namespace"),
        "pod": request.get("pod"),
        "workload_name": request.get("workload_name"),
        "summary": analysis.get("summary"),
        "logs_source": evidence.get("logs_source"),
        "events_source": evidence.get("events_source"),
        "logs_count": len(evidence.get("logs") or []),
        "events_count": len(evidence.get("events") or []),
        "use_ai": bool((payload.get("meta") or {}).get("use_ai")),
        "dashboards_links": (payload.get("links") or {}).get("dashboards") or {},
    }


def _load_event_artifact(path):
    payload = _safe_read_json(path)
    if not isinstance(payload, dict):
        return []
    source = payload.get("source") or {}
    if source and source.get("fingerprint") and source.get("fingerprint") != source_context.source_fingerprint():
        return []
    items = payload.get("events")
    return items if isinstance(items, list) else []


def _parse_target_text(text):
    raw = str(text or "").strip()
    if not raw:
        return {"namespace": "", "pod": "", "workload_name": "", "cluster": "", "instance": ""}
    parts = raw.split("/")
    if len(parts) == 3:
        return {
            "cluster": parts[0],
            "namespace": parts[1],
            "pod": parts[2],
            "workload_name": "",
            "instance": raw,
        }
    if len(parts) == 2:
        return {
            "cluster": "",
            "namespace": parts[0],
            "pod": parts[1],
            "workload_name": "",
            "instance": raw,
        }
    return {
        "cluster": "",
        "namespace": "",
        "pod": "",
        "workload_name": "",
        "instance": raw,
    }


def _risk_score(level):
    mapping = {"critical": 120, "high": 90, "medium": 55, "low": 30, "info": 10}
    return mapping.get(str(level or "").lower(), 40)


def _lifecycle_score(value):
    mapping = {"new": 28, "ongoing": 18, "resolved": -18}
    return mapping.get(str(value or "").lower(), 0)


def _target_key(namespace, pod, workload_name, instance):
    if namespace and pod:
        return "|".join([namespace, pod, "", ""])
    if namespace and workload_name:
        return "|".join([namespace, "", workload_name, ""])
    return "|".join([namespace or "", pod or "", workload_name or "", instance or ""])


def _investigation_target_entry(item):
    key = _target_key(item.get("namespace"), item.get("pod"), item.get("workload_name"), "")
    score = 45 + min(item.get("logs_count") or 0, 50) // 5 + min(item.get("events_count") or 0, 20)
    return {
        "key": key,
        "namespace": item.get("namespace"),
        "pod": item.get("pod"),
        "workload_name": item.get("workload_name"),
        "instance": "",
        "count": 1,
        "latest_generated_at": item.get("generated_at"),
        "latest_investigation_id": item.get("investigation_id"),
        "latest_summary": item.get("summary"),
        "recommendation_score": score,
        "risk_level": "",
        "dominant_risk": "",
        "lifecycle": "",
        "runbook_title": "",
        "runbook_available": False,
        "escalation_reasons": [],
        "signals": [],
        "investigation_count": 1,
        "restart_total": None,
        "restart_increase": None,
        "waiting_reason": "",
        "last_terminated_reason": "",
        "memory_request_bytes": None,
        "memory_limit_bytes": None,
        "cpu_request_cores": None,
        "cpu_limit_cores": None,
        "source_types": ["investigation"],
        "investigation_supported": bool(item.get("namespace") and (item.get("pod") or item.get("workload_name"))),
        "dashboards_links": item.get("dashboards_links") or {},
        "mapped_by": "",
        "service": "",
        "node": "",
        "recommended_reason": "近期有调查记录，可快速复用已有证据与结论。",
    }


def _event_target_entry(event):
    parsed = _parse_target_text(
        event.get("instance")
        or event.get("target")
        or event.get("object")
        or event.get("object_name")
    )
    namespace = event.get("namespace") or parsed["namespace"]
    pod = event.get("pod") or parsed["pod"]
    workload_name = event.get("workload_name") or parsed["workload_name"]
    instance = event.get("instance") or parsed["instance"]
    instance_mapping = _resolve_instance_mapping(instance)
    namespace = namespace or instance_mapping.get("namespace") or ""
    pod = pod or instance_mapping.get("pod") or ""
    workload_name = workload_name or instance_mapping.get("workload_name") or ""
    merge_instance = ""
    if not namespace and not pod and not workload_name:
        merge_instance = instance
    level = event.get("final_risk_level") or event.get("risk_level") or "medium"
    lifecycle = event.get("lifecycle") or ""
    pod_state = event.get("pod_state") or {}
    restart_total = _to_float(pod_state.get("restarts_total"))
    restart_increase = _to_float(pod_state.get("restarts"))
    waiting_reason = str(pod_state.get("waiting_reason") or "").strip()
    last_terminated_reason = str(pod_state.get("terminated_reason") or "").strip()
    score = _risk_score(level) + _lifecycle_score(lifecycle)
    if event.get("runbook"):
        score += 8
    if event.get("escalation_reasons"):
        score += min(len(event.get("escalation_reasons") or []), 3) * 5
    score += min(len(event.get("signals") or []), 4) * 3
    if restart_total is not None:
        if restart_total >= 1000:
            score += 15
        elif restart_total >= 100:
            score += 10
        elif restart_total >= 10:
            score += 6
        elif restart_total > 0:
            score += 3
    if restart_increase is not None:
        if restart_increase >= 20:
            score += 15
        elif restart_increase >= 5:
            score += 8
        elif restart_increase > 0:
            score += 4
    if waiting_reason == "CrashLoopBackOff":
        score += 20
    if last_terminated_reason == "OOMKilled":
        score += 25
    if "not_ready" in (event.get("signals") or []):
        score += 10
    title = ""
    if isinstance(event.get("runbook"), dict):
        title = event["runbook"].get("title") or ""
    return {
        "key": _target_key(namespace, pod, workload_name, merge_instance),
        "namespace": namespace,
        "pod": pod,
        "workload_name": workload_name,
        "instance": instance,
        "count": 1,
        "latest_generated_at": event.get("last_seen") or event.get("generated_at"),
        "latest_investigation_id": "",
        "latest_summary": "",
        "recommendation_score": score,
        "risk_level": level,
        "dominant_risk": event.get("dominant_risk") or "",
        "lifecycle": lifecycle,
        "runbook_title": title,
        "runbook_available": bool(title),
        "escalation_reasons": list(event.get("escalation_reasons") or []),
        "signals": list(event.get("signals") or []),
        "investigation_count": 0,
        "restart_total": restart_total,
        "restart_increase": restart_increase,
        "waiting_reason": waiting_reason,
        "last_terminated_reason": last_terminated_reason,
        "memory_request_bytes": _to_float(pod_state.get("mem_request_bytes")),
        "memory_limit_bytes": _to_float(pod_state.get("mem_limit_bytes")),
        "cpu_request_cores": _to_float(pod_state.get("cpu_request_cores")),
        "cpu_limit_cores": _to_float(pod_state.get("cpu_limit_cores")),
        "source_types": ["event"],
        "investigation_supported": bool(namespace and (pod or workload_name)),
        "dashboards_links": dict((event.get("links") or {}).get("dashboards") or {}),
        "recommended_reason": (
            f"事件管线检测到 {level} 风险，主风险为 {event.get('dominant_risk') or 'unknown'}。"
        ),
        "event_key": event.get("event_key") or "",
        "mapped_by": instance_mapping.get("mapped_by") or "",
        "service": event.get("service") or instance_mapping.get("service") or "",
        "node": event.get("node") or instance_mapping.get("node") or "",
    }


def _merge_target_entry(existing, incoming):
    existing["count"] += incoming.get("count") or 0
    existing["recommendation_score"] += incoming.get("recommendation_score") or 0
    if incoming.get("latest_generated_at") and (incoming["latest_generated_at"] > (existing.get("latest_generated_at") or "")):
        existing["latest_generated_at"] = incoming["latest_generated_at"]
        if incoming.get("latest_summary"):
            existing["latest_summary"] = incoming.get("latest_summary")
        if incoming.get("latest_investigation_id"):
            existing["latest_investigation_id"] = incoming["latest_investigation_id"]
    existing["risk_level"] = incoming.get("risk_level") or existing.get("risk_level")
    existing["dominant_risk"] = incoming.get("dominant_risk") or existing.get("dominant_risk")
    existing["lifecycle"] = incoming.get("lifecycle") or existing.get("lifecycle")
    existing["runbook_title"] = incoming.get("runbook_title") or existing.get("runbook_title")
    existing["runbook_available"] = bool(existing.get("runbook_available") or incoming.get("runbook_available"))
    existing["recommended_reason"] = incoming.get("recommended_reason") or existing.get("recommended_reason")
    existing["investigation_supported"] = bool(existing.get("investigation_supported") or incoming.get("investigation_supported"))
    existing["dashboards_links"].update(incoming.get("dashboards_links") or {})
    existing["mapped_by"] = incoming.get("mapped_by") or existing.get("mapped_by")
    existing["service"] = incoming.get("service") or existing.get("service")
    existing["node"] = incoming.get("node") or existing.get("node")
    existing["investigation_count"] = (existing.get("investigation_count") or 0) + (incoming.get("investigation_count") or 0)
    for key in ("restart_total", "restart_increase", "memory_request_bytes", "memory_limit_bytes", "cpu_request_cores", "cpu_limit_cores"):
        left = existing.get(key)
        right = incoming.get(key)
        if left is None:
            existing[key] = right
        elif right is not None:
            existing[key] = max(left, right)
    existing["waiting_reason"] = incoming.get("waiting_reason") or existing.get("waiting_reason")
    existing["last_terminated_reason"] = incoming.get("last_terminated_reason") or existing.get("last_terminated_reason")
    for source_type in incoming.get("source_types") or []:
        if source_type not in existing["source_types"]:
            existing["source_types"].append(source_type)
    existing["signals"] = list(dict.fromkeys((existing.get("signals") or []) + (incoming.get("signals") or [])))
    existing["escalation_reasons"] = list(dict.fromkeys((existing.get("escalation_reasons") or []) + (incoming.get("escalation_reasons") or [])))
    return existing


def _merge_event_entries(existing, incoming):
    existing["count"] = max(existing.get("count") or 0, incoming.get("count") or 0)
    existing["recommendation_score"] = max(existing.get("recommendation_score") or 0, incoming.get("recommendation_score") or 0)
    if incoming.get("latest_generated_at") and (incoming["latest_generated_at"] > (existing.get("latest_generated_at") or "")):
        existing["latest_generated_at"] = incoming["latest_generated_at"]
    existing["risk_level"] = incoming.get("risk_level") or existing.get("risk_level")
    existing["dominant_risk"] = incoming.get("dominant_risk") or existing.get("dominant_risk")
    existing["lifecycle"] = incoming.get("lifecycle") or existing.get("lifecycle")
    existing["runbook_title"] = incoming.get("runbook_title") or existing.get("runbook_title")
    existing["runbook_available"] = bool(existing.get("runbook_available") or incoming.get("runbook_available"))
    existing["recommended_reason"] = incoming.get("recommended_reason") or existing.get("recommended_reason")
    existing["investigation_supported"] = bool(existing.get("investigation_supported") or incoming.get("investigation_supported"))
    existing["dashboards_links"].update(incoming.get("dashboards_links") or {})
    existing["mapped_by"] = incoming.get("mapped_by") or existing.get("mapped_by")
    existing["service"] = incoming.get("service") or existing.get("service")
    existing["node"] = incoming.get("node") or existing.get("node")
    existing["investigation_count"] = max(existing.get("investigation_count") or 0, incoming.get("investigation_count") or 0)
    for key in ("restart_total", "restart_increase", "memory_request_bytes", "memory_limit_bytes", "cpu_request_cores", "cpu_limit_cores"):
        left = existing.get(key)
        right = incoming.get(key)
        if left is None:
            existing[key] = right
        elif right is not None:
            existing[key] = max(left, right)
    existing["waiting_reason"] = incoming.get("waiting_reason") or existing.get("waiting_reason")
    existing["last_terminated_reason"] = incoming.get("last_terminated_reason") or existing.get("last_terminated_reason")
    existing["signals"] = list(dict.fromkeys((existing.get("signals") or []) + (incoming.get("signals") or [])))
    existing["escalation_reasons"] = list(dict.fromkeys((existing.get("escalation_reasons") or []) + (incoming.get("escalation_reasons") or [])))
    return existing


def _finalize_target_score(item):
    score = int(item.get("recommendation_score") or 0)
    score += min(item.get("investigation_count") or 0, 5) * 6
    if item.get("runbook_available"):
        score += 8
    if item.get("last_terminated_reason") == "OOMKilled":
        score += 18
    if item.get("waiting_reason") == "CrashLoopBackOff":
        score += 15
    if "not_ready" in (item.get("signals") or []):
        score += 8
    restart_increase = _to_float(item.get("restart_increase"))
    if restart_increase is not None:
        if restart_increase >= 20:
            score += 12
        elif restart_increase >= 5:
            score += 6
    restart_total = _to_float(item.get("restart_total"))
    if restart_total is not None:
        if restart_total >= 1000:
            score += 10
        elif restart_total >= 100:
            score += 6
    item["recommendation_score"] = score
    return item


def list_recent_investigations(limit=20):
    hot_items = investigation_storage.list_recent_investigations(limit=limit)
    if hot_items:
        return hot_items[:limit]
    items = []
    for path in _iter_investigation_paths():
        payload = _safe_read_json(path)
        if not payload:
            continue
        items.append(_summary_from_payload(payload))
        if len(items) >= limit:
            break
    return items


def list_investigation_targets(limit=20):
    grouped = {}
    for item in list_recent_investigations(limit=200):
        entry = _investigation_target_entry(item)
        key = entry["key"]
        grouped[key] = _merge_target_entry(grouped[key], entry) if key in grouped else entry

    event_candidates = incident_store.list_incidents(limit=200, include_links=False).get("items") or []
    event_grouped = {}
    for event in event_candidates:
        entry = _event_target_entry(event)
        merge_key = event.get("event_key") or entry["key"]
        event_grouped[merge_key] = _merge_event_entries(event_grouped[merge_key], entry) if merge_key in event_grouped else entry
    for entry in event_grouped.values():
        key = entry["key"]
        grouped[key] = _merge_target_entry(grouped[key], entry) if key in grouped else entry

    values = list(grouped.values())
    values = [
        item
        for item in values
        if not (
            not item.get("investigation_supported")
            and str(item.get("lifecycle") or "").lower() == "resolved"
            and item.get("source_types") == ["event"]
        )
    ]
    values = [_finalize_target_score(item) for item in values]
    values.sort(
        key=lambda item: (
            item.get("recommendation_score") or 0,
            item.get("investigation_count") or 0,
            item.get("count") or 0,
            item.get("latest_generated_at") or "",
        ),
        reverse=True,
    )
    return values[:limit]


def _investigation_index_template(index_pattern):
    return {
        "index_patterns": [index_pattern],
        "template": {
            "settings": {
                "number_of_shards": 1,
                "number_of_replicas": 0,
            },
            "mappings": {
                "dynamic": True,
                "properties": {
                    "investigation_id": {"type": "keyword"},
                    "generated_at": {
                        "type": "date",
                        "format": "yyyy-MM-dd HH:mm:ss||strict_date_optional_time||epoch_millis",
                    },
                    "request": {"type": "object", "dynamic": True},
                    "target": {"type": "object", "dynamic": True},
                    "evidence": {
                        "properties": {
                            "logs_source": {"type": "keyword"},
                            "events_source": {"type": "keyword"},
                            "logs": {
                                "type": "nested",
                                "properties": {
                                    "timestamp": {"type": "keyword"},
                                    "pod": {"type": "keyword"},
                                    "mode": {"type": "keyword"},
                                    "severity": {"type": "keyword"},
                                    "logger": {"type": "keyword"},
                                    "source": {"type": "keyword"},
                                    "message": {
                                        "type": "text",
                                        "fields": {
                                            "keyword": {
                                                "type": "keyword",
                                                "ignore_above": 1024,
                                            }
                                        },
                                    },
                                },
                            },
                            "events": {
                                "type": "nested",
                                "properties": {
                                    "timestamp": {"type": "keyword"},
                                    "type": {"type": "keyword"},
                                    "reason": {"type": "keyword"},
                                    "namespace": {"type": "keyword"},
                                    "name": {"type": "keyword"},
                                    "object_kind": {"type": "keyword"},
                                    "object_name": {"type": "keyword"},
                                    "source_component": {"type": "keyword"},
                                    "source_host": {"type": "keyword"},
                                    "message": {
                                        "type": "text",
                                        "fields": {
                                            "keyword": {
                                                "type": "keyword",
                                                "ignore_above": 1024,
                                            }
                                        },
                                    },
                                },
                            },
                            "prometheus": {"type": "object", "dynamic": True},
                        }
                    },
                    "analysis_input": {"type": "text"},
                    "analysis_prompt": {"type": "text"},
                    "analysis": {"type": "object", "dynamic": True},
                    "meta": {"type": "object", "dynamic": True},
                    "storage": {"type": "object", "dynamic": True},
                }
            },
        },
    }


def _ensure_investigation_index_template(index_pattern):
    if not opensearch_client.is_configured() or not index_pattern:
        return None
    return opensearch_client.put_index_template(
        INVESTIGATION_TEMPLATE_NAME,
        _investigation_index_template(index_pattern),
    )


def _sanitize_opensearch_key(key):
    text = str(key or "")
    if not text or text == "." or text.startswith(".") or text.endswith(".") or ".." in text:
        text = re.sub(r"[.]+", "_", text).strip("_")
    return text or "_"


def _sanitize_for_opensearch(value):
    if isinstance(value, dict):
        return {
            _sanitize_opensearch_key(key): _sanitize_for_opensearch(item)
            for key, item in value.items()
        }
    if isinstance(value, list):
        return [_sanitize_for_opensearch(item) for item in value]
    return value


def _save_local_investigation(investigation_id, payload):
    os.makedirs(INVESTIGATION_DIR, exist_ok=True)
    path = os.path.join(INVESTIGATION_DIR, f"{investigation_id}.json")
    with open(path, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)
    with open(os.path.join(INVESTIGATION_DIR, "latest.json"), "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)
    return path


def load_investigation(investigation_id):
    if investigation_id == "latest":
        path = os.path.join(INVESTIGATION_DIR, "latest.json")
    else:
        path = os.path.join(INVESTIGATION_DIR, f"{investigation_id}.json")
    payload = None
    if os.path.exists(path):
        with open(path, "r", encoding="utf-8") as f:
            payload = json.load(f)
    else:
        pointer = investigation_storage.load_investigation_pointer(investigation_id)
        if pointer and pointer.get("local_path") and os.path.exists(pointer.get("local_path")):
            with open(pointer.get("local_path"), "r", encoding="utf-8") as f:
                payload = json.load(f)
        elif pointer:
            payload = investigation_storage.load_investigation_archive(pointer)
        if payload is None:
            return None
    payload.setdefault("links", {})
    payload["links"].setdefault("dashboards", _build_dashboards_links(payload))
    return payload


def _dashboards_search_ids(request):
    namespace = request.get("namespace") or ""
    pod = request.get("pod") or ""
    if namespace == "langfuse" and pod == "langfuse-clickhouse-shard0-0":
        return "search-langfuse-clickhouse-logs", "search-langfuse-clickhouse-events"
    return None, None


def _dashboard_links_from_saved_search(request):
    if not dashboards_client.is_configured():
        return {}
    logs_search_id, events_search_id = _dashboards_search_ids(request)
    links = {
        "overview_dashboard": dashboards_client.dashboards_view_url("dashboard-auto-inspection-overview"),
        "rca_dashboard": dashboards_client.dashboards_view_url("dashboard-langfuse-clickhouse-rca"),
    }
    if logs_search_id:
        links["logs"] = dashboards_client.discover_saved_search_url(logs_search_id)
    if events_search_id:
        links["events"] = dashboards_client.discover_saved_search_url(events_search_id)
    return links


def _create_dashboards_searches(investigation_id, request):
    if not dashboards_client.is_configured():
        return {}

    logs_query_parts = [f'namespace:"{request["namespace"]}"']
    events_query_parts = [f'namespace:"{request["namespace"]}"']
    if request.get("pod"):
        logs_query_parts.append(f'pod:"{request["pod"]}"')
        events_query_parts.append(f'pod:"{request["pod"]}" or object_name:"{request["pod"]}"')
    elif request.get("workload_name"):
        value = request["workload_name"]
        logs_query_parts.append(f'pod:*{value}* or service:"{value}"')
        events_query_parts.append(f'object_name:*{value}*')

    logs_id = f"investigation-{investigation_id}-logs"
    events_id = f"investigation-{investigation_id}-events"

    dashboards_client.upsert_saved_search(
        logs_id,
        dashboards_client.build_saved_search_payload(
            title=f"Investigation Logs - {request['namespace']}/{request.get('pod') or request.get('workload_name')}",
            description="Generated from auto_inspection RCA workflow",
            index_id="logs-k8s-data-view",
            query=" and ".join(logs_query_parts),
            columns=["namespace", "pod", "container", "severity", "message"],
            sort=[["@timestamp", "desc"]],
        ),
    )
    dashboards_client.upsert_saved_search(
        events_id,
        dashboards_client.build_saved_search_payload(
            title=f"Investigation Events - {request['namespace']}/{request.get('pod') or request.get('workload_name')}",
            description="Generated from auto_inspection RCA workflow",
            index_id="events-k8s-data-view",
            query=" and ".join(events_query_parts),
            columns=["namespace", "object_kind", "object_name", "reason", "message"],
            sort=[["@timestamp", "desc"]],
        ),
    )
    return {
        "logs": dashboards_client.discover_saved_search_url(logs_id),
        "events": dashboards_client.discover_saved_search_url(events_id),
    }


def _build_dashboards_links(payload):
    request = payload.get("request") or {}
    links = _dashboard_links_from_saved_search(request)
    generated = _create_dashboards_searches(payload.get("investigation_id"), request)
    links.update(generated)
    return links


def normalize_request(payload):
    data = payload if isinstance(payload, dict) else {}
    namespace = str(data.get("namespace", "") or "").strip()
    pod = str(data.get("pod", "") or "").strip()
    workload_name = str(data.get("workload_name", "") or "").strip()
    if not namespace:
        raise ValueError("namespace is required.")
    if not pod and not workload_name:
        raise ValueError("pod or workload_name is required.")

    end_ts = _parse_time(data.get("end_ts") or data.get("end"))
    start_ts = _parse_time(data.get("start_ts") or data.get("start"))
    if end_ts is None:
        end_ts = _now_ts()
    if start_ts is None:
        range_hours = _parse_time(data.get("range_hours"))
        range_days = _parse_time(data.get("range_days"))
        if range_hours is not None:
            seconds = max(3600, int(range_hours) * 3600)
        elif range_days is not None:
            seconds = max(3600, int(range_days) * 86400)
        else:
            seconds = DEFAULT_RANGE_HOURS * 3600
        start_ts = end_ts - seconds
    if start_ts >= end_ts:
        raise ValueError("start_ts must be earlier than end_ts.")

    return {
        "cluster": str(data.get("cluster", "") or "").strip(),
        "namespace": namespace,
        "pod": pod,
        "workload_name": workload_name,
        "question": str(data.get("question", "") or "").strip(),
        "query": str(data.get("query", "") or "").strip(),
        "start_ts": start_ts,
        "end_ts": end_ts,
        "max_logs": _bounded_int(
            data.get("max_logs"),
            getattr(config, "AI_INVESTIGATION_MAX_LOGS", 200),
            20,
            500,
        ),
        "max_events": _bounded_int(
            data.get("max_events"),
            getattr(config, "AI_INVESTIGATION_MAX_EVENTS", 100),
            10,
            200,
        ),
        "use_ai": _truthy(data.get("use_ai", getattr(config, "AI_INVESTIGATION_ENABLED", False))),
    }


def run_investigation(payload):
    request = normalize_request(payload)
    investigation_id = datetime.datetime.now().strftime("%Y%m%d%H%M%S") + "-" + uuid.uuid4().hex[:8]
    target_pods = _resolve_target_pods(
        request["namespace"],
        pod=request.get("pod"),
        workload_name=request.get("workload_name"),
    )
    if not target_pods:
        raise RuntimeError("No matching pods found for investigation target.")

    pod_names = [item["name"] for item in target_pods]
    logs = _search_logs_via_opensearch(request, pod_names)
    log_source = "opensearch" if logs else "kubectl"
    if not logs:
        logs = _collect_kubernetes_logs(request, pod_names)

    events = _search_events_via_opensearch(request, pod_names)
    event_source = "opensearch" if events else "kubectl"
    if not events:
        events = _collect_kubernetes_events(request, pod_names)

    prometheus_context = _collect_prometheus_context(request, pod_names)
    target = {
        "cluster": request.get("cluster"),
        "namespace": request["namespace"],
        "pod_names": pod_names,
        "workload_name": request.get("workload_name"),
        "pods": target_pods,
    }

    heuristic = _heuristic_analysis(request, target, logs, events, prometheus_context)
    analysis_input = _build_analysis_input(request, target, logs, events, prometheus_context)
    llm_error = None
    prompt_text = ""
    if request["use_ai"] and getattr(config, "OLLAMA_URL", ""):
        analysis, prompt_text, llm_error = _generate_ai_analysis(
            request,
            target,
            logs,
            events,
            prometheus_context,
            heuristic,
        )
    else:
        analysis = heuristic

    result = {
        "investigation_id": investigation_id,
        "generated_at": _now_str(),
        "request": request,
        "target": target,
        "evidence": {
            "logs_source": log_source,
            "events_source": event_source,
            "logs": logs,
            "events": events,
            "prometheus": prometheus_context,
        },
        "analysis_input": analysis_input,
        "analysis_prompt": prompt_text,
        "analysis": analysis,
        "meta": {
            "status": "ok",
            "use_ai": bool(request["use_ai"]),
            "llm_error": llm_error,
            "opensearch_configured": opensearch_client.is_configured(),
            "k8s_direct_enabled": _kubectl_available(),
        },
    }
    result["links"] = {
        "dashboards": _build_dashboards_links(result),
    }

    local_path = _save_local_investigation(investigation_id, result)
    storage = {
        "local_path": local_path,
        "opensearch": {"indexed": False},
        "hot_store": {"stored": False},
        "cold_store": {"stored": False},
    }
    index_pattern = getattr(config, "OPENSEARCH_INDEX_INVESTIGATIONS", "") or ""
    if opensearch_client.is_configured() and index_pattern:
        write_index = _resolve_write_index(index_pattern)
        try:
            _ensure_investigation_index_template(index_pattern)
            response = opensearch_client.index_document(
                write_index,
                _sanitize_for_opensearch(result),
                document_id=investigation_id,
                refresh=True,
            )
            storage["opensearch"] = {
                "indexed": True,
                "index": write_index,
                "result": response.get("result"),
                "id": response.get("_id"),
            }
        except Exception as exc:
            storage["opensearch"] = {
                "indexed": False,
                "index": write_index,
                "error": str(exc),
            }

    try:
        cold = investigation_storage.archive_investigation_payload(investigation_id, result)
        storage["cold_store"] = cold
    except Exception as exc:
        storage["cold_store"] = {"stored": False, "error": str(exc)}

    try:
        hot = investigation_storage.save_investigation_metadata(
            result,
            local_path=local_path,
            archive=storage.get("cold_store"),
        )
        storage["hot_store"] = hot
    except Exception as exc:
        storage["hot_store"] = {"stored": False, "error": str(exc)}

    result["storage"] = storage
    _save_local_investigation(investigation_id, result)
    return result
