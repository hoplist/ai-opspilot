#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
Lightweight read-only Snapshot Index.

This module precomputes compact TopN signals from live backend data so UI, MCP,
and Codex can consume the same ranked signal set without re-deriving it.
"""

import time

from auto_inspection import business_correlation
from auto_inspection import config
from auto_inspection import context_pack
from auto_inspection import log_search
from auto_inspection import prom_resource_check
from auto_inspection import release_changes


def _safe_int(value, default, minimum=1, maximum=200):
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        parsed = default
    return max(minimum, min(maximum, parsed))


def _now_ts():
    return int(time.time())


def _severity_rank(value):
    return {"alert": 0, "watch": 1, "info": 2, "safe": 3, "unknown": 4}.get(str(value or ""), 9)


def _pod_name(row):
    return row.get("pod") or row.get("instance") or "-"


def _top_resource_signals(resources_payload, limit):
    pod_rows = [item for item in (resources_payload or {}).get("items", []) if item.get("metric") == "pod"]
    signals = context_pack.resource_signals(resources_payload, pod_rows)
    signals = sorted(signals, key=lambda item: (_severity_rank(item.get("severity")), -(abs(item.get("delta") or 0))))
    return signals[:limit]


def _abnormal_pods(resources_payload, limit):
    rows = []
    for row in (resources_payload or {}).get("items", []):
        if row.get("metric") != "pod":
            continue
        signals = context_pack.resource_signals(resources_payload, [row])
        if not signals and str(row.get("usage_zone") or "") not in {"alert", "watch"}:
            continue
        rows.append(
            {
                "pod": _pod_name(row),
                "namespace": str(row.get("instance") or "").split("/", 1)[0] if "/" in str(row.get("instance") or "") else "",
                "node": row.get("node_name"),
                "usage_zone": row.get("usage_zone"),
                "cpu_usage_ratio": row.get("cpu_usage_ratio"),
                "mem_usage_ratio": row.get("mem_usage_ratio") or row.get("mem_ratio"),
                "restarts": row.get("restarts_total") or row.get("restarts"),
                "terminated_reason": row.get("terminated_reason"),
                "signals": signals[:5],
            }
        )
    rows.sort(
        key=lambda item: (
            min([_severity_rank(signal.get("severity")) for signal in item.get("signals") or []] or [_severity_rank(item.get("usage_zone"))]),
            -max([abs(signal.get("delta") or 0) for signal in item.get("signals") or []] or [0]),
        )
    )
    return rows[:limit]


def _release_topn(namespace, range_hours, limit, errors):
    if not namespace:
        return []
    try:
        end_ts = _now_ts()
        payload = release_changes.recent_changes(
            {
                "namespace": namespace,
                "start_ts": end_ts - range_hours * 3600,
                "end_ts": end_ts,
                "range_hours": range_hours,
                "limit": limit,
            }
        )
        if payload.get("errors"):
            errors.extend({"source": "recent_changes", **error} for error in payload.get("errors") or [])
        return (payload.get("items") or [])[:limit]
    except Exception as exc:
        errors.append({"source": "recent_changes", "message": str(exc)})
        return []


def _business_error_topn(namespace, range_hours, limit, errors):
    try:
        end_ts = _now_ts()
        payload = business_correlation.search_business_logs(
            {
                "namespace": namespace,
                "q": "error OR exception OR failed OR fatal OR panic",
                "start_ts": end_ts - range_hours * 3600,
                "end_ts": end_ts,
                "range_hours": range_hours,
                "size": min(limit * 5, 100),
            }
        )
        buckets = {}
        for item in payload.get("items") or []:
            key = item.get("service") or item.get("workload_name") or item.get("pod") or item.get("namespace") or "unknown"
            bucket = buckets.setdefault(key, {"key": key, "count": 0, "examples": []})
            bucket["count"] += 1
            if len(bucket["examples"]) < 3:
                bucket["examples"].append(item)
        rows = sorted(buckets.values(), key=lambda item: item.get("count") or 0, reverse=True)
        return rows[:limit]
    except Exception as exc:
        errors.append({"source": "business_errors", "message": str(exc)})
        return []


def enrich_resource_payload(resources_payload, limit=20):
    resources_payload.setdefault("summary", {})
    resources_payload["summary"]["top_signals"] = _top_resource_signals(resources_payload, limit)
    return resources_payload


def build_snapshot_index(params=None):
    data = params if isinstance(params, dict) else {}
    namespace = str(data.get("namespace") or "").strip()
    range_hours = _safe_int(data.get("range_hours"), 24, 1, 168)
    limit = _safe_int(data.get("limit"), 10, 1, 100)
    end_ts = _now_ts()
    start_ts = end_ts - range_hours * 3600
    errors = []
    resources_payload = None
    try:
        resources_payload = prom_resource_check.build_dashboard_payload(
            prom_resource_check.collect_resource_data(config.PROMETHEUS_URLS, start_ts, end_ts),
            start_ts,
            end_ts,
        )
        enrich_resource_payload(resources_payload, limit=limit * 2)
    except Exception as exc:
        errors.append({"source": "resources", "message": str(exc)})
        resources_payload = {"items": [], "summary": {"top_signals": []}}

    return {
        "mode": "read_only_snapshot_index",
        "window": {"start_ts": start_ts, "end_ts": end_ts, "range_hours": range_hours},
        "request": {"namespace": namespace, "limit": limit},
        "items": {
            "abnormal_pods": _abnormal_pods(resources_payload, limit),
            "resource_trends": (resources_payload.get("summary", {}) or {}).get("top_signals", [])[:limit],
            "recent_changes": _release_topn(namespace, range_hours, limit, errors),
            "business_errors": _business_error_topn(namespace, range_hours, limit, errors),
        },
        "errors": errors,
        "safety": {
            "server_commands": "not_allowed",
            "kubernetes_mutations": "not_allowed",
        },
    }
