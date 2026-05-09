#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
prom_alert_summary.py
Prometheus 告警语义统计（条件成立累计时长，显式时间窗口）
"""

import datetime
import math

from auto_inspection import config
from auto_inspection import prometheus_client

STEP = config.PROM_STEP
MAX_POINTS = config.PROM_MAX_POINTS

def build_jenkins_offline_alert_query():
    custom = (getattr(config, "JENKINS_OFFLINE_ALERT_PROMQL", "") or "").strip()
    if custom:
        return custom

    alert_name = getattr(config, "JENKINS_OFFLINE_ALERT_NAME", "").strip()
    labels = getattr(config, "JENKINS_OFFLINE_ALERT_LABELS", [])
    selectors = []
    for label in labels:
        label = (label or "").strip()
        if not label or not alert_name:
            continue
        selectors.append(
            f'ALERTS{{alertstate="firing",{label}="{alert_name}"}}'
        )

    if not selectors and alert_name:
        selectors.append(
            f'ALERTS{{alertstate="firing",alertname="{alert_name}"}}'
        )

    return " or ".join(selectors)

def build_jenkins_offline_metric_query():
    return (getattr(config, "JENKINS_OFFLINE_METRIC_PROMQL", "") or "").strip()

DISK_FSTYPE_EXCLUDE = "tmpfs|overlay|squashfs|nsfs|autofs|cgroup2?|devtmpfs"

def _disk_mountpoint_selector():
    include = (getattr(config, "RESOURCE_DISK_MOUNT_INCLUDE_RE", "") or "").strip()
    exclude = (getattr(config, "RESOURCE_DISK_MOUNT_EXCLUDE_RE", "") or "").strip()
    if include:
        return f'mountpoint=~"{include}"'
    if exclude:
        return f'mountpoint!~"{exclude}"'
    return ""

def _disk_label_selector():
    parts = [f'fstype!~"{DISK_FSTYPE_EXCLUDE}"']
    mount_sel = _disk_mountpoint_selector()
    if mount_sel:
        parts.append(mount_sel)
    return ",".join(parts)

def build_disk_alert_query():
    selector = _disk_label_selector()
    return f'''
    (
      1 - (
        node_filesystem_avail_bytes{{{selector}}}
        /
        node_filesystem_size_bytes{{{selector}}}
      )
    ) > bool 0.9
    '''

ALERT_SPECS = [
    {
        "category": "Pod内存快速增长预警",
        "query": r'''
        (
          (
            sum by (namespace, pod, container) (
              container_memory_working_set_bytes{namespace!="",pod!="",container!="",container!="POD"}
            )
            -
            sum by (namespace, pod, container) (
              container_memory_working_set_bytes{namespace!="",pod!="",container!="",container!="POD"} offset 15m
            )
          ) > bool 104857600
        )
        and on (namespace, pod, container)
        (
          deriv(
            sum by (namespace, pod, container) (
              container_memory_working_set_bytes{namespace!="",pod!="",container!="",container!="POD"}
            )[15m:]
          ) > bool 0
        )
        '''
    },
    {
        "category": "Pod内存预计触顶预警",
        "query": r'''
        (
          predict_linear(
            sum by (namespace, pod, container) (
              container_memory_working_set_bytes{namespace!="",pod!="",container!="",container!="POD"}
            )[30m:],
            3600
          )
          /
          on (namespace, pod, container)
          kube_pod_container_resource_limits{resource="memory",unit="byte"}
        ) > bool 0.9
        '''
    },
    {
        "category": "Pod内存Limit使用率告警",
        "query": r'''
        (
          sum by (namespace, pod, container) (
            container_memory_working_set_bytes{namespace!="",pod!="",container!="",container!="POD"}
          )
          /
          on (namespace, pod, container)
          kube_pod_container_resource_limits{resource="memory",unit="byte"}
        ) > bool 0.85
        '''
    },
    {
        "category": "磁盘告警",
        "query_builder": build_disk_alert_query,
    },
    {
        "category": "负载告警",
        "query": r'''
        (
          node_load1
          /
          on(instance)
          count without (cpu, mode) (node_cpu_seconds_total{mode="idle"})
        ) > bool 1
        '''
    },
    {
        "category": "Jenkins节点离线告警",
        "query_builder": build_jenkins_offline_alert_query,
        "fallback_query_builder": build_jenkins_offline_metric_query,
        "exclude_nodes": True,
    },
]

def calc_step(start_ts, end_ts):
    return max(STEP, math.ceil((end_ts - start_ts) / MAX_POINTS))

def pick_object_label(metric):
    namespace = metric.get("namespace")
    pod = metric.get("pod")
    container = metric.get("container")
    if namespace and pod and container:
        return f"{namespace}/{pod}/{container}"
    if namespace and pod:
        return f"{namespace}/{pod}"
    for key in ("node_name", "node", "instance", "job"):
        value = metric.get(key)
        if value:
            return value
    return "unknown"

def normalize_node_name(value):
    return (value or "").strip().lower()

def is_excluded_node(node_name):
    if not node_name:
        return False
    normalized = normalize_node_name(node_name)
    for item in getattr(config, "JENKINS_OFFLINE_EXCLUDE_NODES", []):
        if normalized == normalize_node_name(item):
            return True
    return False

def collect_alert_rows(prometheus_urls, start_ts, end_ts):
    step = calc_step(start_ts, end_ts)
    rows = []

    for spec in ALERT_SPECS:
        query = spec.get("query")
        if not query and spec.get("query_builder"):
            query = spec["query_builder"]()
        query = (query or "").strip()
        fallback_query = ""
        if spec.get("fallback_query_builder"):
            fallback_query = (spec["fallback_query_builder"]() or "").strip()
        if not query and not fallback_query:
            continue
        for url in prometheus_urls:
            series = []
            if query:
                series = prometheus_client.query_range(
                    query,
                    start_ts,
                    end_ts,
                    step,
                    url=url,
                    timeout=30,
                )
            if not series and fallback_query:
                series = prometheus_client.query_range(
                    fallback_query,
                    start_ts,
                    end_ts,
                    step,
                    url=url,
                    timeout=30,
                )
            for s in series:
                inst = pick_object_label(s["metric"])
                if spec.get("exclude_nodes") and is_excluded_node(inst):
                    continue
                seconds = sum(float(v) for _, v in s["values"]) * step
                if seconds > 0:
                    rows.append(
                        {
                            "category": spec["category"],
                            "object": inst,
                            "hours": seconds / 3600,
                        }
                    )

    return rows

def render_alert_section(rows, start_ts, end_ts):
    start_str = datetime.datetime.fromtimestamp(start_ts).strftime("%Y-%m-%d %H:%M:%S")
    end_str   = datetime.datetime.fromtimestamp(end_ts).strftime("%Y-%m-%d %H:%M:%S")

    out = []
    out.append(
        f"统计周期：**{start_str} ～ {end_str}**（条件成立累计时长）\n\n"
    )
    out.append("| 分类 | 对象 | 告警累计时长(小时) |\n")
    out.append("| --- | --- | --- |\n")

    if not rows:
        out.append("| 无 | 无 | 0 |\n")
        return "".join(out)

    for row in sorted(rows, key=lambda x: x.get("hours", 0), reverse=True):
        out.append(
            f"| {row['category']} | {row['object']} | {row['hours']:.2f} |\n"
        )

    return "".join(out)

def generate_alert_section(prometheus_urls, start_ts, end_ts):
    rows = collect_alert_rows(prometheus_urls, start_ts, end_ts)
    return render_alert_section(rows, start_ts, end_ts)
