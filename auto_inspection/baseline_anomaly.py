#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
baseline_anomaly.py
基于历史基线的无阈值异常检测
"""

import json
import os
from datetime import datetime

from auto_inspection import config
from auto_inspection import prometheus_client

BASELINE_DIR = "data/baseline"
OUTPUT_FILE = "data/anomalies.json"

METRICS = {
    "cpu": config.PROMQL_CPU,
    "mem": config.PROMQL_MEM,
    "disk": config.PROMQL_DISK,
}

def main():
    anomalies = []

    for metric, promql in METRICS.items():
        baseline_file = f"{BASELINE_DIR}/{metric}.json"
        if not os.path.exists(baseline_file):
            continue

        with open(baseline_file, "r", encoding="utf-8") as f:
            baseline = json.load(f)["baseline"]

        current = prometheus_client.query_instant(
            promql,
            url=config.PROMETHEUS_URL,
            timeout=20,
        )
        for s in current:
            inst = s["metric"].get("instance")
            value = float(s["value"][1])

            if inst not in baseline:
                continue

            base = baseline[inst]
            p95 = base["p95"]

            if p95 <= 0:
                continue

            deviation = (value - p95) / p95

            if deviation > config.ANOMALY_DEVIATION_RATIO:   # 不是阈值，是“显著偏离”
                anomalies.append({
                    "instance": inst,
                    "metric": metric,
                    "current": round(value, 4),
                    "baseline_p95": round(p95, 4),
                    "deviation_ratio": round(deviation, 2),
                })

    with open(OUTPUT_FILE, "w", encoding="utf-8") as f:
        json.dump(
            {
                "generated_at": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
                "anomalies": anomalies,
            },
            f,
            indent=2,
            ensure_ascii=False,
        )

    print(f"[OK] 基线异常检测完成：{OUTPUT_FILE}")

if __name__ == "__main__":
    main()
