#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import datetime
import time
from urllib.parse import quote

import requests

from auto_inspection import config
from auto_inspection import opensearch_client
from auto_inspection import prometheus_client


DEFAULT_PROFILE_TYPE = "process_cpu:cpu:nanoseconds:cpu:nanoseconds"
FALCO_SOURCE_FIELDS = [
    "@timestamp",
    "cluster",
    "namespace",
    "pod",
    "container",
    "node",
    "message",
    "message_normalized",
    "log",
    "log_processed",
    "kubernetes.namespace_name",
    "kubernetes.pod_name",
    "kubernetes.container_name",
    "kubernetes.node_name",
    "output",
    "priority",
    "rule",
    "source",
    "tags",
    "time",
    "output_fields",
]


def _now():
    return int(time.time())


def _range(params):
    data = params if isinstance(params, dict) else {}
    end_ts = int(data.get("end_ts") or data.get("end") or _now())
    if end_ts > 10**12:
        end_ts = end_ts // 1000
    start_raw = data.get("start_ts") or data.get("start")
    if start_raw not in (None, ""):
        start_ts = int(start_raw)
        if start_ts > 10**12:
            start_ts = start_ts // 1000
    else:
        range_hours = float(data.get("range_hours") or 6)
        start_ts = end_ts - int(range_hours * 3600)
    return start_ts, end_ts


def _iso_utc(ts):
    return datetime.datetime.fromtimestamp(int(ts), datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _prom_urls():
    urls = list(getattr(config, "PROMETHEUS_URLS", []) or [])
    clusters = list(getattr(config, "PROMETHEUS_CLUSTERS", []) or [])
    return [
        {"url": url, "cluster": clusters[idx] if idx < len(clusters) else ""}
        for idx, url in enumerate(urls)
        if url
    ]


def _selector_from_matchers(matchers):
    return "{" + ",".join(matchers) + "}" if matchers else ""


def _red_selectors(data, *, errors=False):
    service = str(data.get("service") or data.get("service_name") or "").strip()
    namespace = str(data.get("namespace") or "").strip()
    route = str(data.get("route") or data.get("http_route") or "").strip()
    namespace_labels = ["namespace", "k8s_namespace_name"] if namespace else [""]
    selectors = []
    for namespace_label in namespace_labels:
        matchers = []
        if service:
            matchers.append(f'service_name=~"{service}|.*/{service}|{service}.*"')
        if namespace and namespace_label:
            matchers.append(f'{namespace_label}="{namespace}"')
        if route:
            matchers.append(f'http_route="{route}"')
        if errors:
            matchers.append('http_response_status_code=~"5..|4.."')
        selectors.append(_selector_from_matchers(matchers))
    return selectors


def _rate_any(metrics, selectors, window):
    expressions = []
    for metric in metrics:
        for selector in selectors:
            expressions.append(f"rate({metric}{selector}[{window}])")
    return " or ".join(expressions)


def service_red_metrics(params):
    data = params if isinstance(params, dict) else {}
    service = str(data.get("service") or data.get("service_name") or "").strip()
    namespace = str(data.get("namespace") or "").strip()
    route = str(data.get("route") or data.get("http_route") or "").strip()
    window = str(data.get("rate_window") or "5m").strip()
    limit = max(1, min(int(data.get("limit") or 20), 100))
    started = time.time()

    server_metrics = [
        "http_server_request_duration_seconds_count",
        "beyla_http_server_request_duration_seconds_count",
    ]
    client_metrics = [
        "http_client_request_duration_seconds_count",
        "beyla_http_client_request_duration_seconds_count",
    ]
    rpc_metrics = [
        "rpc_server_duration_seconds_count",
        "beyla_rpc_server_duration_seconds_count",
    ]
    selector = _red_selectors(data)
    error_selector = _red_selectors(data, errors=True)

    queries = {
        "server_rate": (
            f'topk({limit}, sum by (service_name, namespace, http_route, http_request_method, '
            f'k8s_namespace_name, http_response_status_code) ({_rate_any(server_metrics, selector, window)}))'
        ),
        "server_error_rate": (
            f'topk({limit}, sum by (service_name, namespace, http_route, http_response_status_code) '
            f'({_rate_any(server_metrics, error_selector, window)}))'
        ),
        "client_rate": (
            f'topk({limit}, sum by (service_name, namespace, server_address, server_port, http_request_method, '
            f'k8s_namespace_name, http_response_status_code) ({_rate_any(client_metrics, selector, window)}))'
        ),
        "rpc_rate": (
            f'topk({limit}, sum by (service_name, namespace, rpc_system, rpc_service, rpc_method, rpc_grpc_status_code) '
            f'({_rate_any(rpc_metrics, selector, window)}))'
        ),
    }

    results = []
    errors = []
    for prom in _prom_urls():
        prom_items = {"cluster": prom.get("cluster"), "url": prom.get("url"), "queries": {}}
        for name, promql in queries.items():
            try:
                values = prometheus_client.query_instant(promql, url=prom["url"], timeout=20)
                prom_items["queries"][name] = {"promql": promql, "items": values[:limit], "count": len(values)}
            except Exception as exc:
                prom_items["queries"][name] = {"promql": promql, "items": [], "count": 0, "error": str(exc)}
                errors.append({"cluster": prom.get("cluster"), "query": name, "error": str(exc)})
        results.append(prom_items)

    item_count = sum(query.get("count", 0) for prom in results for query in (prom.get("queries") or {}).values())
    return {
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3), "item_count": item_count},
        "source": "prometheus",
        "collector": "beyla/otel",
        "configured": bool(results),
        "request": {"service": service, "namespace": namespace, "route": route, "rate_window": window, "limit": limit},
        "results": results,
        "errors": errors,
        "notes": [
            "This tool expects Beyla/OTel metrics to be exported into Prometheus.",
            "If item_count is 0, verify otel-collector has a Prometheus, remote-write, or compatible exporter path for Beyla metrics.",
        ],
        "safety": {"mode": "readonly", "mutations": False},
    }


def _falco_query(params):
    data = params if isinstance(params, dict) else {}
    start_ts, end_ts = _range(data)
    filters = [
        {
            "bool": {
                "should": [
                    {"term": {"kubernetes.container_name.keyword": "falco"}},
                    {"term": {"container.keyword": "falco"}},
                    {"term": {"app.kubernetes.io/name.keyword": "falco"}},
                ],
                "minimum_should_match": 1,
            }
        },
        {"range": {"@timestamp": {"gte": _iso_utc(start_ts), "lte": _iso_utc(end_ts)}}},
    ]
    for key, fields in {
        "namespace": ["output_fields.k8s.ns.name", "kubernetes.namespace_name", "namespace"],
        "pod": ["output_fields.k8s.pod.name", "kubernetes.pod_name", "pod"],
        "container": ["output_fields.container.name", "kubernetes.container_name", "container"],
        "priority": ["priority"],
        "rule": ["rule"],
    }.items():
        value = str(data.get(key) or "").strip()
        if not value:
            continue
        filters.append(
            {
                "bool": {
                    "should": [{"term": {field: value}} for field in fields]
                    + [{"term": {f"{field}.keyword": value}} for field in fields if not field.endswith(".keyword")],
                    "minimum_should_match": 1,
                }
            }
        )
    q = str(data.get("q") or "").strip()
    must = [{"match_all": {}}]
    if q:
        must = [
            {
                "simple_query_string": {
                    "query": q,
                    "fields": ["message^3", "message_normalized^3", "log^3", "output^3", "rule^2", "priority", "tags", "output_fields.*"],
                    "default_operator": "and",
                }
            }
        ]
    return {"bool": {"must": must, "filter": filters}}


def _normalize_falco_hit(hit):
    source = (hit or {}).get("_source") or {}
    output_fields = source.get("output_fields") or {}
    kubernetes = source.get("kubernetes") or {}
    log_value = source.get("log")
    parsed_log = log_value if isinstance(log_value, dict) else {}
    return {
        "id": hit.get("_id"),
        "index": hit.get("_index"),
        "timestamp": source.get("@timestamp") or source.get("time"),
        "priority": source.get("priority") or parsed_log.get("priority"),
        "rule": source.get("rule") or parsed_log.get("rule"),
        "message": source.get("output") or source.get("message") or source.get("message_normalized") or parsed_log.get("output") or (log_value if isinstance(log_value, str) else None),
        "namespace": output_fields.get("k8s.ns.name") or source.get("namespace") or kubernetes.get("namespace_name"),
        "pod": output_fields.get("k8s.pod.name") or source.get("pod") or kubernetes.get("pod_name"),
        "container": output_fields.get("container.name") or source.get("container") or kubernetes.get("container_name"),
        "process": output_fields.get("proc.name") or output_fields.get("proc.cmdline"),
        "user": output_fields.get("user.name"),
        "tags": source.get("tags") or parsed_log.get("tags"),
        "output_fields": output_fields,
        "raw": source,
    }


def runtime_events_context(params):
    data = params if isinstance(params, dict) else {}
    size = max(1, min(int(data.get("size") or data.get("limit") or 50), 200))
    started = time.time()
    response = opensearch_client.search(
        getattr(config, "OPENSEARCH_INDEX_LOGS", "logs-k8s-*"),
        _falco_query(data),
        size=size,
        sort=[{"@timestamp": {"order": "desc"}}],
        source_includes=FALCO_SOURCE_FIELDS,
        timeout=data.get("timeout"),
    )
    total, hits = opensearch_client.response_hits(response)
    return {
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3), "item_count": len(hits), "total": total},
        "source": "opensearch",
        "collector": "falco",
        "configured": opensearch_client.is_configured(),
        "request": {k: data.get(k) for k in ("namespace", "pod", "container", "rule", "priority", "q", "range_hours", "start_ts", "end_ts") if data.get(k) not in (None, "")},
        "items": [_normalize_falco_hit(hit) for hit in hits],
        "safety": {"mode": "readonly", "mutations": False},
    }


def _pyroscope_url():
    return str(getattr(config, "PYROSCOPE_URL", "") or "http://pyroscope.observability.svc.cluster.local:4040").rstrip("/")


def _pyroscope_post(path, body, timeout=30):
    session = requests.Session()
    session.trust_env = False
    response = session.post(
        f"{_pyroscope_url()}/{path.lstrip('/')}",
        json=body,
        timeout=timeout,
        proxies={"http": None, "https": None},
        headers={"Content-Type": "application/json"},
    )
    response.raise_for_status()
    return response.json() if response.text else {}


def _label_selector(params):
    data = params if isinstance(params, dict) else {}
    explicit = data.get("label_selector") or data.get("labelSelector")
    if explicit:
        return str(explicit)
    labels = {}
    for key in ("service_name", "namespace", "pod", "container", "node"):
        value = str(data.get(key) or "").strip()
        if value:
            labels[key] = value
    service = str(data.get("service") or "").strip()
    if service and "service_name" not in labels:
        labels["service_name"] = service
    parts = []
    for key, value in labels.items():
        escaped = value.replace("\\", "\\\\").replace('"', '\\"')
        parts.append(f'{key}="{escaped}"')
    return "{" + ",".join(parts) + "}"


def _top_stacktrace_nodes(payload, limit):
    nodes = []

    def visit(node, path=None):
        if not isinstance(node, dict):
            return
        name = node.get("name") or node.get("function") or node.get("location") or node.get("label")
        value = node.get("self") or node.get("total") or node.get("value") or 0
        current = (path or []) + ([name] if name else [])
        if name:
            nodes.append({"name": name, "value": value, "path": " ; ".join(current[-8:])})
        for child in node.get("children") or node.get("nodes") or []:
            visit(child, current)

    for key in ("flamegraph", "tree", "root"):
        if isinstance(payload.get(key), dict):
            visit(payload[key])
    if isinstance(payload.get("flamebearer"), dict):
        names = payload["flamebearer"].get("names") or []
        levels = payload["flamebearer"].get("levels") or []
        for level in levels:
            if not isinstance(level, list):
                continue
            for idx in range(3, len(level), 4):
                name_index = level[idx]
                if isinstance(name_index, int) and 0 <= name_index < len(names):
                    nodes.append({"name": names[name_index], "value": level[idx - 1] if idx >= 1 else None})
    return sorted(nodes, key=lambda item: item.get("value") or 0, reverse=True)[:limit]


def profile_hotspots(params):
    data = params if isinstance(params, dict) else {}
    start_ts, end_ts = _range(data)
    start_ms = int(start_ts * 1000)
    end_ms = int(end_ts * 1000)
    limit = max(1, min(int(data.get("limit") or 20), 100))
    max_nodes = max(1, min(int(data.get("max_nodes") or 256), 2048))
    profile_type = str(data.get("profile_type") or data.get("profileTypeID") or DEFAULT_PROFILE_TYPE)
    selector = _label_selector(data)
    started = time.time()

    profile_types = {}
    label_names = {}
    label_values = {}
    stacktraces = {}
    errors = []
    base_body = {"start": start_ms, "end": end_ms}
    for step, path, body, timeout in (
        ("profile_types", "/querier.v1.QuerierService/ProfileTypes", base_body, 20),
        ("label_names", "/querier.v1.QuerierService/LabelNames", base_body, 20),
        ("label_values", "/querier.v1.QuerierService/LabelValues", {**base_body, "name": "service_name"}, 20),
        ("stacktraces", "/querier.v1.QuerierService/SelectMergeStacktraces", {**base_body, "labelSelector": selector, "profileTypeID": profile_type, "maxNodes": max_nodes}, 60),
    ):
        try:
            payload = _pyroscope_post(path, body, timeout=timeout)
            if step == "profile_types":
                profile_types = payload
            elif step == "label_names":
                label_names = payload
            elif step == "label_values":
                label_values = payload
            else:
                stacktraces = payload
        except Exception as exc:
            errors.append({"step": step, "error": str(exc)})

    hotspots = _top_stacktrace_nodes(stacktraces, limit)
    return {
        "meta": {"status": "ok" if stacktraces or not errors else "partial", "query_seconds": round(time.time() - started, 3), "hotspot_count": len(hotspots)},
        "source": "pyroscope",
        "collector": "alloy-pyroscope-ebpf",
        "configured": bool(_pyroscope_url()),
        "request": {"pyroscope_url": _pyroscope_url(), "label_selector": selector, "profile_type": profile_type, "start_ts": start_ts, "end_ts": end_ts, "limit": limit, "max_nodes": max_nodes},
        "profile_types": (profile_types.get("profileTypes") or [])[:50],
        "label_names": (label_names.get("names") or [])[:100],
        "service_names": (label_values.get("names") or [])[:100],
        "hotspots": hotspots,
        "raw": {"stacktrace_keys": sorted(stacktraces.keys()) if isinstance(stacktraces, dict) else []},
        "links": {"pyroscope": f"{_pyroscope_url()}/?query={quote(profile_type + selector)}"},
        "errors": errors,
        "safety": {"mode": "readonly", "mutations": False},
    }
