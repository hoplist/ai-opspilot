#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
dashboard_server.py
Unified backend for resources, alerts, pipeline orchestration, reports, and settings.

Endpoints:
- /api/resources : live resource payload (same shape as offline data.json)
- /api/alerts    : live alert summary
"""

import argparse
import contextlib
import json
import os
import time
import datetime
import io
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse, parse_qs, unquote

from auto_inspection import config
from auto_inspection import alert_notify
from auto_inspection import event_search
from auto_inspection import incident_store
from auto_inspection import investigation_service
from auto_inspection import k8s_inventory
from auto_inspection import business_correlation
from auto_inspection import context_pack
from auto_inspection import deep_observability
from auto_inspection import snapshot_index
from auto_inspection import release_changes
from auto_inspection import argocd_integration
from auto_inspection import gitlab_integration
from auto_inspection import log_search
from auto_inspection import opensearch_client
from auto_inspection import pipeline as inspection_pipeline
from auto_inspection import pod_restart_notify
from auto_inspection import prometheus_client
from auto_inspection import prom_resource_check
from auto_inspection import prom_alert_summary
from auto_inspection.paths import PROJECT_ROOT


def _parse_time(value):
    if not value:
        return None
    value = str(value).strip()
    if not value:
        return None
    if value.isdigit():
        ts = int(value)
        if ts > 10**12:
            ts = ts // 1000
        return ts
    try:
        dt = datetime.datetime.fromisoformat(value)
        return int(dt.timestamp())
    except ValueError:
        for fmt in ("%Y-%m-%d %H:%M:%S", "%Y-%m-%d %H:%M"):
            try:
                dt = datetime.datetime.strptime(value, fmt)
                return int(dt.timestamp())
            except ValueError:
                continue
    return None


def _window(query):
    end_ts = _parse_time(query.get("end", [None])[0])
    if end_ts is None:
        end_ts = int(time.time())
    range_hours = query.get("range_hours", [None])[0]
    range_days = query.get("range_days", [None])[0]
    try:
        range_hours = int(range_hours) if range_hours else None
    except (TypeError, ValueError):
        range_hours = None
    try:
        range_days = int(range_days) if range_days else None
    except (TypeError, ValueError):
        range_days = None
    if range_hours is None:
        range_hours = (range_days * 24) if range_days else config.RANGE_DAYS * 24
    start_ts = end_ts - int(range_hours * 3600)
    return start_ts, end_ts


NOTIFICATION_WEBHOOK_TYPES = {"generic", "feishu", "wecom", "dingtalk"}
DASHBOARD_LINK_KEYS = ("logs", "events", "yaml", "shell", "metrics")
INCIDENT_ARTIFACT_CANDIDATES = (
    "events_with_runbook",
    "events_escalated",
    "events_lifecycle",
    "events",
)
ARTIFACT_SPECS = {
    "targets": {
        "path": "data/targets.json",
        "kind": "json",
        "label": "Discovered targets",
    },
    "baseline_cpu": {
        "path": "data/baseline/cpu.json",
        "kind": "json",
        "label": "CPU baseline",
    },
    "baseline_mem": {
        "path": "data/baseline/mem.json",
        "kind": "json",
        "label": "Memory baseline",
    },
    "baseline_disk": {
        "path": "data/baseline/disk.json",
        "kind": "json",
        "label": "Disk baseline",
    },
    "anomalies": {
        "path": "data/anomalies.json",
        "kind": "json",
        "label": "Baseline anomalies",
    },
    "health_profiles": {
        "path": "data/health_profiles.json",
        "kind": "json",
        "label": "Health profiles",
    },
    "events": {
        "path": "data/events.json",
        "kind": "json",
        "label": "Merged events",
    },
    "events_lifecycle": {
        "path": "data/events_lifecycle.json",
        "kind": "json",
        "label": "Lifecycle events",
    },
    "events_history": {
        "path": "data/events_history.json",
        "kind": "json",
        "label": "Event history",
    },
    "events_escalated": {
        "path": "data/events_escalated.json",
        "kind": "json",
        "label": "Escalated events",
    },
    "events_with_runbook": {
        "path": "data/events_with_runbook.json",
        "kind": "json",
        "label": "Events with runbook",
    },
    "pod_restart_state": {
        "path": "data/pod_restart_notify_state.json",
        "kind": "json",
        "label": "Pod restart notification state",
    },
    "dashboard_data": {
        "path": "dashboard/data.json",
        "kind": "json",
        "label": "Dashboard payload snapshot",
    },
    "report": {
        "path": "outputs/reports/weekly_report.md",
        "kind": "text",
        "label": "Weekly report",
    },
}
STEP_ARTIFACTS = {
    "targets": ["targets"],
    "baseline": ["baseline_cpu", "baseline_mem", "baseline_disk"],
    "anomaly": ["anomalies"],
    "health": ["health_profiles"],
    "merge": ["events"],
    "lifecycle": ["events_lifecycle", "events_history"],
    "escalation": ["events_escalated"],
    "runbook": ["events_with_runbook"],
    "report": ["report", "dashboard_data", "pod_restart_state"],
}


def _notification_settings_payload():
    return {
        "enabled": bool(getattr(config, "POD_RESTART_NOTIFY_ENABLED", False)),
        "webhook_url": getattr(config, "POD_RESTART_NOTIFY_WEBHOOK_URL", "") or "",
        "webhook_type": getattr(config, "POD_RESTART_NOTIFY_WEBHOOK_TYPE", "generic") or "generic",
        "targets": list(getattr(config, "POD_RESTART_NOTIFY_TARGETS", []) or []),
        "state_file": getattr(config, "POD_RESTART_NOTIFY_STATE_FILE", "") or "",
        "config_path": config.get_config_file_path(),
    }


def _truthy(value):
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return value != 0
    return str(value or "").strip().lower() in {"1", "true", "yes", "y", "on"}


def _prometheus_cluster_pairs():
    urls = list(getattr(config, "PROMETHEUS_URLS", []) or [])
    clusters = list(getattr(config, "PROMETHEUS_CLUSTERS", []) or [])
    pairs = []
    for idx, url in enumerate(urls):
        cluster = clusters[idx] if idx < len(clusters) and clusters[idx] else f"cluster-{idx + 1}"
        pairs.append({"cluster": cluster, "url": url})
    return pairs


def _link_settings_payload():
    current = getattr(config, "DASHBOARD_LINK_TEMPLATES", {}) or {}
    payload = {}
    for key in DASHBOARD_LINK_KEYS:
        payload[key] = str(current.get(key, "") or "")
    return payload


def _dashboard_settings_payload():
    notification = _notification_settings_payload()
    return {
        "notification": {
            "enabled": notification["enabled"],
            "webhook_url": notification["webhook_url"],
            "webhook_type": notification["webhook_type"],
            "targets": notification["targets"],
            "state_file": notification["state_file"],
        },
        "links": _link_settings_payload(),
        "meta": {
            "config_path": notification["config_path"],
            "prometheus": _prometheus_cluster_pairs(),
        },
    }


def _runtime_config_payload():
    return {
        "config_path": config.get_config_file_path(),
        "prometheus": _prometheus_cluster_pairs(),
        "kubernetes": {
            "direct_enabled": bool(getattr(config, "K8S_DIRECT_ENABLED", True)),
            "kubectl_bin": getattr(config, "KUBECTL_BIN", "kubectl"),
            "kubeconfig_path": getattr(config, "KUBECONFIG_PATH", ".ssh/config"),
        },
        "opensearch": {
            "configured": opensearch_client.is_configured(),
            "url": getattr(config, "OPENSEARCH_URL", ""),
            "dashboards_url": getattr(config, "OPENSEARCH_DASHBOARDS_URL", ""),
            "verify_ssl": bool(getattr(config, "OPENSEARCH_VERIFY_SSL", True)),
            "timeout": getattr(config, "OPENSEARCH_TIMEOUT", 30),
            "index_logs": getattr(config, "OPENSEARCH_INDEX_LOGS", "logs-k8s-*"),
            "index_events": getattr(config, "OPENSEARCH_INDEX_EVENTS", "events-k8s-*"),
            "index_incidents": getattr(config, "OPENSEARCH_INDEX_INCIDENTS", "inspection-incidents-*"),
            "index_investigations": getattr(
                config,
                "OPENSEARCH_INDEX_INVESTIGATIONS",
                "inspection-investigations-*",
            ),
        },
        "range_days": getattr(config, "RANGE_DAYS", 7),
        "baseline_history_days": getattr(config, "BASELINE_HISTORY_DAYS", 28),
        "ai_summary_mode": getattr(config, "AI_SUMMARY_MODE", "strict"),
        "ai_investigation_enabled": bool(getattr(config, "AI_INVESTIGATION_ENABLED", False)),
        "ollama_url": getattr(config, "OLLAMA_URL", ""),
        "ollama_model": getattr(config, "OLLAMA_MODEL", ""),
        "runbook_file": getattr(config, "RUNBOOK_FILE", ""),
        "resource_output_dir": getattr(config, "RESOURCE_OUTPUT_DIR", "outputs"),
    }


def _health_details_payload():
    payload = {
        "service": "auto_inspection-backend",
        "status": "ok",
        "generated_at": datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "checks": {
            "backend": {"status": "ok"},
            "opensearch": {"status": "not_configured"},
            "prometheus": {"status": "not_configured", "items": []},
        },
    }

    if opensearch_client.is_configured():
        try:
            info = opensearch_client.ping()
            payload["checks"]["opensearch"] = {
                "status": "ok",
                "cluster_name": info.get("cluster_name"),
                "version": ((info.get("version") or {}).get("number")),
            }
        except Exception as exc:
            payload["checks"]["opensearch"] = {"status": "error", "error": str(exc)}
            payload["status"] = "degraded"

    prom_items = []
    pairs = _prometheus_cluster_pairs()
    for item in pairs:
        cluster = item.get("cluster") or "-"
        url = item.get("url") or ""
        try:
            result = prometheus_client.query_instant("up", url=url, timeout=15)
            prom_items.append(
                {
                    "cluster": cluster,
                    "url": url,
                    "status": "ok",
                    "series_count": len(result or []),
                }
            )
        except Exception as exc:
            prom_items.append(
                {
                    "cluster": cluster,
                    "url": url,
                    "status": "error",
                    "error": str(exc),
                }
            )
            payload["status"] = "degraded"
    if pairs:
        payload["checks"]["prometheus"] = {
            "status": "ok" if all(item.get("status") == "ok" for item in prom_items) else "error",
            "items": prom_items,
        }
    return payload


def _normalize_targets(value):
    if value is None:
        return []
    if isinstance(value, str):
        value = value.replace("\r", "\n")
        parts = []
        for chunk in value.split("\n"):
            parts.extend(chunk.split(","))
        return [item.strip() for item in parts if item.strip()]
    if isinstance(value, (list, tuple, set)):
        return [str(item).strip() for item in value if str(item).strip()]
    text = str(value).strip()
    return [text] if text else []


def _read_text_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()


def _read_json_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def _artifact_abs_path(name):
    spec = ARTIFACT_SPECS.get(name)
    if not spec:
        raise KeyError(name)
    return os.path.join(str(PROJECT_ROOT), spec["path"])


def _artifact_payload(name, include_content=False):
    spec = ARTIFACT_SPECS.get(name)
    if not spec:
        raise KeyError(name)

    abs_path = _artifact_abs_path(name)
    payload = {
        "name": name,
        "label": spec["label"],
        "kind": spec["kind"],
        "path": spec["path"],
        "exists": os.path.exists(abs_path),
    }
    if not payload["exists"]:
        return payload

    stat = os.stat(abs_path)
    payload["size_bytes"] = stat.st_size
    payload["updated_at"] = datetime.datetime.fromtimestamp(stat.st_mtime).strftime(
        "%Y-%m-%d %H:%M:%S"
    )

    if spec["kind"] == "json":
        data = _read_json_file(abs_path)
        if isinstance(data, dict):
            if "generated_at" in data:
                payload["generated_at"] = data["generated_at"]
            if "event_count" in data:
                payload["event_count"] = data["event_count"]
        if include_content:
            payload["content"] = data
    else:
        if include_content:
            payload["content"] = _read_text_file(abs_path)

    return payload


def _artifact_collection_payload(include_content=False):
    items = []
    for name in ARTIFACT_SPECS:
        items.append(_artifact_payload(name, include_content=include_content))
    return {"items": items}


def _select_incident_artifact_name():
    for name in INCIDENT_ARTIFACT_CANDIDATES:
        if _artifact_payload(name).get("exists"):
            return name
    return INCIDENT_ARTIFACT_CANDIDATES[-1]


def _pipeline_steps_payload():
    items = []
    for name, mod, func in inspection_pipeline.STEPS:
        items.append(
            {
                "name": name,
                "module": mod,
                "function": func,
                "artifacts": list(STEP_ARTIFACTS.get(name, [])),
            }
        )
    return items


def _normalize_pipeline_request(payload):
    data = payload if isinstance(payload, dict) else {}
    steps = _normalize_targets(data.get("steps"))
    skip = _normalize_targets(data.get("skip"))
    from_step = str(data.get("from_step", data.get("from", "")) or "").strip() or None
    to_step = str(data.get("to_step", data.get("to", "")) or "").strip() or None
    if steps and (from_step or to_step):
        raise ValueError("Use either 'steps' or 'from_step/to_step', not both.")
    return {
        "steps": steps,
        "skip": skip,
        "from_step": from_step,
        "to_step": to_step,
        "continue_on_error": _truthy(data.get("continue_on_error")),
    }


def _resolve_pipeline_selection(request):
    args = argparse.Namespace(
        list=False,
        steps=",".join(request["steps"]) if request.get("steps") else None,
        from_step=request.get("from_step"),
        to_step=request.get("to_step"),
        skip=",".join(request["skip"]) if request.get("skip") else None,
    )
    return inspection_pipeline.resolve_steps(args)


def _run_pipeline_request(request):
    selected = _resolve_pipeline_selection(request)
    started_at = time.time()
    results = []
    stopped = False

    for name, mod, func in inspection_pipeline.STEPS:
        if name not in selected:
            continue

        stdout_buffer = io.StringIO()
        stderr_buffer = io.StringIO()
        step_started_at = time.time()
        try:
            with contextlib.redirect_stdout(stdout_buffer), contextlib.redirect_stderr(stderr_buffer):
                module = __import__(mod, fromlist=[func])
                getattr(module, func)()
            results.append(
                {
                    "step": name,
                    "module": mod,
                    "function": func,
                    "status": "ok",
                    "duration_seconds": round(time.time() - step_started_at, 3),
                    "stdout": stdout_buffer.getvalue().strip(),
                    "stderr": stderr_buffer.getvalue().strip(),
                    "artifacts": list(STEP_ARTIFACTS.get(name, [])),
                }
            )
        except Exception as exc:
            results.append(
                {
                    "step": name,
                    "module": mod,
                    "function": func,
                    "status": "error",
                    "duration_seconds": round(time.time() - step_started_at, 3),
                    "stdout": stdout_buffer.getvalue().strip(),
                    "stderr": stderr_buffer.getvalue().strip(),
                    "error": str(exc),
                    "artifacts": list(STEP_ARTIFACTS.get(name, [])),
                }
            )
            stopped = not request.get("continue_on_error")
            if stopped:
                break

    failed = [item for item in results if item["status"] != "ok"]
    status = "ok"
    if failed and results:
        status = "partial" if any(item["status"] == "ok" for item in results) else "error"
    elif failed:
        status = "error"

    artifact_names = []
    for step_name in selected:
        artifact_names.extend(STEP_ARTIFACTS.get(step_name, []))

    return {
        "selected_steps": selected,
        "results": results,
        "artifacts": [
            _artifact_payload(name)
            for name in dict.fromkeys(artifact_names)
        ],
        "meta": {
            "status": status,
            "query_seconds": round(time.time() - started_at, 3),
            "continue_on_error": bool(request.get("continue_on_error")),
            "stopped_early": stopped,
            "completed_steps": len([item for item in results if item["status"] == "ok"]),
            "failed_steps": len(failed),
        },
    }


def _backend_overview_payload():
    incident_artifact = _select_incident_artifact_name()
    return {
        "service": "auto_inspection-backend",
        "generated_at": datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "config": _runtime_config_payload(),
        "pipeline": {
            "steps": _pipeline_steps_payload(),
        },
        "artifacts": _artifact_collection_payload()["items"],
        "report": _artifact_payload("report"),
        "incidents": _artifact_payload(incident_artifact),
    }


def _normalize_notification_settings(payload):
    current = _notification_settings_payload()
    data = payload if isinstance(payload, dict) else {}
    webhook_type = str(data.get("webhook_type", current["webhook_type"]) or "generic").strip().lower()
    if webhook_type not in NOTIFICATION_WEBHOOK_TYPES:
        webhook_type = "generic"
    state_file = str(data.get("state_file", current["state_file"]) or current["state_file"]).strip()
    if not state_file:
        state_file = "data/pod_restart_notify_state.json"
    return {
        "enabled": bool(data.get("enabled", current["enabled"])),
        "webhook_url": str(data.get("webhook_url", current["webhook_url"]) or "").strip(),
        "webhook_type": webhook_type,
        "targets": _normalize_targets(data.get("targets", current["targets"])),
        "state_file": state_file,
    }


def _normalize_link_templates(payload):
    current = _link_settings_payload()
    data = payload if isinstance(payload, dict) else {}
    normalized = {}
    for key in DASHBOARD_LINK_KEYS:
        normalized[key] = str(data.get(key, current.get(key, "")) or "").strip()
    return normalized


def _read_config_json(path):
    if not path or not os.path.exists(path):
        return {}
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
        if isinstance(data, dict):
            return data
    except (OSError, json.JSONDecodeError):
        pass
    return {}


def _write_config_json(path, data):
    directory = os.path.dirname(path)
    if directory:
        os.makedirs(directory, exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)


def _apply_notification_settings(settings):
    config_path = config.get_config_file_path()
    data = _read_config_json(config_path)
    data["POD_RESTART_NOTIFY_ENABLED"] = bool(settings.get("enabled"))
    data["POD_RESTART_NOTIFY_WEBHOOK_URL"] = settings.get("webhook_url", "")
    data["POD_RESTART_NOTIFY_WEBHOOK_TYPE"] = settings.get("webhook_type", "generic")
    data["POD_RESTART_NOTIFY_TARGETS"] = list(settings.get("targets", []))
    data["POD_RESTART_NOTIFY_STATE_FILE"] = settings.get(
        "state_file",
        "data/pod_restart_notify_state.json",
    )
    _write_config_json(config_path, data)

    setattr(config, "POD_RESTART_NOTIFY_ENABLED", data["POD_RESTART_NOTIFY_ENABLED"])
    setattr(config, "POD_RESTART_NOTIFY_WEBHOOK_URL", data["POD_RESTART_NOTIFY_WEBHOOK_URL"])
    setattr(config, "POD_RESTART_NOTIFY_WEBHOOK_TYPE", data["POD_RESTART_NOTIFY_WEBHOOK_TYPE"])
    setattr(config, "POD_RESTART_NOTIFY_TARGETS", data["POD_RESTART_NOTIFY_TARGETS"])
    setattr(config, "POD_RESTART_NOTIFY_STATE_FILE", data["POD_RESTART_NOTIFY_STATE_FILE"])
    return _notification_settings_payload()


def _normalize_dashboard_settings(payload):
    data = payload if isinstance(payload, dict) else {}
    return {
        "notification": _normalize_notification_settings(data.get("notification", data)),
        "links": _normalize_link_templates(data.get("links", {})),
    }


def _apply_dashboard_settings(settings):
    config_path = config.get_config_file_path()
    data = _read_config_json(config_path)
    notification = settings.get("notification", {})
    links = settings.get("links", {})

    data["POD_RESTART_NOTIFY_ENABLED"] = bool(notification.get("enabled"))
    data["POD_RESTART_NOTIFY_WEBHOOK_URL"] = notification.get("webhook_url", "")
    data["POD_RESTART_NOTIFY_WEBHOOK_TYPE"] = notification.get("webhook_type", "generic")
    data["POD_RESTART_NOTIFY_TARGETS"] = list(notification.get("targets", []))
    data["POD_RESTART_NOTIFY_STATE_FILE"] = notification.get(
        "state_file",
        "data/pod_restart_notify_state.json",
    )
    data["DASHBOARD_LINK_TEMPLATES"] = {
        key: str(links.get(key, "") or "")
        for key in DASHBOARD_LINK_KEYS
    }
    _write_config_json(config_path, data)

    setattr(config, "POD_RESTART_NOTIFY_ENABLED", data["POD_RESTART_NOTIFY_ENABLED"])
    setattr(config, "POD_RESTART_NOTIFY_WEBHOOK_URL", data["POD_RESTART_NOTIFY_WEBHOOK_URL"])
    setattr(config, "POD_RESTART_NOTIFY_WEBHOOK_TYPE", data["POD_RESTART_NOTIFY_WEBHOOK_TYPE"])
    setattr(config, "POD_RESTART_NOTIFY_TARGETS", data["POD_RESTART_NOTIFY_TARGETS"])
    setattr(config, "POD_RESTART_NOTIFY_STATE_FILE", data["POD_RESTART_NOTIFY_STATE_FILE"])
    setattr(config, "DASHBOARD_LINK_TEMPLATES", data["DASHBOARD_LINK_TEMPLATES"])
    return _dashboard_settings_payload()


def _build_test_event():
    now_str = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    return {
        "state_key": "ui-test|default|pod-restart-test",
        "cluster": "ui-test",
        "namespace": "default",
        "pod": "pod-restart-test",
        "instance": "default/pod-restart-test",
        "node_name": "ui-test-node",
        "phase": "Running",
        "pod_status": "Running",
        "terminated_reason": "ManualTest",
        "terminated_exitcode": 137,
        "last_terminated_time": int(time.time()),
        "restart_window_hours": int(getattr(config, "RESOURCE_POD_RESTART_HOURS", 24)),
        "restarts_recent": 1,
        "restarts_total": 1,
        "restart_delta": 1,
        "counter_reset": False,
        "detected_at": now_str,
    }


class Handler(SimpleHTTPRequestHandler):
    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path == "/api/health":
            return self._handle_health()
        if parsed.path == "/api/health/details":
            return self._handle_health_details()
        if parsed.path == "/api/backend/overview":
            return self._handle_backend_overview()
        if parsed.path == "/api/config":
            return self._handle_runtime_config()
        if parsed.path == "/api/search/status":
            return self._handle_search_status()
        if parsed.path == "/api/search/logs":
            return self._handle_log_search(parse_qs(parsed.query))
        if parsed.path == "/api/search/business-logs":
            return self._handle_business_log_search(parse_qs(parsed.query))
        if parsed.path == "/api/traces/search":
            return self._handle_trace_search(parse_qs(parsed.query))
        if parsed.path == "/api/business/correlate":
            return self._handle_business_correlate(parse_qs(parsed.query))
        if parsed.path == "/api/context/pod":
            return self._handle_context_pack("pod", parse_qs(parsed.query))
        if parsed.path == "/api/context/workload":
            return self._handle_context_pack("workload", parse_qs(parsed.query))
        if parsed.path == "/api/context/service":
            return self._handle_context_pack("service", parse_qs(parsed.query))
        if parsed.path == "/api/context/incident":
            return self._handle_context_pack("incident", parse_qs(parsed.query))
        if parsed.path == "/api/context/namespace":
            return self._handle_context_pack("namespace", parse_qs(parsed.query))
        if parsed.path == "/api/k8s/namespaces":
            return self._handle_k8s_namespaces(parse_qs(parsed.query))
        if parsed.path == "/api/k8s/pods":
            return self._handle_k8s_pods(parse_qs(parsed.query))
        if parsed.path == "/api/k8s/pods/abnormal":
            return self._handle_k8s_abnormal_pods(parse_qs(parsed.query))
        if parsed.path == "/api/k8s/workloads":
            return self._handle_k8s_workloads(parse_qs(parsed.query))
        if parsed.path == "/api/k8s/services":
            return self._handle_k8s_services(parse_qs(parsed.query))
        if parsed.path == "/api/k8s/resources/search":
            return self._handle_k8s_resource_search(parse_qs(parsed.query))
        if parsed.path == "/api/k8s/resources/count":
            return self._handle_k8s_resource_count(parse_qs(parsed.query))
        if parsed.path == "/api/k8s/cluster/overview":
            return self._handle_k8s_cluster_overview(parse_qs(parsed.query))
        if parsed.path == "/api/snapshot-index":
            return self._handle_snapshot_index(parse_qs(parsed.query))
        if parsed.path == "/api/releases/workload":
            return self._handle_release_workload(parse_qs(parsed.query))
        if parsed.path == "/api/releases/recent-changes":
            return self._handle_release_recent_changes(parse_qs(parsed.query))
        if parsed.path == "/api/releases/correlate":
            return self._handle_release_correlate(parse_qs(parsed.query))
        if parsed.path == "/api/argocd/app-status":
            return self._handle_argocd_app_status(parse_qs(parsed.query))
        if parsed.path == "/api/argocd/app-history":
            return self._handle_argocd_app_history(parse_qs(parsed.query))
        if parsed.path == "/api/argocd/diff-summary":
            return self._handle_argocd_diff_summary(parse_qs(parsed.query))
        if parsed.path == "/api/gitlab/recent-commits":
            return self._handle_gitlab_recent_commits(parse_qs(parsed.query))
        if parsed.path == "/api/gitlab/commit-detail":
            return self._handle_gitlab_commit_detail(parse_qs(parsed.query))
        if parsed.path == "/api/gitlab/pipeline-status":
            return self._handle_gitlab_pipeline_status(parse_qs(parsed.query))
        if parsed.path == "/api/gitlab/release-context":
            return self._handle_gitlab_release_context(parse_qs(parsed.query))
        if parsed.path == "/api/gitlab/merge-requests":
            return self._handle_gitlab_merge_requests(parse_qs(parsed.query))
        if parsed.path == "/api/gitlab/tags":
            return self._handle_gitlab_tags(parse_qs(parsed.query))
        if parsed.path == "/api/gitlab/artifacts":
            return self._handle_gitlab_artifacts(parse_qs(parsed.query))
        if parsed.path == "/api/gitlab/image-digest-context":
            return self._handle_gitlab_image_digest_context(parse_qs(parsed.query))
        if parsed.path == "/api/observability/service-red-metrics":
            return self._handle_service_red_metrics(parse_qs(parsed.query))
        if parsed.path == "/api/observability/runtime-events":
            return self._handle_runtime_events_context(parse_qs(parsed.query))
        if parsed.path == "/api/observability/profile-hotspots":
            return self._handle_profile_hotspots(parse_qs(parsed.query))
        if parsed.path == "/api/search/events":
            return self._handle_event_search(parse_qs(parsed.query))
        if parsed.path == "/api/investigation-targets":
            return self._handle_investigation_targets(parse_qs(parsed.query))
        if parsed.path == "/api/investigations":
            return self._handle_investigations_list(parse_qs(parsed.query))
        if parsed.path == "/api/incidents/list":
            return self._handle_incidents_list(parse_qs(parsed.query))
        if parsed.path == "/api/incidents/search":
            return self._handle_incidents_search(parse_qs(parsed.query))
        if parsed.path.startswith("/api/investigations/"):
            investigation_id = unquote(parsed.path[len("/api/investigations/"):]).strip("/")
            return self._handle_investigation_get(investigation_id)
        if parsed.path == "/api/resources":
            return self._handle_resources(parse_qs(parsed.query))
        if parsed.path == "/api/alerts":
            return self._handle_alerts(parse_qs(parsed.query))
        if parsed.path == "/api/pipeline/steps":
            return self._handle_pipeline_steps()
        if parsed.path == "/api/artifacts":
            return self._handle_artifacts_list(parse_qs(parsed.query))
        if parsed.path.startswith("/api/artifacts/"):
            name = unquote(parsed.path[len("/api/artifacts/"):]).strip("/")
            return self._handle_artifact_detail(name, parse_qs(parsed.query))
        if parsed.path == "/api/incidents":
            return self._handle_incidents()
        if parsed.path == "/api/report":
            return self._handle_report()
        if parsed.path == "/api/dashboard-settings":
            return self._handle_dashboard_settings_get()
        if parsed.path == "/api/notification-settings":
            return self._handle_notification_settings_get()
        return super().do_GET()

    def do_POST(self):
        parsed = urlparse(self.path)
        if parsed.path == "/api/pipeline/run":
            return self._handle_pipeline_run()
        if parsed.path == "/api/investigate":
            return self._handle_investigate()
        if parsed.path == "/api/report/generate":
            return self._handle_report_generate()
        if parsed.path == "/api/dashboard-settings":
            return self._handle_dashboard_settings_post()
        if parsed.path == "/api/dashboard-settings/test-notification":
            return self._handle_dashboard_settings_test_notification()
        if parsed.path == "/api/notification-settings":
            return self._handle_notification_settings_post()
        if parsed.path == "/api/notification-settings/test":
            return self._handle_notification_settings_test()
        if parsed.path == "/api/alerts/notify":
            return self._handle_alert_notify()
        self._send_json({"error": "Not found"}, status=404)

    def _send_json(self, payload, status=200):
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def _read_json_body(self):
        length = int(self.headers.get("Content-Length", "0") or 0)
        if length <= 0:
            return {}
        raw = self.rfile.read(length)
        if not raw:
            return {}
        try:
            data = json.loads(raw.decode("utf-8"))
        except json.JSONDecodeError as exc:
            raise ValueError(f"Invalid JSON body: {exc}") from exc
        if not isinstance(data, dict):
            raise ValueError("JSON body must be an object.")
        return data

    def _handle_health(self):
        self._send_json(
            {
                "service": "auto_inspection-backend",
                "status": "ok",
                "generated_at": datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
            }
        )

    def _handle_health_details(self):
        payload = _health_details_payload()
        status = 200 if payload.get("status") == "ok" else 503
        self._send_json(payload, status=status)

    def _handle_backend_overview(self):
        self._send_json(_backend_overview_payload())

    def _handle_runtime_config(self):
        self._send_json({"config": _runtime_config_payload()})

    def _handle_search_status(self):
        payload = {
            "configured": opensearch_client.is_configured(),
            "logs_index": getattr(config, "OPENSEARCH_INDEX_LOGS", "logs-k8s-*"),
            "events_index": getattr(config, "OPENSEARCH_INDEX_EVENTS", "events-k8s-*"),
        }
        if payload["configured"]:
            try:
                info = opensearch_client.ping()
                payload["status"] = "ok"
                payload["cluster_name"] = info.get("cluster_name")
                payload["version"] = ((info.get("version") or {}).get("number"))
            except Exception as exc:
                payload["status"] = "error"
                payload["error"] = str(exc)
                self._send_json(payload, status=502)
                return
        else:
            payload["status"] = "not_configured"
        self._send_json(payload)

    def _handle_pipeline_steps(self):
        self._send_json({"steps": _pipeline_steps_payload()})

    def _handle_artifacts_list(self, query):
        include_content = _truthy((query.get("include_content") or [""])[0])
        self._send_json(_artifact_collection_payload(include_content=include_content))

    def _handle_artifact_detail(self, name, query):
        try:
            include_content = True
            if query:
                include_content = _truthy((query.get("include_content") or ["true"])[0])
            artifact = _artifact_payload(name, include_content=include_content)
        except KeyError:
            self._send_json({"error": f"Unknown artifact: {name}"}, status=404)
            return
        if not artifact.get("exists"):
            self._send_json({"artifact": artifact, "error": "Artifact not found."}, status=404)
            return
        self._send_json({"artifact": artifact})

    def _handle_incidents(self):
        name = _select_incident_artifact_name()
        artifact = _artifact_payload(name, include_content=True)
        if not artifact.get("exists"):
            self._send_json({"artifact": artifact, "error": "Incident artifact not found."}, status=404)
            return
        self._send_json({"artifact": artifact})

    def _handle_report(self):
        artifact = _artifact_payload("report", include_content=True)
        if not artifact.get("exists"):
            self._send_json({"artifact": artifact, "error": "Report not found."}, status=404)
            return
        self._send_json({"artifact": artifact})

    def _handle_investigation_get(self, investigation_id):
        if not investigation_id:
            self._send_json({"error": "Investigation id is required."}, status=400)
            return
        payload = investigation_service.load_investigation(investigation_id)
        if payload is None:
            self._send_json({"error": f"Investigation not found: {investigation_id}"}, status=404)
            return
        self._send_json(payload)

    def _handle_investigations_list(self, query):
        limit_raw = (query.get("limit") or [20])[0]
        try:
            limit = max(1, min(int(limit_raw), 200))
        except (TypeError, ValueError):
            limit = 20
        self._send_json({"items": investigation_service.list_recent_investigations(limit=limit)})

    def _handle_investigation_targets(self, query):
        limit_raw = (query.get("limit") or [20])[0]
        try:
            limit = max(1, min(int(limit_raw), 200))
        except (TypeError, ValueError):
            limit = 20
        self._send_json({"items": investigation_service.list_investigation_targets(limit=limit)})

    def _handle_incidents_list(self, query):
        limit_raw = (query.get("limit") or [20])[0]
        try:
            limit = max(1, min(int(limit_raw), 200))
        except (TypeError, ValueError):
            limit = 20
        self._send_json(incident_store.list_incidents(limit=limit))

    def _handle_incidents_search(self, query):
        q = (query.get("q") or [""])[0]
        namespace = (query.get("namespace") or [""])[0]
        pod = (query.get("pod") or [""])[0]
        limit_raw = (query.get("limit") or [20])[0]
        try:
            limit = max(1, min(int(limit_raw), 200))
        except (TypeError, ValueError):
            limit = 20
        self._send_json(
            incident_store.search_incidents(
                q=q,
                namespace=namespace,
                pod=pod,
                limit=limit,
            )
        )

    def _handle_pipeline_run(self):
        try:
            payload = self._read_json_body()
            request = _normalize_pipeline_request(payload)
            result = _run_pipeline_request(request)
            status = 200 if result["meta"]["status"] in {"ok", "partial"} else 500
            self._send_json(result, status=status)
        except ValueError as exc:
            self._send_json({"error": str(exc)}, status=400)
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_investigate(self):
        started_at = time.time()
        try:
            payload = self._read_json_body()
            result = investigation_service.run_investigation(payload)
            result.setdefault("meta", {})
            result["meta"]["query_seconds"] = round(time.time() - started_at, 3)
            self._send_json(result)
        except ValueError as exc:
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {
                        "status": "error",
                        "query_seconds": round(time.time() - started_at, 3),
                    },
                },
                status=400,
            )
        except Exception as exc:
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {
                        "status": "error",
                        "query_seconds": round(time.time() - started_at, 3),
                    },
                },
                status=500,
            )

    def _handle_report_generate(self):
        try:
            payload = self._read_json_body()
            request = _normalize_pipeline_request(
                {
                    **payload,
                    "steps": ["report"],
                    "from_step": None,
                    "to_step": None,
                    "skip": [],
                }
            )
            result = _run_pipeline_request(request)
            report = _artifact_payload("report", include_content=True)
            response = {
                **result,
                "report": report,
            }
            status = 200 if result["meta"]["status"] in {"ok", "partial"} else 500
            self._send_json(response, status=status)
        except ValueError as exc:
            self._send_json({"error": str(exc)}, status=400)
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_resources(self, query):
        started_at = time.time()
        try:
            start_ts, end_ts = _window(query)
            data = prom_resource_check.collect_resource_data(
                config.PROMETHEUS_URLS,
                start_ts,
                end_ts,
            )
            payload = prom_resource_check.build_dashboard_payload(
                data,
                start_ts,
                end_ts,
            )
            snapshot_index.enrich_resource_payload(payload)
            item_count = len(payload.get("items") or [])
            pod_state_count = len(payload.get("pod_states") or [])
            payload["meta"] = {
                "status": "ok" if (item_count or pod_state_count) else "empty",
                "query_seconds": round(time.time() - started_at, 3),
                "item_count": item_count,
                "pod_state_count": pod_state_count,
                "prometheus_count": len(getattr(config, "PROMETHEUS_URLS", []) or []),
            }
            if not item_count and not pod_state_count:
                payload["error"] = "未获取到资源数据，请检查 Prometheus 地址、Exporter 指标以及资源筛选配置。"
                self._send_json(payload, status=502)
                return
            self._send_json(payload)
        except Exception as exc:
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {
                        "status": "error",
                        "query_seconds": round(time.time() - started_at, 3),
                    },
                },
                status=500,
            )

    def _handle_snapshot_index(self, query):
        started_at = time.time()
        try:
            params = self._build_search_params(query, ("namespace", "limit"))
            if query.get("range_hours"):
                params["range_hours"] = (query.get("range_hours") or [""])[0]
            payload = snapshot_index.build_snapshot_index(params)
            payload["meta"] = {
                "status": "ok" if not payload.get("errors") else "partial",
                "query_seconds": round(time.time() - started_at, 3),
            }
            self._send_json(payload)
        except Exception as exc:
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {"status": "error", "query_seconds": round(time.time() - started_at, 3)},
                },
                status=500,
            )

    def _handle_alerts(self, query):
        started_at = time.time()
        try:
            start_ts, end_ts = _window(query)
            rows = prom_alert_summary.collect_alert_rows(
                prometheus_urls=config.PROMETHEUS_URLS,
                start_ts=start_ts,
                end_ts=end_ts,
            )
            start_str = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(start_ts))
            end_str = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(end_ts))
            rows = sorted(rows, key=lambda x: x.get("hours", 0), reverse=True)
            payload = {
                "generated_at": end_str,
                "window": {"start": start_str, "end": end_str},
                "items": rows,
                "meta": {
                    "status": "ok",
                    "query_seconds": round(time.time() - started_at, 3),
                    "item_count": len(rows),
                },
            }
            self._send_json(payload)
        except Exception as exc:
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {
                        "status": "error",
                        "query_seconds": round(time.time() - started_at, 3),
                    },
                },
                status=500,
            )

    def _handle_alert_notify(self):
        try:
            payload = self._read_json_body()
            result = alert_notify.process_alerts(
                enabled=payload.get("enabled"),
                webhook_url=payload.get("webhook_url"),
                webhook_type=payload.get("webhook_type"),
                range_hours=payload.get("range_hours"),
                cooldown_seconds=payload.get("cooldown_seconds"),
                min_hours=payload.get("min_hours"),
                dry_run=_truthy(payload.get("dry_run")),
            )
            self._send_json(result)
        except ValueError as exc:
            self._send_json({"error": str(exc)}, status=400)
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _k8s_inventory_params(self, query):
        params = {}
        for key in (
            "namespace",
            "q",
            "status",
            "node",
            "owner_kind",
            "owner_name",
            "kind",
            "type",
            "kinds",
            "limit",
        ):
            values = query.get(key)
            if not values:
                continue
            value = values[0]
            if value is None:
                continue
            value = str(value).strip()
            if value == "":
                continue
            params[key] = value
        return params

    def _handle_k8s_inventory_call(self, query, func):
        started_at = time.time()
        try:
            self._send_json(func(self._k8s_inventory_params(query)))
        except Exception as exc:
            message = str(exc)
            status = 503 if any(
                token in message
                for token in ("Kubernetes API", "kubectl", "Direct Kubernetes access", "in-cluster")
            ) else 500
            self._send_json(
                {
                    "error": message,
                    "meta": {
                        "status": "error",
                        "query_seconds": round(time.time() - started_at, 3),
                    },
                },
                status=status,
            )

    def _handle_k8s_namespaces(self, query):
        return self._handle_k8s_inventory_call(query, k8s_inventory.list_namespaces)

    def _handle_k8s_pods(self, query):
        return self._handle_k8s_inventory_call(query, k8s_inventory.list_pods)

    def _handle_k8s_abnormal_pods(self, query):
        return self._handle_k8s_inventory_call(query, k8s_inventory.list_abnormal_pods)

    def _handle_k8s_workloads(self, query):
        return self._handle_k8s_inventory_call(query, k8s_inventory.list_workloads)

    def _handle_k8s_services(self, query):
        return self._handle_k8s_inventory_call(query, k8s_inventory.list_services)

    def _handle_k8s_resource_search(self, query):
        return self._handle_k8s_inventory_call(query, k8s_inventory.search_resources)

    def _handle_k8s_resource_count(self, query):
        return self._handle_k8s_inventory_call(query, k8s_inventory.count_resources)

    def _handle_k8s_cluster_overview(self, query):
        return self._handle_k8s_inventory_call(query, k8s_inventory.cluster_overview)

    def _build_search_params(self, query, allowed_keys):
        params = {}
        start_ts, end_ts = _window(query)
        params["start_ts"] = start_ts
        params["end_ts"] = end_ts

        for key in allowed_keys:
            values = query.get(key)
            if not values:
                continue
            value = values[0]
            if value is None:
                continue
            value = str(value).strip()
            if value == "":
                continue
            params[key] = value

        size_value = (query.get("size") or [None])[0]
        from_value = (query.get("from") or [None])[0]
        try:
            params["size"] = int(size_value) if size_value not in (None, "") else 50
        except (TypeError, ValueError):
            params["size"] = 50
        try:
            params["from"] = int(from_value) if from_value not in (None, "") else 0
        except (TypeError, ValueError):
            params["from"] = 0
        return params

    def _handle_log_search(self, query):
        started_at = time.time()
        try:
            params = self._build_search_params(
                query,
                (
                    "q",
                    "cluster",
                    "namespace",
                    "workload_name",
                    "pod",
                    "container",
                    "node",
                    "service",
                    "biz_line",
                    "business_key",
                    "frontend_service",
                    "backend_service",
                    "domain",
                    "route",
                    "version",
                    "trace_id",
                    "span_id",
                    "request_id",
                    "event_id",
                    "tenant_id",
                    "user_id",
                    "order_id",
                    "error_code",
                    "severity",
                ),
            )
            payload = log_search.search_logs(params)
            payload["meta"] = {
                "status": "ok",
                "query_seconds": round(time.time() - started_at, 3),
                "configured": opensearch_client.is_configured(),
                "item_count": len(payload.get("items") or []),
                "total": payload.get("total", 0),
            }
            self._send_json(payload)
        except Exception as exc:
            status = 503 if "OPENSEARCH_URL is not configured" in str(exc) else 500
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {
                        "status": "error",
                        "query_seconds": round(time.time() - started_at, 3),
                        "configured": opensearch_client.is_configured(),
                    },
                },
                status=status,
            )

    def _handle_business_log_search(self, query):
        started_at = time.time()
        try:
            params = self._build_search_params(
                query,
                (
                    "q",
                    "cluster",
                    "namespace",
                    "workload_name",
                    "pod",
                    "container",
                    "node",
                    "service",
                    "biz_line",
                    "business_key",
                    "frontend_service",
                    "backend_service",
                    "domain",
                    "route",
                    "version",
                    "trace_id",
                    "span_id",
                    "request_id",
                    "event_id",
                    "tenant_id",
                    "user_id",
                    "order_id",
                    "error_code",
                    "severity",
                ),
            )
            payload = business_correlation.search_business_logs(params)
            payload.setdefault("meta", {})
            payload["meta"].update(
                {
                    "status": "ok",
                    "query_seconds": round(time.time() - started_at, 3),
                    "configured": opensearch_client.is_configured(),
                    "item_count": len(payload.get("items") or []),
                    "total": payload.get("total", 0),
                }
            )
            self._send_json(payload)
        except Exception as exc:
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {
                        "status": "error",
                        "query_seconds": round(time.time() - started_at, 3),
                    },
                },
                status=500,
            )

    def _handle_trace_search(self, query):
        started_at = time.time()
        try:
            params = self._build_search_params(
                query,
                (
                    "q",
                    "trace_id",
                    "span_id",
                    "service",
                    "domain",
                    "route",
                    "request_id",
                    "event_id",
                    "business_key",
                    "error",
                ),
            )
            payload = business_correlation.search_traces(params)
            payload.setdefault("meta", {})
            payload["meta"].update(
                {
                    "status": "ok",
                    "query_seconds": round(time.time() - started_at, 3),
                    "configured": opensearch_client.is_configured(),
                    "item_count": len(payload.get("items") or []),
                    "total": payload.get("total", 0),
                }
            )
            self._send_json(payload)
        except Exception as exc:
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {
                        "status": "error",
                        "query_seconds": round(time.time() - started_at, 3),
                    },
                },
                status=500,
            )

    def _handle_business_correlate(self, query):
        started_at = time.time()
        try:
            params = self._build_search_params(
                query,
                (
                    "q",
                    "cluster",
                    "namespace",
                    "workload_name",
                    "pod",
                    "service",
                    "backend_service",
                    "frontend_service",
                    "business_key",
                    "domain",
                    "route",
                    "version",
                    "trace_id",
                    "span_id",
                    "request_id",
                    "event_id",
                    "tenant_id",
                    "user_id",
                    "order_id",
                    "error_code",
                ),
            )
            payload = business_correlation.correlate_business_context(params)
            payload.setdefault("meta", {})
            payload["meta"].update(
                {
                    "status": "ok" if not payload.get("errors") else "partial",
                    "query_seconds": round(time.time() - started_at, 3),
                    "configured": opensearch_client.is_configured(),
                }
            )
            self._send_json(payload)
        except Exception as exc:
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {
                        "status": "error",
                        "query_seconds": round(time.time() - started_at, 3),
                    },
                },
                status=500,
            )

    def _handle_context_pack(self, target_type, query):
        started_at = time.time()
        try:
            params = self._build_search_params(
                query,
                (
                    "q",
                    "cluster",
                    "namespace",
                    "pod",
                    "workload_name",
                    "workload_kind",
                    "service",
                    "app_name",
                    "application",
                    "ref",
                    "branch",
                    "sha",
                    "commit",
                    "revision",
                    "project_id",
                    "project",
                    "project_path",
                    "symptom",
                    "incident_id",
                    "id",
                    "size",
                ),
            )
            if query.get("range_hours"):
                params["range_hours"] = (query.get("range_hours") or [""])[0]
            payload = context_pack.build_context_pack(target_type, params)
            payload["meta"] = {
                "status": payload.get("summary", {}).get("status", "ok"),
                "query_seconds": round(time.time() - started_at, 3),
                "data_source_count": len(payload.get("data_sources") or {}),
                "error_count": len(payload.get("errors") or []),
            }
            status = 200
            if payload["meta"]["status"] == "empty":
                status = 404
            elif payload["meta"]["status"] == "error":
                status = 502
            self._send_json(payload, status=status)
        except ValueError as exc:
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {"status": "error", "query_seconds": round(time.time() - started_at, 3)},
                },
                status=400,
            )
        except Exception as exc:
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {"status": "error", "query_seconds": round(time.time() - started_at, 3)},
                },
                status=500,
            )

    def _release_params(self, query):
        params = self._build_search_params(
            query,
            (
                "namespace",
                "pod",
                "workload_name",
                "workload_kind",
                "service",
                "include_configmaps",
                "limit",
            ),
        )
        start_ts, end_ts = _window(query)
        params["start_ts"] = start_ts
        params["end_ts"] = end_ts
        return params

    def _argocd_params(self, query):
        params = {}
        for key in ("app_name", "application", "name", "refresh", "limit"):
            value = (query.get(key) or [""])[0]
            if value not in (None, ""):
                params[key] = value
        return params

    def _gitlab_params(self, query):
        params = {}
        for key in (
            "project_id",
            "project",
            "project_path",
            "ref",
            "branch",
            "ref_name",
            "sha",
            "commit",
            "revision",
            "status",
            "limit",
            "per_page",
            "since",
            "until",
            "app_name",
            "application",
            "history_limit",
            "mr_limit",
            "tag_limit",
            "artifact_limit",
            "registry_limit",
            "state",
            "source_branch",
            "target_branch",
            "search",
            "order_by",
            "sort",
            "tag",
            "tag_name",
            "name",
            "scope",
            "job",
            "job_name",
            "with_artifacts",
            "namespace",
            "pod",
            "workload_name",
            "workload_kind",
            "image",
        ):
            value = (query.get(key) or [""])[0]
            if value not in (None, ""):
                params[key] = value
        return params

    def _handle_release_workload(self, query):
        started_at = time.time()
        try:
            payload = release_changes.release_for_workload(self._release_params(query))
            payload["meta"] = {
                "status": "ok" if not payload.get("errors") else "partial",
                "query_seconds": round(time.time() - started_at, 3),
            }
            self._send_json(payload)
        except Exception as exc:
            self._send_json({"error": str(exc), "meta": {"status": "error", "query_seconds": round(time.time() - started_at, 3)}}, status=500)

    def _handle_release_recent_changes(self, query):
        started_at = time.time()
        try:
            payload = release_changes.recent_changes(self._release_params(query))
            payload["meta"] = {
                "status": "ok" if not payload.get("errors") else "partial",
                "query_seconds": round(time.time() - started_at, 3),
                "item_count": len(payload.get("items") or []),
            }
            self._send_json(payload)
        except Exception as exc:
            self._send_json({"error": str(exc), "meta": {"status": "error", "query_seconds": round(time.time() - started_at, 3)}}, status=500)

    def _handle_release_correlate(self, query):
        started_at = time.time()
        try:
            payload = release_changes.correlate_change_with_incident(self._release_params(query))
            payload["meta"] = {
                "status": "ok",
                "query_seconds": round(time.time() - started_at, 3),
            }
            self._send_json(payload)
        except Exception as exc:
            self._send_json({"error": str(exc), "meta": {"status": "error", "query_seconds": round(time.time() - started_at, 3)}}, status=500)

    def _handle_argocd_app_status(self, query):
        try:
            self._send_json(argocd_integration.app_status(self._argocd_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_argocd_app_history(self, query):
        try:
            self._send_json(argocd_integration.app_history(self._argocd_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_argocd_diff_summary(self, query):
        try:
            self._send_json(argocd_integration.diff_summary(self._argocd_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_gitlab_recent_commits(self, query):
        try:
            self._send_json(gitlab_integration.recent_commits(self._gitlab_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_gitlab_commit_detail(self, query):
        try:
            self._send_json(gitlab_integration.commit_detail(self._gitlab_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_gitlab_pipeline_status(self, query):
        try:
            self._send_json(gitlab_integration.pipeline_status(self._gitlab_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_gitlab_release_context(self, query):
        try:
            self._send_json(gitlab_integration.release_context(self._gitlab_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_gitlab_merge_requests(self, query):
        try:
            self._send_json(gitlab_integration.merge_requests(self._gitlab_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_gitlab_tags(self, query):
        try:
            self._send_json(gitlab_integration.tags(self._gitlab_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_gitlab_artifacts(self, query):
        try:
            self._send_json(gitlab_integration.artifacts(self._gitlab_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_gitlab_image_digest_context(self, query):
        try:
            self._send_json(gitlab_integration.image_digest_context(self._gitlab_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _deep_observability_params(self, query):
        return self._build_search_params(
            query,
            (
                "service",
                "service_name",
                "namespace",
                "pod",
                "container",
                "node",
                "route",
                "http_route",
                "rule",
                "priority",
                "q",
                "rate_window",
                "limit",
                "profile_type",
                "profileTypeID",
                "label_selector",
                "labelSelector",
                "max_nodes",
            ),
        )

    def _handle_service_red_metrics(self, query):
        try:
            self._send_json(deep_observability.service_red_metrics(self._deep_observability_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_runtime_events_context(self, query):
        try:
            self._send_json(deep_observability.runtime_events_context(self._deep_observability_params(query)))
        except Exception as exc:
            status = 503 if "OPENSEARCH_URL is not configured" in str(exc) else 500
            self._send_json({"error": str(exc)}, status=status)

    def _handle_profile_hotspots(self, query):
        try:
            self._send_json(deep_observability.profile_hotspots(self._deep_observability_params(query)))
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_event_search(self, query):
        started_at = time.time()
        try:
            params = self._build_search_params(
                query,
                ("q", "cluster", "namespace", "pod", "reason", "type"),
            )
            payload = event_search.search_events(params)
            payload["meta"] = {
                "status": "ok",
                "query_seconds": round(time.time() - started_at, 3),
                "configured": opensearch_client.is_configured(),
                "item_count": len(payload.get("items") or []),
                "total": payload.get("total", 0),
            }
            self._send_json(payload)
        except Exception as exc:
            status = 503 if "OPENSEARCH_URL is not configured" in str(exc) else 500
            self._send_json(
                {
                    "error": str(exc),
                    "meta": {
                        "status": "error",
                        "query_seconds": round(time.time() - started_at, 3),
                        "configured": opensearch_client.is_configured(),
                    },
                },
                status=status,
            )

    def _handle_notification_settings_get(self):
        self._send_json({"settings": _notification_settings_payload()})

    def _handle_notification_settings_post(self):
        try:
            payload = self._read_json_body()
            settings = _normalize_notification_settings(payload)
            saved = _apply_notification_settings(settings)
            self._send_json({"ok": True, "settings": saved})
        except ValueError as exc:
            self._send_json({"error": str(exc)}, status=400)
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_dashboard_settings_get(self):
        self._send_json({"settings": _dashboard_settings_payload()})

    def _handle_dashboard_settings_post(self):
        try:
            payload = self._read_json_body()
            settings = _normalize_dashboard_settings(payload)
            saved = _apply_dashboard_settings(settings)
            self._send_json({"ok": True, "settings": saved})
        except ValueError as exc:
            self._send_json({"error": str(exc)}, status=400)
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_dashboard_settings_test_notification(self):
        try:
            payload = self._read_json_body()
            settings = _normalize_dashboard_settings(payload)
            notification = settings.get("notification", {})
            if not notification.get("webhook_url"):
                self._send_json({"error": "Webhook URL is required."}, status=400)
                return
            pod_restart_notify.send_events(
                [_build_test_event()],
                notification["webhook_url"],
                notification["webhook_type"],
            )
            self._send_json({"ok": True, "message": "Test notification sent."})
        except ValueError as exc:
            self._send_json({"error": str(exc)}, status=400)
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)

    def _handle_notification_settings_test(self):
        try:
            payload = self._read_json_body()
            settings = _normalize_notification_settings(payload)
            if not settings.get("webhook_url"):
                self._send_json({"error": "Webhook URL is required."}, status=400)
                return
            pod_restart_notify.send_events(
                [_build_test_event()],
                settings["webhook_url"],
                settings["webhook_type"],
            )
            self._send_json({"ok": True, "message": "Test notification sent."})
        except ValueError as exc:
            self._send_json({"error": str(exc)}, status=400)
        except Exception as exc:
            self._send_json({"error": str(exc)}, status=500)


def main():
    parser = argparse.ArgumentParser(description="Unified auto_inspection backend server")
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", default=8080, type=int)
    args = parser.parse_args()

    base_dir = str(PROJECT_ROOT)
    handler = lambda *handler_args, **handler_kwargs: Handler(
        *handler_args, directory=base_dir, **handler_kwargs
    )
    httpd = ThreadingHTTPServer((args.host, args.port), handler)
    print(f"[OK] Backend server running at http://{args.host}:{args.port}")
    httpd.serve_forever()


if __name__ == "__main__":
    main()
