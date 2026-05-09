#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import argparse
import json

import requests

from auto_inspection import config


_SESSION = requests.Session()
_SESSION.trust_env = False

NUMERIC_TYPES = {
    "byte",
    "double",
    "float",
    "half_float",
    "integer",
    "long",
    "scaled_float",
    "short",
    "unsigned_long",
}
STRING_TYPES = {
    "keyword",
    "constant_keyword",
    "text",
    "wildcard",
    "match_only_text",
}


def _vega_terms_spec(index, field, title, *, size=10, time_field="@timestamp", legend_title=None, value_title="Count"):
    return {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": title,
        "data": {
            "url": {
                "%context%": True,
                "%timefield%": time_field,
                "index": index,
                "body": {
                    "size": 0,
                    "aggs": {
                        "items": {
                            "terms": {
                                "field": field,
                                "size": size,
                                "order": {"_count": "desc"},
                            }
                        }
                    },
                },
            },
            "format": {"property": "aggregations.items.buckets"},
        },
        "mark": {"type": "bar", "cornerRadiusEnd": 3},
        "encoding": {
            "x": {"field": "key", "type": "nominal", "sort": "-y", "title": legend_title or field},
            "y": {"field": "doc_count", "type": "quantitative", "title": value_title},
            "tooltip": [
                {"field": "key", "type": "nominal", "title": legend_title or field},
                {"field": "doc_count", "type": "quantitative", "title": value_title},
            ],
            "color": {
                "field": "key",
                "type": "nominal",
                "legend": None,
                "scale": {"scheme": "tableau10"},
            },
        },
        "config": {
            "background": "transparent",
            "axis": {"labelColor": "#2f2f36", "titleColor": "#2f2f36"},
            "title": {"color": "#1d1d1f", "fontSize": 15, "anchor": "start"},
        },
    }


def _vega_date_histogram_spec(index, title, *, interval="30m", time_field="@timestamp", query=None, value_title="Count"):
    body = {
        "size": 0,
        "aggs": {
            "timeline": {
                "date_histogram": {
                    "field": time_field,
                    "fixed_interval": interval,
                    "min_doc_count": 0,
                }
            }
        },
    }
    if query:
        body["query"] = query
    return {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": title,
        "data": {
            "url": {
                "%context%": True,
                "%timefield%": time_field,
                "index": index,
                "body": body,
            },
            "format": {"property": "aggregations.timeline.buckets"},
        },
        "mark": {"type": "line", "point": True, "strokeWidth": 2},
        "encoding": {
            "x": {"field": "key_as_string", "type": "temporal", "title": "Time"},
            "y": {"field": "doc_count", "type": "quantitative", "title": value_title},
            "tooltip": [
                {"field": "key_as_string", "type": "temporal", "title": "Time"},
                {"field": "doc_count", "type": "quantitative", "title": value_title},
            ],
            "color": {"value": "#2563eb"},
        },
        "config": {
            "background": "transparent",
            "axis": {"labelColor": "#2f2f36", "titleColor": "#2f2f36"},
            "title": {"color": "#1d1d1f", "fontSize": 15, "anchor": "start"},
        },
    }


def _vega_filters_date_histogram_spec(index, title, filters, *, interval="1h", time_field="@timestamp", value_title="Count"):
    body = {
        "size": 0,
        "aggs": {
            "timeline": {
                "date_histogram": {
                    "field": time_field,
                    "fixed_interval": interval,
                    "min_doc_count": 0,
                },
                "aggs": {
                    "categories": {
                        "filters": {
                            "filters": {
                                item["id"]: item["query"]
                                for item in filters
                            }
                        }
                    }
                },
            }
        },
    }
    return {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": title,
        "data": {
            "url": {
                "%context%": True,
                "%timefield%": time_field,
                "index": index,
                "body": body,
            },
            "format": {"property": "aggregations.timeline.buckets"},
        },
        "transform": [
            *[
                {"calculate": f"datum.categories.buckets['{item['id']}'].doc_count", "as": item["id"]}
                for item in filters
            ],
            {
                "fold": [item["id"] for item in filters],
                "as": ["series", "doc_count"],
            },
        ],
        "mark": {"type": "line", "point": True, "strokeWidth": 2},
        "encoding": {
            "x": {"field": "key_as_string", "type": "temporal", "title": "Time"},
            "y": {"field": "doc_count", "type": "quantitative", "title": value_title},
            "color": {
                "field": "series",
                "type": "nominal",
                "scale": {"scheme": "tableau10"},
            },
            "tooltip": [
                {"field": "key_as_string", "type": "temporal", "title": "Time"},
                {"field": "series", "type": "nominal", "title": "Series"},
                {"field": "doc_count", "type": "quantitative", "title": value_title},
            ],
        },
        "config": {
            "background": "transparent",
            "axis": {"labelColor": "#2f2f36", "titleColor": "#2f2f36"},
            "title": {"color": "#1d1d1f", "fontSize": 15, "anchor": "start"},
        },
    }


def _vega_heatmap_spec(index, title, *, interval="1d", time_field="@timestamp", category_field="namespace.keyword", category_title="Namespace"):
    body = {
        "size": 0,
        "aggs": {
            "timeline": {
                "date_histogram": {
                    "field": time_field,
                    "fixed_interval": interval,
                    "min_doc_count": 0,
                },
                "aggs": {
                    "categories": {
                        "terms": {
                            "field": category_field,
                            "size": 20,
                            "order": {"_count": "desc"},
                        }
                    }
                },
            }
        },
    }
    return {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": title,
        "data": {
            "url": {
                "%context%": True,
                "%timefield%": time_field,
                "index": index,
                "body": body,
            },
            "format": {"property": "aggregations.timeline.buckets"},
        },
        "transform": [
            {"flatten": ["categories.buckets"], "as": ["category_bucket"]},
            {"calculate": "datum.key_as_string", "as": "bucket_time"},
            {"calculate": "datum.category_bucket.key", "as": "category"},
            {"calculate": "datum.category_bucket.doc_count", "as": "doc_count"},
        ],
        "mark": {"type": "rect"},
        "encoding": {
            "x": {"field": "bucket_time", "type": "temporal", "title": "Time"},
            "y": {"field": "category", "type": "nominal", "title": category_title},
            "color": {
                "field": "doc_count",
                "type": "quantitative",
                "scale": {"scheme": "oranges"},
                "title": "Count",
            },
            "tooltip": [
                {"field": "bucket_time", "type": "temporal", "title": "Time"},
                {"field": "category", "type": "nominal", "title": category_title},
                {"field": "doc_count", "type": "quantitative", "title": "Count"},
            ],
        },
        "config": {
            "background": "transparent",
            "axis": {"labelColor": "#2f2f36", "titleColor": "#2f2f36"},
            "title": {"color": "#1d1d1f", "fontSize": 15, "anchor": "start"},
        },
    }

DATA_VIEWS = [
    {
        "id": "logs-k8s-data-view",
        "title": "logs-k8s-*",
        "time_field": "@timestamp",
        "description": "Container logs collected by Fluent Bit",
    },
    {
        "id": "events-k8s-data-view",
        "title": "events-k8s-*",
        "time_field": "@timestamp",
        "description": "Kubernetes events collected by Fluent Bit",
    },
    {
        "id": "inspection-investigations-data-view",
        "title": "inspection-investigations-*",
        "time_field": "generated_at",
        "description": "AI investigation results generated by auto_inspection",
    },
    {
        "id": "inspection-incidents-data-view",
        "title": "inspection-incidents-*",
        "time_field": "generated_at",
        "description": "Current incidents generated by auto_inspection",
    },
]

SAVED_SEARCHES = [
    {
        "id": "search-k8s-logs-recent-errors",
        "title": "K8s Logs - Recent Errors",
        "description": "Recent stderr or error-level container logs",
        "index_id": "logs-k8s-data-view",
        "columns": [
            "namespace",
            "pod",
            "container",
            "service",
            "stream",
            "severity",
            "logger",
            "exception_type",
            "message",
        ],
        "sort": [["@timestamp", "desc"]],
        "query": 'severity:("error" or "warn" or "fatal") or stream:"stderr" or exception_type:* or message:*Exception* or message:*panic* or message:*Traceback*',
    },
    {
        "id": "search-k8s-events-warnings",
        "title": "K8s Events - Recent Warnings",
        "description": "Recent warning events from Kubernetes",
        "index_id": "events-k8s-data-view",
        "columns": [
            "metadata.namespace",
            "involvedObject.kind",
            "involvedObject.name",
            "reason",
            "message",
            "source.component",
        ],
        "sort": [["@timestamp", "desc"]],
        "query": 'type:"Warning"',
    },
    {
        "id": "search-investigations-recent",
        "title": "Investigations - Recent Results",
        "description": "Latest investigation outputs stored in OpenSearch",
        "index_id": "inspection-investigations-data-view",
        "columns": [
            "generated_at",
            "investigation_id",
            "request.namespace",
            "request.pod",
            "analysis.summary",
        ],
        "sort": [["generated_at", "desc"]],
        "query": "*",
    },
    {
        "id": "search-incidents-current",
        "title": "Incidents - Current",
        "description": "Latest incident records stored in OpenSearch",
        "index_id": "inspection-incidents-data-view",
        "columns": [
            "generated_at",
            "namespace",
            "pod",
            "final_risk_level",
            "dominant_risk",
            "signals",
        ],
        "sort": [["generated_at", "desc"]],
        "query": "*",
    },
    {
        "id": "search-langfuse-clickhouse-logs",
        "title": "Langfuse ClickHouse - CrashLoop Logs",
        "description": "Focused logs for langfuse clickhouse shard crash investigation",
        "index_id": "logs-k8s-data-view",
        "columns": [
            "namespace",
            "pod",
            "stream",
            "severity",
            "exception_type",
            "message",
        ],
        "sort": [["@timestamp", "desc"]],
        "query": 'namespace:"langfuse" and pod:"langfuse-clickhouse-shard0-0"',
    },
    {
        "id": "search-langfuse-clickhouse-events",
        "title": "Langfuse ClickHouse - Events",
        "description": "Focused events for langfuse clickhouse shard crash investigation",
        "index_id": "events-k8s-data-view",
        "columns": [
            "metadata.namespace",
            "involvedObject.name",
            "reason",
            "type",
            "message",
        ],
        "sort": [["@timestamp", "desc"]],
        "query": 'metadata.namespace:"langfuse" and involvedObject.name:"langfuse-clickhouse-shard0-0"',
    },
]

VISUALIZATIONS = [
    {
        "id": "viz-noisy-pods",
        "title": "Top Noisy Pods",
        "description": "Pods with the highest recent log volume",
        "spec": _vega_terms_spec(
            "logs-k8s-*",
            "pod.keyword",
            "Top Noisy Pods",
            size=12,
            legend_title="Pod",
            value_title="Log Count",
        ),
    },
    {
        "id": "viz-logs-by-namespace",
        "title": "Logs by Namespace",
        "description": "Top namespaces by log volume",
        "spec": _vega_terms_spec(
            "logs-k8s-*",
            "kubernetes.namespace_name.keyword",
            "Logs by Namespace",
            size=12,
            legend_title="Namespace",
            value_title="Log Count",
        ),
    },
    {
        "id": "viz-events-by-reason",
        "title": "Events by Reason",
        "description": "Top Kubernetes event reasons",
        "spec": _vega_terms_spec(
            "events-k8s-*",
            "reason.keyword",
            "Events by Reason",
            size=12,
            legend_title="Reason",
            value_title="Event Count",
        ),
    },
    {
        "id": "viz-incidents-by-namespace",
        "title": "Incidents by Namespace",
        "description": "Namespaces with current incident pressure",
        "spec": _vega_terms_spec(
            "inspection-incidents-*",
            "namespace.keyword",
            "Incidents by Namespace",
            size=12,
            time_field="generated_at",
            legend_title="Namespace",
            value_title="Incident Count",
        ),
    },
    {
        "id": "viz-investigations-by-namespace",
        "title": "Investigations by Namespace",
        "description": "Namespaces with recent investigation activity",
        "spec": _vega_terms_spec(
            "inspection-investigations-*",
            "request.namespace.keyword",
            "Investigations by Namespace",
            size=12,
            time_field="generated_at",
            legend_title="Namespace",
            value_title="Investigation Count",
        ),
    },
    {
        "id": "viz-investigation-count-trend",
        "title": "Investigation Count Trend",
        "description": "Investigation volume over time",
        "spec": _vega_date_histogram_spec(
            "inspection-investigations-*",
            "Investigation Count Trend",
            interval="6h",
            time_field="generated_at",
            value_title="Investigation Count",
        ),
    },
    {
        "id": "viz-incidents-oom-crashloop-trend",
        "title": "OOM / CrashLoop Trend",
        "description": "Trend of OOM and CrashLoop-like incidents",
        "spec": _vega_filters_date_histogram_spec(
            "inspection-incidents-*",
            "OOM / CrashLoop Trend",
            [
                {
                    "id": "oom",
                    "query": {
                        "bool": {
                            "should": [
                                {"term": {"signals": "oom"}},
                                {"term": {"dominant_risk": "mem"}},
                            ],
                            "minimum_should_match": 1,
                        }
                    },
                },
                {
                    "id": "crashloop",
                    "query": {
                        "bool": {
                            "should": [
                                {"term": {"signals": "waiting"}},
                                {"term": {"pod_state.waiting_reason.keyword": "CrashLoopBackOff"}},
                            ],
                            "minimum_should_match": 1,
                        }
                    },
                },
            ],
            interval="6h",
            time_field="generated_at",
            value_title="Incident Count",
        ),
    },
    {
        "id": "viz-incidents-namespace-heatmap",
        "title": "Namespace Incident Heatmap",
        "description": "Incident density by namespace over time",
        "spec": _vega_heatmap_spec(
            "inspection-incidents-*",
            "Namespace Incident Heatmap",
            interval="1d",
            time_field="generated_at",
            category_field="namespace.keyword",
            category_title="Namespace",
        ),
    },
    {
        "id": "viz-langfuse-logs-over-time",
        "title": "Langfuse ClickHouse Logs Over Time",
        "description": "Timeline of langfuse clickhouse logs",
        "spec": _vega_date_histogram_spec(
            "logs-k8s-*",
            "Langfuse ClickHouse Logs Over Time",
            interval="30m",
            query={
                "bool": {
                    "filter": [
                        {"term": {"kubernetes.namespace_name.keyword": "langfuse"}},
                        {"term": {"kubernetes.pod_name.keyword": "langfuse-clickhouse-shard0-0"}},
                    ]
                }
            },
            value_title="Log Count",
        ),
    },
    {
        "id": "viz-langfuse-events-by-reason",
        "title": "Langfuse ClickHouse Events by Reason",
        "description": "Top event reasons for langfuse clickhouse",
        "spec": _vega_terms_spec(
            "events-k8s-*",
            "reason.keyword",
            "Langfuse ClickHouse Events by Reason",
            size=8,
            legend_title="Reason",
            value_title="Event Count",
            time_field="@timestamp",
        ),
        "query_override": {
            "bool": {
                "filter": [
                    {"term": {"metadata.namespace.keyword": "langfuse"}},
                    {"term": {"involvedObject.name.keyword": "langfuse-clickhouse-shard0-0"}},
                ]
            }
        },
    },
]

DASHBOARDS = [
    {
        "id": "dashboard-auto-inspection-overview",
        "title": "Auto Inspection - Operations Overview",
        "description": "Overview dashboard for logs, events, and investigation results",
        "time_from": "now-24h",
        "time_to": "now",
        "panels": [
            {"panel_ref": "panel_0", "object_type": "visualization", "object_id": "viz-noisy-pods", "x": 0, "y": 0, "w": 12, "h": 12},
            {"panel_ref": "panel_1", "object_type": "visualization", "object_id": "viz-logs-by-namespace", "x": 12, "y": 0, "w": 12, "h": 12},
            {"panel_ref": "panel_2", "object_type": "visualization", "object_id": "viz-events-by-reason", "x": 24, "y": 0, "w": 12, "h": 12},
            {"panel_ref": "panel_3", "object_type": "visualization", "object_id": "viz-incidents-by-namespace", "x": 36, "y": 0, "w": 12, "h": 12},
            {"panel_ref": "panel_4", "object_type": "visualization", "object_id": "viz-incidents-oom-crashloop-trend", "x": 0, "y": 12, "w": 24, "h": 13},
            {"panel_ref": "panel_5", "object_type": "visualization", "object_id": "viz-incidents-namespace-heatmap", "x": 24, "y": 12, "w": 24, "h": 13},
            {"panel_ref": "panel_6", "object_type": "visualization", "object_id": "viz-investigation-count-trend", "x": 0, "y": 25, "w": 24, "h": 12},
            {"panel_ref": "panel_7", "object_type": "visualization", "object_id": "viz-investigations-by-namespace", "x": 24, "y": 25, "w": 24, "h": 12},
            {"panel_ref": "panel_8", "object_type": "search", "object_id": "search-incidents-current", "x": 0, "y": 37, "w": 16, "h": 14},
            {"panel_ref": "panel_9", "object_type": "search", "object_id": "search-k8s-logs-recent-errors", "x": 16, "y": 37, "w": 16, "h": 14},
            {"panel_ref": "panel_10", "object_type": "search", "object_id": "search-k8s-events-warnings", "x": 32, "y": 37, "w": 16, "h": 14},
            {"panel_ref": "panel_11", "object_type": "search", "object_id": "search-investigations-recent", "x": 0, "y": 51, "w": 48, "h": 14},
        ],
    },
    {
        "id": "dashboard-langfuse-clickhouse-rca",
        "title": "Auto Inspection - Langfuse ClickHouse RCA",
        "description": "Focused dashboard for the langfuse clickhouse crashloop case",
        "time_from": "now-7d",
        "time_to": "now",
        "panels": [
            {"panel_ref": "panel_0", "object_type": "visualization", "object_id": "viz-langfuse-logs-over-time", "x": 0, "y": 0, "w": 30, "h": 14},
            {"panel_ref": "panel_1", "object_type": "visualization", "object_id": "viz-langfuse-events-by-reason", "x": 30, "y": 0, "w": 18, "h": 14},
            {"panel_ref": "panel_2", "object_type": "search", "object_id": "search-langfuse-clickhouse-logs", "x": 0, "y": 14, "w": 30, "h": 18},
            {"panel_ref": "panel_3", "object_type": "search", "object_id": "search-langfuse-clickhouse-events", "x": 30, "y": 14, "w": 18, "h": 18},
            {"panel_ref": "panel_4", "object_type": "search", "object_id": "search-investigations-recent", "x": 0, "y": 32, "w": 48, "h": 12},
        ],
    },
]


def _dashboards_base_url():
    url = (getattr(config, "OPENSEARCH_DASHBOARDS_URL", "") or "").strip()
    if not url:
        raise RuntimeError("OPENSEARCH_DASHBOARDS_URL is not configured.")
    return url.rstrip("/")


def _opensearch_base_url():
    url = (getattr(config, "OPENSEARCH_URL", "") or "").strip()
    if not url:
        raise RuntimeError("OPENSEARCH_URL is not configured.")
    return url.rstrip("/")


def _dashboards_request(method, path, *, payload=None):
    url = f"{_dashboards_base_url()}/{path.lstrip('/')}"
    response = _SESSION.request(
        method,
        url,
        json=payload,
        timeout=30,
        proxies={"http": None, "https": None},
        headers={"osd-xsrf": "true", "Content-Type": "application/json"},
    )
    response.raise_for_status()
    return response.json()


def _opensearch_request(method, path, *, payload=None):
    url = f"{_opensearch_base_url()}/{path.lstrip('/')}"
    response = _SESSION.request(
        method,
        url,
        json=payload,
        timeout=30,
        proxies={"http": None, "https": None},
        headers={"Content-Type": "application/json"},
        verify=bool(getattr(config, "OPENSEARCH_VERIFY_SSL", True)),
        auth=((getattr(config, "OPENSEARCH_USERNAME", "") or "") or None) and (
            getattr(config, "OPENSEARCH_USERNAME", ""),
            getattr(config, "OPENSEARCH_PASSWORD", ""),
        ),
    )
    response.raise_for_status()
    return response.json()


def _map_osd_type(es_type):
    if es_type in STRING_TYPES:
        return "string"
    if es_type in NUMERIC_TYPES:
        return "number"
    if es_type == "date":
        return "date"
    if es_type == "boolean":
        return "boolean"
    if es_type == "ip":
        return "ip"
    return None


def _field_caps_fields(index_pattern):
    data = _opensearch_request("GET", f"{index_pattern}/_field_caps?fields=*")
    fields = []
    seen = set()

    meta_fields = [
        {"name": "@timestamp", "type": "date", "esTypes": ["date"], "searchable": True, "aggregatable": True, "readFromDocValues": True},
        {"name": "_id", "type": "string", "esTypes": ["_id"], "searchable": True, "aggregatable": True, "readFromDocValues": False},
        {"name": "_index", "type": "string", "esTypes": ["_index"], "searchable": True, "aggregatable": True, "readFromDocValues": False},
        {"name": "_score", "type": "number", "esTypes": [], "searchable": False, "aggregatable": False, "readFromDocValues": False},
        {"name": "_source", "type": "_source", "esTypes": ["_source"], "searchable": False, "aggregatable": False, "readFromDocValues": False},
    ]
    for item in meta_fields:
        fields.append({"count": 0, "scripted": False, **item})
        seen.add(item["name"])

    for field_name in sorted((data.get("fields") or {}).keys()):
        if field_name in seen:
            continue
        type_map = (data["fields"].get(field_name) or {})
        es_types = sorted(type_map.keys())
        if not es_types:
            continue
        primary = es_types[0]
        osd_type = _map_osd_type(primary)
        if not osd_type:
            continue
        searchable = any(bool(item.get("searchable")) for item in type_map.values())
        aggregatable = any(bool(item.get("aggregatable")) for item in type_map.values())
        fields.append(
            {
                "count": 0,
                "name": field_name,
                "type": osd_type,
                "esTypes": es_types,
                "scripted": False,
                "searchable": searchable,
                "aggregatable": aggregatable,
                "readFromDocValues": aggregatable and osd_type in {"date", "number", "boolean", "ip", "string"},
            }
        )
    return fields


def _ensure_data_view(item):
    fields = _field_caps_fields(item["title"])
    payload = {
        "attributes": {
            "title": item["title"],
            "timeFieldName": item["time_field"],
            "fields": json.dumps(fields, separators=(",", ":")),
            "fieldAttrs": "{}",
            "sourceFilters": "[]",
            "typeMeta": "{}",
        }
    }
    return _dashboards_request(
        "POST",
        f"api/saved_objects/index-pattern/{item['id']}?overwrite=true",
        payload=payload,
    )


def _ensure_saved_search(item):
    payload = {
        "attributes": {
            "title": item["title"],
            "description": item["description"],
            "hits": 0,
            "version": 1,
            "columns": item["columns"],
            "sort": item["sort"],
            "kibanaSavedObjectMeta": {
                "searchSourceJSON": json.dumps(
                    {
                        "query": {
                            "query": item["query"],
                            "language": "kuery",
                        },
                        "filter": [],
                        "indexRefName": "kibanaSavedObjectMeta.searchSourceJSON.index",
                    }
                )
            },
        },
        "references": [
            {
                "id": item["index_id"],
                "name": "kibanaSavedObjectMeta.searchSourceJSON.index",
                "type": "index-pattern",
            }
        ],
    }
    return _dashboards_request(
        "POST",
        f"api/saved_objects/search/{item['id']}?overwrite=true",
        payload=payload,
    )


def _ensure_visualization(item):
    spec = item["spec"]
    if item.get("query_override"):
        spec = json.loads(json.dumps(spec))
        spec["data"]["url"]["body"]["query"] = item["query_override"]
    payload = {
        "attributes": {
            "title": item["title"],
            "description": item["description"],
            "visState": json.dumps(
                {
                    "title": item["title"],
                    "type": "vega",
                    "params": {"spec": json.dumps(spec)},
                    "aggs": [],
                }
            ),
            "uiStateJSON": "{}",
            "version": 1,
            "kibanaSavedObjectMeta": {
                "searchSourceJSON": json.dumps(
                    {
                        "query": {"query": "", "language": "kuery"},
                        "filter": [],
                    }
                )
            },
        }
    }
    return _dashboards_request(
        "POST",
        f"api/saved_objects/visualization/{item['id']}?overwrite=true",
        payload=payload,
    )


def _ensure_dashboard(item):
    panels = []
    references = []
    for idx, panel in enumerate(item["panels"]):
        panel_index = f"panel-{idx}"
        panels.append(
            {
                "version": "2.19.5",
                "gridData": {
                    "x": panel["x"],
                    "y": panel["y"],
                    "w": panel["w"],
                    "h": panel["h"],
                    "i": panel_index,
                },
                "panelIndex": panel_index,
                "embeddableConfig": {},
                "panelRefName": panel["panel_ref"],
            }
        )
        references.append(
            {
                "id": panel["object_id"],
                "name": panel["panel_ref"],
                "type": panel.get("object_type", "search"),
            }
        )

    payload = {
        "attributes": {
            "title": item["title"],
            "description": item["description"],
            "hits": 0,
            "version": 1,
            "timeRestore": True,
            "timeFrom": item["time_from"],
            "timeTo": item["time_to"],
            "refreshInterval": {"pause": True, "value": 0},
            "panelsJSON": json.dumps(panels),
            "optionsJSON": json.dumps({"hidePanelTitles": False, "useMargins": True}),
            "kibanaSavedObjectMeta": {
                "searchSourceJSON": json.dumps(
                    {
                        "query": {"query": "*", "language": "kuery"},
                        "filter": [],
                    }
                )
            },
        },
        "references": references,
    }
    return _dashboards_request(
        "POST",
        f"api/saved_objects/dashboard/{item['id']}?overwrite=true",
        payload=payload,
    )


def _ensure_default_index(index_id):
    status = _dashboards_request("GET", "api/status")
    version = ((status.get("version") or {}).get("number")) or "2.19.5"
    build_num = ((status.get("version") or {}).get("build_number")) or 0
    payload = {
        "attributes": {
            "buildNum": build_num,
            "defaultIndex": index_id,
        }
    }
    return _dashboards_request(
        "POST",
        f"api/saved_objects/config/{version}?overwrite=true",
        payload=payload,
    )


def _delete_temporary_objects():
    temp_objects = [
        ("search", "tmp-search-1a8c6dcb"),
        ("dashboard", "tmp-dashboard-c8f77e0f"),
        ("visualization", "tmp-vega-18072db8"),
    ]
    for object_type, object_id in temp_objects:
        try:
            _dashboards_request("DELETE", f"api/saved_objects/{object_type}/{object_id}")
        except Exception:
            pass


def bootstrap():
    created = {"data_views": [], "searches": [], "visualizations": [], "dashboards": [], "default_index": None}
    for item in DATA_VIEWS:
        _ensure_data_view(item)
        created["data_views"].append(item["id"])
    for item in SAVED_SEARCHES:
        _ensure_saved_search(item)
        created["searches"].append(item["id"])
    for item in VISUALIZATIONS:
        _ensure_visualization(item)
        created["visualizations"].append(item["id"])
    for item in DASHBOARDS:
        _ensure_dashboard(item)
        created["dashboards"].append(item["id"])
    _ensure_default_index("logs-k8s-data-view")
    created["default_index"] = "logs-k8s-data-view"
    _delete_temporary_objects()
    return created


def main(argv=None):
    parser = argparse.ArgumentParser(description="Bootstrap OpenSearch Dashboards data views and saved searches.")
    parser.parse_args(argv)
    result = bootstrap()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
