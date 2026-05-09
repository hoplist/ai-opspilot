#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
weekly_inspection.py
Prometheus 巡检总控（告警 + 资源 + AI，总时间窗口一致）
"""

from datetime import datetime, date
import os
import traceback
import importlib

from auto_inspection import config

OUTPUT_MD = "outputs/reports/weekly_report.md"

def safe_run(title, func, *args, **kwargs):
    try:
        return func(*args, **kwargs)
    except Exception as e:
        tb = traceback.format_exc(limit=5)
        return (
            f"⚠️ **{title} 生成失败**：{e}\n\n"
            f"```text\n{tb}\n```\n"
        )

def h2(t):
    return f"## {t}\n"

def main():
    # =========================
    # 时间窗口（唯一真源）
    # =========================
    now_dt   = datetime.now()
    end_ts   = int(now_dt.timestamp())
    start_ts = end_ts - config.RANGE_DAYS * 86400

    start_str = datetime.fromtimestamp(start_ts).strftime("%Y-%m-%d %H:%M:%S")
    end_str   = datetime.fromtimestamp(end_ts).strftime("%Y-%m-%d %H:%M:%S")
    week      = date.today().strftime("%Y-W%U")

    # =========================
    # 模块加载
    # =========================
    alert_mod = importlib.import_module("auto_inspection.prom_alert_summary")
    res_mod   = importlib.import_module("auto_inspection.prom_resource_check")
    ai_mod    = importlib.import_module("auto_inspection.ai_summary")
    pod_notify_mod = importlib.import_module("auto_inspection.pod_restart_notify")

    parts = []
    facts_errors = []

    # =========================
    # 报告头
    # =========================
    parts.append(f"# 运维巡检周报（{week}）\n")
    parts.append(f"> 生成时间：{end_str}\n")
    parts.append(
        f"> 统计周期：{start_str} ～ {end_str}（{config.RANGE_DAYS} 天）\n"
    )
    parts.append("> 数据来源：Prometheus（条件成立累计时长 / 资源事实）\n\n")

    # =========================
    # 1. 告警
    # =========================
    parts.append(h2("1. 告警"))
    alert_rows = safe_run(
        "告警统计",
        alert_mod.collect_alert_rows,
        prometheus_urls=config.PROMETHEUS_URLS,
        start_ts=start_ts,
        end_ts=end_ts,
    )
    if isinstance(alert_rows, str):
        parts.append(alert_rows)
        facts_errors.append("告警统计生成失败")
        alert_rows = []
    else:
        parts.append(alert_mod.render_alert_section(alert_rows, start_ts, end_ts))
    parts.append("\n")

    # =========================
    # 2. 资源
    # =========================
    parts.append(h2("2. 资源"))
    resource_data = safe_run(
        "资源巡检",
        res_mod.collect_resource_data,
        prometheus_urls=config.PROMETHEUS_URLS,
        start_ts=start_ts,
        end_ts=end_ts,
    )
    if isinstance(resource_data, str):
        parts.append(resource_data)
        facts_errors.append("资源巡检生成失败")
        resource_data = {}
    else:
        parts.append(res_mod.render_resource_section(resource_data, start_ts, end_ts))
        output_result = safe_run(
            "资源输出",
            res_mod.write_resource_outputs,
            resource_data,
            start_ts,
            end_ts,
            output_dir=getattr(config, "RESOURCE_OUTPUT_DIR", "outputs"),
        )
        try:
            notify_result = pod_notify_mod.process_resource_data(
                resource_data,
                start_ts,
                end_ts,
            )
            if notify_result.get("sent"):
                print(f"[OK] pod restart notifications sent: {notify_result['sent']}")
        except Exception as exc:
            facts_errors.append("pod restart notification failed")
            print(f"[WARN] pod restart notification failed: {exc}")
        if isinstance(output_result, str):
            facts_errors.append("资源输出生成失败")
    parts.append("\n")

    # =========================
    # 3. AI 总结
    # =========================
    parts.append(h2("3. AI 总结（自动生成）"))

    ai_input = []
    for p in parts:
        if p.startswith("## 3. AI"):
            break
        ai_input.append(p)

    ai_text = safe_run(
        "AI 总结",
        ai_mod.generate_ai_summary_section,
        markdown_text="".join(ai_input),
        ollama_url=config.OLLAMA_URL,
        model=config.OLLAMA_MODEL,
        timeout=config.OLLAMA_TIMEOUT,
        facts={
            "alerts": alert_rows,
            "resources": resource_data,
            "errors": facts_errors,
        },
    )

    parts.append(ai_text)
    parts.append("\n")

    # =========================
    # 写文件
    # =========================
    output_dir = os.path.dirname(OUTPUT_MD)
    if output_dir:
        os.makedirs(output_dir, exist_ok=True)
    with open(OUTPUT_MD, "w", encoding="utf-8") as f:
        f.write("".join(parts))

    print(f"[OK] 巡检报告生成完成：{OUTPUT_MD}")

if __name__ == "__main__":
    main()
