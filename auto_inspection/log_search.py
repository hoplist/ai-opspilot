#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import datetime

from auto_inspection import config
from auto_inspection import opensearch_client


DEFAULT_LOG_SOURCE_FIELDS = [
    "@timestamp",
    "cluster",
    "namespace",
    "workload_kind",
    "workload_name",
    "pod",
    "container",
    "node",
    "service",
    "biz_line",
    "business_key",
    "frontend_service",
    "backend_service",
    "domain",
    "host",
    "route",
    "version",
    "trace_id",
    "span_id",
    "parent_span_id",
    "request_id",
    "event_id",
    "tenant_id",
    "user_id",
    "order_id",
    "error_code",
    "severity",
    "stack_language",
    "message_normalized",
    "exception_type",
    "exception_message",
    "message",
    "log",
    "stream",
    "time",
    "log_processed",
    "exception.type",
    "exception.message",
    "kubernetes.namespace_name",
    "kubernetes.pod_name",
    "kubernetes.container_name",
    "kubernetes.node_name",
]


def _iso_utc(timestamp):
    if timestamp is None:
        return None
    return datetime.datetime.fromtimestamp(
        int(timestamp),
        datetime.timezone.utc,
    ).strftime("%Y-%m-%dT%H:%M:%SZ")


def _term_filters(params):
    mapping = {
        "cluster": ["cluster"],
        "namespace": ["namespace", "kubernetes.namespace_name"],
        "workload_name": ["workload_name"],
        "pod": ["pod", "kubernetes.pod_name"],
        "container": ["container", "kubernetes.container_name"],
        "node": ["node", "kubernetes.node_name"],
        "service": ["service"],
        "biz_line": ["biz_line"],
        "business_key": ["business_key"],
        "frontend_service": ["frontend_service", "service"],
        "backend_service": ["backend_service", "service"],
        "domain": ["domain", "host", "http.host", "url.domain"],
        "route": ["route", "http.route", "url.path"],
        "version": ["version", "app.version", "service.version"],
        "trace_id": ["trace_id", "trace.id", "traceId"],
        "span_id": ["span_id", "span.id", "spanId"],
        "request_id": ["request_id", "request.id", "requestId", "x_request_id"],
        "event_id": ["event_id", "event.id", "eventId"],
        "tenant_id": ["tenant_id", "tenant.id", "tenantId"],
        "user_id": ["user_id", "user.id", "userId"],
        "order_id": ["order_id", "order.id", "orderId"],
        "error_code": ["error_code", "error.code", "err_code", "code"],
        "severity": ["severity"],
    }

    filters = []
    for key, fields in mapping.items():
        value = str(params.get(key, "") or "").strip()
        if not value:
            continue
        should = []
        for field in fields:
            should.append({"term": {field: value}})
            if not field.endswith(".keyword"):
                should.append({"term": {f"{field}.keyword": value}})
        filters.append(
            {
                "bool": {
                    "should": should,
                    "minimum_should_match": 1,
                }
            }
        )
    return filters


def build_log_query(params):
    data = params if isinstance(params, dict) else {}
    must = []
    filters = _term_filters(data)

    query_text = str(data.get("q", "") or "").strip()
    if query_text:
        must.append(
            {
                "simple_query_string": {
                    "query": query_text,
                    "fields": [
                        "message^4",
                        "message_normalized^4",
                        "log^4",
                        "log_processed.msg^4",
                        "exception.message^3",
                        "exception_message^3",
                        "exception.type^2",
                        "exception_type^2",
                        "stack_language^2",
                        "kubernetes.container_name^2",
                        "kubernetes.pod_name^2",
                        "logger^2",
                        "service^2",
                        "biz_line^2",
                        "business_key^2",
                        "trace_id^3",
                        "span_id^2",
                        "request_id^3",
                        "event_id^3",
                        "tenant_id^2",
                        "user_id^2",
                        "order_id^2",
                        "error_code^3",
                        "route^2",
                        "domain^2",
                        "host^2",
                        "version^2",
                        "pod^2",
                        "workload_name^2",
                        "raw",
                    ],
                    "default_operator": "and",
                }
            }
        )

    start_ts = data.get("start_ts")
    end_ts = data.get("end_ts")
    if start_ts is not None or end_ts is not None:
        range_body = {}
        if start_ts is not None:
            range_body["gte"] = _iso_utc(start_ts)
        if end_ts is not None:
            range_body["lte"] = _iso_utc(end_ts)
        filters.append({"range": {"@timestamp": range_body}})

    query = {"bool": {}}
    query["bool"]["must"] = must or [{"match_all": {}}]
    if filters:
        query["bool"]["filter"] = filters
    return query


def normalize_log_hit(hit):
    source = (hit or {}).get("_source") or {}
    kubernetes = source.get("kubernetes") or {}
    log_value = source.get("log")
    log_info = log_value if isinstance(log_value, dict) else {}
    processed = source.get("log_processed") or {}
    exception = source.get("exception") or {}
    logger = source.get("logger") or processed.get("logger") or processed.get("caller") or log_info.get("logger")
    severity = source.get("severity") or processed.get("level") or log_info.get("level")
    message = (
        source.get("message")
        or source.get("message_normalized")
        or processed.get("msg")
        or (log_value if isinstance(log_value, str) else None)
    )
    return {
        "id": hit.get("_id"),
        "index": hit.get("_index"),
        "score": hit.get("_score"),
        "timestamp": source.get("@timestamp"),
        "cluster": source.get("cluster"),
        "namespace": source.get("namespace") or kubernetes.get("namespace_name"),
        "workload_kind": source.get("workload_kind"),
        "workload_name": source.get("workload_name"),
        "pod": source.get("pod") or kubernetes.get("pod_name"),
        "container": source.get("container") or kubernetes.get("container_name"),
        "node": source.get("node") or kubernetes.get("node_name"),
        "service": source.get("service"),
        "biz_line": source.get("biz_line"),
        "business_key": source.get("business_key"),
        "frontend_service": source.get("frontend_service"),
        "backend_service": source.get("backend_service"),
        "domain": source.get("domain") or source.get("host"),
        "route": source.get("route"),
        "version": source.get("version"),
        "trace_id": source.get("trace_id") or source.get("traceId"),
        "span_id": source.get("span_id") or source.get("spanId"),
        "parent_span_id": source.get("parent_span_id") or source.get("parentSpanId"),
        "request_id": source.get("request_id") or source.get("requestId"),
        "event_id": source.get("event_id") or source.get("eventId"),
        "tenant_id": source.get("tenant_id") or source.get("tenantId"),
        "user_id": source.get("user_id") or source.get("userId"),
        "order_id": source.get("order_id") or source.get("orderId"),
        "error_code": source.get("error_code") or source.get("errorCode"),
        "severity": severity,
        "logger": logger,
        "message": message,
        "message_normalized": source.get("message_normalized") or message,
        "stream": source.get("stream"),
        "original_log": log_value,
        "stack_language": source.get("stack_language") or exception.get("language"),
        "exception_type": source.get("exception_type") or exception.get("type"),
        "exception_message": source.get("exception_message") or exception.get("message"),
        "raw": source,
    }


def search_logs(params):
    data = params if isinstance(params, dict) else {}
    size = max(1, min(int(data.get("size") or 50), 500))
    from_ = max(0, int(data.get("from") or 0))
    response = opensearch_client.search(
        getattr(config, "OPENSEARCH_INDEX_LOGS", "logs-k8s-*"),
        build_log_query(data),
        size=size,
        from_=from_,
        sort=[{"@timestamp": {"order": "desc"}}],
        source_includes=DEFAULT_LOG_SOURCE_FIELDS,
        timeout=data.get("timeout"),
    )
    total, hits = opensearch_client.response_hits(response)
    return {
        "query": {
            "index": getattr(config, "OPENSEARCH_INDEX_LOGS", "logs-k8s-*"),
            "size": size,
            "from": from_,
            "filters": {
                key: data.get(key)
                for key in (
                    "q",
                    "cluster",
                    "namespace",
                    "workload_name",
                    "pod",
                    "container",
                    "node",
                    "service",
                    "biz_line",
                    "business_key",
                    "frontend_service",
                    "backend_service",
                    "domain",
                    "route",
                    "version",
                    "trace_id",
                    "span_id",
                    "request_id",
                    "event_id",
                    "tenant_id",
                    "user_id",
                    "order_id",
                    "error_code",
                    "severity",
                    "start_ts",
                    "end_ts",
                )
                if data.get(key) not in (None, "")
            },
        },
        "total": total,
        "items": [normalize_log_hit(hit) for hit in hits],
    }
