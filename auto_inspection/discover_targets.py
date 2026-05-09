#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
discover_targets.py
从 Prometheus 自动发现巡检对象（instance / job）
"""

import json
import os
from datetime import datetime

from auto_inspection import config
from auto_inspection import prometheus_client
from auto_inspection import source_context

OUTPUT_FILE = "data/targets.json"

def main():
    os.makedirs("data", exist_ok=True)
    source, changed = source_context.ensure_current_source_state()

    instances = prometheus_client.label_values(
        "instance",
        url=config.PROMETHEUS_URL,
        timeout=15,
    )
    jobs = prometheus_client.label_values(
        "job",
        url=config.PROMETHEUS_URL,
        timeout=15,
    )

    result = {
        "generated_at": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "source": source,
        "instances": sorted(instances),
        "jobs": sorted(jobs),
    }

    with open(OUTPUT_FILE, "w", encoding="utf-8") as f:
        json.dump(result, f, indent=2, ensure_ascii=False)

    print(f"[OK] 发现巡检对象完成：{OUTPUT_FILE}")
    print(f"     instance 数量：{len(instances)}")
    print(f"     job 数量：{len(jobs)}")

if __name__ == "__main__":
    main()
