#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import json
import os
import time
from datetime import datetime

from auto_inspection import config
from auto_inspection import prom_alert_summary
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
        return {"alerts": {}}
    try:
        with open(resolved, "r", encoding="utf-8") as f:
            data = json.load(f)
        if isinstance(data, dict) and isinstance(data.get("alerts"), dict):
            return data
    except (OSError, json.JSONDecodeError):
        pass
    return {"alerts": {}}


def _save_state(path, state):
    resolved = _resolve_path(path)
    if not resolved:
        return
    directory = os.path.dirname(resolved)
    if directory:
        os.makedirs(directory, exist_ok=True)
    with open(resolved, "w", encoding="utf-8") as f:
        json.dump(state, f, ensure_ascii=False, indent=2)


def _alert_key(row):
    return "|".join([str(row.get("category") or ""), str(row.get("object") or "")])


def _filter_rows(rows, min_hours):
    filtered = []
    for row in rows or []:
        try:
            hours = float(row.get("hours") or 0)
        except (TypeError, ValueError):
            hours = 0
        if hours >= min_hours:
            filtered.append(row)
    return filtered


def select_notification_rows(rows, state, now_ts, cooldown_seconds, min_hours):
    state = state if isinstance(state, dict) else {"alerts": {}}
    alert_state = state.setdefault("alerts", {})
    selected = []
    active_keys = set()
    for row in _filter_rows(rows, min_hours):
        key = _alert_key(row)
        active_keys.add(key)
        previous = alert_state.get(key) if isinstance(alert_state.get(key), dict) else {}
        last_sent_at = float(previous.get("last_sent_at") or 0)
        should_send = last_sent_at <= 0 or (now_ts - last_sent_at) >= cooldown_seconds
        alert_state[key] = {
            "category": row.get("category"),
            "object": row.get("object"),
            "hours": row.get("hours"),
            "last_seen_at": now_ts,
            "last_sent_at": previous.get("last_sent_at") or 0,
            "send_count": int(previous.get("send_count") or 0),
            "status": "firing",
        }
        if should_send:
            selected.append(row)
            alert_state[key]["last_sent_at"] = now_ts
            alert_state[key]["send_count"] += 1
    for key, item in list(alert_state.items()):
        if key not in active_keys and isinstance(item, dict):
            item["status"] = "resolved"
            item["resolved_at"] = now_ts
    state["updated_at"] = now_ts
    return selected, state


def format_message(rows, window):
    lines = [f"RCA alert summary: {len(rows)} firing item(s)"]
    if window:
        lines.append(f"window={window.get('start') or '-'} ~ {window.get('end') or '-'}")
    for row in rows:
        lines.append(f"- {row.get('category')}: {row.get('object')}持续 {float(row.get('hours') or 0):.2f}h")
    lines.append("RCA: http://192.168.48.200:32180/api/alerts?range_hours=1")
    return "\n".join(lines)


def build_webhook_payload(rows, window, webhook_type):
    message = format_message(rows, window)
    normalized_type = str(webhook_type or "generic").strip().lower()
    if normalized_type == "feishu":
        return {"msg_type": "text", "content": {"text": message}}
    if normalized_type in {"wecom", "dingtalk"}:
        return {"msgtype": "text", "text": {"content": message}}
    return {
        "event": "alert_summary",
        "generated_at": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "count": len(rows),
        "text": message,
        "items": rows,
    }


def send_rows(rows, window, webhook_url, webhook_type):
    if not rows:
        return None
    payload = build_webhook_payload(rows, window, webhook_type)
    return request_data(
        "POST",
        webhook_url,
        payload=payload,
        timeout=20,
        retries=config.REQUEST_RETRIES,
        backoff_seconds=config.REQUEST_BACKOFF_SECONDS,
        expect_json=False,
    )


def process_alerts(
    *,
    enabled=None,
    webhook_url=None,
    webhook_type=None,
    state_file=None,
    range_hours=None,
    cooldown_seconds=None,
    min_hours=None,
    dry_run=False,
):
    enabled = config.ALERT_NOTIFY_ENABLED if enabled is None else bool(enabled)
    webhook_url = config.ALERT_NOTIFY_WEBHOOK_URL if webhook_url is None else webhook_url
    webhook_type = config.ALERT_NOTIFY_WEBHOOK_TYPE if webhook_type is None else webhook_type
    state_file = config.ALERT_NOTIFY_STATE_FILE if state_file is None else state_file
    range_hours = int(range_hours or config.ALERT_NOTIFY_RANGE_HOURS or 1)
    cooldown_seconds = int(cooldown_seconds or config.ALERT_NOTIFY_COOLDOWN_SECONDS or 1800)
    min_hours = float(config.ALERT_NOTIFY_MIN_HOURS if min_hours is None else min_hours)
    end_ts = int(time.time())
    start_ts = end_ts - range_hours * 3600

    rows = prom_alert_summary.collect_alert_rows(config.PROMETHEUS_URLS, start_ts, end_ts)
    window = {
        "start_ts": start_ts,
        "end_ts": end_ts,
        "start": datetime.fromtimestamp(start_ts).strftime("%Y-%m-%d %H:%M:%S"),
        "end": datetime.fromtimestamp(end_ts).strftime("%Y-%m-%d %H:%M:%S"),
    }
    state = _load_state(state_file)
    selected, next_state = select_notification_rows(rows, state, end_ts, cooldown_seconds, min_hours)
    if not dry_run:
        _save_state(state_file, next_state)

    result = {
        "enabled": enabled,
        "dry_run": bool(dry_run),
        "window": window,
        "firing_count": len(rows),
        "selected_count": len(selected),
        "sent": 0,
        "skipped": 0,
        "items": selected,
    }
    if not enabled or not str(webhook_url or "").strip():
        result["skipped"] = 1
        return result
    if not selected:
        return result
    if not dry_run:
        send_rows(selected, window, webhook_url, webhook_type)
        result["sent"] = len(selected)
    return result
