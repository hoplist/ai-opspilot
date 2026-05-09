#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
event_escalation.py
事件升级策略（持续 / 回归 / 多信号）
"""

import json
import os
from datetime import datetime

from auto_inspection import config

INPUT_FILE = "data/events_lifecycle.json"
HISTORY_FILE = "data/events_history.json"
OUTPUT_FILE = "data/events_escalated.json"

def load_json(path, default):
    if not os.path.exists(path):
        return default
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)

def save_json(path, data):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)

def upgrade(level):
    idx = config.RISK_ORDER.index(level)
    return config.RISK_ORDER[min(idx + 1, len(config.RISK_ORDER) - 1)]

def weeks_between(start, end):
    return max(1, (end - start).days // 7 + 1)

def main():
    now = datetime.now()

    lifecycle_data = load_json(INPUT_FILE, {}).get("events", [])
    history = load_json(HISTORY_FILE, {}).get("events", {})

    escalated = []

    for evt in lifecycle_data:
        level = evt.get("risk_level", "medium")
        reasons = []

        # ===== 1. 持续升级 =====
        if evt["lifecycle"] == "ongoing":
            first = datetime.strptime(evt["first_seen"], "%Y-%m-%d %H:%M:%S")
            weeks = weeks_between(first, now)

            if weeks >= config.ESCALATION_POLICY["ongoing_weeks_critical"]:
                if level != "critical":
                    level = "critical"
                    reasons.append(f"持续 {weeks} 周未恢复")

        # ===== 2. 回归升级 =====
        if evt["lifecycle"] == "new" and config.ESCALATION_POLICY["regression_boost"]:
            hist = history.get(evt["event_key"])
            if hist and hist.get("lifecycle") == "resolved":
                level = upgrade(level)
                reasons.append("问题回归（已修复后再次出现）")

        # ===== 3. 多信号升级 =====
        signals = evt.get("signals", [])
        if len(signals) >= config.ESCALATION_POLICY["multi_signal_threshold"]:
            new_level = upgrade(level)
            if new_level != level:
                level = new_level
                reasons.append(f"多信号叠加（{len(signals)} 项）")

        escalated.append({
            **evt,
            "final_risk_level": level,
            "escalation_reasons": reasons,
        })

    output = {
        "generated_at": now.strftime("%Y-%m-%d %H:%M:%S"),
        "event_count": len(escalated),
        "events": sorted(
            escalated,
            key=lambda x: config.RISK_ORDER.index(x["final_risk_level"]),
            reverse=True,
        ),
    }

    save_json(OUTPUT_FILE, output)

    print(f"[OK] 事件升级策略执行完成：{OUTPUT_FILE}")

if __name__ == "__main__":
    main()
