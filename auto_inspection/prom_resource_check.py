#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
prom_resource_check.py
资源余量巡检（CPU / 内存 / 磁盘，磁盘含挂载点）
"""

import datetime
import math
import os
import json
import re
from urllib.parse import urlparse
from auto_inspection import config
from auto_inspection import prometheus_client
from auto_inspection.paths import PROJECT_ROOT

GROUP_DISK = "disk"
GROUP_COMPUTE = "compute"
GROUP_POD = "pod"

DISK_FSTYPE_EXCLUDE = "tmpfs|overlay|squashfs|nsfs|autofs|cgroup2?|devtmpfs"

def _disk_mountpoint_selector():
    include = (getattr(config, "RESOURCE_DISK_MOUNT_INCLUDE_RE", "") or "").strip()
    exclude = (getattr(config, "RESOURCE_DISK_MOUNT_EXCLUDE_RE", "") or "").strip()
    if include:
        return f'mountpoint=~"{include}"'
    if exclude:
        return f'mountpoint!~"{exclude}"'
    return ""

def _mountpoint_allowed(mountpoint):
    if not mountpoint:
        return False
    include = (getattr(config, "RESOURCE_DISK_MOUNT_INCLUDE_RE", "") or "").strip()
    exclude = (getattr(config, "RESOURCE_DISK_MOUNT_EXCLUDE_RE", "") or "").strip()
    if include:
        try:
            if re.search(include, mountpoint) is None:
                return False
        except re.error:
            return True
    if exclude:
        try:
            if re.search(exclude, mountpoint):
                return False
        except re.error:
            return True
    return True

def _disk_label_selector():
    parts = [f'fstype!~"{DISK_FSTYPE_EXCLUDE}"']
    mount_sel = _disk_mountpoint_selector()
    if mount_sel:
        parts.append(mount_sel)
    return ",".join(parts)

def _pod_label_selector():
    selectors = ['container!="POD"', 'container!=""']
    ns_inc = (getattr(config, "RESOURCE_POD_NAMESPACE_INCLUDE_RE", "") or "").strip()
    ns_exc = (getattr(config, "RESOURCE_POD_NAMESPACE_EXCLUDE_RE", "") or "").strip()
    pod_inc = (getattr(config, "RESOURCE_POD_NAME_INCLUDE_RE", "") or "").strip()
    pod_exc = (getattr(config, "RESOURCE_POD_NAME_EXCLUDE_RE", "") or "").strip()
    if ns_inc:
        selectors.append(f'namespace=~"{ns_inc}"')
    if ns_exc:
        selectors.append(f'namespace!~"{ns_exc}"')
    if pod_inc:
        selectors.append(f'pod=~"{pod_inc}"')
    if pod_exc:
        selectors.append(f'pod!~"{pod_exc}"')
    return ",".join(selectors)

def _pod_kube_selector():
    selectors = ['container!="POD"', 'container!=""']
    ns_inc = (getattr(config, "RESOURCE_POD_NAMESPACE_INCLUDE_RE", "") or "").strip()
    ns_exc = (getattr(config, "RESOURCE_POD_NAMESPACE_EXCLUDE_RE", "") or "").strip()
    pod_inc = (getattr(config, "RESOURCE_POD_NAME_INCLUDE_RE", "") or "").strip()
    pod_exc = (getattr(config, "RESOURCE_POD_NAME_EXCLUDE_RE", "") or "").strip()
    if ns_inc:
        selectors.append(f'namespace=~"{ns_inc}"')
    if ns_exc:
        selectors.append(f'namespace!~"{ns_exc}"')
    if pod_inc:
        selectors.append(f'pod=~"{pod_inc}"')
    if pod_exc:
        selectors.append(f'pod!~"{pod_exc}"')
    return ",".join(selectors)


def _pod_kube_metric_expr(metric_name, *, resource=None, unit=None):
    selector = _pod_kube_selector()
    if metric_name in {"kube_pod_container_resource_requests", "kube_pod_container_resource_limits"}:
        extras = []
        if resource:
            extras.append(f'resource="{resource}"')
        if unit:
            extras.append(f'unit="{unit}"')
        selector = _join_selector(selector, extras)
    return f"{metric_name}{{{selector}}}"


def _pod_cpu_request_expr():
    old_expr = _pod_kube_metric_expr("kube_pod_container_resource_requests_cpu_cores")
    new_expr = _pod_kube_metric_expr("kube_pod_container_resource_requests", resource="cpu", unit="core")
    return f"(({old_expr}) or ({new_expr}))"


def _pod_cpu_limit_expr():
    old_expr = _pod_kube_metric_expr("kube_pod_container_resource_limits_cpu_cores")
    new_expr = _pod_kube_metric_expr("kube_pod_container_resource_limits", resource="cpu", unit="core")
    return f"(({old_expr}) or ({new_expr}))"


def _pod_mem_request_expr():
    old_expr = _pod_kube_metric_expr("kube_pod_container_resource_requests_memory_bytes")
    new_expr = _pod_kube_metric_expr("kube_pod_container_resource_requests", resource="memory", unit="byte")
    return f"(({old_expr}) or ({new_expr}))"


def _pod_mem_limit_expr():
    old_expr = _pod_kube_metric_expr("kube_pod_container_resource_limits_memory_bytes")
    new_expr = _pod_kube_metric_expr("kube_pod_container_resource_limits", resource="memory", unit="byte")
    return f"(({old_expr}) or ({new_expr}))"

def _pod_status_selector():
    selectors = []
    ns_inc = (getattr(config, "RESOURCE_POD_NAMESPACE_INCLUDE_RE", "") or "").strip()
    ns_exc = (getattr(config, "RESOURCE_POD_NAMESPACE_EXCLUDE_RE", "") or "").strip()
    pod_inc = (getattr(config, "RESOURCE_POD_NAME_INCLUDE_RE", "") or "").strip()
    pod_exc = (getattr(config, "RESOURCE_POD_NAME_EXCLUDE_RE", "") or "").strip()
    if ns_inc:
        selectors.append(f'namespace=~"{ns_inc}"')
    if ns_exc:
        selectors.append(f'namespace!~"{ns_exc}"')
    if pod_inc:
        selectors.append(f'pod=~"{pod_inc}"')
    if pod_exc:
        selectors.append(f'pod!~"{pod_exc}"')
    return ",".join(selectors)

def _pod_network_selector():
    selectors = ['pod!=""', 'namespace!=""']
    ns_inc = (getattr(config, "RESOURCE_POD_NAMESPACE_INCLUDE_RE", "") or "").strip()
    ns_exc = (getattr(config, "RESOURCE_POD_NAMESPACE_EXCLUDE_RE", "") or "").strip()
    pod_inc = (getattr(config, "RESOURCE_POD_NAME_INCLUDE_RE", "") or "").strip()
    pod_exc = (getattr(config, "RESOURCE_POD_NAME_EXCLUDE_RE", "") or "").strip()
    if ns_inc:
        selectors.append(f'namespace=~"{ns_inc}"')
    if ns_exc:
        selectors.append(f'namespace!~"{ns_exc}"')
    if pod_inc:
        selectors.append(f'pod=~"{pod_inc}"')
    if pod_exc:
        selectors.append(f'pod!~"{pod_exc}"')
    return ",".join(selectors)

def _join_selector(base, extras):
    parts = []
    if base:
        parts.append(base)
    parts.extend([item for item in extras if item])
    return ",".join(parts)

def _pod_resource_base_query(kind):
    baseline = (getattr(config, "RESOURCE_POD_BASELINE", "auto") or "auto").strip().lower()
    if kind == "cpu":
        limits = f"sum by (namespace,pod) {_pod_cpu_limit_expr()}"
        requests = f"sum by (namespace,pod) {_pod_cpu_request_expr()}"
        allocatable = (
            "max by (namespace,pod) ("
            f"max by (namespace,pod,node) (kube_pod_info{{{_pod_kube_selector()}}}) "
            "* on(node) group_left "
            'max by (node) (kube_node_status_allocatable{resource="cpu",unit="core"})'
            ")"
        )
    else:
        limits = f"sum by (namespace,pod) {_pod_mem_limit_expr()}"
        requests = f"sum by (namespace,pod) {_pod_mem_request_expr()}"
        allocatable = (
            "max by (namespace,pod) ("
            f"max by (namespace,pod,node) (kube_pod_info{{{_pod_kube_selector()}}}) "
            "* on(node) group_left "
            'max by (node) (kube_node_status_allocatable{resource="memory",unit="byte"})'
            ")"
        )
    if baseline == "limits":
        return limits
    if baseline == "requests":
        return requests
    if baseline == "allocatable":
        return allocatable
    return f"({limits}) or ({requests}) or ({allocatable})"

def _pod_cpu_query():
    selector = _pod_label_selector()
    usage = f"sum by (namespace,pod) (rate(container_cpu_usage_seconds_total{{{selector}}}[5m]))"
    base = _pod_resource_base_query("cpu")
    return f"({usage}) / clamp_min(({base}), 0.001)"

def _pod_mem_query():
    selector = _pod_label_selector()
    usage = f"sum by (namespace,pod) (container_memory_working_set_bytes{{{selector}}})"
    base = _pod_resource_base_query("mem")
    return f"({usage}) / clamp_min(({base}), 1)"

def _pod_mem_ratio_query():
    selector = _pod_label_selector()
    usage = f"sum by (namespace,pod) (container_memory_working_set_bytes{{{selector}}})"
    limit = f"sum by (namespace,pod) {_pod_mem_limit_expr()}"
    return f"({usage}) / clamp_min(({limit}), 1)"

def _pod_cpu_usage_query():
    selector = _pod_label_selector()
    return f"sum by (namespace,pod) (rate(container_cpu_usage_seconds_total{{{selector}}}[5m]))"

def _pod_cpu_request_query():
    return f"sum by (namespace,pod) {_pod_cpu_request_expr()}"

def _pod_mem_usage_query():
    selector = _pod_label_selector()
    return f"sum by (namespace,pod) (container_memory_working_set_bytes{{{selector}}})"

def _pod_mem_limit_query():
    return f"sum by (namespace,pod) {_pod_mem_limit_expr()}"

def _pod_cpu_limit_query():
    return f"sum by (namespace,pod) {_pod_cpu_limit_expr()}"

def _pod_mem_request_query():
    return f"sum by (namespace,pod) {_pod_mem_request_expr()}"

def _pod_mem_rss_query():
    selector = _pod_label_selector()
    return f"sum by (namespace,pod) (container_memory_rss{{{selector}}})"

def _pod_mem_rate_query():
    selector = _pod_label_selector()
    return f"sum by (namespace,pod) (rate(container_memory_working_set_bytes{{{selector}}}[5m]))"

def _pod_net_rx_bytes_rate_query():
    selector = _pod_network_selector()
    return f"sum by (namespace,pod) (rate(container_network_receive_bytes_total{{{selector}}}[5m]))"

def _pod_net_tx_bytes_rate_query():
    selector = _pod_network_selector()
    return f"sum by (namespace,pod) (rate(container_network_transmit_bytes_total{{{selector}}}[5m]))"

def _pod_net_rx_packets_rate_query():
    selector = _pod_network_selector()
    return f"sum by (namespace,pod) (rate(container_network_receive_packets_total{{{selector}}}[5m]))"

def _pod_net_tx_packets_rate_query():
    selector = _pod_network_selector()
    return f"sum by (namespace,pod) (rate(container_network_transmit_packets_total{{{selector}}}[5m]))"

def _pod_net_rx_errors_rate_query():
    selector = _pod_network_selector()
    return f"sum by (namespace,pod) (rate(container_network_receive_errors_total{{{selector}}}[5m]))"

def _pod_net_tx_errors_rate_query():
    selector = _pod_network_selector()
    return f"sum by (namespace,pod) (rate(container_network_transmit_errors_total{{{selector}}}[5m]))"

def _pod_net_rx_drops_rate_query():
    selector = _pod_network_selector()
    return f"sum by (namespace,pod) (rate(container_network_receive_packets_dropped_total{{{selector}}}[5m]))"

def _pod_net_tx_drops_rate_query():
    selector = _pod_network_selector()
    return f"sum by (namespace,pod) (rate(container_network_transmit_packets_dropped_total{{{selector}}}[5m]))"

def _pod_restart_rate_query():
    selector = _pod_kube_selector()
    return f"sum by (namespace,pod) (rate(kube_pod_container_status_restarts_total{{{selector}}}[5m]))"

def _pod_cpu_throttle_ratio_query():
    selector = _pod_label_selector()
    throttled = f"sum by (namespace,pod) (rate(container_cpu_cfs_throttled_periods_total{{{selector}}}[5m]))"
    periods = f"sum by (namespace,pod) (rate(container_cpu_cfs_periods_total{{{selector}}}[5m]))"
    return f"({throttled}) / clamp_min(({periods}), 1)"

def _pod_cpu_throttled_seconds_rate_query():
    selector = _pod_label_selector()
    return (
        "sum by (namespace,pod) "
        f"(rate(container_cpu_cfs_throttled_seconds_total{{{selector}}}[5m]))"
    )

def _pod_cpu_cfs_periods_rate_query():
    selector = _pod_label_selector()
    return (
        "sum by (namespace,pod) "
        f"(rate(container_cpu_cfs_periods_total{{{selector}}}[5m]))"
    )

def _pod_fs_usage_query():
    selector = _pod_label_selector()
    return f"sum by (namespace,pod) (container_fs_usage_bytes{{{selector}}})"

def _pod_fs_limit_query():
    selector = _pod_label_selector()
    return f"sum by (namespace,pod) (container_fs_limit_bytes{{{selector}}})"

def _pod_container_oom_events_query():
    selector = _pod_label_selector()
    return f"sum by (namespace,pod) (container_oom_events_total{{{selector}}})"

def _pod_ready_query():
    selector = _pod_kube_selector()
    return f"sum by (namespace,pod) (kube_pod_container_status_ready{{{selector}}})"

def _pod_ready_total_query():
    selector = _pod_kube_selector()
    return f"count by (namespace,pod) (kube_pod_container_status_ready{{{selector}}})"

def _pod_running_query():
    selector = _join_selector(_pod_status_selector(), ['phase="Running"'])
    return f"max by (namespace,pod) (kube_pod_status_phase{{{selector}}})"

def _pod_pending_query():
    selector = _join_selector(_pod_status_selector(), ['phase="Pending"'])
    return f"max by (namespace,pod) (kube_pod_status_phase{{{selector}}})"

def _pod_pending_reason_query():
    selector = _join_selector(_pod_status_selector(), ['reason!=""'])
    return f"max by (namespace,pod,reason) (kube_pod_status_reason{{{selector}}})"

def _pod_phase_query():
    selector = _pod_status_selector()
    return f"max by (namespace,pod,phase) (kube_pod_status_phase{{{selector}}})"

def _pod_waiting_reason_query():
    selector = _join_selector(_pod_kube_selector(), ['reason!=""'])
    return (
        "max by (namespace,pod,reason) "
        f"(kube_pod_container_status_waiting_reason{{{selector}}})"
    )

def _pod_terminated_reason_query():
    selector = _join_selector(_pod_kube_selector(), ['reason!=""'])
    return (
        "max by (namespace,pod,reason) "
        f"(kube_pod_container_status_terminated_reason{{{selector}}}) "
        "or "
        "max by (namespace,pod,reason) "
        f"(kube_pod_container_status_last_terminated_reason{{{selector}}})"
    )

def _pod_terminated_exitcode_query():
    selector = _pod_kube_selector()
    return (
        "max by (namespace,pod) "
        f"(kube_pod_container_status_last_terminated_exitcode{{{selector}}})"
    )

def _pod_restart_query(hours):
    selector = _pod_kube_selector()
    return f"sum by (namespace,pod) (increase(kube_pod_container_status_restarts_total{{{selector}}}[{hours}h]))"

def _pod_restart_total_query():
    selector = _pod_kube_selector()
    return f"sum by (namespace,pod) (kube_pod_container_status_restarts_total{{{selector}}})"

def _pod_last_terminated_time_query():
    selector = _pod_kube_selector()
    return f"max by (namespace,pod) (kube_pod_container_status_last_terminated_time{{{selector}}})"

def _pod_oom_query():
    selector = _pod_kube_selector()
    return f'max by (namespace,pod) (kube_pod_container_status_last_terminated_reason{{{selector},reason="OOMKilled"}})'

def _pod_node_name_query():
    selector = _pod_kube_selector()
    return f"max by (namespace,pod,node) (kube_pod_info{{{selector}}})"

def _node_metric_with_node_label(expr):
    node_info = "kube_node_info"
    kubelet_map = "kubelet_node_name"
    direct_from_instance = 'label_replace(' + expr + ', "node", "$1", "instance", "([^:]+)(?::\\\\d+)?")'
    node_name_map = f'label_replace({node_info}, "instance", "$1", "node", "(.+)")'
    map_9100 = f'label_replace({node_info}, "instance", "$1:9100", "internal_ip", "(.+)")'
    map_10250 = f'label_replace({node_info}, "instance", "$1:10250", "internal_ip", "(.+)")'
    map_10255 = f'label_replace({node_info}, "instance", "$1:10255", "internal_ip", "(.+)")'
    uname_base = 'label_replace(node_uname_info, "node", "$1", "nodename", "(.+)")'
    uname_9100 = f'label_replace({uname_base}, "instance", "$1:9100", "instance", "(.+)")'
    uname_10250 = f'label_replace({uname_base}, "instance", "$1:10250", "instance", "(.+)")'
    uname_10255 = f'label_replace({uname_base}, "instance", "$1:10255", "instance", "(.+)")'
    return (
        f"({expr}) "
        f"or ({direct_from_instance}) "
        f"or ({expr} * on(instance) group_left(node) {kubelet_map}) "
        f"or ({expr} * on(instance) group_left(node) {node_name_map}) "
        f"or ({expr} * on(instance) group_left(node) {map_10250}) "
        f"or ({expr} * on(instance) group_left(node) {map_10255}) "
        f"or ({expr} * on(instance) group_left(node) {map_9100}) "
        f"or ({expr} * on(instance) group_left(node) {uname_base}) "
        f"or ({expr} * on(instance) group_left(node) {uname_10250}) "
        f"or ({expr} * on(instance) group_left(node) {uname_10255}) "
        f"or ({expr} * on(instance) group_left(node) {uname_9100})"
    )

def _pod_node_metric_query(expr):
    selector = _pod_kube_selector()
    pod_nodes = f"max by (namespace,pod,node) (kube_pod_info{{{selector}}})"
    node_expr = _node_metric_with_node_label(expr)
    return (
        "max by (namespace,pod) ("
        f"{pod_nodes} * on(node) group_left "
        f"max by (node) ({node_expr})"
        ")"
    )

def _pod_node_oom_query():
    return _pod_node_metric_query("rate(node_vmstat_oom_kill[5m])")

def _pod_node_mem_available_query():
    return _pod_node_metric_query("node_memory_MemAvailable_bytes")

def _pod_node_mem_total_query():
    return _pod_node_metric_query("node_memory_MemTotal_bytes")

def _pod_node_load1_query():
    return _pod_node_metric_query("node_load1")

def _pod_node_cpu_usage_query():
    expr = '1 - avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m]))'
    return _pod_node_metric_query(expr)

def _pod_node_up_query():
    return _pod_node_metric_query("max by (instance) (up)")

def _pod_node_scrape_samples_query():
    return _pod_node_metric_query("max by (instance) (scrape_samples_scraped)")

def _pod_node_scrape_samples_post_query():
    return _pod_node_metric_query("max by (instance) (scrape_samples_post_metric_relabeling)")

def _disk_usage_query():
    selector = _disk_label_selector()
    return f'''
    1 - (
      node_filesystem_avail_bytes{{{selector}}}
      /
      node_filesystem_size_bytes{{{selector}}}
    )
    '''

def _disk_size_query():
    selector = _disk_label_selector()
    return f'node_filesystem_size_bytes{{{selector}}}'

def _disk_avail_query():
    selector = _disk_label_selector()
    return f'node_filesystem_avail_bytes{{{selector}}}'

RESOURCE_SPECS = [
    {
        "key": "cpu",
        "name": "CPU 使用率",
        "query": r'''
        1 - avg by (instance, job)(
          rate(node_cpu_seconds_total{mode="idle"}[5m])
        )
        ''',
        "red": 0.85,
        "yellow": 0.60,
        "group": GROUP_COMPUTE,
        "key_fields": ["instance", "job"],
    },
    {
        "key": "mem",
        "name": "Mem 使用率",
        "query": r'''
        1 - (
          node_memory_MemAvailable_bytes
          /
          node_memory_MemTotal_bytes
        )
        ''',
        "red": 0.90,
        "yellow": 0.70,
        "group": GROUP_COMPUTE,
        "key_fields": ["instance", "job"],
    },
    {
        "key": "pod_cpu",
        "name": "Pod CPU 使用率",
        "query_builder": _pod_cpu_query,
        "red": 0.90,
        "yellow": 0.80,
        "group": GROUP_POD,
        "key_fields": ["namespace", "pod"],
    },
    {
        "key": "pod_mem",
        "name": "Pod Mem 使用率",
        "query_builder": _pod_mem_query,
        "red": 0.90,
        "yellow": 0.80,
        "group": GROUP_POD,
        "key_fields": ["namespace", "pod"],
    },
    {
        "key": "disk",
        "name": "Disk 使用率",
        "query_builder": _disk_usage_query,
        "red": 0.90,
        "yellow": 0.80,
        "group": GROUP_DISK,
        "key_fields": ["instance", "mountpoint"],
    },
]

def _parse_value(value):
    if value in ("NaN", None):
        return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None

def _fmt_pct(value):
    if value is None:
        return "n/a"
    return f"{value * 100:.2f}%"

def _fmt_bytes(value):
    if value is None:
        return "n/a"
    try:
        value = float(value)
    except (TypeError, ValueError):
        return "n/a"
    units = ["B", "KB", "MB", "GB", "TB", "PB"]
    idx = 0
    while value >= 1024 and idx < len(units) - 1:
        value /= 1024.0
        idx += 1
    return f"{value:.2f}{units[idx]}"

def _format_target(instance, mountpoint=None):
    if mountpoint:
        return f"{instance} {mountpoint}"
    return instance

def _build_key(labels, key_fields):
    return tuple(labels.get(field, "") for field in key_fields)

def _default_cluster_name(url):
    try:
        parsed = urlparse(url)
        host = parsed.hostname or parsed.netloc or parsed.path or url
        if parsed.port:
            return f"{host}:{parsed.port}"
        return host
    except Exception:
        return url

def _build_cluster_map(prometheus_urls):
    urls = prometheus_urls or []
    configured = getattr(config, "PROMETHEUS_CLUSTERS", []) or []
    configured = [str(item).strip() for item in configured if str(item).strip()]
    if not urls:
        return {}
    use_cluster = bool(configured) or len(urls) > 1
    if not use_cluster:
        return {}
    mapping = {}
    for idx, url in enumerate(urls):
        name = configured[idx] if idx < len(configured) else ""
        if not name:
            name = _default_cluster_name(url)
        mapping[url] = name
    return mapping

def _group_label(labels, group_labels):
    parts = []
    for key in group_labels:
        value = labels.get(key)
        if value:
            parts.append(f"{key}={value}")
    return " ".join(parts) if parts else "-"

def _trend_label(recent_avg, prev_avg, flat_delta):
    if recent_avg is None or prev_avg is None:
        return "-"
    delta = recent_avg - prev_avg
    if abs(delta) < flat_delta:
        return "持平"
    return "上升" if delta > 0 else "下降"

def _forecast_days(current, recent_avg, prev_avg, threshold, window_days):
    if current is None or recent_avg is None or prev_avg is None:
        return None
    if window_days <= 0:
        return None
    slope = (recent_avg - prev_avg) / window_days
    if slope <= 0:
        return None
    days = (threshold - current) / slope
    return max(days, 0.0)

def _forecast_days_from_slope(current, slope_per_day, threshold):
    if current is None or slope_per_day is None:
        return None
    if slope_per_day <= 0:
        return None
    days = (threshold - current) / slope_per_day
    return max(days, 0.0)

def calc_step(start_ts, end_ts):
    return max(
        config.PROM_STEP,
        math.ceil((end_ts - start_ts) / config.PROM_MAX_POINTS),
    )

def _classify_level(spec, value):
    if value is None:
        return None
    low_bad = spec.get("low_bad", False)
    if (not low_bad and value >= spec["red"]) or (low_bad and value <= spec["red"]):
        return "red"
    if (not low_bad and value >= spec["yellow"]) or (low_bad and value <= spec["yellow"]):
        return "yellow"
    return "green"

def _collect_instant_values(prometheus_urls, promql, key_fields):
    values = {}
    cluster_map = _build_cluster_map(prometheus_urls)
    for url in prometheus_urls:
        cluster_name = cluster_map.get(url)
        series = prometheus_client.query_instant(
            promql.strip(),
            url=url,
            timeout=20,
        )
        for s in series:
            labels = s.get("metric", {}) or {}
            if cluster_name and "cluster" not in labels:
                labels = dict(labels)
                labels["cluster"] = cluster_name
            raw = None
            if "value" in s and len(s["value"]) >= 2:
                raw = s["value"][1]
            v = _parse_value(raw)
            if v is None:
                continue
            key = _build_key(labels, key_fields)
            prev = values.get(key)
            if prev is None or v > prev["value"]:
                values[key] = {"labels": labels, "value": v, "key": key}
    return list(values.values())

def _collect_range_stats(prometheus_urls, promql, start_ts, end_ts, step, key_fields):
    stats = {}
    cluster_map = _build_cluster_map(prometheus_urls)
    for url in prometheus_urls:
        cluster_name = cluster_map.get(url)
        series = prometheus_client.query_range(
            promql.strip(),
            start_ts,
            end_ts,
            step,
            url=url,
            timeout=30,
        )
        for s in series:
            labels = s.get("metric", {}) or {}
            if cluster_name and "cluster" not in labels:
                labels = dict(labels)
                labels["cluster"] = cluster_name
            key = _build_key(labels, key_fields)
            state = stats.get(key)
            if state is None:
                state = {"labels": labels, "max": None, "sum": 0.0, "count": 0}
                stats[key] = state
            for _, raw in s.get("values", []):
                v = _parse_value(raw)
                if v is None:
                    continue
                if state["max"] is None or v > state["max"]:
                    state["max"] = v
                state["sum"] += v
                state["count"] += 1
    for state in stats.values():
        state["avg"] = state["sum"] / state["count"] if state["count"] else None
    return stats

def _collect_regression_stats(prometheus_urls, promql, start_ts, end_ts, step, key_fields):
    stats = {}
    cluster_map = _build_cluster_map(prometheus_urls)
    for url in prometheus_urls:
        cluster_name = cluster_map.get(url)
        series = prometheus_client.query_range(
            promql.strip(),
            start_ts,
            end_ts,
            step,
            url=url,
            timeout=30,
        )
        for s in series:
            labels = s.get("metric", {}) or {}
            if cluster_name and "cluster" not in labels:
                labels = dict(labels)
                labels["cluster"] = cluster_name
            key = _build_key(labels, key_fields)
            points = []
            for ts_raw, raw in s.get("values", []):
                v = _parse_value(raw)
                if v is None:
                    continue
                try:
                    ts_val = float(ts_raw)
                except (TypeError, ValueError):
                    continue
                points.append((ts_val, v))
            if len(points) < 2:
                continue
            points.sort(key=lambda p: p[0])
            t0 = points[0][0]
            sum_x = 0.0
            sum_y = 0.0
            sum_xx = 0.0
            sum_xy = 0.0
            for ts_val, v in points:
                x = (ts_val - t0) / 3600.0
                sum_x += x
                sum_y += v
                sum_xx += x * x
                sum_xy += x * v
            n = float(len(points))
            denom = n * sum_xx - sum_x * sum_x
            if denom == 0:
                continue
            slope_per_hour = (n * sum_xy - sum_x * sum_y) / denom
            slope_per_day = slope_per_hour * 24.0
            stats[key] = {
                "labels": labels,
                "slope_per_day": slope_per_day,
                "count": len(points),
            }
    return stats

def _index_by_key(entries):
    return {entry["key"]: entry for entry in entries}

def _index_values(entries):
    return {entry["key"]: entry.get("value") for entry in entries}

def collect_resource_data(prometheus_urls, start_ts, end_ts):
    step = calc_step(start_ts, end_ts)
    only_disk = bool(getattr(config, "RESOURCE_ONLY_DISK", False))
    group_labels = getattr(config, "RESOURCE_GROUP_LABELS", []) or []
    pod_group_labels = getattr(config, "RESOURCE_POD_GROUP_LABELS", []) or []
    pod_enabled = bool(getattr(config, "RESOURCE_POD_ENABLED", False))
    if only_disk:
        pod_enabled = False
    cluster_map = _build_cluster_map(prometheus_urls)
    cluster_enabled = bool(cluster_map)
    trend_enabled = bool(getattr(config, "RESOURCE_TREND_ENABLED", False))
    trend_days = int(getattr(config, "RESOURCE_TREND_DAYS", 7))
    pod_trend_days = int(getattr(config, "RESOURCE_POD_TREND_DAYS", max(1, trend_days)))
    pod_short_window_minutes = int(getattr(config, "RESOURCE_POD_SHORT_WINDOW_MINUTES", 30))
    forecast_hours = int(getattr(config, "RESOURCE_FORECAST_WINDOW_HOURS", 72))
    flat_delta = float(getattr(config, "RESOURCE_TREND_FLAT_DELTA", 0.02))

    remaining_current_threshold = float(getattr(config, "RESOURCE_REMAINING_CURRENT", 0.30))
    remaining_max_threshold = float(getattr(config, "RESOURCE_REMAINING_MAX", 0.50))
    abundant_current_threshold = float(getattr(config, "RESOURCE_REMAINING_ABUNDANT_CURRENT", 0.20))
    abundant_max_threshold = float(getattr(config, "RESOURCE_REMAINING_ABUNDANT_MAX", 0.30))
    usage_alert = float(getattr(config, "RESOURCE_USAGE_ALERT", 0.80))

    bucketed = {"red": [], "yellow": [], "green": []}
    entries = []
    surplus_entries = []
    pressure_entries = []
    spec_by_key = {spec["key"]: spec for spec in RESOURCE_SPECS}

    if trend_enabled:
        recent_start = end_ts - trend_days * 86400
        prev_start = end_ts - trend_days * 86400 * 2
        pod_recent_start = end_ts - pod_trend_days * 86400
        pod_prev_start = end_ts - pod_trend_days * 86400 * 2
        pod_short_recent_start = end_ts - pod_short_window_minutes * 60
        pod_short_prev_start = end_ts - pod_short_window_minutes * 120
        if forecast_hours > 0:
            forecast_start = end_ts - forecast_hours * 3600
        else:
            forecast_start = None
    else:
        recent_start = prev_start = pod_recent_start = pod_prev_start = pod_short_recent_start = pod_short_prev_start = forecast_start = None

    pod_meta = {}
    pod_oom_stats = {}
    pod_key_fields = ["namespace", "pod"]
    if cluster_enabled and "cluster" not in pod_key_fields:
        pod_key_fields = pod_key_fields + ["cluster"]
    if pod_enabled:
        require_ksm = bool(getattr(config, "RESOURCE_POD_REQUIRE_KSM", True))
        pod_oom_stats = _collect_range_stats(
            prometheus_urls,
            _pod_oom_query(),
            start_ts,
            end_ts,
            step,
            pod_key_fields,
        )
        cpu_usage_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_cpu_usage_query(),
                pod_key_fields,
            )
        )
        cpu_req_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_cpu_request_query(),
                pod_key_fields,
            )
        )
        cpu_limit_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_cpu_limit_query(),
                pod_key_fields,
            )
        )
        mem_usage_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_mem_usage_query(),
                pod_key_fields,
            )
        )
        mem_request_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_mem_request_query(),
                pod_key_fields,
            )
        )
        mem_limit_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_mem_limit_query(),
                pod_key_fields,
            )
        )
        mem_rss_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_mem_rss_query(),
                pod_key_fields,
            )
        )
        mem_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_mem_rate_query(),
                pod_key_fields,
            )
        )
        net_rx_bytes_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_net_rx_bytes_rate_query(),
                pod_key_fields,
            )
        )
        net_tx_bytes_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_net_tx_bytes_rate_query(),
                pod_key_fields,
            )
        )
        net_rx_packets_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_net_rx_packets_rate_query(),
                pod_key_fields,
            )
        )
        net_tx_packets_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_net_tx_packets_rate_query(),
                pod_key_fields,
            )
        )
        net_rx_errors_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_net_rx_errors_rate_query(),
                pod_key_fields,
            )
        )
        net_tx_errors_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_net_tx_errors_rate_query(),
                pod_key_fields,
            )
        )
        net_rx_drops_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_net_rx_drops_rate_query(),
                pod_key_fields,
            )
        )
        net_tx_drops_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_net_tx_drops_rate_query(),
                pod_key_fields,
            )
        )
        mem_ratio_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_mem_ratio_query(),
                pod_key_fields,
            )
        )
        cpu_throttle_ratio_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_cpu_throttle_ratio_query(),
                pod_key_fields,
            )
        )
        restart_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_restart_rate_query(),
                pod_key_fields,
            )
        )
        cpu_throttled_seconds_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_cpu_throttled_seconds_rate_query(),
                pod_key_fields,
            )
        )
        cpu_cfs_periods_rate_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_cpu_cfs_periods_rate_query(),
                pod_key_fields,
            )
        )
        fs_usage_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_fs_usage_query(),
                pod_key_fields,
            )
        )
        fs_limit_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_fs_limit_query(),
                pod_key_fields,
            )
        )
        oom_events_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_container_oom_events_query(),
                pod_key_fields,
            )
        )
        ready_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_ready_query(),
                pod_key_fields,
            )
        )
        ready_total_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_ready_total_query(),
                pod_key_fields,
            )
        )
        running_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_running_query(),
                pod_key_fields,
            )
        )
        pending_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_pending_query(),
                pod_key_fields,
            )
        )
        restart_hours = int(getattr(config, "RESOURCE_POD_RESTART_HOURS", 24))
        restarts_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_restart_query(restart_hours),
                pod_key_fields,
            )
        )
        restarts_total_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_restart_total_query(),
                pod_key_fields,
            )
        )
        last_terminated_time_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_last_terminated_time_query(),
                pod_key_fields,
            )
        )
        node_oom_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_node_oom_query(),
                pod_key_fields,
            )
        )
        node_mem_available_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_node_mem_available_query(),
                pod_key_fields,
            )
        )
        node_mem_total_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_node_mem_total_query(),
                pod_key_fields,
            )
        )
        node_load1_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_node_load1_query(),
                pod_key_fields,
            )
        )
        node_cpu_usage_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_node_cpu_usage_query(),
                pod_key_fields,
            )
        )
        node_up_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_node_up_query(),
                pod_key_fields,
            )
        )
        node_scrape_samples_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_node_scrape_samples_query(),
                pod_key_fields,
            )
        )
        node_scrape_samples_post_map = _index_values(
            _collect_instant_values(
                prometheus_urls,
                _pod_node_scrape_samples_post_query(),
                pod_key_fields,
            )
        )
        pending_reason_entries = _collect_instant_values(
            prometheus_urls,
            _pod_pending_reason_query(),
            pod_key_fields + ["reason"],
        )
        pending_reason_map = {}
        for entry in pending_reason_entries:
            reason = (entry.get("labels") or {}).get("reason")
            value = entry.get("value")
            if not reason or value is None or value <= 0:
                continue
            base_key = entry.get("key", ())[:-1]
            if base_key and base_key not in pending_reason_map:
                pending_reason_map[base_key] = reason

        node_name_entries = _collect_instant_values(
            prometheus_urls,
            _pod_node_name_query(),
            pod_key_fields + ["node"],
        )
        node_name_map = {}
        for entry in node_name_entries:
            node_name = (entry.get("labels") or {}).get("node")
            value = entry.get("value")
            if not node_name or value is None or value <= 0:
                continue
            base_key = entry.get("key", ())[:-1]
            if base_key and base_key not in node_name_map:
                node_name_map[base_key] = node_name

        waiting_reason_entries = _collect_instant_values(
            prometheus_urls,
            _pod_waiting_reason_query(),
            pod_key_fields + ["reason"],
        )
        waiting_reason_map = {}
        for entry in waiting_reason_entries:
            reason = (entry.get("labels") or {}).get("reason")
            value = entry.get("value")
            if not reason or value is None or value <= 0:
                continue
            base_key = entry.get("key", ())[:-1]
            if base_key and base_key not in waiting_reason_map:
                waiting_reason_map[base_key] = reason

        terminated_reason_entries = _collect_instant_values(
            prometheus_urls,
            _pod_terminated_reason_query(),
            pod_key_fields + ["reason"],
        )
        terminated_reason_map = {}
        for entry in terminated_reason_entries:
            reason = (entry.get("labels") or {}).get("reason")
            value = entry.get("value")
            if not reason or value is None or value <= 0:
                continue
            base_key = entry.get("key", ())[:-1]
            if base_key and base_key not in terminated_reason_map:
                terminated_reason_map[base_key] = reason

        terminated_exitcode_entries = _collect_instant_values(
            prometheus_urls,
            _pod_terminated_exitcode_query(),
            pod_key_fields,
        )
        terminated_exitcode_map = _index_values(terminated_exitcode_entries)

        phase_entries = _collect_instant_values(
            prometheus_urls,
            _pod_phase_query(),
            pod_key_fields + ["phase"],
        )
        phase_map = {}
        phase_rank = {
            "Running": 0,
            "Pending": 1,
            "Failed": 2,
            "Unknown": 3,
            "Succeeded": 4,
        }
        for entry in phase_entries:
            phase = (entry.get("labels") or {}).get("phase")
            value = entry.get("value")
            if not phase or value is None or value <= 0:
                continue
            base_key = entry.get("key", ())[:-1]
            if not base_key:
                continue
            current = phase_map.get(base_key)
            if current is None:
                phase_map[base_key] = phase
                continue
            if phase_rank.get(phase, 99) < phase_rank.get(current, 99):
                phase_map[base_key] = phase

        all_pod_keys = set()
        mem_ratio_recent_stats = {}
        mem_ratio_prev_stats = {}
        cpu_throttle_recent_stats = {}
        cpu_throttle_prev_stats = {}
        if trend_enabled and recent_start is not None and prev_start is not None:
            mem_ratio_recent_stats = _collect_range_stats(
                prometheus_urls,
                _pod_mem_ratio_query(),
                recent_start,
                end_ts,
                calc_step(recent_start, end_ts),
                pod_key_fields,
            )
            mem_ratio_prev_stats = _collect_range_stats(
                prometheus_urls,
                _pod_mem_ratio_query(),
                prev_start,
                recent_start,
                calc_step(prev_start, recent_start),
                pod_key_fields,
            )
            cpu_throttle_recent_stats = _collect_range_stats(
                prometheus_urls,
                _pod_cpu_throttle_ratio_query(),
                recent_start,
                end_ts,
                calc_step(recent_start, end_ts),
                pod_key_fields,
            )
            cpu_throttle_prev_stats = _collect_range_stats(
                prometheus_urls,
                _pod_cpu_throttle_ratio_query(),
                prev_start,
                recent_start,
                calc_step(prev_start, recent_start),
                pod_key_fields,
            )

        for mapping in (
            cpu_usage_map,
            cpu_req_map,
            cpu_limit_map,
            mem_usage_map,
            mem_request_map,
            mem_limit_map,
            mem_rss_map,
            mem_rate_map,
            net_rx_bytes_rate_map,
            net_tx_bytes_rate_map,
            net_rx_packets_rate_map,
            net_tx_packets_rate_map,
            net_rx_errors_rate_map,
            net_tx_errors_rate_map,
            net_rx_drops_rate_map,
            net_tx_drops_rate_map,
            mem_ratio_map,
            cpu_throttle_ratio_map,
            cpu_throttled_seconds_rate_map,
            cpu_cfs_periods_rate_map,
            fs_usage_map,
            fs_limit_map,
            oom_events_map,
            ready_map,
            ready_total_map,
            running_map,
            pending_map,
            restarts_map,
            restart_rate_map,
            restarts_total_map,
            pending_reason_map,
            waiting_reason_map,
            terminated_reason_map,
            terminated_exitcode_map,
            last_terminated_time_map,
            node_oom_map,
            node_mem_available_map,
            node_mem_total_map,
            node_load1_map,
            node_cpu_usage_map,
            node_up_map,
            node_scrape_samples_map,
            node_scrape_samples_post_map,
            node_name_map,
            phase_map,
            pod_oom_stats,
        ):
            all_pod_keys.update(mapping.keys())

        if require_ksm:
            ksm_keys = set()
            for mapping in (
                phase_map,
                node_name_map,
                ready_map,
                ready_total_map,
                running_map,
                pending_map,
            ):
                ksm_keys.update(mapping.keys())
            if ksm_keys:
                all_pod_keys = {key for key in all_pod_keys if key in ksm_keys}

        for key in all_pod_keys:
            label_map = dict(zip(pod_key_fields, key))
            namespace = label_map.get("namespace") or ""
            pod_name = label_map.get("pod") or ""
            cluster_name = label_map.get("cluster") or "-"
            instance_name = "/".join([part for part in (namespace, pod_name) if part]) or "unknown"
            cpu_usage = cpu_usage_map.get(key)
            cpu_req = cpu_req_map.get(key)
            cpu_limit = cpu_limit_map.get(key)
            cpu_req_ratio = (
                cpu_usage / cpu_req if cpu_usage is not None and cpu_req and cpu_req > 0 else None
            )
            mem_usage = mem_usage_map.get(key)
            mem_request = mem_request_map.get(key)
            mem_limit = mem_limit_map.get(key)
            mem_limit_ratio = (
                mem_usage / mem_limit
                if mem_usage is not None and mem_limit and mem_limit > 0
                else None
            )
            mem_ratio = mem_ratio_map.get(key)
            mem_ratio_recent = mem_ratio_recent_stats.get(key, {}).get("avg")
            mem_ratio_prev = mem_ratio_prev_stats.get(key, {}).get("avg")
            mem_ratio_trend = (
                _trend_label(mem_ratio_recent, mem_ratio_prev, flat_delta)
                if trend_enabled
                else "-"
            )
            mem_rss = mem_rss_map.get(key)
            mem_rate = mem_rate_map.get(key)
            net_rx_bytes_rate = net_rx_bytes_rate_map.get(key)
            net_tx_bytes_rate = net_tx_bytes_rate_map.get(key)
            net_rx_packets_rate = net_rx_packets_rate_map.get(key)
            net_tx_packets_rate = net_tx_packets_rate_map.get(key)
            net_rx_errors_rate = net_rx_errors_rate_map.get(key)
            net_tx_errors_rate = net_tx_errors_rate_map.get(key)
            net_rx_drops_rate = net_rx_drops_rate_map.get(key)
            net_tx_drops_rate = net_tx_drops_rate_map.get(key)
            cpu_throttle_ratio = cpu_throttle_ratio_map.get(key)
            cpu_throttle_recent = cpu_throttle_recent_stats.get(key, {}).get("avg")
            cpu_throttle_prev = cpu_throttle_prev_stats.get(key, {}).get("avg")
            cpu_throttle_trend = (
                _trend_label(cpu_throttle_recent, cpu_throttle_prev, flat_delta)
                if trend_enabled
                else "-"
            )
            cpu_throttled_seconds_rate = cpu_throttled_seconds_rate_map.get(key)
            cpu_cfs_periods_rate = cpu_cfs_periods_rate_map.get(key)
            fs_usage = fs_usage_map.get(key)
            fs_limit = fs_limit_map.get(key)
            fs_usage_ratio = (
                fs_usage / fs_limit if fs_usage is not None and fs_limit and fs_limit > 0 else None
            )
            oom_events = oom_events_map.get(key)
            restart_rate = restart_rate_map.get(key)
            ready_count = ready_map.get(key)
            ready_total = ready_total_map.get(key)
            ready_ratio = (
                ready_count / ready_total
                if ready_count is not None and ready_total
                else None
            )
            running = running_map.get(key)
            pending = pending_map.get(key)
            phase = phase_map.get(key)
            status = "Running" if phase == "Running" else "NotRunning"
            if phase is None:
                if running is not None and running > 0:
                    status = "Running"
                    phase = "Running"
                elif pending is not None and pending > 0:
                    phase = "Pending"
            pending_reason = pending_reason_map.get(key) if phase == "Pending" else None
            oom_state = pod_oom_stats.get(key, {}).get("max")
            pod_meta[key] = {
                "cluster": cluster_name,
                "namespace": namespace,
                "pod": pod_name,
                "instance": instance_name,
                "pod_status": status,
                "pending_reason": pending_reason,
                "phase": phase,
                "waiting_reason": waiting_reason_map.get(key),
                "terminated_reason": terminated_reason_map.get(key),
                "terminated_exitcode": terminated_exitcode_map.get(key),
                "last_terminated_time": last_terminated_time_map.get(key),
                "cpu_usage_cores": cpu_usage,
                "cpu_request_cores": cpu_req,
                "cpu_limit_cores": cpu_limit,
                "cpu_request_limit_ratio": cpu_req / cpu_limit
                if cpu_req is not None and cpu_limit and cpu_limit > 0
                else None,
                "mem_working_set_bytes": mem_usage,
                "mem_request_bytes": mem_request,
                "mem_limit_bytes": mem_limit,
                "mem_request_limit_ratio": mem_request / mem_limit
                if mem_request is not None and mem_limit and mem_limit > 0
                else None,
                "mem_rss_bytes": mem_rss,
                "mem_rate_bytes": mem_rate,
                "net_rx_bytes_rate": net_rx_bytes_rate,
                "net_tx_bytes_rate": net_tx_bytes_rate,
                "net_rx_packets_rate": net_rx_packets_rate,
                "net_tx_packets_rate": net_tx_packets_rate,
                "net_rx_errors_rate": net_rx_errors_rate,
                "net_tx_errors_rate": net_tx_errors_rate,
                "net_rx_drops_rate": net_rx_drops_rate,
                "net_tx_drops_rate": net_tx_drops_rate,
                "mem_ratio": mem_ratio,
                "mem_ratio_trend": mem_ratio_trend,
                "cpu_throttle_ratio": cpu_throttle_ratio,
                "cpu_throttle_trend": cpu_throttle_trend,
                "cpu_throttled_seconds_rate": cpu_throttled_seconds_rate,
                "cpu_cfs_periods_rate": cpu_cfs_periods_rate,
                "fs_usage_ratio": fs_usage_ratio,
                "oom_events_total": oom_events,
                "ready_count": ready_count,
                "ready_total": ready_total,
                "ready_ratio": ready_ratio,
                "restarts": restarts_map.get(key),
                "restarts_rate": restart_rate,
                "restarts_total": restarts_total_map.get(key),
                "node_oom_rate": node_oom_map.get(key),
                "node_mem_available_bytes": node_mem_available_map.get(key),
                "node_mem_total_bytes": node_mem_total_map.get(key),
                "node_load1": node_load1_map.get(key),
                "node_cpu_usage_ratio": node_cpu_usage_map.get(key),
                "node_up": node_up_map.get(key),
                "node_scrape_samples": node_scrape_samples_map.get(key),
                "node_scrape_samples_post": node_scrape_samples_post_map.get(key),
                "node_name": node_name_map.get(key),
                "restart_window_hours": restart_hours,
                "cpu_request_ratio": cpu_req_ratio,
                "mem_limit_ratio": mem_limit_ratio,
                "oom": oom_state > 0 if oom_state is not None else None,
            }

    for spec in RESOURCE_SPECS:
        if only_disk and spec.get("group") != GROUP_DISK:
            continue
        if spec.get("group") == GROUP_POD and not pod_enabled:
            continue

        query = spec.get("query")
        if not query and spec.get("query_builder"):
            query = spec["query_builder"]()

        key_fields = list(spec.get("key_fields", ["instance"]))
        if cluster_enabled and "cluster" not in key_fields:
            key_fields = key_fields + ["cluster"]
        size_by_key = {}
        avail_by_key = {}
        if spec.get("key") == "disk":
            size_entries = _collect_instant_values(
                prometheus_urls,
                _disk_size_query(),
                key_fields,
            )
            avail_entries = _collect_instant_values(
                prometheus_urls,
                _disk_avail_query(),
                key_fields,
            )
            size_by_key = {entry["key"]: entry.get("value") for entry in size_entries}
            avail_by_key = {entry["key"]: entry.get("value") for entry in avail_entries}

        current_values = _collect_instant_values(
            prometheus_urls,
            query,
            key_fields,
        )
        current_by_key = _index_by_key(current_values)

        range_stats = _collect_range_stats(
            prometheus_urls,
            query,
            start_ts,
            end_ts,
            step,
            key_fields,
        )

        recent_stats = {}
        prev_stats = {}
        short_recent_stats = {}
        short_prev_stats = {}
        forecast_stats = {}
        if trend_enabled and recent_start is not None and prev_start is not None:
            stats_recent_start = pod_recent_start if spec.get("group") == GROUP_POD else recent_start
            stats_prev_start = pod_prev_start if spec.get("group") == GROUP_POD else prev_start
            recent_stats = _collect_range_stats(
                prometheus_urls,
                query,
                stats_recent_start,
                end_ts,
                calc_step(stats_recent_start, end_ts),
                key_fields,
            )
            prev_stats = _collect_range_stats(
                prometheus_urls,
                query,
                stats_prev_start,
                stats_recent_start,
                calc_step(stats_prev_start, stats_recent_start),
                key_fields,
            )
        if (
            spec.get("group") == GROUP_POD
            and trend_enabled
            and pod_short_recent_start is not None
            and pod_short_prev_start is not None
        ):
            short_recent_stats = _collect_range_stats(
                prometheus_urls,
                query,
                pod_short_recent_start,
                end_ts,
                calc_step(pod_short_recent_start, end_ts),
                key_fields,
            )
            short_prev_stats = _collect_range_stats(
                prometheus_urls,
                query,
                pod_short_prev_start,
                pod_short_recent_start,
                calc_step(pod_short_prev_start, pod_short_recent_start),
                key_fields,
            )
        forecast_enabled = trend_enabled and spec.get("key") == "disk"
        if forecast_enabled and forecast_start is not None:
            forecast_stats = _collect_regression_stats(
                prometheus_urls,
                query,
                forecast_start,
                end_ts,
                calc_step(forecast_start, end_ts),
                key_fields,
            )

        all_keys = set(current_by_key) | set(range_stats)
        for key in all_keys:
            current_entry = current_by_key.get(key)
            range_entry = range_stats.get(key)
            labels = {}
            if current_entry:
                labels = current_entry.get("labels") or {}
            elif range_entry:
                labels = range_entry.get("labels") or {}

            inst = labels.get("instance", "unknown")
            mountpoint = labels.get("mountpoint")
            namespace = labels.get("namespace")
            pod_name = labels.get("pod")
            if spec.get("group") == GROUP_POD:
                pod_parts = [part for part in (namespace, pod_name) if part]
                inst = "/".join(pod_parts) if pod_parts else "unknown"
                mountpoint = None
            current = current_entry.get("value") if current_entry else None
            period_max = range_entry.get("max") if range_entry else None
            period_avg = range_entry.get("avg") if range_entry else None
            recent_avg = recent_stats.get(key, {}).get("avg") if trend_enabled else None
            prev_avg = prev_stats.get(key, {}).get("avg") if trend_enabled else None
            short_recent_avg = short_recent_stats.get(key, {}).get("avg") if trend_enabled else None
            short_prev_avg = short_prev_stats.get(key, {}).get("avg") if trend_enabled else None
            slope_per_day = forecast_stats.get(key, {}).get("slope_per_day") if forecast_enabled else None
            forecast_days = (
                _forecast_days_from_slope(current, slope_per_day, usage_alert)
                if forecast_enabled
                else None
            )
            remaining_current_value = 1 - current if current is not None else None
            remaining_min_value = 1 - period_max if period_max is not None else None
            size_bytes = size_by_key.get(key)
            avail_bytes = avail_by_key.get(key)
            used_bytes = None
            if size_bytes is not None and avail_bytes is not None:
                used_bytes = max(size_bytes - avail_bytes, 0)
            if spec.get("group") == GROUP_DISK:
                if not _mountpoint_allowed(mountpoint):
                    continue
                if size_bytes is not None and size_bytes <= 0:
                    continue
            group_label = _group_label(
                labels,
                pod_group_labels if spec.get("group") == GROUP_POD else group_labels,
            )
            pod_stats = pod_meta.get(key, {}) if spec.get("group") == GROUP_POD else {}
            if spec.get("group") == GROUP_POD and pod_stats.get("phase") == "Succeeded":
                continue
            oom_value = pod_stats.get("oom")
            if (
                spec.get("key") == "disk"
                and current is None
                and period_max is None
                and size_bytes is None
                and avail_bytes is None
            ):
                continue

            entries.append(
                {
                    "name": spec["name"],
                    "key": spec["key"],
                    "group": spec["group"],
                    "instance": inst,
                    "mountpoint": mountpoint,
                    "labels": labels,
                    "group_label": group_label,
                    "current": current,
                    "period_max": period_max,
                    "period_avg": period_avg,
                    "recent_avg": recent_avg,
                    "prev_avg": prev_avg,
                    "short_recent_avg": short_recent_avg,
                    "short_prev_avg": short_prev_avg,
                    "trend": _trend_label(recent_avg, prev_avg, flat_delta)
                    if trend_enabled
                    else "-",
                    "forecast_alert_days": forecast_days,
                    "oom": oom_value,
                    "pod_status": pod_stats.get("pod_status"),
                    "pending_reason": pod_stats.get("pending_reason"),
                    "phase": pod_stats.get("phase"),
                    "waiting_reason": pod_stats.get("waiting_reason"),
                    "terminated_reason": pod_stats.get("terminated_reason"),
                    "terminated_exitcode": pod_stats.get("terminated_exitcode"),
                    "last_terminated_time": pod_stats.get("last_terminated_time"),
                    "cpu_usage_cores": pod_stats.get("cpu_usage_cores"),
                    "cpu_request_cores": pod_stats.get("cpu_request_cores"),
                    "cpu_limit_cores": pod_stats.get("cpu_limit_cores"),
                    "cpu_request_limit_ratio": pod_stats.get("cpu_request_limit_ratio"),
                    "mem_working_set_bytes": pod_stats.get("mem_working_set_bytes"),
                    "mem_request_bytes": pod_stats.get("mem_request_bytes"),
                    "mem_limit_bytes": pod_stats.get("mem_limit_bytes"),
                    "mem_request_limit_ratio": pod_stats.get("mem_request_limit_ratio"),
                    "mem_rss_bytes": pod_stats.get("mem_rss_bytes"),
                    "mem_rate_bytes": pod_stats.get("mem_rate_bytes"),
                    "net_rx_bytes_rate": pod_stats.get("net_rx_bytes_rate"),
                    "net_tx_bytes_rate": pod_stats.get("net_tx_bytes_rate"),
                    "net_rx_packets_rate": pod_stats.get("net_rx_packets_rate"),
                    "net_tx_packets_rate": pod_stats.get("net_tx_packets_rate"),
                    "net_rx_errors_rate": pod_stats.get("net_rx_errors_rate"),
                    "net_tx_errors_rate": pod_stats.get("net_tx_errors_rate"),
                    "net_rx_drops_rate": pod_stats.get("net_rx_drops_rate"),
                    "net_tx_drops_rate": pod_stats.get("net_tx_drops_rate"),
                    "mem_ratio": pod_stats.get("mem_ratio"),
                    "mem_ratio_trend": pod_stats.get("mem_ratio_trend"),
                    "cpu_throttle_ratio": pod_stats.get("cpu_throttle_ratio"),
                    "cpu_throttle_trend": pod_stats.get("cpu_throttle_trend"),
                    "cpu_throttled_seconds_rate": pod_stats.get("cpu_throttled_seconds_rate"),
                    "cpu_cfs_periods_rate": pod_stats.get("cpu_cfs_periods_rate"),
                    "fs_usage_ratio": pod_stats.get("fs_usage_ratio"),
                    "oom_events_total": pod_stats.get("oom_events_total"),
                    "ready_count": pod_stats.get("ready_count"),
                    "ready_total": pod_stats.get("ready_total"),
                    "ready_ratio": pod_stats.get("ready_ratio"),
                    "restarts": pod_stats.get("restarts"),
                    "restarts_rate": pod_stats.get("restarts_rate"),
                    "restarts_total": pod_stats.get("restarts_total"),
                    "node_oom_rate": pod_stats.get("node_oom_rate"),
                    "node_mem_available_bytes": pod_stats.get("node_mem_available_bytes"),
                    "node_mem_total_bytes": pod_stats.get("node_mem_total_bytes"),
                    "node_load1": pod_stats.get("node_load1"),
                    "node_cpu_usage_ratio": pod_stats.get("node_cpu_usage_ratio"),
                    "node_up": pod_stats.get("node_up"),
                    "node_scrape_samples": pod_stats.get("node_scrape_samples"),
                    "node_scrape_samples_post": pod_stats.get("node_scrape_samples_post"),
                    "node_name": pod_stats.get("node_name"),
                    "restart_window_hours": pod_stats.get("restart_window_hours"),
                    "cpu_request_ratio": pod_stats.get("cpu_request_ratio"),
                    "mem_limit_ratio": pod_stats.get("mem_limit_ratio"),
                    "remaining_current": remaining_current_value,
                    "remaining_min": remaining_min_value,
                    "size_bytes": size_bytes,
                    "avail_bytes": avail_bytes,
                    "used_bytes": used_bytes,
                }
            )

            level = _classify_level(spec, current)
            if level:
                bucketed[level].append(
                    {
                        "name": spec["name"],
                        "instance": _format_target(inst, mountpoint),
                        "mountpoint": mountpoint,
                        "raw_instance": inst,
                        "value": current,
                        "group": spec["group"],
                        "key": spec["key"],
                    }
                )
            if level in {"red", "yellow"}:
                severity = "严重" if level == "red" else "关注"
                if spec["group"] == GROUP_DISK:
                    suggestion = "建议清理，必要时评估扩容" if level == "red" else "建议清理"
                else:
                    suggestion = "建议关注负载" if level == "red" else "建议观察"
                pressure_entries.append(
                    {
                        "metric": spec["key"],
                        "name": spec["name"],
                        "group": spec["group"],
                        "instance": inst,
                        "mountpoint": mountpoint,
                        "group_label": group_label,
                        "current": current,
                        "period_max": period_max,
                        "recent_avg": recent_avg,
                        "prev_avg": prev_avg,
                        "size_bytes": size_bytes,
                        "avail_bytes": avail_bytes,
                        "used_bytes": used_bytes,
                        "trend": _trend_label(recent_avg, prev_avg, flat_delta)
                        if trend_enabled
                        else "-",
                        "forecast_alert_days": forecast_days,
                        "oom": oom_value,
                        "level": severity,
                        "suggestion": suggestion,
                    }
                )

            if current is None or period_max is None:
                continue
            if (
                current <= remaining_current_threshold
                and period_max <= remaining_max_threshold
            ):
                surplus_level = (
                    "富余"
                    if (
                        current <= abundant_current_threshold
                        and period_max <= abundant_max_threshold
                    )
                    else "一般"
                )
                suggestion = "可评估缩容/合并" if surplus_level == "富余" else "保留"
                surplus_entries.append(
                    {
                        "metric": spec["key"],
                        "name": spec["name"],
                        "group": spec["group"],
                        "instance": inst,
                        "mountpoint": mountpoint,
                        "group_label": group_label,
                        "current": current,
                        "period_max": period_max,
                        "recent_avg": recent_avg,
                        "prev_avg": prev_avg,
                        "size_bytes": size_bytes,
                        "avail_bytes": avail_bytes,
                        "used_bytes": used_bytes,
                        "trend": _trend_label(recent_avg, prev_avg, flat_delta)
                        if trend_enabled
                        else "-",
                        "forecast_alert_days": forecast_days,
                        "oom": oom_value,
                        "level": surplus_level,
                        "suggestion": suggestion,
                    }
                )

    return {
        "entries": entries,
        "pod_states": sorted(
            [dict(value) for value in pod_meta.values()],
            key=lambda item: (
                item.get("cluster") or "",
                item.get("namespace") or "",
                item.get("pod") or "",
            ),
        ),
        "surplus_entries": surplus_entries,
        "pressure_entries": pressure_entries,
        "bucketed": bucketed,
        "only_disk": only_disk,
        "trend_enabled": trend_enabled,
        "trend_days": trend_days,
        "pod_trend_days": pod_trend_days,
        "pod_short_window_minutes": pod_short_window_minutes,
        "spec_by_key": spec_by_key,
        "thresholds": {
            "current": remaining_current_threshold,
            "max": remaining_max_threshold,
            "abundant_current": abundant_current_threshold,
            "abundant_max": abundant_max_threshold,
        },
    }

def render_resource_section(resource_data, start_ts, end_ts):
    start_str = datetime.datetime.fromtimestamp(start_ts).strftime("%Y-%m-%d %H:%M:%S")
    end_str = datetime.datetime.fromtimestamp(end_ts).strftime("%Y-%m-%d %H:%M:%S")
    only_disk = resource_data.get("only_disk", False)
    trend_enabled = resource_data.get("trend_enabled", False)
    trend_days = resource_data.get("trend_days", 7)
    thresholds = resource_data.get("thresholds", {})
    surplus_entries = resource_data.get("surplus_entries", []) or []
    pressure_entries = resource_data.get("pressure_entries", []) or []
    spec_by_key = resource_data.get("spec_by_key", {}) or {}

    out = []
    out.append(
        f"统计周期：**{start_str} ～ {end_str}**（用于余量筛选）\n"
    )
    if only_disk:
        out.append("说明：仅关注磁盘容量（Disk 使用率）。\n")
    else:
        out.append("说明：CPU / Mem / Disk 余量按统一规则筛选。\n")
    out.append(
        f"余量规则：当前<= {_fmt_pct(thresholds.get('current'))} 且 周期内最高<= {_fmt_pct(thresholds.get('max'))} 视为余量；"
        f"富余：当前<= {_fmt_pct(thresholds.get('abundant_current'))} 且 周期内最高<= {_fmt_pct(thresholds.get('abundant_max'))}。\n"
    )
    if spec_by_key:
        disk = spec_by_key.get("disk")
        cpu = spec_by_key.get("cpu")
        mem = spec_by_key.get("mem")
        if disk or cpu or mem:
            lines = []
            if disk:
                lines.append(
                    f"Disk 高占用阈值：>= {_fmt_pct(disk.get('yellow'))} 关注 / >= {_fmt_pct(disk.get('red'))} 严重"
                )
            if cpu:
                lines.append(
                    f"CPU 高占用阈值：>= {_fmt_pct(cpu.get('yellow'))} 关注 / >= {_fmt_pct(cpu.get('red'))} 严重"
                )
            if mem:
                lines.append(
                    f"Mem 高占用阈值：>= {_fmt_pct(mem.get('yellow'))} 关注 / >= {_fmt_pct(mem.get('red'))} 严重"
                )
            out.append("高占用规则（当前值）： " + "；".join(lines) + "。\n")
    out.append("仅列持续低负载（周期内最高低于阈值），避免偶发低值误判。\n")
    if trend_enabled:
        out.append(f"趋势对比：近{trend_days}天均值 vs 前{trend_days}天均值。\n")
    out.append("\n")

    def _render_surplus_table(title, items, include_mountpoint, include_bytes=False):
        out.append(f"### {title}余量清单\n")
        if not items:
            out.append("无\n\n")
            return

        group_enabled = any(item.get("group_label") not in (None, "", "-") for item in items)
        bytes_enabled = include_bytes and any(item.get("size_bytes") for item in items)
        cols = []
        if group_enabled:
            cols.append("分组")
        cols.append("机器")
        if include_mountpoint:
            cols.append("挂载点")
        if bytes_enabled:
            cols.extend(["容量", "已用", "可用"])
        cols.append("当前使用率")
        cols.append("周期内最高")
        cols.append("余量等级")
        cols.append("建议")
        if trend_enabled:
            cols.extend([f"近{trend_days}天均值", f"前{trend_days}天均值", "趋势"])

        out.append("| " + " | ".join(cols) + " |\n")
        out.append("| " + " | ".join(["---"] * len(cols)) + " |\n")

        def _sort_key(item):
            return (
                item.get("group_label") or "",
                item.get("instance") or "",
                item.get("mountpoint") or "",
            )

        for item in sorted(items, key=_sort_key):
            row = []
            if group_enabled:
                row.append(item.get("group_label", "-"))
            row.append(item.get("instance", "unknown"))
            if include_mountpoint:
                row.append(item.get("mountpoint") or "-")
            if bytes_enabled:
                row.append(_fmt_bytes(item.get("size_bytes")))
                row.append(_fmt_bytes(item.get("used_bytes")))
                row.append(_fmt_bytes(item.get("avail_bytes")))
            row.append(_fmt_pct(item.get("current")))
            row.append(_fmt_pct(item.get("period_max")))
            row.append(item.get("level", ""))
            row.append(item.get("suggestion", ""))
            if trend_enabled:
                row.append(_fmt_pct(item.get("recent_avg")))
                row.append(_fmt_pct(item.get("prev_avg")))
                row.append(item.get("trend") or "-")
            out.append("| " + " | ".join(row) + " |\n")
        out.append("\n")

    def _render_pressure_table(title, items, include_mountpoint, include_bytes=False):
        out.append(f"### {title}高占用清单（建议清理优先）\n")
        if not items:
            out.append("无\n\n")
            return

        group_enabled = any(item.get("group_label") not in (None, "", "-") for item in items)
        bytes_enabled = include_bytes and any(item.get("size_bytes") for item in items)
        cols = []
        if group_enabled:
            cols.append("分组")
        cols.append("机器")
        if include_mountpoint:
            cols.append("挂载点")
        if bytes_enabled:
            cols.extend(["容量", "已用", "可用"])
        cols.append("当前使用率")
        cols.append("周期内最高")
        cols.append("等级")
        cols.append("建议")
        if trend_enabled:
            cols.extend([f"近{trend_days}天均值", f"前{trend_days}天均值", "趋势"])

        out.append("| " + " | ".join(cols) + " |\n")
        out.append("| " + " | ".join(["---"] * len(cols)) + " |\n")

        def _sort_key(item):
            level_rank = 0 if item.get("level") == "严重" else 1
            return (
                level_rank,
                item.get("group_label") or "",
                item.get("instance") or "",
                item.get("mountpoint") or "",
            )

        for item in sorted(items, key=_sort_key):
            row = []
            if group_enabled:
                row.append(item.get("group_label", "-"))
            row.append(item.get("instance", "unknown"))
            if include_mountpoint:
                row.append(item.get("mountpoint") or "-")
            if bytes_enabled:
                row.append(_fmt_bytes(item.get("size_bytes")))
                row.append(_fmt_bytes(item.get("used_bytes")))
                row.append(_fmt_bytes(item.get("avail_bytes")))
            row.append(_fmt_pct(item.get("current")))
            row.append(_fmt_pct(item.get("period_max")))
            row.append(item.get("level", ""))
            row.append(item.get("suggestion", ""))
            if trend_enabled:
                row.append(_fmt_pct(item.get("recent_avg")))
                row.append(_fmt_pct(item.get("prev_avg")))
                row.append(item.get("trend") or "-")
            out.append("| " + " | ".join(row) + " |\n")
        out.append("\n")

    disk_items = [i for i in surplus_entries if i.get("metric") == "disk"]
    disk_pressure = [i for i in pressure_entries if i.get("metric") == "disk"]
    _render_pressure_table("磁盘", disk_pressure, include_mountpoint=True, include_bytes=True)
    _render_surplus_table("磁盘", disk_items, include_mountpoint=True, include_bytes=True)

    if only_disk:
        return "".join(out)

    cpu_items = [i for i in surplus_entries if i.get("metric") == "cpu"]
    mem_items = [i for i in surplus_entries if i.get("metric") == "mem"]
    cpu_pressure = [i for i in pressure_entries if i.get("metric") == "cpu"]
    mem_pressure = [i for i in pressure_entries if i.get("metric") == "mem"]
    _render_pressure_table("CPU（低优先级）", cpu_pressure, include_mountpoint=False)
    _render_pressure_table("Mem（低优先级）", mem_pressure, include_mountpoint=False)
    _render_surplus_table("CPU", cpu_items, include_mountpoint=False)
    _render_surplus_table("Mem", mem_items, include_mountpoint=False)

    return "".join(out)

def generate_resource_section(prometheus_urls, start_ts, end_ts):
    data = collect_resource_data(prometheus_urls, start_ts, end_ts)
    return render_resource_section(data, start_ts, end_ts)

def build_dashboard_payload(resource_data, start_ts, end_ts):
    usage_alert = float(getattr(config, "RESOURCE_USAGE_ALERT", 0.80))
    usage_watch = float(getattr(config, "RESOURCE_USAGE_WATCH", 0.70))
    remaining_alert = 1 - usage_alert
    remaining_watch = 1 - usage_watch

    start_str = datetime.datetime.fromtimestamp(start_ts).strftime("%Y-%m-%d %H:%M:%S")
    end_str = datetime.datetime.fromtimestamp(end_ts).strftime("%Y-%m-%d %H:%M:%S")
    trend_enabled = bool(resource_data.get("trend_enabled", False))
    trend_days = int(resource_data.get("trend_days", 7))
    pod_trend_days = int(resource_data.get("pod_trend_days", trend_days))
    pod_short_window_minutes = int(resource_data.get("pod_short_window_minutes", 30))

    def _usage_zone(value):
        if value is None:
            return "unknown"
        if value >= usage_alert:
            return "alert"
        if value >= usage_watch:
            return "watch"
        return "safe"

    def _remaining_zone(value):
        if value is None:
            return "unknown"
        if value <= remaining_alert:
            return "alert"
        if value <= remaining_watch:
            return "watch"
        return "safe"

    def _extract_cluster(item):
        labels = item.get("labels") or {}
        cluster = labels.get("cluster")
        if not cluster:
            group_label = item.get("group_label") or ""
            for token in group_label.split():
                if token.startswith("cluster="):
                    cluster = token.split("=", 1)[1]
                    break
        return cluster or "-"

    items = []
    pod_combined = {}
    for item in resource_data.get("entries", []) or []:
        key = item.get("key")
        if key in {"pod_cpu", "pod_mem"}:
            cluster = _extract_cluster(item)
            pod_key = (item.get("instance"), cluster)
            state = pod_combined.get(pod_key)
            if state is None:
                state = {
                    "metric": "pod",
                    "instance": item.get("instance"),
                    "mountpoint": None,
                    "cluster": cluster,
                    "group": item.get("group_label") or "-",
                    "trend": item.get("trend"),
                    "forecast_alert_days": item.get("forecast_alert_days"),
                    "oom": item.get("oom"),
                    "pod_status": item.get("pod_status"),
                    "pending_reason": item.get("pending_reason"),
                    "phase": item.get("phase"),
                    "waiting_reason": item.get("waiting_reason"),
                    "terminated_reason": item.get("terminated_reason"),
                    "terminated_exitcode": item.get("terminated_exitcode"),
                    "last_terminated_time": item.get("last_terminated_time"),
                    "cpu_usage_cores": item.get("cpu_usage_cores"),
                    "cpu_request_cores": item.get("cpu_request_cores"),
                    "cpu_limit_cores": item.get("cpu_limit_cores"),
                    "cpu_request_limit_ratio": item.get("cpu_request_limit_ratio"),
                    "mem_working_set_bytes": item.get("mem_working_set_bytes"),
                    "mem_request_bytes": item.get("mem_request_bytes"),
                    "mem_limit_bytes": item.get("mem_limit_bytes"),
                    "mem_request_limit_ratio": item.get("mem_request_limit_ratio"),
                    "mem_rss_bytes": item.get("mem_rss_bytes"),
                    "mem_rate_bytes": item.get("mem_rate_bytes"),
                    "net_rx_bytes_rate": item.get("net_rx_bytes_rate"),
                    "net_tx_bytes_rate": item.get("net_tx_bytes_rate"),
                    "net_rx_packets_rate": item.get("net_rx_packets_rate"),
                    "net_tx_packets_rate": item.get("net_tx_packets_rate"),
                    "net_rx_errors_rate": item.get("net_rx_errors_rate"),
                    "net_tx_errors_rate": item.get("net_tx_errors_rate"),
                    "net_rx_drops_rate": item.get("net_rx_drops_rate"),
                    "net_tx_drops_rate": item.get("net_tx_drops_rate"),
                    "mem_ratio": item.get("mem_ratio"),
                    "mem_ratio_trend": item.get("mem_ratio_trend"),
                    "cpu_throttle_ratio": item.get("cpu_throttle_ratio"),
                    "cpu_throttle_trend": item.get("cpu_throttle_trend"),
                    "cpu_throttled_seconds_rate": item.get("cpu_throttled_seconds_rate"),
                    "cpu_cfs_periods_rate": item.get("cpu_cfs_periods_rate"),
                    "fs_usage_ratio": item.get("fs_usage_ratio"),
                    "oom_events_total": item.get("oom_events_total"),
                    "ready_count": item.get("ready_count"),
                    "ready_total": item.get("ready_total"),
                    "ready_ratio": item.get("ready_ratio"),
                    "restarts": item.get("restarts"),
                    "restarts_rate": item.get("restarts_rate"),
                    "restarts_total": item.get("restarts_total"),
                    "node_oom_rate": item.get("node_oom_rate"),
                    "node_mem_available_bytes": item.get("node_mem_available_bytes"),
                    "node_mem_total_bytes": item.get("node_mem_total_bytes"),
                    "node_load1": item.get("node_load1"),
                    "node_cpu_usage_ratio": item.get("node_cpu_usage_ratio"),
                    "node_up": item.get("node_up"),
                    "node_scrape_samples": item.get("node_scrape_samples"),
                    "node_scrape_samples_post": item.get("node_scrape_samples_post"),
                    "node_name": item.get("node_name"),
                    "restart_window_hours": item.get("restart_window_hours"),
                    "cpu_request_ratio": item.get("cpu_request_ratio"),
                    "mem_limit_ratio": item.get("mem_limit_ratio"),
                    "cpu_usage": None,
                    "cpu_max": None,
                    "mem_usage": None,
                    "mem_max": None,
                    "cpu_trend": None,
                    "mem_trend": None,
                    "cpu_short_recent_avg": None,
                    "cpu_short_prev_avg": None,
                    "mem_short_recent_avg": None,
                    "mem_short_prev_avg": None,
                }
                pod_combined[pod_key] = state
            if key == "pod_cpu":
                state["cpu_usage"] = item.get("current")
                state["cpu_max"] = item.get("period_max")
                state["cpu_trend"] = item.get("trend")
                state["cpu_recent_avg"] = item.get("recent_avg")
                state["cpu_prev_avg"] = item.get("prev_avg")
                state["cpu_short_recent_avg"] = item.get("short_recent_avg")
                state["cpu_short_prev_avg"] = item.get("short_prev_avg")
                if item.get("forecast_alert_days") is not None:
                    state["forecast_alert_days"] = item.get("forecast_alert_days")
            if key == "pod_mem":
                state["mem_usage"] = item.get("current")
                state["mem_max"] = item.get("period_max")
                state["mem_trend"] = item.get("trend")
                state["mem_recent_avg"] = item.get("recent_avg")
                state["mem_prev_avg"] = item.get("prev_avg")
                state["mem_short_recent_avg"] = item.get("short_recent_avg")
                state["mem_short_prev_avg"] = item.get("short_prev_avg")
                if item.get("forecast_alert_days") is not None:
                    state["forecast_alert_days"] = item.get("forecast_alert_days")
            if item.get("oom") is not None:
                state["oom"] = item.get("oom")
            if item.get("pod_status"):
                state["pod_status"] = item.get("pod_status")
            if item.get("phase"):
                state["phase"] = item.get("phase")
            if item.get("waiting_reason"):
                state["waiting_reason"] = item.get("waiting_reason")
            if item.get("terminated_reason"):
                state["terminated_reason"] = item.get("terminated_reason")
            if item.get("terminated_exitcode") is not None:
                state["terminated_exitcode"] = item.get("terminated_exitcode")
            if item.get("last_terminated_time") is not None:
                state["last_terminated_time"] = item.get("last_terminated_time")
            if item.get("cpu_usage_cores") is not None:
                state["cpu_usage_cores"] = item.get("cpu_usage_cores")
            if item.get("cpu_request_cores") is not None:
                state["cpu_request_cores"] = item.get("cpu_request_cores")
            if item.get("cpu_limit_cores") is not None:
                state["cpu_limit_cores"] = item.get("cpu_limit_cores")
            if item.get("cpu_request_limit_ratio") is not None:
                state["cpu_request_limit_ratio"] = item.get("cpu_request_limit_ratio")
            if item.get("mem_working_set_bytes") is not None:
                state["mem_working_set_bytes"] = item.get("mem_working_set_bytes")
            if item.get("mem_request_bytes") is not None:
                state["mem_request_bytes"] = item.get("mem_request_bytes")
            if item.get("mem_limit_bytes") is not None:
                state["mem_limit_bytes"] = item.get("mem_limit_bytes")
            if item.get("mem_request_limit_ratio") is not None:
                state["mem_request_limit_ratio"] = item.get("mem_request_limit_ratio")
            if item.get("mem_rss_bytes") is not None:
                state["mem_rss_bytes"] = item.get("mem_rss_bytes")
            if item.get("mem_rate_bytes") is not None:
                state["mem_rate_bytes"] = item.get("mem_rate_bytes")
            if item.get("net_rx_bytes_rate") is not None:
                state["net_rx_bytes_rate"] = item.get("net_rx_bytes_rate")
            if item.get("net_tx_bytes_rate") is not None:
                state["net_tx_bytes_rate"] = item.get("net_tx_bytes_rate")
            if item.get("net_rx_packets_rate") is not None:
                state["net_rx_packets_rate"] = item.get("net_rx_packets_rate")
            if item.get("net_tx_packets_rate") is not None:
                state["net_tx_packets_rate"] = item.get("net_tx_packets_rate")
            if item.get("net_rx_errors_rate") is not None:
                state["net_rx_errors_rate"] = item.get("net_rx_errors_rate")
            if item.get("net_tx_errors_rate") is not None:
                state["net_tx_errors_rate"] = item.get("net_tx_errors_rate")
            if item.get("net_rx_drops_rate") is not None:
                state["net_rx_drops_rate"] = item.get("net_rx_drops_rate")
            if item.get("net_tx_drops_rate") is not None:
                state["net_tx_drops_rate"] = item.get("net_tx_drops_rate")
            if item.get("mem_ratio") is not None:
                state["mem_ratio"] = item.get("mem_ratio")
            if item.get("mem_ratio_trend"):
                state["mem_ratio_trend"] = item.get("mem_ratio_trend")
            if item.get("cpu_throttle_ratio") is not None:
                state["cpu_throttle_ratio"] = item.get("cpu_throttle_ratio")
            if item.get("cpu_throttle_trend"):
                state["cpu_throttle_trend"] = item.get("cpu_throttle_trend")
            if item.get("cpu_throttled_seconds_rate") is not None:
                state["cpu_throttled_seconds_rate"] = item.get("cpu_throttled_seconds_rate")
            if item.get("cpu_cfs_periods_rate") is not None:
                state["cpu_cfs_periods_rate"] = item.get("cpu_cfs_periods_rate")
            if item.get("fs_usage_ratio") is not None:
                state["fs_usage_ratio"] = item.get("fs_usage_ratio")
            if item.get("oom_events_total") is not None:
                state["oom_events_total"] = item.get("oom_events_total")
            if item.get("restarts_rate") is not None:
                state["restarts_rate"] = item.get("restarts_rate")
            if item.get("restarts_total") is not None:
                state["restarts_total"] = item.get("restarts_total")
            if item.get("node_oom_rate") is not None:
                state["node_oom_rate"] = item.get("node_oom_rate")
            if item.get("node_mem_available_bytes") is not None:
                state["node_mem_available_bytes"] = item.get("node_mem_available_bytes")
            if item.get("node_mem_total_bytes") is not None:
                state["node_mem_total_bytes"] = item.get("node_mem_total_bytes")
            if item.get("node_load1") is not None:
                state["node_load1"] = item.get("node_load1")
            if item.get("node_cpu_usage_ratio") is not None:
                state["node_cpu_usage_ratio"] = item.get("node_cpu_usage_ratio")
            if item.get("node_up") is not None:
                state["node_up"] = item.get("node_up")
            if item.get("node_scrape_samples") is not None:
                state["node_scrape_samples"] = item.get("node_scrape_samples")
            if item.get("node_scrape_samples_post") is not None:
                state["node_scrape_samples_post"] = item.get("node_scrape_samples_post")
            if item.get("node_name"):
                state["node_name"] = item.get("node_name")
            continue

        usage_current = item.get("current")
        remaining_current = item.get("remaining_current")
        cluster = _extract_cluster(item)
        items.append(
            {
                "metric": item.get("key"),
                "instance": item.get("instance"),
                "mountpoint": item.get("mountpoint"),
                "cluster": cluster,
                "group": item.get("group_label") or "-",
                "usage_current": usage_current,
                "usage_max": item.get("period_max"),
                "remaining_current": remaining_current,
                "remaining_min": item.get("remaining_min"),
                "size_bytes": item.get("size_bytes"),
                "used_bytes": item.get("used_bytes"),
                "avail_bytes": item.get("avail_bytes"),
                "trend": item.get("trend"),
                "forecast_alert_days": item.get("forecast_alert_days"),
                "oom": item.get("oom"),
                "usage_zone": _usage_zone(usage_current),
                "remaining_zone": _remaining_zone(remaining_current),
            }
        )

    for state in pod_combined.values():
        usage_values = [v for v in (state.get("cpu_usage"), state.get("mem_usage")) if v is not None]
        max_values = [v for v in (state.get("cpu_max"), state.get("mem_max")) if v is not None]
        usage_current = max(usage_values) if usage_values else None
        usage_max = max(max_values) if max_values else None
        remaining_current = 1 - usage_current if usage_current is not None else None
        remaining_min = 1 - usage_max if usage_max is not None else None
        trend = state.get("cpu_trend") or state.get("mem_trend") or "-"
        if state.get("cpu_usage") is not None and state.get("mem_usage") is not None:
            trend = state.get("cpu_trend") if state["cpu_usage"] >= state["mem_usage"] else state.get("mem_trend")
        forecast_days = state.get("forecast_alert_days")
        items.append(
            {
                "metric": "pod",
                "instance": state.get("instance"),
                "mountpoint": None,
                "cluster": state.get("cluster"),
                "group": state.get("group"),
                "usage_current": usage_current,
                "usage_max": usage_max,
                "remaining_current": remaining_current,
                "remaining_min": remaining_min,
                "size_bytes": None,
                "used_bytes": None,
                "avail_bytes": None,
                "trend": trend,
                "forecast_alert_days": forecast_days,
                "oom": state.get("oom"),
                "pod_status": state.get("pod_status"),
                "pending_reason": state.get("pending_reason"),
                "phase": state.get("phase"),
                "waiting_reason": state.get("waiting_reason"),
                "terminated_reason": state.get("terminated_reason"),
                "terminated_exitcode": state.get("terminated_exitcode"),
                "last_terminated_time": state.get("last_terminated_time"),
                "cpu_usage_cores": state.get("cpu_usage_cores"),
                "cpu_usage_ratio": state.get("cpu_usage"),
                "cpu_request_cores": state.get("cpu_request_cores"),
                "cpu_limit_cores": state.get("cpu_limit_cores"),
                "cpu_request_limit_ratio": state.get("cpu_request_limit_ratio"),
                "mem_working_set_bytes": state.get("mem_working_set_bytes"),
                "mem_usage_ratio": state.get("mem_usage"),
                "mem_request_bytes": state.get("mem_request_bytes"),
                "mem_limit_bytes": state.get("mem_limit_bytes"),
                "mem_request_limit_ratio": state.get("mem_request_limit_ratio"),
                "mem_rss_bytes": state.get("mem_rss_bytes"),
                "mem_rate_bytes": state.get("mem_rate_bytes"),
                "cpu_trend": state.get("cpu_trend"),
                "cpu_recent_avg": state.get("cpu_recent_avg"),
                "cpu_prev_avg": state.get("cpu_prev_avg"),
                "cpu_short_recent_avg": state.get("cpu_short_recent_avg"),
                "cpu_short_prev_avg": state.get("cpu_short_prev_avg"),
                "mem_trend": state.get("mem_trend"),
                "mem_recent_avg": state.get("mem_recent_avg"),
                "mem_prev_avg": state.get("mem_prev_avg"),
                "mem_short_recent_avg": state.get("mem_short_recent_avg"),
                "mem_short_prev_avg": state.get("mem_short_prev_avg"),
                "net_rx_bytes_rate": state.get("net_rx_bytes_rate"),
                "net_tx_bytes_rate": state.get("net_tx_bytes_rate"),
                "net_rx_packets_rate": state.get("net_rx_packets_rate"),
                "net_tx_packets_rate": state.get("net_tx_packets_rate"),
                "net_rx_errors_rate": state.get("net_rx_errors_rate"),
                "net_tx_errors_rate": state.get("net_tx_errors_rate"),
                "net_rx_drops_rate": state.get("net_rx_drops_rate"),
                "net_tx_drops_rate": state.get("net_tx_drops_rate"),
                "mem_ratio": state.get("mem_ratio"),
                "mem_ratio_trend": state.get("mem_ratio_trend"),
                "cpu_throttle_ratio": state.get("cpu_throttle_ratio"),
                "cpu_throttle_trend": state.get("cpu_throttle_trend"),
                "cpu_throttled_seconds_rate": state.get("cpu_throttled_seconds_rate"),
                "cpu_cfs_periods_rate": state.get("cpu_cfs_periods_rate"),
                "fs_usage_ratio": state.get("fs_usage_ratio"),
                "oom_events_total": state.get("oom_events_total"),
                "ready_count": state.get("ready_count"),
                "ready_total": state.get("ready_total"),
                "ready_ratio": state.get("ready_ratio"),
                "restarts": state.get("restarts"),
                "restarts_rate": state.get("restarts_rate"),
                "restarts_total": state.get("restarts_total"),
                "node_oom_rate": state.get("node_oom_rate"),
                "node_mem_available_bytes": state.get("node_mem_available_bytes"),
                "node_mem_total_bytes": state.get("node_mem_total_bytes"),
                "node_load1": state.get("node_load1"),
                "node_cpu_usage_ratio": state.get("node_cpu_usage_ratio"),
                "node_up": state.get("node_up"),
                "node_scrape_samples": state.get("node_scrape_samples"),
                "node_scrape_samples_post": state.get("node_scrape_samples_post"),
                "node_name": state.get("node_name"),
                "restart_window_hours": state.get("restart_window_hours"),
                "cpu_request_ratio": state.get("cpu_request_ratio"),
                "mem_limit_ratio": state.get("mem_limit_ratio"),
                "usage_zone": _usage_zone(usage_current),
                "remaining_zone": _remaining_zone(remaining_current),
            }
        )

    return {
        "generated_at": end_str,
        "window": {"start": start_str, "end": end_str},
        "usage_thresholds": {"alert": usage_alert, "watch": usage_watch},
        "remaining_thresholds": {"alert": remaining_alert, "watch": remaining_watch},
        "trend": {"enabled": trend_enabled, "days": trend_days},
        "pod_short_trend": {"window_minutes": pod_short_window_minutes},
        "pod_trend_rules": {
            "enabled": bool(getattr(config, "RESOURCE_POD_TREND_ANOMALY_ENABLED", True)),
            "watch_ratio": float(getattr(config, "RESOURCE_POD_TREND_WATCH_RATIO", 1.5)),
            "alert_ratio": float(getattr(config, "RESOURCE_POD_TREND_ALERT_RATIO", 2.0)),
            "watch_delta": float(getattr(config, "RESOURCE_POD_TREND_WATCH_DELTA", 0.10)),
            "alert_delta": float(getattr(config, "RESOURCE_POD_TREND_ALERT_DELTA", 0.20)),
            "min_current": float(getattr(config, "RESOURCE_POD_TREND_MIN_CURRENT", 0.30)),
            "baseline_floor": float(getattr(config, "RESOURCE_POD_TREND_BASELINE_FLOOR", 0.05)),
            "days": pod_trend_days,
        },
        "pod_states": resource_data.get("pod_states", []) or [],
        "items": items,
    }

def _build_pressure_markdown(
    title,
    items,
    *,
    trend_enabled,
    trend_days,
    include_mountpoint,
    include_bytes,
    include_metric,
):
    lines = []
    lines.append(f"# {title}高占用清单（建议清理优先）")
    if not items:
        lines.append("无")
        return "\n".join(lines) + "\n"

    group_enabled = any(item.get("group_label") not in (None, "", "-") for item in items)
    bytes_enabled = include_bytes and any(item.get("size_bytes") for item in items)
    cols = []
    if include_metric:
        cols.append("指标")
    if group_enabled:
        cols.append("分组")
    cols.append("机器")
    if include_mountpoint:
        cols.append("挂载点")
    if bytes_enabled:
        cols.extend(["容量", "已用", "可用"])
    cols.append("当前使用率")
    cols.append("周期内最高")
    cols.append("等级")
    cols.append("建议")
    if trend_enabled:
        cols.extend([f"近{trend_days}天均值", f"前{trend_days}天均值", "趋势"])

    lines.append("| " + " | ".join(cols) + " |")
    lines.append("| " + " | ".join(["---"] * len(cols)) + " |")

    def _sort_key(item):
        level_rank = 0 if item.get("level") == "严重" else 1
        return (
            level_rank,
            item.get("metric") or "",
            item.get("group_label") or "",
            item.get("instance") or "",
            item.get("mountpoint") or "",
        )

    for item in sorted(items, key=_sort_key):
        row = []
        if include_metric:
            row.append(item.get("metric", "-"))
        if group_enabled:
            row.append(item.get("group_label", "-"))
        row.append(item.get("instance", "unknown"))
        if include_mountpoint:
            row.append(item.get("mountpoint") or "-")
        if bytes_enabled:
            row.append(_fmt_bytes(item.get("size_bytes")))
            row.append(_fmt_bytes(item.get("used_bytes")))
            row.append(_fmt_bytes(item.get("avail_bytes")))
        row.append(_fmt_pct(item.get("current")))
        row.append(_fmt_pct(item.get("period_max")))
        row.append(item.get("level", ""))
        row.append(item.get("suggestion", ""))
        if trend_enabled:
            row.append(_fmt_pct(item.get("recent_avg")))
            row.append(_fmt_pct(item.get("prev_avg")))
            row.append(item.get("trend") or "-")
        lines.append("| " + " | ".join(row) + " |")

    return "\n".join(lines) + "\n"

def _build_surplus_markdown(
    title,
    items,
    *,
    trend_enabled,
    trend_days,
    include_mountpoint,
    include_bytes,
    include_metric,
):
    lines = []
    lines.append(f"# {title}余量清单")
    if not items:
        lines.append("无")
        return "\n".join(lines) + "\n"

    group_enabled = any(item.get("group_label") not in (None, "", "-") for item in items)
    bytes_enabled = include_bytes and any(item.get("size_bytes") for item in items)
    cols = []
    if include_metric:
        cols.append("指标")
    if group_enabled:
        cols.append("分组")
    cols.append("机器")
    if include_mountpoint:
        cols.append("挂载点")
    if bytes_enabled:
        cols.extend(["容量", "已用", "可用"])
    cols.append("当前使用率")
    cols.append("周期内最高")
    cols.append("余量等级")
    cols.append("建议")
    if trend_enabled:
        cols.extend([f"近{trend_days}天均值", f"前{trend_days}天均值", "趋势"])

    lines.append("| " + " | ".join(cols) + " |")
    lines.append("| " + " | ".join(["---"] * len(cols)) + " |")

    def _sort_key(item):
        return (
            item.get("metric") or "",
            item.get("group_label") or "",
            item.get("instance") or "",
            item.get("mountpoint") or "",
        )

    for item in sorted(items, key=_sort_key):
        row = []
        if include_metric:
            row.append(item.get("metric", "-"))
        if group_enabled:
            row.append(item.get("group_label", "-"))
        row.append(item.get("instance", "unknown"))
        if include_mountpoint:
            row.append(item.get("mountpoint") or "-")
        if bytes_enabled:
            row.append(_fmt_bytes(item.get("size_bytes")))
            row.append(_fmt_bytes(item.get("used_bytes")))
            row.append(_fmt_bytes(item.get("avail_bytes")))
        row.append(_fmt_pct(item.get("current")))
        row.append(_fmt_pct(item.get("period_max")))
        row.append(item.get("level", ""))
        row.append(item.get("suggestion", ""))
        if trend_enabled:
            row.append(_fmt_pct(item.get("recent_avg")))
            row.append(_fmt_pct(item.get("prev_avg")))
            row.append(item.get("trend") or "-")
        lines.append("| " + " | ".join(row) + " |")

    return "\n".join(lines) + "\n"

def write_resource_outputs(resource_data, start_ts, end_ts, output_dir=None):
    base_dir = output_dir or getattr(config, "RESOURCE_OUTPUT_DIR", "outputs")
    alerts_dir = os.path.join(base_dir, "alerts", "prewarn")
    spare_dir = os.path.join(base_dir, "spare_resources")
    os.makedirs(alerts_dir, exist_ok=True)
    os.makedirs(spare_dir, exist_ok=True)

    start_str = datetime.datetime.fromtimestamp(start_ts).strftime("%Y-%m-%d %H:%M:%S")
    end_str = datetime.datetime.fromtimestamp(end_ts).strftime("%Y-%m-%d %H:%M:%S")

    trend_enabled = bool(resource_data.get("trend_enabled", False))
    trend_days = int(resource_data.get("trend_days", 7))
    entries = resource_data.get("entries", []) or []
    usage_alert = float(getattr(config, "RESOURCE_USAGE_ALERT", 0.80))
    usage_watch = float(getattr(config, "RESOURCE_USAGE_WATCH", 0.70))
    remaining_alert = 1 - usage_alert
    remaining_watch = 1 - usage_watch

    def _write(path, content):
        with open(path, "w", encoding="utf-8") as f:
            f.write(content)
    def _write_json(path, data):
        with open(path, "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=2)

    header = (
        f"> 统计周期：{start_str} ～ {end_str}\n"
        f"> 采样时间：{end_str}\n\n"
    )

    def _classify_usage(value):
        if value is None:
            return "未知"
        if value >= usage_alert:
            return "告警"
        if value >= usage_watch:
            return "关注"
        return "安全"

    def _classify_remaining(value):
        if value is None:
            return "未知"
        if value <= remaining_alert:
            return "告警"
        if value <= remaining_watch:
            return "关注"
        return "安全"

    def _fmt_oom(value):
        if value is None:
            return "-"
        return "是" if value else "否"

    def _split(entries, metric):
        return [item for item in entries if item.get("key") == metric]

    def _render_zone_table(items, *, include_metric, include_mountpoint, include_bytes, mode):
        group_enabled = any(item.get("group_label") not in (None, "", "-") for item in items)
        bytes_enabled = include_bytes and any(item.get("size_bytes") for item in items)
        oom_enabled = any(
            (item.get("key") or "").startswith("pod_") for item in items
        )
        cols = []
        if include_metric:
            cols.append("指标")
        if group_enabled:
            cols.append("分组")
        cols.append("机器")
        if include_mountpoint:
            cols.append("挂载点")
        if bytes_enabled:
            cols.extend(["容量", "已用", "可用"])
        if oom_enabled:
            cols.append("OOM")
        if mode == "usage":
            cols.extend(["当前使用率", "周期内最高"])
        else:
            cols.extend(["当前剩余率", "周期内最低"])
        if trend_enabled:
            cols.extend([f"近{trend_days}天均值", f"前{trend_days}天均值", "趋势"])

        lines = []
        lines.append("| " + " | ".join(cols) + " |")
        lines.append("| " + " | ".join(["---"] * len(cols)) + " |")

        def _sort_key(item):
            return (
                item.get("key") or "",
                item.get("group_label") or "",
                item.get("instance") or "",
                item.get("mountpoint") or "",
            )

        for item in sorted(items, key=_sort_key):
            row = []
            if include_metric:
                row.append(item.get("key", "-"))
            if group_enabled:
                row.append(item.get("group_label", "-"))
            row.append(item.get("instance", "unknown"))
            if include_mountpoint:
                row.append(item.get("mountpoint") or "-")
            if bytes_enabled:
                row.append(_fmt_bytes(item.get("size_bytes")))
                row.append(_fmt_bytes(item.get("used_bytes")))
                row.append(_fmt_bytes(item.get("avail_bytes")))
            if oom_enabled:
                row.append(_fmt_oom(item.get("oom")))
            if mode == "usage":
                row.append(_fmt_pct(item.get("current")))
                row.append(_fmt_pct(item.get("period_max")))
            else:
                row.append(_fmt_pct(item.get("remaining_current")))
                row.append(_fmt_pct(item.get("remaining_min")))
            if trend_enabled:
                row.append(_fmt_pct(item.get("recent_avg")))
                row.append(_fmt_pct(item.get("prev_avg")))
                row.append(item.get("trend") or "-")
            lines.append("| " + " | ".join(row) + " |")
        return "\n".join(lines) + "\n"

    def _build_doc(title, items, *, include_metric, include_mountpoint, include_bytes, mode):
        lines = [f"# {title}资源全量清单"]
        if mode == "usage":
            lines.append(
                f"> 分类规则：当前使用率 >= {_fmt_pct(usage_alert)} 为【告警】；"
                f"{_fmt_pct(usage_watch)}～{_fmt_pct(usage_alert)} 为【关注】；"
                f"< {_fmt_pct(usage_watch)} 为【安全】。"
            )
        else:
            lines.append(
                f"> 分类规则：当前剩余率 <= {_fmt_pct(remaining_alert)} 为【告警】；"
                f"{_fmt_pct(remaining_alert)}～{_fmt_pct(remaining_watch)} 为【关注】；"
                f"> {_fmt_pct(remaining_watch)} 为【安全】。"
            )
        lines.append("")

        def _zone_items(zone, classifier):
            return [item for item in items if classifier(item) == zone]

        def _section(title_label, zone_items):
            lines.append(f"## {title_label}")
            if not zone_items:
                lines.append("无\n")
            else:
                lines.append(
                    _render_zone_table(
                        zone_items,
                        include_metric=include_metric,
                        include_mountpoint=include_mountpoint,
                        include_bytes=include_bytes,
                        mode=mode,
                    )
                )

        if mode == "usage":
            classifier = lambda item: _classify_usage(item.get("current"))
        else:
            classifier = lambda item: _classify_remaining(item.get("remaining_current"))

        _section("告警区", _zone_items("告警", classifier))
        _section("关注区", _zone_items("关注", classifier))
        _section("安全区", _zone_items("安全", classifier))
        _section("未知区", _zone_items("未知", classifier))

        return "\n".join(lines) + "\n"

    metric_order = ["disk", "cpu", "mem", "pod_cpu", "pod_mem"]
    metrics = [m for m in metric_order if any(item.get("key") == m for item in entries)]
    metric_titles = {
        "disk": "磁盘",
        "cpu": "CPU",
        "mem": "Mem",
        "pod_cpu": "Pod CPU",
        "pod_mem": "Pod Mem",
    }

    # 使用率告警（当前量）
    combined_usage = header + _build_doc(
        "综合（当前量/使用率）",
        entries,
        include_metric=True,
        include_mountpoint=True,
        include_bytes=True,
        mode="usage",
    )
    _write(os.path.join(alerts_dir, "combined.md"), combined_usage)

    for metric in metrics:
        items = _split(entries, metric)
        title = metric_titles.get(metric, metric.upper())
        include_mount = metric == "disk"
        include_bytes = metric == "disk"
        content = header + _build_doc(
            f"{title}（当前量/使用率）",
            items,
            include_metric=False,
            include_mountpoint=include_mount,
            include_bytes=include_bytes,
            mode="usage",
        )
        _write(os.path.join(alerts_dir, f"{metric}.md"), content)

    # 余量（剩余资源量）
    combined_remaining = header + _build_doc(
        "综合（剩余资源量）",
        entries,
        include_metric=True,
        include_mountpoint=True,
        include_bytes=True,
        mode="remaining",
    )
    _write(os.path.join(spare_dir, "combined.md"), combined_remaining)

    for metric in metrics:
        items = _split(entries, metric)
        title = metric_titles.get(metric, metric.upper())
        include_mount = metric == "disk"
        include_bytes = metric == "disk"
        content = header + _build_doc(
            f"{title}（剩余资源量）",
            items,
            include_metric=False,
            include_mountpoint=include_mount,
            include_bytes=include_bytes,
            mode="remaining",
        )
        _write(os.path.join(spare_dir, f"{metric}.md"), content)

    # Dashboard JSON
    dashboard_dir = os.path.join(base_dir, "dashboard")
    os.makedirs(dashboard_dir, exist_ok=True)

    def _usage_zone(value):
        if value is None:
            return "unknown"
        if value >= usage_alert:
            return "alert"
        if value >= usage_watch:
            return "watch"
        return "safe"

    def _remaining_zone(value):
        if value is None:
            return "unknown"
        if value <= remaining_alert:
            return "alert"
        if value <= remaining_watch:
            return "watch"
        return "safe"

    dashboard_payload = build_dashboard_payload(resource_data, start_ts, end_ts)
    _write_json(os.path.join(dashboard_dir, "data.json"), dashboard_payload)
    # Optional convenience copy for hosting /dashboard as web root.
    project_dashboard_dir = os.path.join(str(PROJECT_ROOT), "dashboard")
    if os.path.isdir(project_dashboard_dir):
        _write_json(os.path.join(project_dashboard_dir, "data.json"), dashboard_payload)
