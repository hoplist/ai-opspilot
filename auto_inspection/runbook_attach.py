#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
runbook_attach.py
为最终事件自动绑定 Runbook 行动建议
"""

import json
import os
from datetime import datetime

from auto_inspection import config
from auto_inspection import incident_store
from auto_inspection import source_context

EVENTS_FILE = "data/events_escalated.json"
OUTPUT_FILE = "data/events_with_runbook.json"

DEFAULT_RUNBOOK = {
    "title": "通用排查",
    "checks": ["检查系统状态", "查看近期变更"],
    "analysis": ["确认是否为已知问题"],
    "actions": ["根据实际情况处理"],
}


def load_json(path, default=None):
    if not os.path.exists(path):
        return default
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def normalize_signals(signals):
    return sorted({s for s in signals if isinstance(s, str) and s.strip()})


def select_runbook(evt, runbooks):
    if not isinstance(runbooks, dict):
        return DEFAULT_RUNBOOK

    default = runbooks.get("default", DEFAULT_RUNBOOK)
    by_combo = runbooks.get("by_signal_combo", {})
    by_dominant = runbooks.get("by_dominant_risk", {})
    by_signal = runbooks.get("by_signal", {})
    by_level = runbooks.get("by_final_risk_level", {})

    signals = normalize_signals(evt.get("signals", []))
    if signals:
        combo_key = "+".join(signals)
        if combo_key in by_combo:
            return by_combo[combo_key]

    dominant = evt.get("dominant_risk")
    if dominant in by_dominant:
        return by_dominant[dominant]

    if signals:
        for sig in sorted(
            signals,
            key=lambda s: config.RISK_WEIGHT.get(s, 0),
            reverse=True,
        ):
            if sig in by_signal:
                return by_signal[sig]

    level = evt.get("final_risk_level") or evt.get("risk_level")
    if level in by_level:
        return by_level[level]

    return default


def main():
    events = load_json(EVENTS_FILE, {}).get("events", [])
    runbooks = load_json(config.RUNBOOK_FILE, {})

    enriched = []

    for evt in events:
        rb = select_runbook(evt, runbooks)
        enriched.append({
            **evt,
            "runbook": rb or DEFAULT_RUNBOOK,
        })

    output = {
        "generated_at": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "source": source_context.source_metadata(),
        "event_count": len(enriched),
        "events": enriched
    }

    with open(OUTPUT_FILE, "w", encoding="utf-8") as f:
        json.dump(output, f, indent=2, ensure_ascii=False)

    try:
        sync_result = incident_store.sync_events(output)
        if sync_result.get("indexed"):
            print(
                f"[OK] incidents synced to OpenSearch: index={sync_result['index']} count={sync_result['count']}"
            )
    except Exception as exc:
        print(f"[WARN] incident sync skipped: {exc}")

    print(f"[OK] Runbook 已绑定：{OUTPUT_FILE}")


if __name__ == "__main__":
    main()
