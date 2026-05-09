#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import datetime
import time
from collections import Counter, defaultdict
from urllib.parse import quote

from auto_inspection.investigation_service import (
    _k8s_api_request,
    _k8s_in_cluster_available,
    _run_kubectl,
)


MAX_LIMIT = 500
DEFAULT_LIMIT = 100
ALL_NAMESPACES = {"", "*", "all", "_all", "all-namespaces"}
WORKLOAD_KINDS = {
    "deployment": {
        "kind": "Deployment",
        "plural": "deployments",
        "kubectl": "deployments",
    },
    "statefulset": {
        "kind": "StatefulSet",
        "plural": "statefulsets",
        "kubectl": "statefulsets",
    },
    "daemonset": {
        "kind": "DaemonSet",
        "plural": "daemonsets",
        "kubectl": "daemonsets",
    },
    "replicaset": {
        "kind": "ReplicaSet",
        "plural": "replicasets",
        "kubectl": "replicasets",
    },
}
WORKLOAD_ALIASES = {
    "deploy": "deployment",
    "deployments": "deployment",
    "deployment": "deployment",
    "sts": "statefulset",
    "statefulsets": "statefulset",
    "statefulset": "statefulset",
    "ds": "daemonset",
    "daemonsets": "daemonset",
    "daemonset": "daemonset",
    "rs": "replicaset",
    "replicasets": "replicaset",
    "replicaset": "replicaset",
}


def _param(params, key, default=""):
    value = (params or {}).get(key, default)
    if isinstance(value, (list, tuple)):
        value = value[0] if value else default
    if value is None:
        return default
    return str(value).strip()


def _int_param(params, key, default=DEFAULT_LIMIT, minimum=1, maximum=MAX_LIMIT):
    value = _param(params, key, "")
    if value == "":
        return default
    try:
        number = int(value)
    except (TypeError, ValueError):
        return default
    return max(minimum, min(maximum, number))


def _namespace_scope(params):
    namespace = _param(params, "namespace", "")
    if namespace.strip().lower() in ALL_NAMESPACES:
        return ""
    return namespace


def _source_name():
    return "kubernetes_api" if _k8s_in_cluster_available() else "kubectl"


def _meta(started_at, *, total=0, item_count=0, errors=None):
    errors = list(errors or [])
    return {
        "status": "partial" if errors else "ok",
        "source": _source_name(),
        "query_seconds": round(time.time() - started_at, 3),
        "total": int(total or 0),
        "item_count": int(item_count or 0),
        "errors": errors,
    }


def _quote_namespace(namespace):
    return quote(str(namespace or "").strip(), safe="")


def _load_json(api_path, kubectl_args):
    if _k8s_in_cluster_available():
        return _k8s_api_request("GET", api_path, expect_json=True)
    return _run_kubectl(kubectl_args, expect_json=True)


def _load_items(api_path, kubectl_args):
    payload = _load_json(api_path, kubectl_args)
    return payload.get("items") or []


def _load_namespaces():
    return _load_items("/api/v1/namespaces", ["get", "namespaces", "-o", "json"])


def _load_nodes():
    return _load_items("/api/v1/nodes", ["get", "nodes", "-o", "json"])


def _load_pods(namespace=""):
    if namespace:
        quoted = _quote_namespace(namespace)
        return _load_items(
            f"/api/v1/namespaces/{quoted}/pods",
            ["get", "pods", "-n", namespace, "-o", "json"],
        )
    return _load_items("/api/v1/pods", ["get", "pods", "-A", "-o", "json"])


def _load_services(namespace=""):
    if namespace:
        quoted = _quote_namespace(namespace)
        return _load_items(
            f"/api/v1/namespaces/{quoted}/services",
            ["get", "services", "-n", namespace, "-o", "json"],
        )
    return _load_items("/api/v1/services", ["get", "services", "-A", "-o", "json"])


def _load_workload_kind(kind_key, namespace=""):
    spec = WORKLOAD_KINDS[kind_key]
    plural = spec["plural"]
    kubectl_name = spec["kubectl"]
    if namespace:
        quoted = _quote_namespace(namespace)
        return _load_items(
            f"/apis/apps/v1/namespaces/{quoted}/{plural}",
            ["get", kubectl_name, "-n", namespace, "-o", "json"],
        )
    return _load_items(
        f"/apis/apps/v1/{plural}",
        ["get", kubectl_name, "-A", "-o", "json"],
    )


def _parse_timestamp(value):
    value = str(value or "").strip()
    if not value:
        return None
    try:
        return datetime.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


def _age_seconds(value):
    dt = _parse_timestamp(value)
    if dt is None:
        return None
    now = datetime.datetime.now(datetime.timezone.utc)
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=datetime.timezone.utc)
    return max(0, int((now - dt).total_seconds()))


def _owner(metadata):
    refs = metadata.get("ownerReferences") or []
    if not refs:
        return {"kind": "", "name": ""}
    ref = refs[0] or {}
    return {"kind": ref.get("kind") or "", "name": ref.get("name") or ""}


def _images_from_pod_spec(spec):
    images = []
    for container in (spec.get("initContainers") or []) + (spec.get("containers") or []):
        image = container.get("image")
        if image and image not in images:
            images.append(image)
    return images


def _state_kind_and_reason(container_status):
    state = container_status.get("state") or {}
    for kind in ("waiting", "terminated", "running"):
        payload = state.get(kind)
        if not payload:
            continue
        return {
            "kind": kind,
            "reason": payload.get("reason") or "",
            "message": payload.get("message") or "",
            "exit_code": payload.get("exitCode"),
            "started_at": payload.get("startedAt"),
            "finished_at": payload.get("finishedAt"),
        }
    return {"kind": "", "reason": "", "message": ""}


def _unique(values):
    result = []
    seen = set()
    for value in values:
        value = str(value or "").strip()
        if not value or value in seen:
            continue
        seen.add(value)
        result.append(value)
    return result


def _pod_abnormal_reasons(summary, raw_statuses):
    reasons = []
    phase = summary.get("phase") or ""
    if summary.get("terminating"):
        reasons.append("Terminating")
    if phase not in {"Running", "Succeeded"}:
        reasons.append(phase or "UnknownPhase")
    if phase == "Running" and summary.get("ready_containers", 0) < summary.get("total_containers", 0):
        reasons.append("NotReady")
    for item in raw_statuses:
        state = _state_kind_and_reason(item)
        if state.get("kind") == "waiting":
            reason = state.get("reason") or "Waiting"
            if reason not in {"ContainerCreating", "PodInitializing"}:
                reasons.append(reason)
        if state.get("kind") == "terminated":
            reason = state.get("reason") or "Terminated"
            if reason not in {"Completed"}:
                reasons.append(reason)
        last_terminated = ((item.get("lastState") or {}).get("terminated")) or {}
        reason = last_terminated.get("reason")
        if reason in {"OOMKilled", "Error"}:
            reasons.append(reason)
    for condition in summary.get("not_ready_conditions") or []:
        reason = condition.get("reason") or condition.get("type")
        if reason:
            reasons.append(reason)
    return _unique(reasons)


def _summarize_pod(pod):
    metadata = pod.get("metadata") or {}
    spec = pod.get("spec") or {}
    status = pod.get("status") or {}
    owner = _owner(metadata)
    container_statuses = status.get("containerStatuses") or []
    init_statuses = status.get("initContainerStatuses") or []
    all_statuses = init_statuses + container_statuses
    ready_containers = sum(1 for item in container_statuses if item.get("ready"))
    total_containers = len(spec.get("containers") or [])
    states = []
    for item in all_statuses:
        state = _state_kind_and_reason(item)
        state["name"] = item.get("name") or ""
        state["restart_count"] = int(item.get("restartCount") or 0)
        states.append(state)

    not_ready_conditions = []
    for condition in status.get("conditions") or []:
        if condition.get("status") == "True" and condition.get("type") not in {"Ready", "ContainersReady"}:
            continue
        if condition.get("status") != "True":
            not_ready_conditions.append(
                {
                    "type": condition.get("type"),
                    "reason": condition.get("reason") or "",
                    "message": condition.get("message") or "",
                }
            )

    created_at = metadata.get("creationTimestamp")
    summary = {
        "kind": "Pod",
        "namespace": metadata.get("namespace") or "",
        "name": metadata.get("name") or "",
        "phase": status.get("phase") or "",
        "ready": f"{ready_containers}/{total_containers}",
        "ready_containers": ready_containers,
        "total_containers": total_containers,
        "restarts_total": sum(int(item.get("restartCount") or 0) for item in all_statuses),
        "node": spec.get("nodeName") or "",
        "pod_ip": status.get("podIP") or "",
        "host_ip": status.get("hostIP") or "",
        "owner_kind": owner["kind"],
        "owner_name": owner["name"],
        "qos_class": status.get("qosClass") or "",
        "created_at": created_at,
        "age_seconds": _age_seconds(created_at),
        "terminating": bool(metadata.get("deletionTimestamp")),
        "images": _images_from_pod_spec(spec),
        "labels": metadata.get("labels") or {},
        "container_states": states,
        "waiting_reasons": _unique(
            state.get("reason") for state in states if state.get("kind") == "waiting"
        ),
        "terminated_reasons": _unique(
            state.get("reason") for state in states if state.get("kind") == "terminated"
        ),
        "not_ready_conditions": not_ready_conditions,
    }
    reasons = _pod_abnormal_reasons(summary, all_statuses)
    summary["abnormal"] = bool(reasons)
    summary["abnormal_reasons"] = reasons
    return summary


def _workload_replica_summary(kind, spec, status):
    desired = spec.get("replicas")
    if kind == "DaemonSet":
        desired = status.get("desiredNumberScheduled") or 0
        return {
            "desired": int(desired or 0),
            "ready": int(status.get("numberReady") or 0),
            "available": int(status.get("numberAvailable") or 0),
            "updated": int(status.get("updatedNumberScheduled") or 0),
            "current": int(status.get("currentNumberScheduled") or 0),
            "unavailable": int(status.get("numberUnavailable") or 0),
        }
    return {
        "desired": int(desired or 0),
        "ready": int(status.get("readyReplicas") or 0),
        "available": int(status.get("availableReplicas") or 0),
        "updated": int(status.get("updatedReplicas") or 0),
        "current": int(status.get("currentReplicas") or status.get("replicas") or 0),
        "unavailable": int(status.get("unavailableReplicas") or 0),
    }


def _summarize_workload(item, kind):
    metadata = item.get("metadata") or {}
    spec = item.get("spec") or {}
    status = item.get("status") or {}
    template = spec.get("template") or {}
    pod_spec = template.get("spec") or {}
    replicas = _workload_replica_summary(kind, spec, status)
    created_at = metadata.get("creationTimestamp")
    owner = _owner(metadata)
    desired = replicas.get("desired", 0)
    ready = replicas.get("ready", 0)
    abnormal = desired > 0 and ready < desired
    return {
        "kind": kind,
        "namespace": metadata.get("namespace") or "",
        "name": metadata.get("name") or "",
        "desired": desired,
        "ready": ready,
        "available": replicas.get("available", 0),
        "updated": replicas.get("updated", 0),
        "current": replicas.get("current", 0),
        "unavailable": replicas.get("unavailable", 0),
        "abnormal": abnormal,
        "abnormal_reasons": ["ReadyReplicasBelowDesired"] if abnormal else [],
        "owner_kind": owner["kind"],
        "owner_name": owner["name"],
        "images": _images_from_pod_spec(pod_spec),
        "selector": spec.get("selector") or {},
        "labels": metadata.get("labels") or {},
        "created_at": created_at,
        "age_seconds": _age_seconds(created_at),
    }


def _summarize_service(service):
    metadata = service.get("metadata") or {}
    spec = service.get("spec") or {}
    created_at = metadata.get("creationTimestamp")
    ports = []
    for port in spec.get("ports") or []:
        ports.append(
            {
                "name": port.get("name") or "",
                "protocol": port.get("protocol") or "TCP",
                "port": port.get("port"),
                "target_port": port.get("targetPort"),
                "node_port": port.get("nodePort"),
            }
        )
    return {
        "kind": "Service",
        "namespace": metadata.get("namespace") or "",
        "name": metadata.get("name") or "",
        "type": spec.get("type") or "ClusterIP",
        "cluster_ip": spec.get("clusterIP") or "",
        "external_ips": spec.get("externalIPs") or [],
        "ports": ports,
        "selector": spec.get("selector") or {},
        "labels": metadata.get("labels") or {},
        "created_at": created_at,
        "age_seconds": _age_seconds(created_at),
    }


def _summarize_namespace(namespace):
    metadata = namespace.get("metadata") or {}
    status = namespace.get("status") or {}
    created_at = metadata.get("creationTimestamp")
    return {
        "kind": "Namespace",
        "name": metadata.get("name") or "",
        "phase": status.get("phase") or "",
        "labels": metadata.get("labels") or {},
        "created_at": created_at,
        "age_seconds": _age_seconds(created_at),
    }


def _summarize_node(node):
    metadata = node.get("metadata") or {}
    status = node.get("status") or {}
    info = status.get("nodeInfo") or {}
    labels = metadata.get("labels") or {}
    addresses = {}
    for item in status.get("addresses") or []:
        address_type = item.get("type")
        if address_type:
            addresses[address_type] = item.get("address") or ""
    ready_status = "Unknown"
    ready_reason = ""
    for condition in status.get("conditions") or []:
        if condition.get("type") == "Ready":
            ready_status = condition.get("status") or "Unknown"
            ready_reason = condition.get("reason") or ""
            break
    roles = []
    for key in labels:
        prefix = "node-role.kubernetes.io/"
        if key.startswith(prefix):
            role = key[len(prefix):] or "control-plane"
            roles.append(role)
    created_at = metadata.get("creationTimestamp")
    return {
        "kind": "Node",
        "name": metadata.get("name") or "",
        "ready": ready_status == "True",
        "ready_status": ready_status,
        "ready_reason": ready_reason,
        "roles": sorted(roles),
        "kernel_version": info.get("kernelVersion") or "",
        "kubelet_version": info.get("kubeletVersion") or "",
        "os_image": info.get("osImage") or "",
        "container_runtime_version": info.get("containerRuntimeVersion") or "",
        "addresses": addresses,
        "labels": labels,
        "created_at": created_at,
        "age_seconds": _age_seconds(created_at),
    }


def _matches_text(item, query):
    query = str(query or "").strip().lower()
    if not query:
        return True
    haystack_parts = []
    for key in (
        "kind",
        "namespace",
        "name",
        "phase",
        "node",
        "owner_kind",
        "owner_name",
        "type",
        "cluster_ip",
        "ready_status",
    ):
        haystack_parts.append(str(item.get(key) or ""))
    for key in ("labels", "images", "waiting_reasons", "terminated_reasons", "abnormal_reasons"):
        haystack_parts.append(str(item.get(key) or ""))
    haystack = " ".join(haystack_parts).lower()
    return all(token in haystack for token in query.split())


def _filter_limit(items, *, params, predicate=None, sort_key=None):
    query = _param(params, "q", "")
    filtered = []
    for item in items:
        if query and not _matches_text(item, query):
            continue
        if predicate is not None and not predicate(item):
            continue
        filtered.append(item)
    if sort_key is not None:
        filtered.sort(key=sort_key)
    limit = _int_param(params, "limit", DEFAULT_LIMIT)
    return filtered, filtered[:limit]


def _pod_status_matches(item, status):
    status = str(status or "").strip().lower()
    if not status:
        return True
    phase = str(item.get("phase") or "").lower()
    reasons = " ".join(item.get("abnormal_reasons") or []).lower()
    waiting = " ".join(item.get("waiting_reasons") or []).lower()
    if status in {"abnormal", "problem", "problems", "not_ready", "not-ready"}:
        return bool(item.get("abnormal"))
    if status in {"not_running", "not-running"}:
        return phase != "running"
    if status in {"running", "succeeded", "failed", "pending", "unknown"}:
        return phase == status
    return status in reasons or status in waiting or status in phase


def list_namespaces(params=None):
    started_at = time.time()
    items = [_summarize_namespace(item) for item in _load_namespaces()]
    total_items, limited_items = _filter_limit(
        items,
        params=params or {},
        sort_key=lambda item: item.get("name") or "",
    )
    return {
        "items": limited_items,
        "summary": {
            "namespace_count": len(total_items),
            "phases": dict(Counter(item.get("phase") or "Unknown" for item in total_items)),
        },
        "meta": _meta(started_at, total=len(total_items), item_count=len(limited_items)),
    }


def list_pods(params=None):
    params = params or {}
    started_at = time.time()
    namespace = _namespace_scope(params)
    items = [_summarize_pod(item) for item in _load_pods(namespace)]
    node = _param(params, "node", "")
    owner_kind = _param(params, "owner_kind", "").lower()
    owner_name = _param(params, "owner_name", "")
    status = _param(params, "status", "")

    def predicate(item):
        if node and node not in item.get("node", ""):
            return False
        if owner_kind and owner_kind != str(item.get("owner_kind") or "").lower():
            return False
        if owner_name and owner_name not in str(item.get("owner_name") or ""):
            return False
        return _pod_status_matches(item, status)

    total_items, limited_items = _filter_limit(
        items,
        params=params,
        predicate=predicate,
        sort_key=lambda item: (
            0 if item.get("abnormal") else 1,
            -(item.get("restarts_total") or 0),
            item.get("namespace") or "",
            item.get("name") or "",
        ),
    )
    return {
        "items": limited_items,
        "summary": _pod_summary(total_items),
        "meta": _meta(started_at, total=len(total_items), item_count=len(limited_items)),
    }


def list_abnormal_pods(params=None):
    params = dict(params or {})
    params["status"] = "abnormal"
    return list_pods(params)


def list_workloads(params=None):
    params = params or {}
    started_at = time.time()
    namespace = _namespace_scope(params)
    kind = _param(params, "kind", "all").lower()
    kind_keys = []
    if kind in {"", "all", "*"}:
        kind_keys = list(WORKLOAD_KINDS.keys())
    else:
        normalized = WORKLOAD_ALIASES.get(kind)
        if normalized:
            kind_keys = [normalized]
        else:
            raise ValueError(f"Unsupported workload kind: {kind}")
    items = []
    for kind_key in kind_keys:
        kind_name = WORKLOAD_KINDS[kind_key]["kind"]
        for item in _load_workload_kind(kind_key, namespace):
            items.append(_summarize_workload(item, kind_name))

    total_items, limited_items = _filter_limit(
        items,
        params=params,
        sort_key=lambda item: (
            0 if item.get("abnormal") else 1,
            item.get("kind") or "",
            item.get("namespace") or "",
            item.get("name") or "",
        ),
    )
    return {
        "items": limited_items,
        "summary": {
            "workload_count": len(total_items),
            "abnormal_workload_count": sum(1 for item in total_items if item.get("abnormal")),
            "by_kind": dict(Counter(item.get("kind") or "Unknown" for item in total_items)),
        },
        "meta": _meta(started_at, total=len(total_items), item_count=len(limited_items)),
    }


def list_services(params=None):
    params = params or {}
    started_at = time.time()
    namespace = _namespace_scope(params)
    service_type = _param(params, "type", "").lower()
    items = [_summarize_service(item) for item in _load_services(namespace)]

    def predicate(item):
        if not service_type:
            return True
        return service_type == str(item.get("type") or "").lower()

    total_items, limited_items = _filter_limit(
        items,
        params=params,
        predicate=predicate,
        sort_key=lambda item: (
            item.get("namespace") or "",
            item.get("name") or "",
        ),
    )
    return {
        "items": limited_items,
        "summary": {
            "service_count": len(total_items),
            "by_type": dict(Counter(item.get("type") or "Unknown" for item in total_items)),
        },
        "meta": _meta(started_at, total=len(total_items), item_count=len(limited_items)),
    }


def search_resources(params=None):
    params = params or {}
    started_at = time.time()
    namespace = _namespace_scope(params)
    kind_values = _param(params, "kinds", "pods,workloads,services,namespaces")
    requested = {item.strip().lower() for item in kind_values.split(",") if item.strip()}
    if not requested or "all" in requested or "*" in requested:
        requested = {"pods", "workloads", "services", "namespaces"}

    limit = _int_param(params, "limit", DEFAULT_LIMIT)
    load_params = dict(params)
    load_params["limit"] = str(MAX_LIMIT)
    if namespace:
        load_params["namespace"] = namespace

    items = []
    errors = []
    loaders = [
        ({"namespace", "namespaces", "ns"}, list_namespaces),
        ({"pod", "pods"}, list_pods),
        ({"workload", "workloads", "deployment", "deployments", "statefulset", "daemonset", "replicaset"}, list_workloads),
        ({"service", "services", "svc"}, list_services),
    ]
    for aliases, loader in loaders:
        if not aliases.intersection(requested):
            continue
        try:
            payload = loader(load_params)
            items.extend(payload.get("items") or [])
            errors.extend((payload.get("meta") or {}).get("errors") or [])
        except Exception as exc:
            errors.append(str(exc))

    items.sort(key=lambda item: (item.get("kind") or "", item.get("namespace") or "", item.get("name") or ""))
    limited_items = items[:limit]
    return {
        "items": limited_items,
        "summary": {
            "resource_count": len(items),
            "by_kind": dict(Counter(item.get("kind") or "Unknown" for item in items)),
        },
        "meta": _meta(started_at, total=len(items), item_count=len(limited_items), errors=errors),
    }


def count_resources(params=None):
    params = params or {}
    started_at = time.time()
    namespace = _namespace_scope(params)
    errors = []

    namespaces = []
    nodes = []
    pods = []
    services = []
    workloads = []
    try:
        namespaces = [_summarize_namespace(item) for item in _load_namespaces()]
    except Exception as exc:
        errors.append(f"namespaces: {exc}")
    try:
        nodes = [_summarize_node(item) for item in _load_nodes()]
    except Exception as exc:
        errors.append(f"nodes: {exc}")
    try:
        pods = [_summarize_pod(item) for item in _load_pods(namespace)]
    except Exception as exc:
        errors.append(f"pods: {exc}")
    try:
        services = [_summarize_service(item) for item in _load_services(namespace)]
    except Exception as exc:
        errors.append(f"services: {exc}")
    try:
        for kind_key, spec in WORKLOAD_KINDS.items():
            for item in _load_workload_kind(kind_key, namespace):
                workloads.append(_summarize_workload(item, spec["kind"]))
    except Exception as exc:
        errors.append(f"workloads: {exc}")

    namespace_names = [item.get("name") for item in namespaces if item.get("name")]
    if namespace:
        namespace_names = [namespace]
    namespace_breakdown = _namespace_breakdown(namespace_names, pods, workloads, services)
    summary = {
        "namespace_count": len(namespace_names),
        "node_count": len(nodes),
        "node_ready_count": sum(1 for item in nodes if item.get("ready")),
        "pod_count": len(pods),
        "abnormal_pod_count": sum(1 for item in pods if item.get("abnormal")),
        "pods_by_phase": dict(Counter(item.get("phase") or "Unknown" for item in pods)),
        "workload_count": len(workloads),
        "abnormal_workload_count": sum(1 for item in workloads if item.get("abnormal")),
        "workloads_by_kind": dict(Counter(item.get("kind") or "Unknown" for item in workloads)),
        "service_count": len(services),
        "services_by_type": dict(Counter(item.get("type") or "Unknown" for item in services)),
        "namespace_breakdown": namespace_breakdown,
    }
    return {
        "summary": summary,
        "meta": _meta(started_at, total=1, item_count=1, errors=errors),
    }


def cluster_overview(params=None):
    params = params or {}
    started_at = time.time()
    limit = _int_param(params, "limit", 50)
    errors = []
    counts = {}
    namespaces = []
    nodes = []
    abnormal_pods = []

    try:
        count_payload = count_resources(params)
        counts = count_payload.get("summary") or {}
        errors.extend((count_payload.get("meta") or {}).get("errors") or [])
    except Exception as exc:
        errors.append(f"counts: {exc}")
    try:
        namespaces = list_namespaces({"q": _param(params, "q", ""), "limit": str(limit)}).get("items") or []
    except Exception as exc:
        errors.append(f"namespaces: {exc}")
    try:
        nodes = [_summarize_node(item) for item in _load_nodes()]
    except Exception as exc:
        errors.append(f"nodes: {exc}")
    try:
        abnormal_params = dict(params)
        abnormal_params["limit"] = str(limit)
        abnormal_pods = list_abnormal_pods(abnormal_params).get("items") or []
    except Exception as exc:
        errors.append(f"abnormal_pods: {exc}")

    nodes.sort(key=lambda item: (0 if not item.get("ready") else 1, item.get("name") or ""))
    return {
        "summary": counts,
        "nodes": nodes[:limit],
        "namespaces": namespaces[:limit],
        "top_abnormal_pods": abnormal_pods[:limit],
        "meta": _meta(started_at, total=1, item_count=1, errors=errors),
    }


def _pod_summary(items):
    return {
        "pod_count": len(items),
        "abnormal_pod_count": sum(1 for item in items if item.get("abnormal")),
        "by_phase": dict(Counter(item.get("phase") or "Unknown" for item in items)),
        "top_waiting_reasons": dict(Counter(reason for item in items for reason in item.get("waiting_reasons") or [])),
        "top_abnormal_reasons": dict(Counter(reason for item in items for reason in item.get("abnormal_reasons") or [])),
    }


def _namespace_breakdown(namespace_names, pods, workloads, services):
    data = defaultdict(
        lambda: {
            "pod_count": 0,
            "abnormal_pod_count": 0,
            "workload_count": 0,
            "abnormal_workload_count": 0,
            "service_count": 0,
        }
    )
    for namespace in namespace_names:
        data[namespace]
    for pod in pods:
        namespace = pod.get("namespace") or ""
        data[namespace]["pod_count"] += 1
        if pod.get("abnormal"):
            data[namespace]["abnormal_pod_count"] += 1
    for workload in workloads:
        namespace = workload.get("namespace") or ""
        data[namespace]["workload_count"] += 1
        if workload.get("abnormal"):
            data[namespace]["abnormal_workload_count"] += 1
    for service in services:
        namespace = service.get("namespace") or ""
        data[namespace]["service_count"] += 1
    return dict(sorted(data.items()))
