#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
Read-only Evidence Pack aggregation for Codex/MCP/CLI consumers.

The pack intentionally composes existing backend data sources instead of
executing cluster commands. Source failures are reported in-band so callers can
distinguish "no matching data" from "data source unavailable".
"""

import datetime
import time

from auto_inspection import business_correlation
from auto_inspection import config
from auto_inspection import deep_observability
from auto_inspection import event_search
from auto_inspection import incident_store
from auto_inspection import log_search
from auto_inspection import prom_resource_check
from auto_inspection import release_changes
from auto_inspection import gitlab_integration


SYMPTOM_QUERY_HINTS = {
    "oom": "OOMKilled OR out of memory OR memory limit OR exit_code=137",
    "memory": "OOMKilled OR out of memory OR memory limit OR exit_code=137",
    "crashloop": "CrashLoopBackOff OR BackOff OR exit_code OR exception OR error",
    "probe": "Readiness probe failed OR Liveness probe failed OR Unhealthy OR timeout",
    "pending": "FailedScheduling OR Pending OR insufficient OR taint OR node selector",
    "imagepull": "ImagePullBackOff OR ErrImagePull OR pull image OR registry",
    "latency": "timeout OR latency OR slow OR deadline exceeded OR connection reset",
    "error": "error OR exception OR failed OR fatal OR panic",
    "unknown": "error OR warning OR failed OR exception OR BackOff",
}


def _now_ts():
    return int(time.time())


def _iso(ts):
    return datetime.datetime.fromtimestamp(int(ts)).strftime("%Y-%m-%d %H:%M:%S")


def _safe_int(value, default, minimum=None, maximum=None):
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        parsed = default
    if minimum is not None:
        parsed = max(minimum, parsed)
    if maximum is not None:
        parsed = min(maximum, parsed)
    return parsed


def _safe_float(value):
    try:
        if value is None or value == "":
            return None
        return float(value)
    except (TypeError, ValueError):
        return None


def _clean_params(params):
    return {k: v for k, v in (params or {}).items() if v not in (None, "")}


def _symptom_query(symptom, q):
    explicit = str(q or "").strip()
    if explicit:
        return explicit
    key = str(symptom or "unknown").strip().lower()
    return SYMPTOM_QUERY_HINTS.get(key, SYMPTOM_QUERY_HINTS["unknown"])


def _source_status(payload, item_key="items"):
    if isinstance(payload, dict) and payload.get("errors"):
        return "partial"
    count = 0
    if isinstance(payload, dict):
        value = payload.get(item_key)
        if isinstance(value, list):
            count = len(value)
        elif isinstance(value, dict):
            count = len(value)
        meta = payload.get("meta") or {}
        if not count and isinstance(meta, dict):
            for key in ("item_count", "hotspot_count", "total"):
                if meta.get(key):
                    count = int(meta.get(key) or 0)
                    break
    return "ok" if count else "empty"


def _call_source(name, errors, data_sources, fn, *, item_key="items"):
    started = time.time()
    try:
        payload = fn()
        data_sources[name] = {
            "status": _source_status(payload, item_key=item_key),
            "query_seconds": round(time.time() - started, 3),
        }
        if isinstance(payload, dict):
            value = payload.get(item_key)
            meta = payload.get("meta") or {}
            if isinstance(value, list):
                data_sources[name]["item_count"] = len(value)
            if isinstance(meta, dict):
                for key in ("item_count", "hotspot_count", "total"):
                    if meta.get(key) is not None:
                        data_sources[name][key] = meta.get(key)
            if payload.get("total") is not None:
                data_sources[name]["total"] = payload.get("total")
        return payload
    except Exception as exc:
        data_sources[name] = {
            "status": "error",
            "query_seconds": round(time.time() - started, 3),
            "message": str(exc),
        }
        errors.append({"source": name, "message": str(exc)})
        return None


def _match_text(item):
    parts = []
    for key in ("cluster", "group", "instance", "pod", "namespace", "node_name"):
        parts.append(str((item or {}).get(key) or ""))
    return " ".join(parts).lower()


def _filter_pod_resources(resources_payload, *, namespace="", pod="", workload_name=""):
    items = (resources_payload or {}).get("items") or []
    namespace_text = str(namespace or "").strip().lower()
    pod_text = str(pod or "").strip().lower()
    workload_text = str(workload_name or "").strip().lower()
    filtered = []
    for item in items:
        if item.get("metric") != "pod":
            continue
        hay = _match_text(item)
        if namespace_text and namespace_text not in hay:
            continue
        if pod_text and pod_text not in hay:
            continue
        if not pod_text and workload_text and workload_text not in hay:
            continue
        filtered.append(item)
    return filtered


def _trend_signal(row, metric, rules):
    current = _safe_float(row.get(f"{metric}_usage_ratio") or row.get(metric))
    recent = _safe_float(row.get(f"{metric}_short_recent_avg"))
    previous = _safe_float(row.get(f"{metric}_short_prev_avg"))
    if recent is None or previous is None:
        return None
    baseline_floor = _safe_float(rules.get("baseline_floor")) or 0.05
    baseline = max(previous, baseline_floor)
    ratio = recent / baseline if baseline else None
    delta = recent - previous
    min_current = _safe_float(rules.get("min_current")) or 0.30
    watch_ratio = _safe_float(rules.get("watch_ratio")) or 1.5
    alert_ratio = _safe_float(rules.get("alert_ratio")) or 2.0
    watch_delta = _safe_float(rules.get("watch_delta")) or 0.10
    alert_delta = _safe_float(rules.get("alert_delta")) or 0.20
    severity = "info"
    if current is not None and current >= min_current:
        if ratio is not None and ratio >= alert_ratio and delta >= alert_delta:
            severity = "alert"
        elif ratio is not None and ratio >= watch_ratio and delta >= watch_delta:
            severity = "watch"
    return {
        "type": f"{metric}_short_trend",
        "severity": severity,
        "current": current,
        "recent_avg": recent,
        "baseline_avg": previous,
        "ratio": round(ratio, 3) if ratio is not None else None,
        "delta": round(delta, 4),
        "message": f"{metric.upper()} short-window usage changed from {previous:.3f} to {recent:.3f}.",
    }


def resource_signals(resources_payload, pod_rows):
    rules = (resources_payload or {}).get("pod_trend_rules") or {}
    signals = []
    for row in pod_rows:
        pod_name = row.get("instance")
        for metric in ("cpu", "mem"):
            signal = _trend_signal(row, metric, rules)
            if signal:
                signal["pod"] = pod_name
                signals.append(signal)
        if row.get("oom") or row.get("oom_events_total"):
            signals.append({"type": "oom", "severity": "alert", "pod": pod_name, "message": "Pod has OOM evidence in resource metrics."})
        if str(row.get("terminated_reason") or "").lower() == "oomkilled":
            signals.append({"type": "terminated_reason", "severity": "alert", "pod": pod_name, "message": "Last terminated reason is OOMKilled."})
        if _safe_float(row.get("restarts")) and _safe_float(row.get("restarts")) > 0:
            signals.append({"type": "restarts", "severity": "watch", "pod": pod_name, "message": "Pod restart counter increased in the configured window."})
        if _safe_float(row.get("cpu_throttle_ratio")) and _safe_float(row.get("cpu_throttle_ratio")) >= 0.2:
            signals.append({"type": "cpu_throttle", "severity": "watch", "pod": pod_name, "message": "CPU throttling ratio is elevated."})
        if _safe_float(row.get("fs_usage_ratio")) and _safe_float(row.get("fs_usage_ratio")) >= 0.8:
            signals.append({"type": "fs_usage", "severity": "watch", "pod": pod_name, "message": "Pod filesystem usage is elevated."})
    order = {"alert": 0, "watch": 1, "info": 2}
    return sorted(signals, key=lambda item: order.get(item.get("severity"), 9))


def _compact_items(payload, limit):
    items = (payload or {}).get("items") or []
    return items[:limit]


def _timeline(logs, events, changes, incidents, limit=20):
    rows = []
    for item in _compact_items(events, limit):
        rows.append({"source": "event", "time": item.get("timestamp") or item.get("@timestamp"), "summary": item.get("message") or item.get("reason")})
    for item in _compact_items(logs, limit):
        rows.append({"source": "log", "time": item.get("timestamp") or item.get("@timestamp"), "summary": item.get("message") or item.get("log")})
    for item in _compact_items(changes, limit):
        rows.append({"source": "release", "time": item.get("change_time") or item.get("created_at"), "summary": item.get("name") or item.get("kind")})
    for item in _compact_items(incidents, limit):
        rows.append({"source": "incident", "time": item.get("timestamp") or item.get("created_at"), "summary": item.get("title") or item.get("message") or item.get("reason")})
    rows = [row for row in rows if row.get("summary")]
    rows.sort(key=lambda row: str(row.get("time") or ""), reverse=True)
    return rows[:limit]


def _deep_observability_params(*, namespace, pod, workload_filter, service, start_ts, end_ts, range_hours, size):
    service_name = service or ""
    return _clean_params(
        {
            "namespace": namespace,
            "pod": pod,
            "service": service_name,
            "service_name": service_name,
            "range_hours": range_hours,
            "start_ts": start_ts,
            "end_ts": end_ts,
            "size": min(size, 50),
            "limit": min(size, 50),
        }
    )


def _deep_observability_signals(service_red, runtime_events, profile_hotspots):
    signals = []
    red_count = ((service_red or {}).get("meta") or {}).get("item_count") or 0
    if red_count:
        signals.append(
            {
                "type": "service_red_metrics",
                "severity": "info",
                "message": f"Beyla/OTel service RED metrics returned {red_count} series.",
            }
        )
    runtime_count = ((runtime_events or {}).get("meta") or {}).get("item_count") or 0
    if runtime_count:
        signals.append(
            {
                "type": "runtime_events",
                "severity": "watch",
                "message": f"Falco runtime events returned {runtime_count} records in the window.",
            }
        )
    hotspot_count = ((profile_hotspots or {}).get("meta") or {}).get("hotspot_count") or 0
    if hotspot_count:
        signals.append(
            {
                "type": "profile_hotspots",
                "severity": "info",
                "message": f"Pyroscope profile hotspots returned {hotspot_count} hot nodes.",
            }
        )
    return signals


def build_context_pack(target_type, params):
    target_type = str(target_type or "").strip().lower()
    if target_type not in {"pod", "workload", "service", "incident", "namespace"}:
        raise ValueError("target_type must be pod, workload, service, incident, or namespace")

    data = params if isinstance(params, dict) else {}
    namespace = str(data.get("namespace") or "").strip()
    pod = str(data.get("pod") or "").strip()
    workload_name = str(data.get("workload_name") or "").strip()
    workload_kind = str(data.get("workload_kind") or "").strip()
    service = str(data.get("service") or "").strip()
    app_name = str(data.get("app_name") or data.get("application") or "").strip()
    incident_id = str(data.get("incident_id") or data.get("id") or "").strip()
    symptom = str(data.get("symptom") or "unknown").strip().lower()
    range_hours = _safe_int(data.get("range_hours"), 6, 1, 168)
    size = _safe_int(data.get("size"), 30, 1, 200)

    if target_type != "incident" and not namespace:
        raise ValueError("namespace is required")
    if target_type == "pod" and not (pod or workload_name):
        raise ValueError("pod or workload_name is required for pod context")
    if target_type == "workload" and not (workload_name or service):
        raise ValueError("workload_name or service is required for workload context")
    if target_type == "service" and not service:
        raise ValueError("service is required for service context")
    if target_type == "incident" and not (incident_id or data.get("q") or namespace or pod):
        raise ValueError("incident_id, q, namespace, or pod is required for incident context")

    end_ts = _now_ts()
    start_ts = end_ts - range_hours * 3600
    query = incident_id or _symptom_query(symptom, data.get("q", ""))
    workload_filter = workload_name or service
    base_params = _clean_params(
        {
            "namespace": namespace,
            "pod": pod if target_type in {"pod", "incident"} else "",
            "workload_name": workload_filter if target_type in {"pod", "workload", "service"} else "",
            "service": service or workload_name,
            "q": query,
            "start_ts": start_ts,
            "end_ts": end_ts,
            "range_hours": range_hours,
            "size": size,
        }
    )
    release_params = _clean_params(
        {
            "namespace": namespace,
            "pod": pod if target_type in {"pod", "incident"} else "",
            "workload_name": workload_filter,
            "workload_kind": workload_kind,
            "service": service,
            "start_ts": start_ts,
            "end_ts": end_ts,
            "range_hours": range_hours,
            "limit": size,
        }
    )

    errors = []
    data_sources = {}

    logs = _call_source("logs", errors, data_sources, lambda: log_search.search_logs(base_params))
    events = _call_source("events", errors, data_sources, lambda: event_search.search_events(base_params))
    incidents = _call_source(
        "incidents",
        errors,
        data_sources,
        lambda: incident_store.search_incidents(q=query, namespace=namespace, pod=pod, limit=min(size, 100)),
    )
    business = _call_source(
        "business_context",
        errors,
        data_sources,
        lambda: business_correlation.correlate_business_context(base_params),
        item_key="logs",
    )
    release = None
    if namespace and (pod or workload_filter):
        release = _call_source("release", errors, data_sources, lambda: release_changes.release_for_workload(release_params), item_key="workload")
    changes = None
    if namespace:
        changes = _call_source("recent_changes", errors, data_sources, lambda: release_changes.recent_changes(release_params))
    gitlab_release = None
    if app_name:
        gitlab_release = _call_source(
            "gitlab_release_context",
            errors,
            data_sources,
            lambda: gitlab_integration.release_context(
                {
                    "app_name": app_name,
                    "ref": data.get("ref") or data.get("branch") or "main",
                    "sha": data.get("sha") or data.get("commit") or data.get("revision") or "",
                    "project_id": data.get("project_id") or data.get("project") or data.get("project_path") or "",
                }
            ),
        )
    resources = _call_source(
        "resources",
        errors,
        data_sources,
        lambda: prom_resource_check.build_dashboard_payload(
            prom_resource_check.collect_resource_data(config.PROMETHEUS_URLS, start_ts, end_ts),
            start_ts,
            end_ts,
        ),
    )
    deep_params = _deep_observability_params(
        namespace=namespace,
        pod=pod,
        workload_filter=workload_filter,
        service=service,
        start_ts=start_ts,
        end_ts=end_ts,
        range_hours=range_hours,
        size=size,
    )
    service_red = None
    runtime_events = None
    profile_hotspots = None
    if target_type in {"pod", "workload", "service", "namespace"} and namespace:
        service_red = _call_source(
            "service_red_metrics",
            errors,
            data_sources,
            lambda: deep_observability.service_red_metrics(deep_params),
            item_key="results",
        )
        runtime_events = _call_source(
            "runtime_events_context",
            errors,
            data_sources,
            lambda: deep_observability.runtime_events_context(deep_params),
        )
        profile_hotspots = _call_source(
            "profile_hotspots",
            errors,
            data_sources,
            lambda: deep_observability.profile_hotspots(deep_params),
            item_key="hotspots",
        )

    if target_type == "incident" and not (namespace or pod or workload_filter):
        pod_rows = []
    else:
        pod_rows = _filter_pod_resources(resources, namespace=namespace, pod=pod, workload_name=workload_filter)
    signals = resource_signals(resources, pod_rows)
    signals.extend(_deep_observability_signals(service_red, runtime_events, profile_hotspots))
    if resources is not None:
        data_sources.setdefault("resources", {})["pod_match_count"] = len(pod_rows)
        if not pod_rows:
            errors.append({"source": "resources", "message": "No matching pod resource rows found for the requested target."})

    source_statuses = [source.get("status") for source in data_sources.values()]
    status = "ok"
    if any(value == "error" for value in source_statuses):
        status = "partial"
    if not any(value in {"ok", "partial"} for value in source_statuses):
        status = "error"
    elif not any(
        [
            _compact_items(logs, 1),
            _compact_items(events, 1),
            _compact_items(incidents, 1),
            pod_rows,
            signals,
            ((service_red or {}).get("meta") or {}).get("item_count") if isinstance(service_red, dict) else None,
            ((runtime_events or {}).get("meta") or {}).get("item_count") if isinstance(runtime_events, dict) else None,
            ((profile_hotspots or {}).get("meta") or {}).get("hotspot_count") if isinstance(profile_hotspots, dict) else None,
            (release or {}).get("workload") if isinstance(release, dict) else None,
        ]
    ):
        status = "empty"

    return {
        "mode": "read_only_evidence_pack",
        "target_type": target_type,
        "target": {
            "namespace": namespace,
            "pod": pod,
            "workload_name": workload_name,
            "workload_kind": workload_kind,
            "service": service,
            "app_name": app_name,
            "incident_id": incident_id,
        },
        "request": {
            "symptom": symptom,
            "query": query,
            "range_hours": range_hours,
            "size": size,
        },
        "window": {"start_ts": start_ts, "end_ts": end_ts, "start": _iso(start_ts), "end": _iso(end_ts)},
        "summary": {
            "status": status,
            "top_signals": signals[:10],
            "matched_pods": [row.get("instance") for row in pod_rows[:20]],
            "missing_or_failed_sources": [name for name, source in data_sources.items() if source.get("status") in {"error", "empty"}],
        },
        "data_sources": data_sources,
        "evidence": {
            "resources": {
                "pod_short_trend": (resources or {}).get("pod_short_trend"),
                "pod_trend_rules": (resources or {}).get("pod_trend_rules"),
                "pods": pod_rows[: min(size, 50)],
            },
            "logs": _compact_items(logs, size),
            "events": _compact_items(events, size),
            "incidents": _compact_items(incidents, min(size, 50)),
            "business_context": business,
            "release": release,
            "recent_changes": _compact_items(changes, min(size, 50)),
            "gitlab_release_context": gitlab_release,
            "deep_observability": {
                "service_red_metrics": service_red,
                "runtime_events_context": runtime_events,
                "profile_hotspots": profile_hotspots,
            },
            "timeline": _timeline(logs, events, changes, incidents, limit=min(size, 50)),
        },
        "errors": errors,
        "safety": {
            "server_commands": "not_allowed",
            "kubernetes_mutations": "not_allowed",
            "notes": "Evidence Pack only reads configured backend data sources.",
        },
    }
