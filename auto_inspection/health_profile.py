#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
health_profile.py
基于 Prometheus 指标生成节点健康画像（Health Profile）
"""

import json
import os
from datetime import datetime

from auto_inspection import config
from auto_inspection import prometheus_client

TARGETS_FILE = "data/targets.json"
OUTPUT_FILE = "data/health_profiles.json"

# ==========
# 简化版信号分级（后面你可以无阈值升级）
# ==========

def classify(value, levels):
    for name, threshold in levels:
        if value >= threshold:
            return name
    return "normal"

def load_targets():
    with open(TARGETS_FILE, "r", encoding="utf-8") as f:
        return json.load(f)["instances"]

def main():
    os.makedirs("data", exist_ok=True)
    instances = load_targets()

    profiles = []

    # ===== 查询指标 =====
    cpu_data = prometheus_client.query_instant(
        config.PROMQL_CPU,
        url=config.PROMETHEUS_URL,
        timeout=15,
    )
    mem_data = prometheus_client.query_instant(
        config.PROMQL_MEM,
        url=config.PROMETHEUS_URL,
        timeout=15,
    )
    disk_data = prometheus_client.query_instant(
        config.PROMQL_DISK,
        url=config.PROMETHEUS_URL,
        timeout=15,
    )
    swap_data = prometheus_client.query_instant(
        config.PROMQL_SWAP_ACTIVE,
        url=config.PROMETHEUS_URL,
        timeout=15,
    )

    def map_by_instance(series):
        m = {}
        for s in series:
            inst = s["metric"].get("instance")
            if inst:
                m[inst] = float(s["value"][1])
        return m

    cpu_map = map_by_instance(cpu_data)
    mem_map = map_by_instance(mem_data)
    disk_map = map_by_instance(disk_data)
    swap_map = map_by_instance(swap_data)

    for inst in instances:
        cpu = cpu_map.get(inst, 0.0)
        mem = mem_map.get(inst, 0.0)
        disk = disk_map.get(inst, 0.0)
        swap = swap_map.get(inst, 0.0)

        signals = {
            "cpu_pressure": classify(cpu, [("critical", 0.8), ("high", 0.6)]),
            "memory_pressure": classify(mem, [("critical", 0.7), ("high", 0.5)]),
            "disk_pressure": classify(disk, [("critical", 0.9), ("high", 0.8)]),
            "swap_activity": "active" if swap > 0 else "normal",
        }

        score = 100
        for s in signals.values():
            if s == "high":
                score -= 15
            elif s == "critical":
                score -= 30

        dominant = max(signals, key=lambda k: (
            2 if signals[k] == "critical"
            else 1 if signals[k] == "high"
            else 0
        ))

        profiles.append({
            "instance": inst,
            "signals": signals,
            "health_score": max(score, 0),
            "dominant_risk": dominant,
        })

    output = {
        "generated_at": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "profiles": sorted(profiles, key=lambda x: x["health_score"]),
    }

    with open(OUTPUT_FILE, "w", encoding="utf-8") as f:
        json.dump(output, f, indent=2, ensure_ascii=False)

    print(f"[OK] 节点健康画像生成完成：{OUTPUT_FILE}")

if __name__ == "__main__":
    main()
