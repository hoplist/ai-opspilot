#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
baseline_builder.py
为每个 instance 构建历史基线（P50 / P95）
"""

import json
import os
import math
import statistics
from datetime import datetime, timedelta

from auto_inspection import config
from auto_inspection import prometheus_client
from auto_inspection import stats

TARGETS_FILE = "data/targets.json"
BASELINE_DIR = "data/baseline"

METRICS = {
    "cpu": config.PROMQL_CPU,
    "mem": config.PROMQL_MEM,
    "disk": config.PROMQL_DISK,
}

def main():
    os.makedirs(BASELINE_DIR, exist_ok=True)

    with open(TARGETS_FILE, "r", encoding="utf-8") as f:
        instances = json.load(f)["instances"]

    end = int(datetime.now().timestamp())
    start = int((datetime.now() - timedelta(days=config.BASELINE_HISTORY_DAYS)).timestamp())
    step = max(config.BASELINE_STEP, math.ceil((end - start) / config.BASELINE_MAX_POINTS))

    for metric, promql in METRICS.items():
        baseline = {}

        series = prometheus_client.query_range(
            promql,
            start,
            end,
            step,
            url=config.PROMETHEUS_URL,
            timeout=60,
        )
        for s in series:
            inst = s["metric"].get("instance")
            if inst not in instances:
                continue
            values = [float(v) for _, v in s["values"] if v not in ("NaN", None)]
            if not values:
                continue

            baseline[inst] = {
                "p50": stats.percentile(values, 0.5),
                "p95": stats.percentile(values, 0.95),
                "mean": statistics.mean(values),
            }

        with open(f"{BASELINE_DIR}/{metric}.json", "w", encoding="utf-8") as f:
            json.dump(
                {
                    "generated_at": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
                    "days": config.BASELINE_HISTORY_DAYS,
                    "metric": metric,
                    "baseline": baseline,
                },
                f,
                indent=2,
                ensure_ascii=False,
            )

        print(f"[OK] 基线生成完成：{metric}")

if __name__ == "__main__":
    main()
