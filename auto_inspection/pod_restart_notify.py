#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import json
import math
import os
from datetime import datetime

from auto_inspection import config
from auto_inspection import prom_resource_check
from auto_inspection.http_client import request_data
from auto_inspection.paths import PROJECT_ROOT


def _resolve_path(path):
    if not path:
        return ""
    if os.path.isabs(path):
        return path
    return os.path.join(str(PROJECT_ROOT), path)


def _load_state(path):
    resolved = _resolve_path(path)
    if not resolved or not os.path.exists(resolved):
        return {"pods": {}}
    try:
        with open(resolved, "r", encoding="utf-8") as f:
            data = json.load(f)
        if isinstance(data, dict) and isinstance(data.get("pods"), dict):
            return data
    except (OSError, json.JSONDecodeError):
        pass
    return {"pods": {}}


def _save_state(path, state):
    resolved = _resolve_path(path)
    if not resolved:
        return
    directory = os.path.dirname(resolved)
    if directory:
        os.makedirs(directory, exist_ok=True)
    with open(resolved, "w", encoding="utf-8") as f:
        json.dump(state, f, ensure_ascii=False, indent=2)


def _normalize_text(value):
    return str(value or "").strip()


def _normalize_total(value):
    try:
        value = float(value)
    except (TypeError, ValueError):
        return 0
    if value <= 0:
        return 0
    return int(round(value))


def _normalize_delta(value):
    try:
        value = float(value)
    except (TypeError, ValueError):
        return 0
    if value <= 0:
        return 0
    return int(math.ceil(value - 1e-9))


def _fmt_time(value):
    try:
        ts = float(value)
    except (TypeError, ValueError):
        return "-"
    if ts <= 0:
        return "-"
    return datetime.fromtimestamp(ts).strftime("%Y-%m-%d %H:%M:%S")


def _split_pod_instance(value):
    text = _normalize_text(value)
    if not text:
        return "", ""
    if "/" not in text:
        return "", text
    namespace, pod = text.split("/", 1)
    return namespace.strip(), pod.strip()


def _parse_targets(targets):
    parsed = set()
    for raw in targets or []:
        text = _normalize_text(raw)
        if text:
            parsed.add(text)
    return parsed


def _build_pod_key(item):
    cluster = _normalize_text(item.get("cluster")) or "-"
    namespace = _normalize_text(item.get("namespace"))
    pod = _normalize_text(item.get("pod"))
    return f"{cluster}|{namespace}|{pod}"


def _match_target(item, targets):
    if not targets:
        return False
    cluster = _normalize_text(item.get("cluster")) or "-"
    namespace = _normalize_text(item.get("namespace"))
    pod = _normalize_text(item.get("pod"))
    candidates = {pod}
    if namespace and pod:
        candidates.add(f"{namespace}/{pod}")
    if cluster and namespace and pod:
        candidates.add(f"{cluster}/{namespace}/{pod}")
    return any(candidate in targets for candidate in candidates)


def extract_monitored_pods(resource_data, start_ts, end_ts, targets):
    parsed_targets = _parse_targets(targets)
    if not parsed_targets:
        return []
    monitored = []
    pod_states = resource_data.get("pod_states") or []
    if pod_states:
        for item in pod_states:
            namespace = _normalize_text(item.get("namespace"))
            pod = _normalize_text(item.get("pod"))
            pod_item = {
                "cluster": _normalize_text(item.get("cluster")) or "-",
                "namespace": namespace,
                "pod": pod,
                "instance": _normalize_text(item.get("instance")) or f"{namespace}/{pod}".strip("/"),
                "node_name": _normalize_text(item.get("node_name")),
                "phase": _normalize_text(item.get("phase")),
                "pod_status": _normalize_text(item.get("pod_status")),
                "terminated_reason": _normalize_text(item.get("terminated_reason")),
                "terminated_exitcode": item.get("terminated_exitcode"),
                "last_terminated_time": item.get("last_terminated_time"),
                "restarts": item.get("restarts"),
                "restarts_total": item.get("restarts_total"),
                "restart_window_hours": item.get("restart_window_hours"),
            }
            if _match_target(pod_item, parsed_targets):
                monitored.append(pod_item)
    else:
        payload = prom_resource_check.build_dashboard_payload(resource_data, start_ts, end_ts)
        for item in payload.get("items", []) or []:
            if item.get("metric") != "pod":
                continue
            namespace, pod = _split_pod_instance(item.get("instance"))
            pod_item = {
                "cluster": _normalize_text(item.get("cluster")) or "-",
                "namespace": namespace,
                "pod": pod,
                "instance": _normalize_text(item.get("instance")),
                "node_name": _normalize_text(item.get("node_name")),
                "phase": _normalize_text(item.get("phase")),
                "pod_status": _normalize_text(item.get("pod_status")),
                "terminated_reason": _normalize_text(item.get("terminated_reason")),
                "terminated_exitcode": item.get("terminated_exitcode"),
                "last_terminated_time": item.get("last_terminated_time"),
                "restarts": item.get("restarts"),
                "restarts_total": item.get("restarts_total"),
                "restart_window_hours": item.get("restart_window_hours"),
            }
            if _match_target(pod_item, parsed_targets):
                monitored.append(pod_item)
    monitored.sort(key=lambda item: (item["cluster"], item["namespace"], item["pod"]))
    return monitored


def _build_state_entry(item, now_str, *, last_notified_total):
    current_total = _normalize_total(item.get("restarts_total"))
    return {
        "cluster": item.get("cluster") or "-",
        "namespace": item.get("namespace") or "",
        "pod": item.get("pod") or "",
        "instance": item.get("instance") or "",
        "node_name": item.get("node_name") or "",
        "phase": item.get("phase") or "",
        "pod_status": item.get("pod_status") or "",
        "terminated_reason": item.get("terminated_reason") or "",
        "terminated_exitcode": item.get("terminated_exitcode"),
        "last_terminated_time": item.get("last_terminated_time"),
        "restart_window_hours": item.get("restart_window_hours"),
        "last_seen_total": current_total,
        "last_seen_at": now_str,
        "last_notified_total": max(0, int(last_notified_total)),
    }


def detect_restart_events(monitored_pods, state, now_str):
    state = state if isinstance(state, dict) else {"pods": {}}
    pods_state = state.setdefault("pods", {})
    events = []

    for item in monitored_pods:
        key = _build_pod_key(item)
        current_total = _normalize_total(item.get("restarts_total"))
        recent_restarts = _normalize_delta(item.get("restarts"))
        current_entry = pods_state.get(key)

        if not isinstance(current_entry, dict):
            pods_state[key] = _build_state_entry(
                item,
                now_str,
                last_notified_total=current_total,
            )
            continue

        last_notified_total = _normalize_total(current_entry.get("last_notified_total"))
        counter_reset = current_total < last_notified_total
        restart_delta = 0
        if current_total > last_notified_total:
            restart_delta = current_total - last_notified_total
        elif counter_reset and recent_restarts > 0:
            restart_delta = recent_restarts

        pods_state[key] = _build_state_entry(
            item,
            now_str,
            last_notified_total=last_notified_total,
        )

        if restart_delta <= 0:
            continue

        events.append(
            {
                "state_key": key,
                "cluster": item.get("cluster") or "-",
                "namespace": item.get("namespace") or "",
                "pod": item.get("pod") or "",
                "instance": item.get("instance") or "",
                "node_name": item.get("node_name") or "",
                "phase": item.get("phase") or "",
                "pod_status": item.get("pod_status") or "",
                "terminated_reason": item.get("terminated_reason") or "",
                "terminated_exitcode": item.get("terminated_exitcode"),
                "last_terminated_time": item.get("last_terminated_time"),
                "restart_window_hours": item.get("restart_window_hours"),
                "restarts_recent": recent_restarts,
                "restarts_total": current_total,
                "restart_delta": restart_delta,
                "counter_reset": counter_reset,
                "detected_at": now_str,
            }
        )

    state["updated_at"] = now_str
    return events, state


def mark_events_notified(state, events, now_str):
    pods_state = (state or {}).setdefault("pods", {})
    for event in events or []:
        current_entry = pods_state.get(event.get("state_key"))
        if not isinstance(current_entry, dict):
            continue
        current_entry["last_notified_total"] = _normalize_total(event.get("restarts_total"))
        current_entry["last_notified_at"] = now_str
    state["updated_at"] = now_str
    return state


def format_notification_message(events):
    lines = [f"Pod restart detected: {len(events)}"]
    for event in events:
        title = "/".join(
            [
                part
                for part in (
                    event.get("cluster"),
                    event.get("namespace"),
                    event.get("pod"),
                )
                if part
            ]
        )
        lines.append(
            f"- {title}: +{event.get('restart_delta', 0)} restart(s), total={event.get('restarts_total', 0)}"
        )
        lines.append(
            f"  phase={event.get('phase') or '-'}, node={event.get('node_name') or '-'}, window={event.get('restart_window_hours') or '-'}h"
        )
        lines.append(
            f"  terminated={event.get('terminated_reason') or '-'}, exit={event.get('terminated_exitcode') if event.get('terminated_exitcode') is not None else '-'}, last={_fmt_time(event.get('last_terminated_time'))}"
        )
        if event.get("counter_reset"):
            lines.append("  note=restart counter reset after pod recreation")
    return "\n".join(lines)


def build_webhook_payload(events, webhook_type):
    message = format_notification_message(events)
    normalized_type = _normalize_text(webhook_type).lower() or "generic"
    if normalized_type == "feishu":
        return {"msg_type": "text", "content": {"text": message}}
    if normalized_type in {"wecom", "dingtalk"}:
        return {"msgtype": "text", "text": {"content": message}}
    return {
        "event": "pod_restart",
        "generated_at": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "count": len(events),
        "text": message,
        "items": events,
    }


def send_events(events, webhook_url, webhook_type):
    if not events:
        return None
    payload = build_webhook_payload(events, webhook_type)
    return request_data(
        "POST",
        webhook_url,
        payload=payload,
        timeout=20,
        retries=config.REQUEST_RETRIES,
        backoff_seconds=config.REQUEST_BACKOFF_SECONDS,
        expect_json=False,
    )


def process_resource_data(
    resource_data,
    start_ts,
    end_ts,
    *,
    enabled=None,
    webhook_url=None,
    webhook_type=None,
    targets=None,
    state_file=None,
):
    enabled = config.POD_RESTART_NOTIFY_ENABLED if enabled is None else enabled
    webhook_url = config.POD_RESTART_NOTIFY_WEBHOOK_URL if webhook_url is None else webhook_url
    webhook_type = config.POD_RESTART_NOTIFY_WEBHOOK_TYPE if webhook_type is None else webhook_type
    targets = config.POD_RESTART_NOTIFY_TARGETS if targets is None else targets
    state_file = config.POD_RESTART_NOTIFY_STATE_FILE if state_file is None else state_file

    result = {
        "enabled": bool(enabled),
        "matched": 0,
        "sent": 0,
        "skipped": 0,
        "events": [],
    }
    if not enabled:
        return result

    parsed_targets = _parse_targets(targets)
    if not parsed_targets or not _normalize_text(webhook_url):
        result["skipped"] = 1
        return result

    monitored = extract_monitored_pods(resource_data, start_ts, end_ts, parsed_targets)
    result["matched"] = len(monitored)

    now_str = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    state = _load_state(state_file)
    events, next_state = detect_restart_events(monitored, state, now_str)
    _save_state(state_file, next_state)

    if not events:
        return result

    send_events(events, webhook_url, webhook_type)
    notified_state = mark_events_notified(next_state, events, now_str)
    _save_state(state_file, notified_state)

    result["sent"] = len(events)
    result["events"] = events
    return result
