#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import datetime
import time

from auto_inspection import investigation_service


WORKLOAD_KIND_TO_RESOURCE = {
    "deployment": "deployments",
    "deployments": "deployments",
    "statefulset": "statefulsets",
    "statefulsets": "statefulsets",
    "daemonset": "daemonsets",
    "daemonsets": "daemonsets",
    "replicaset": "replicasets",
    "replicasets": "replicasets",
}


def _now_ts():
    return int(time.time())


def _parse_time(value):
    text = str(value or "").strip()
    if not text:
        return None
    if text.isdigit():
        ts = int(text)
        return ts // 1000 if ts > 10**12 else ts
    normalized = text.replace("Z", "+00:00")
    try:
        return int(datetime.datetime.fromisoformat(normalized).timestamp())
    except ValueError:
        return None


def _window(params):
    data = params if isinstance(params, dict) else {}
    end_ts = _parse_time(data.get("end")) or _now_ts()
    try:
        range_hours = int(data.get("range_hours") or 24)
    except (TypeError, ValueError):
        range_hours = 24
    return end_ts - max(1, range_hours) * 3600, end_ts


def _api(path, params=None):
    return investigation_service._k8s_api_request("GET", path, params=params or {}, expect_json=True)


def _metadata(item):
    return (item or {}).get("metadata") or {}


def _spec(item):
    return (item or {}).get("spec") or {}


def _status(item):
    return (item or {}).get("status") or {}


def _annotations(item):
    return _metadata(item).get("annotations") or {}


def _labels(item):
    return _metadata(item).get("labels") or {}


def _containers(item):
    spec = _spec(item)
    template = spec.get("template") or {}
    pod_spec = template.get("spec") or {}
    return pod_spec.get("containers") or []


def _container_images(item):
    return [
        {
            "name": container.get("name"),
            "image": container.get("image"),
            "image_pull_policy": container.get("imagePullPolicy"),
        }
        for container in _containers(item)
    ]


def _helm_metadata(item):
    labels = _labels(item)
    annotations = _annotations(item)
    return {
        "release_name": annotations.get("meta.helm.sh/release-name") or labels.get("app.kubernetes.io/instance"),
        "release_namespace": annotations.get("meta.helm.sh/release-namespace"),
        "chart": labels.get("helm.sh/chart"),
        "managed_by": labels.get("app.kubernetes.io/managed-by"),
        "app_name": labels.get("app.kubernetes.io/name"),
        "app_version": labels.get("app.kubernetes.io/version"),
    }


def _revision_metadata(item):
    annotations = _annotations(item)
    return {
        "deployment_revision": annotations.get("deployment.kubernetes.io/revision"),
        "controller_revision_hash": _labels(item).get("controller-revision-hash"),
        "restarted_at": annotations.get("kubectl.kubernetes.io/restartedAt"),
        "change_cause": annotations.get("kubernetes.io/change-cause"),
    }


def _workload_summary(item, kind):
    metadata = _metadata(item)
    status = _status(item)
    spec = _spec(item)
    return {
        "kind": kind,
        "name": metadata.get("name"),
        "namespace": metadata.get("namespace"),
        "uid": metadata.get("uid"),
        "created_at": metadata.get("creationTimestamp"),
        "resource_version": metadata.get("resourceVersion"),
        "generation": metadata.get("generation"),
        "observed_generation": status.get("observedGeneration"),
        "replicas": spec.get("replicas"),
        "ready_replicas": status.get("readyReplicas"),
        "updated_replicas": status.get("updatedReplicas"),
        "available_replicas": status.get("availableReplicas"),
        "images": _container_images(item),
        "selector": spec.get("selector"),
        "labels": _labels(item),
        "annotations": {
            key: value
            for key, value in _annotations(item).items()
            if key.startswith("meta.helm.sh/")
            or key in {
                "deployment.kubernetes.io/revision",
                "kubectl.kubernetes.io/restartedAt",
                "kubernetes.io/change-cause",
            }
        },
        "helm": _helm_metadata(item),
        "revision": _revision_metadata(item),
    }


def _configmap_summary(item):
    metadata = _metadata(item)
    data = item.get("data") or {}
    binary_data = item.get("binaryData") or {}
    return {
        "kind": "ConfigMap",
        "name": metadata.get("name"),
        "namespace": metadata.get("namespace"),
        "uid": metadata.get("uid"),
        "created_at": metadata.get("creationTimestamp"),
        "resource_version": metadata.get("resourceVersion"),
        "labels": _labels(item),
        "annotations": _annotations(item),
        "data_keys": sorted(data.keys()),
        "binary_data_keys": sorted(binary_data.keys()),
    }


def _pod_owner(namespace, pod_name):
    if not pod_name:
        return None
    pod = _api(f"/api/v1/namespaces/{namespace}/pods/{pod_name}")
    owner = ((_metadata(pod).get("ownerReferences") or [{}])[0]) or {}
    return {
        "kind": owner.get("kind"),
        "name": owner.get("name"),
        "pod": pod,
    }


def _get_apps_resource(namespace, resource, name):
    return _api(f"/apis/apps/v1/namespaces/{namespace}/{resource}/{name}")


def _list_apps_resource(namespace, resource):
    return _api(f"/apis/apps/v1/namespaces/{namespace}/{resource}").get("items") or []


def _resolve_workload(namespace, workload_name="", workload_kind="", pod=""):
    errors = []
    owner = None
    if pod and not workload_name:
        try:
            owner = _pod_owner(namespace, pod)
            workload_name = owner.get("name") or ""
            workload_kind = owner.get("kind") or ""
        except Exception as exc:
            errors.append({"source": "pod_owner", "message": str(exc)})

    kind_key = str(workload_kind or "").strip().lower()
    resource = WORKLOAD_KIND_TO_RESOURCE.get(kind_key)
    workload = None
    resolved_kind = workload_kind

    if workload_name and resource:
        workload = _get_apps_resource(namespace, resource, workload_name)
        resolved_kind = workload_kind
    elif workload_name:
        for kind, candidate_resource in (
            ("Deployment", "deployments"),
            ("StatefulSet", "statefulsets"),
            ("DaemonSet", "daemonsets"),
            ("ReplicaSet", "replicasets"),
        ):
            try:
                workload = _get_apps_resource(namespace, candidate_resource, workload_name)
                resolved_kind = kind
                resource = candidate_resource
                break
            except Exception:
                continue

    if resolved_kind == "ReplicaSet" and workload is not None:
        rs_owner = ((_metadata(workload).get("ownerReferences") or [{}])[0]) or {}
        if rs_owner.get("kind") == "Deployment" and rs_owner.get("name"):
            try:
                workload = _get_apps_resource(namespace, "deployments", rs_owner["name"])
                resolved_kind = "Deployment"
                resource = "deployments"
            except Exception as exc:
                errors.append({"source": "replicaset_owner", "message": str(exc)})

    return workload, resolved_kind or "", resource or "", owner, errors


def release_for_workload(params):
    data = params if isinstance(params, dict) else {}
    namespace = str(data.get("namespace") or "").strip()
    if not namespace:
        raise ValueError("namespace is required")
    workload, kind, resource, owner, errors = _resolve_workload(
        namespace,
        workload_name=str(data.get("workload_name") or "").strip(),
        workload_kind=str(data.get("workload_kind") or "").strip(),
        pod=str(data.get("pod") or "").strip(),
    )
    if workload is None:
        return {
            "mode": "read_only_release_for_workload",
            "safety": {"server_commands": "not_allowed", "kubernetes_mutations": "not_allowed"},
            "request": data,
            "workload": None,
            "owner": owner,
            "errors": errors + [{"source": "workload", "message": "workload not found"}],
        }
    return {
        "mode": "read_only_release_for_workload",
        "safety": {"server_commands": "not_allowed", "kubernetes_mutations": "not_allowed"},
        "request": data,
        "workload": _workload_summary(workload, kind),
        "resource": resource,
        "owner": owner,
        "errors": errors,
    }


def recent_changes(params):
    data = params if isinstance(params, dict) else {}
    namespace = str(data.get("namespace") or "").strip()
    if not namespace:
        raise ValueError("namespace is required")
    start_ts, end_ts = _window(data)
    try:
        limit = max(1, min(int(data.get("limit") or 50), 200))
    except (TypeError, ValueError):
        limit = 50
    workload_filter = str(data.get("workload_name") or data.get("service") or "").strip()
    include_configmaps = str(data.get("include_configmaps", "true")).lower() not in {"0", "false", "no"}

    items = []
    errors = []
    for kind, resource in (
        ("Deployment", "deployments"),
        ("StatefulSet", "statefulsets"),
        ("DaemonSet", "daemonsets"),
        ("ReplicaSet", "replicasets"),
    ):
        try:
            for item in _list_apps_resource(namespace, resource):
                summary = _workload_summary(item, kind)
                hay = " ".join(
                    [
                        summary.get("name") or "",
                        summary.get("helm", {}).get("release_name") or "",
                        " ".join([image.get("image") or "" for image in summary.get("images") or []]),
                    ]
                )
                if workload_filter and workload_filter not in hay:
                    continue
                ts = _parse_time(summary.get("revision", {}).get("restarted_at")) or _parse_time(summary.get("created_at")) or 0
                summary["change_time"] = summary.get("revision", {}).get("restarted_at") or summary.get("created_at")
                summary["change_ts"] = ts
                if start_ts <= ts <= end_ts or workload_filter:
                    items.append(summary)
        except Exception as exc:
            errors.append({"source": resource, "message": str(exc)})

    if include_configmaps:
        try:
            cms = _api(f"/api/v1/namespaces/{namespace}/configmaps").get("items") or []
            for item in cms:
                summary = _configmap_summary(item)
                hay = " ".join([summary.get("name") or "", " ".join(summary.get("data_keys") or [])])
                if workload_filter and workload_filter not in hay:
                    continue
                ts = _parse_time(summary.get("created_at")) or 0
                summary["change_time"] = summary.get("created_at")
                summary["change_ts"] = ts
                if start_ts <= ts <= end_ts or workload_filter:
                    items.append(summary)
        except Exception as exc:
            errors.append({"source": "configmaps", "message": str(exc)})

    items.sort(key=lambda item: item.get("change_ts") or 0, reverse=True)
    return {
        "mode": "read_only_recent_changes",
        "safety": {"server_commands": "not_allowed", "kubernetes_mutations": "not_allowed"},
        "window": {"start_ts": start_ts, "end_ts": end_ts},
        "request": data,
        "items": items[:limit],
        "errors": errors,
    }


def correlate_change_with_incident(params):
    data = params if isinstance(params, dict) else {}
    release = release_for_workload(data)
    change_params = dict(data)
    workload = release.get("workload") if isinstance(release, dict) else None
    if isinstance(workload, dict) and not change_params.get("workload_name") and not change_params.get("service"):
        change_params["workload_name"] = workload.get("name")
    changes = recent_changes(change_params)
    return {
        "mode": "read_only_change_correlation",
        "safety": {"server_commands": "not_allowed", "kubernetes_mutations": "not_allowed"},
        "request": data,
        "release": release,
        "recent_changes": changes,
        "assessment": {
            "has_workload": bool(release.get("workload")),
            "change_count": len(changes.get("items") or []),
            "notes": [
                "Kubernetes metadata does not expose a full update history by itself.",
                "Helm history is inferred from workload labels/annotations; Helm Secret contents are not read.",
                "Use Argo CD or CI/CD read-only APIs later for exact commit and deployment history.",
            ],
        },
    }
