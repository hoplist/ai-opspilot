#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
5_ai_summary.py
AI 巡检总结模块（只做解释层，不参与计算）

依赖：
- 本地 Ollama
- HTTP API: /api/generate
"""

from typing import Optional

from auto_inspection import config
from auto_inspection.http_client import request_json


def _fmt_pct(value):
    try:
        return f"{float(value) * 100:.2f}%"
    except (TypeError, ValueError):
        return "n/a"


def _dedupe(items):
    seen = set()
    out = []
    for item in items:
        if item in seen:
            continue
        seen.add(item)
        out.append(item)
    return out


def _generate_strict_summary(facts):
    alerts = facts.get("alerts") or []
    resources = facts.get("resources") or {}
    errors = facts.get("errors") or []

    bucketed = resources.get("bucketed") or {}
    red_items = bucketed.get("red") or []
    yellow_items = bucketed.get("yellow") or []
    prewarn_items = resources.get("prewarn") or []

    lines = []
    lines.append("### 一句话结论")
    if errors:
        lines.append("告警/资源统计存在失败项，摘要仅基于可用数据。")
    elif alerts or red_items or yellow_items:
        lines.append("统计周期内出现告警或当前资源高位，需关注相关节点稳定性。")
    else:
        lines.append("本周未发现告警与资源异常，整体稳定。")

    lines.append("")
    lines.append("### 已发生风险（红色）")
    red_lines = []
    for row in sorted(alerts, key=lambda x: x.get("hours", 0), reverse=True):
        red_lines.append(
            f"- 告警累计时长（统计周期）：{row['category']} {row['object']} = {row['hours']:.2f} 小时"
        )
    for item in sorted(
        red_items,
        key=lambda x: (-x.get("value", 0), x.get("name", ""), x.get("instance", "")),
    ):
        red_lines.append(
            f"- 当前资源：{item['name']} {item['instance']} = {_fmt_pct(item['value'])}"
        )
    lines.extend(red_lines or ["当前未发现红色风险。"])

    lines.append("")
    lines.append("### 潜在风险（黄色）")
    yellow_lines = []
    for item in sorted(
        yellow_items,
        key=lambda x: (-x.get("value", 0), x.get("name", ""), x.get("instance", "")),
    ):
        yellow_lines.append(
            f"- 当前资源：{item['name']} {item['instance']} = {_fmt_pct(item['value'])}"
        )
    lines.extend(yellow_lines or ["当前未发现黄色风险。"])

    lines.append("")
    lines.append("### 下周重点关注")
    focus_items = []
    for item in red_items:
        focus_items.append(f"{item['instance']} {item['name']}")
    for item in yellow_items:
        focus_items.append(f"{item['instance']} {item['name']}")
    for item in prewarn_items:
        if item.get("type") == "disk":
            focus_items.append(f"{item['instance']} Disk 使用率预警候选")
        elif item.get("type") == "mem":
            focus_items.append(f"{item['instance']} MemAvailable 预警候选")
    for row in alerts:
        focus_items.append(f"{row['object']} {row['category']}")

    focus_items = _dedupe(focus_items)
    if focus_items:
        for idx, item in enumerate(focus_items[:3], start=1):
            lines.append(f"{idx}. {item}")
    else:
        lines.append("1. 本周未发现需要重点关注的节点。")

    return "\n".join(lines).strip()


def _generate_llm_summary(markdown_text, ollama_url, model, timeout):
    prompt = f"""
你是资深 SRE，正在编写一份【每周运维巡检总结】。

下面是系统自动生成的巡检事实（告警 + 资源），这些内容都来自 Prometheus，
你【只能基于这些事实总结】，不允许引入任何报告中不存在的信息。

【巡检事实开始】
{markdown_text}
【巡检事实结束】

【强制要求】
1. 不允许编造任何新的指标、事件或结论
2. 如果某类风险在报告中不存在，明确写“本周未发现”
3. 所有原因推断必须标注为【假设】
4. 语言要像资深运维写给领导和研发的总结，克制、专业、可执行
5. 不要复述表格原文，要做“提炼”

【输出结构（严格按这个来）】
### 一句话结论
（一句话概括整体稳定性）

### 已发生风险（红色）
- 事实：
- 影响：
- 处理建议：

### 潜在风险（黄色）
- 事实：
- 风险：
- 建议：

### 下周重点关注（最多 3 条）
1.
2.
3.
"""

    try:
        data = request_json(
            "POST",
            ollama_url,
            payload={
                "model": model,
                "prompt": prompt,
                "stream": False,
            },
            timeout=timeout,
            retries=config.REQUEST_RETRIES,
            backoff_seconds=config.REQUEST_BACKOFF_SECONDS,
        )

        if "error" in data:
            raise RuntimeError(data["error"])

        text = data.get("response", "").strip()
        if not text:
            return "（AI 未返回有效内容）"

        return text

    except Exception as e:
        # ?? 失败也要“可读”，不能炸周报
        return f"?? AI 总结生成失败：{e}"


def generate_ai_summary_section(
    markdown_text: str,
    ollama_url: str,
    model: str,
    timeout: int = 180,
    facts: Optional[dict] = None,
) -> str:
    """
    输入：已经生成好的巡检 Markdown（事实）
    输出：AI 总结文本（可直接写入周报）
    """
    mode = (getattr(config, "AI_SUMMARY_MODE", "strict") or "strict").strip().lower()
    if mode in {"off", "none", "disabled"}:
        return "（AI 总结已关闭）"
    if mode == "strict":
        return _generate_strict_summary(facts or {})
    return _generate_llm_summary(markdown_text, ollama_url, model, timeout)
