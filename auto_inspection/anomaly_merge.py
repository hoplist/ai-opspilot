#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
anomaly_merge.py
将原子异常合并为“可行动事件”
"""

import json
import os
import time
from datetime import datetime

from auto_inspection import config
from auto_inspection import prom_resource_check

ANOMALIES_FILE = "data/anomalies.json"
HEALTH_FILE = "data/health_profiles.json"
OUTPUT_FILE = "data/events.json"

def load_json(path):
    if not os.path.exists(path):
        return {}
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def _to_float(value):
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def _pod_ready_ratio(pod):
    ready_count = _to_float(pod.get("ready_count"))
    ready_total = _to_float(pod.get("ready_total"))
    if ready_count is None or ready_total in (None, 0):
        return None
    return ready_count / ready_total


def _append_signal(signals, name):
    if name not in signals:
        signals.append(name)


def _pod_events():
    end_ts = int(time.time())
    hours = max(6, int(getattr(config, "RESOURCE_POD_RESTART_HOURS", 24) or 24))
    start_ts = end_ts - hours * 3600
    resource_data = prom_resource_check.collect_resource_data(
        getattr(config, "PROMETHEUS_URLS", []) or [],
        start_ts,
        end_ts,
    )
    pod_states = resource_data.get("pod_states") or []
    events = []

    for pod in pod_states:
        namespace = str(pod.get("namespace") or "").strip()
        pod_name = str(pod.get("pod") or "").strip()
        if not namespace or not pod_name:
            continue

        signals = []
        score = 0
        dominant = "restart"
        waiting_reason = str(pod.get("waiting_reason") or "").strip()
        terminated_reason = str(pod.get("terminated_reason") or "").strip()
        pending_reason = str(pod.get("pending_reason") or "").strip()
        phase = str(pod.get("phase") or "").strip()
        ready_ratio = _pod_ready_ratio(pod)
        restarts = _to_float(pod.get("restarts"))
        restarts_total = _to_float(pod.get("restarts_total"))
        mem_working = _to_float(pod.get("mem_working_set_bytes"))
        mem_limit = _to_float(pod.get("mem_limit_bytes"))
        mem_limit_ratio = (mem_working / mem_limit) if mem_working is not None and mem_limit not in (None, 0) else None

        if waiting_reason:
            _append_signal(signals, "waiting")
            score += 25
        if pending_reason:
            _append_signal(signals, "pending")
            score += 20
        if phase and phase not in {"Running", "Succeeded"}:
            _append_signal(signals, "phase")
            score += 15
        if ready_ratio is not None and ready_ratio < 1:
            _append_signal(signals, "not_ready")
            score += 20
        if restarts is not None and restarts > 0:
            _append_signal(signals, "restart")
            score += min(20, int(restarts * 5))
        if restarts_total is not None and restarts_total >= 3 and "restart" not in signals:
            _append_signal(signals, "restart")
            score += 10
        if terminated_reason:
            _append_signal(signals, "terminated")
            score += 10
        if terminated_reason == "OOMKilled" or pod.get("oom") or (mem_limit_ratio is not None and mem_limit_ratio >= 0.95):
            _append_signal(signals, "oom")
            _append_signal(signals, "mem")
            score += 35
            dominant = "mem"
        elif "restart" in signals:
            dominant = "restart"
        elif waiting_reason:
            dominant = "waiting"

        if not signals:
            continue

        if score >= 70:
            level = "critical"
        elif score >= 40:
            level = "high"
        else:
            level = "medium"

        events.append(
            {
                "object_type": "pod",
                "cluster": pod.get("cluster") or "",
                "namespace": namespace,
                "pod": pod_name,
                "service": pod.get("service") or "",
                "node_name": pod.get("node_name") or "",
                "instance": f"{namespace}/{pod_name}",
                "risk_level": level,
                "risk_score": score,
                "dominant_risk": dominant,
                "signals": signals,
                "health_score": None,
                "health_signals": None,
                "anomalies": [],
                "pod_state": pod,
            }
        )

    return events

def main():
    anomalies = load_json(ANOMALIES_FILE).get("anomalies", [])
    health_profiles = {
        p["instance"]: p
        for p in load_json(HEALTH_FILE).get("profiles", [])
    }

    events = {}

    # ===== 1. 按 instance 合并异常
    for a in anomalies:
        inst = a["instance"]
        metric = a["metric"]

        events.setdefault(inst, {
            "instance": inst,
            "metrics": [],
            "details": [],
        })

        events[inst]["metrics"].append(metric)
        events[inst]["details"].append(a)

    merged_events = []

    # ===== 2. 计算风险等级 & 主风险
    for inst, evt in events.items():
        metrics = evt["metrics"]
        unique_metrics = sorted(set(metrics))
        if not unique_metrics:
            continue

        # 计算综合风险分
        risk_score = sum(config.RISK_WEIGHT.get(m, 1) for m in unique_metrics)

        if risk_score >= 7:
            level = "critical"
        elif risk_score >= 4:
            level = "high"
        else:
            level = "medium"

        dominant = max(unique_metrics, key=lambda m: config.RISK_WEIGHT.get(m, 1))

        # 引入健康画像作为上下文（可选，但很值）
        health = health_profiles.get(inst, {})

        merged_events.append({
            "instance": inst,
            "object_type": "instance",
            "risk_level": level,
            "risk_score": risk_score,
            "dominant_risk": dominant,
            "signals": unique_metrics,
            "health_score": health.get("health_score"),
            "health_signals": health.get("signals"),
            "anomalies": evt["details"],
        })

    merged_events.extend(_pod_events())

    output = {
        "generated_at": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "event_count": len(merged_events),
        "events": sorted(
            merged_events,
            key=lambda x: (config.RISK_ORDER.index(x["risk_level"]), x["risk_score"]),
            reverse=True,
        ),
    }

    os.makedirs("data", exist_ok=True)
    with open(OUTPUT_FILE, "w", encoding="utf-8") as f:
        json.dump(output, f, indent=2, ensure_ascii=False)

    print(f"[OK] 异常合并完成：{OUTPUT_FILE}")
    print(f"     生成事件数：{len(merged_events)}")

if __name__ == "__main__":
    main()
