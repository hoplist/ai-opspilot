#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
event_lifecycle.py
为异常事件打生命周期状态（new / ongoing / resolved）
"""

import json
import os
from datetime import datetime, timedelta

from auto_inspection import config
from auto_inspection import source_context

EVENTS_FILE = "data/events.json"
HISTORY_FILE = "data/events_history.json"
OUTPUT_FILE = "data/events_lifecycle.json"


def load_json(path, default):
    if not os.path.exists(path):
        return default
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def save_json(path, data):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)


def make_event_key(evt):
    return f"{evt['instance']}:{evt['dominant_risk']}"


def prune_history(history, now_dt, retention_days):
    if retention_days <= 0:
        return history, 0
    cutoff = now_dt - timedelta(days=retention_days)
    kept = {}
    pruned = 0
    for key, item in history.items():
        last_seen_str = item.get("last_seen")
        try:
            last_seen = datetime.strptime(last_seen_str, "%Y-%m-%d %H:%M:%S")
        except Exception:
            kept[key] = item
            continue
        if last_seen < cutoff:
            pruned += 1
        else:
            kept[key] = item
    return kept, pruned


def main():
    now_dt = datetime.now()
    now = now_dt.strftime("%Y-%m-%d %H:%M:%S")
    source = source_context.source_metadata()

    current = load_json(EVENTS_FILE, {}).get("events", [])
    history_payload = load_json(HISTORY_FILE, {})
    history = history_payload.get("events", {})
    history_source = ((history_payload or {}).get("source") or {}).get("fingerprint")
    if history_source and history_source != source["fingerprint"]:
        history = {}

    current_keys = set()
    lifecycle_events = []

    # ==========
    # 本周期事件
    # ==========
    for evt in current:
        key = make_event_key(evt)
        current_keys.add(key)

        if key not in history:
            status = "new"
            first_seen = now
        else:
            status = "ongoing"
            first_seen = history[key]["first_seen"]

        lifecycle_events.append({
            **evt,
            "event_key": key,
            "lifecycle": status,
            "first_seen": first_seen,
            "last_seen": now,
        })

        # 更新历史
        history[key] = {
            "instance": evt["instance"],
            "dominant_risk": evt["dominant_risk"],
            "first_seen": first_seen,
            "last_seen": now,
            "lifecycle": status,
        }

    # ==========
    # 已恢复事件
    # ==========
    for key, old in history.items():
        if key not in current_keys:
            lifecycle_events.append({
                "event_key": key,
                "instance": old["instance"],
                "dominant_risk": old["dominant_risk"],
                "lifecycle": "resolved",
                "first_seen": old["first_seen"],
                "last_seen": now,
            })

            # 标记为 resolved（但保留在历史中）
            history[key]["lifecycle"] = "resolved"
            history[key]["last_seen"] = now

    output = {
        "generated_at": now,
        "source": source,
        "event_count": len(lifecycle_events),
        "events": lifecycle_events,
    }

    history, pruned = prune_history(
        history,
        now_dt,
        config.HISTORY_RETENTION_DAYS,
    )

    save_json(OUTPUT_FILE, output)
    save_json(HISTORY_FILE, {"source": source, "events": history})

    print(f"[OK] 事件生命周期生成完成：{OUTPUT_FILE}")
    print(f"     new / ongoing / resolved 已标注")
    if pruned:
        print(f"     history 已清理：{pruned}")

if __name__ == "__main__":
    main()
