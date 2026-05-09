#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import datetime

from auto_inspection import config
from auto_inspection import log_search
from auto_inspection import opensearch_client


TRACE_SOURCE_FIELDS = [
    "@timestamp",
    "trace_id",
    "traceId",
    "span_id",
    "spanId",
    "parent_span_id",
    "parentSpanId",
    "service",
    "service_name",
    "service.name",
    "span_name",
    "name",
    "span_kind",
    "kind",
    "duration_ms",
    "duration",
    "status_code",
    "status.code",
    "error",
    "http_route",
    "http.route",
    "http_method",
    "http.method",
    "http_status_code",
    "http.status_code",
    "rpc_method",
    "rpc.method",
    "db_system",
    "db.system",
    "db_operation",
    "db.operation",
    "domain",
    "host",
    "route",
    "request_id",
    "event_id",
    "business_key",
    "resource",
    "attributes",
]


def _iso_utc(timestamp):
    if timestamp is None:
        return None
    return datetime.datetime.fromtimestamp(
        int(timestamp),
        datetime.timezone.utc,
    ).strftime("%Y-%m-%dT%H:%M:%SZ")


def _first(*values):
    for value in values:
        if value not in (None, ""):
            return value
    return None


def _nested(source, dotted):
    current = source
    for part in str(dotted).split("."):
        if not isinstance(current, dict):
            return None
        current = current.get(part)
    return current


def _strip_suffix(value, suffix):
    text = str(value or "").strip()
    suffix = str(suffix or "").strip()
    if text and suffix and text.endswith(suffix):
        return text[: -len(suffix)]
    return text


def infer_business_context(params):
    data = params if isinstance(params, dict) else {}
    explicit_key = str(data.get("business_key") or "").strip()
    service = str(data.get("service") or "").strip()
    backend = str(data.get("backend_service") or "").strip()
    frontend = str(data.get("frontend_service") or "").strip()
    domain = str(data.get("domain") or "").strip()
    suffixes = getattr(config, "BUSINESS_DOMAIN_SUFFIXES", []) or []
    backend_suffix = getattr(config, "BUSINESS_BACKEND_SUFFIX", "-server")
    frontend_suffix = getattr(config, "BUSINESS_FRONTEND_SUFFIX", "-web")
    service_map = getattr(config, "BUSINESS_SERVICE_MAP", {}) or {}

    key = explicit_key
    if not key and service in service_map:
        key = str((service_map.get(service) or {}).get("business_key") or "").strip()
    if not key and backend:
        key = _strip_suffix(backend, backend_suffix)
    if not key and frontend:
        key = _strip_suffix(frontend, frontend_suffix)
    if not key and service:
        if service.endswith(backend_suffix):
            key = _strip_suffix(service, backend_suffix)
        elif service.endswith(frontend_suffix):
            key = _strip_suffix(service, frontend_suffix)
        else:
            key = service
    if not key and domain:
        host = domain.split(":", 1)[0]
        key = host.split(".", 1)[0]

    mapped = service_map.get(key) if key in service_map else {}
    if not backend:
        backend = str((mapped or {}).get("backend_service") or "").strip()
    if not frontend:
        frontend = str((mapped or {}).get("frontend_service") or "").strip()
    if not backend and key:
        backend = f"{key}{backend_suffix}"
    if not frontend and key:
        frontend = f"{key}{frontend_suffix}"

    domains = []
    if domain:
        domains.append(domain)
    mapped_domains = (mapped or {}).get("domains") or []
    if isinstance(mapped_domains, str):
        mapped_domains = [mapped_domains]
    domains.extend([item for item in mapped_domains if item])
    if key:
        domains.extend([f"{key}.{suffix}" for suffix in suffixes if suffix])
    domains = list(dict.fromkeys(domains))

    services = list(dict.fromkeys([item for item in [service, backend, frontend] if item]))
    return {
        "business_key": key,
        "backend_service": backend,
        "frontend_service": frontend,
        "domains": domains,
        "services": services,
        "rules": {
            "backend_suffix": backend_suffix,
            "frontend_suffix": frontend_suffix,
            "domain_suffixes": suffixes,
        },
    }


def _term_filter(fields, value):
    text = str(value or "").strip()
    if not text:
        return None
    should = []
    for field in fields:
        should.append({"term": {field: text}})
        if not field.endswith(".keyword"):
            should.append({"term": {f"{field}.keyword": text}})
    return {"bool": {"should": should, "minimum_should_match": 1}}


def _trace_query(params):
    data = params if isinstance(params, dict) else {}
    must = []
    filters = []
    mappings = {
        "trace_id": ["trace_id", "traceId"],
        "span_id": ["span_id", "spanId"],
        "service": ["service", "service_name", "service.name", "resource.service.name"],
        "domain": ["domain", "host", "http.host", "attributes.http.host"],
        "route": ["route", "http_route", "http.route", "attributes.http.route"],
        "request_id": ["request_id", "request.id", "attributes.request_id"],
        "event_id": ["event_id", "event.id", "attributes.event_id"],
        "business_key": ["business_key", "attributes.business_key"],
    }
    for key, fields in mappings.items():
        item = _term_filter(fields, data.get(key))
        if item:
            filters.append(item)

    q = str(data.get("q", "") or "").strip()
    if q:
        must.append(
            {
                "simple_query_string": {
                    "query": q,
                    "fields": [
                        "trace_id^4",
                        "traceId^4",
                        "span_name^3",
                        "name^3",
                        "service^3",
                        "service_name^3",
                        "service.name^3",
                        "http_route^2",
                        "http.route^2",
                        "route^2",
                        "domain^2",
                        "host^2",
                        "status_code^2",
                        "error^2",
                    ],
                    "default_operator": "and",
                }
            }
        )

    if data.get("error") not in (None, ""):
        filters.append({"term": {"error": str(data.get("error")).lower() in {"1", "true", "yes"}}})

    start_ts = data.get("start_ts")
    end_ts = data.get("end_ts")
    if start_ts is not None or end_ts is not None:
        range_body = {}
        if start_ts is not None:
            range_body["gte"] = _iso_utc(start_ts)
        if end_ts is not None:
            range_body["lte"] = _iso_utc(end_ts)
        filters.append({"range": {"@timestamp": range_body}})

    query = {"bool": {"must": must or [{"match_all": {}}]}}
    if filters:
        query["bool"]["filter"] = filters
    return query


def _normalize_trace_hit(hit):
    source = (hit or {}).get("_source") or {}
    resource = source.get("resource") or {}
    attrs = source.get("attributes") or {}
    service_name = _first(
        source.get("service"),
        source.get("service_name"),
        _nested(source, "service.name"),
        _nested(resource, "service.name"),
        attrs.get("service.name"),
    )
    return {
        "id": hit.get("_id"),
        "index": hit.get("_index"),
        "score": hit.get("_score"),
        "timestamp": source.get("@timestamp"),
        "trace_id": _first(source.get("trace_id"), source.get("traceId")),
        "span_id": _first(source.get("span_id"), source.get("spanId")),
        "parent_span_id": _first(source.get("parent_span_id"), source.get("parentSpanId")),
        "service": service_name,
        "span_name": _first(source.get("span_name"), source.get("name")),
        "span_kind": _first(source.get("span_kind"), source.get("kind")),
        "duration_ms": _first(source.get("duration_ms"), source.get("duration")),
        "status_code": _first(source.get("status_code"), _nested(source, "status.code")),
        "error": source.get("error"),
        "route": _first(source.get("route"), source.get("http_route"), _nested(source, "http.route"), attrs.get("http.route")),
        "domain": _first(source.get("domain"), source.get("host"), _nested(source, "http.host"), attrs.get("http.host")),
        "request_id": _first(source.get("request_id"), attrs.get("request_id")),
        "event_id": _first(source.get("event_id"), attrs.get("event_id")),
        "business_key": _first(source.get("business_key"), attrs.get("business_key")),
        "raw": source,
    }


def search_traces(params):
    data = params if isinstance(params, dict) else {}
    size = max(1, min(int(data.get("size") or 50), 500))
    from_ = max(0, int(data.get("from") or 0))
    index = getattr(config, "OPENSEARCH_INDEX_TRACES", "otel-traces-*")
    response = opensearch_client.search(
        index,
        _trace_query(data),
        size=size,
        from_=from_,
        sort=[{"@timestamp": {"order": "desc"}}],
        source_includes=TRACE_SOURCE_FIELDS,
        timeout=data.get("timeout"),
    )
    total, hits = opensearch_client.response_hits(response)
    return {
        "query": {
            "index": index,
            "size": size,
            "from": from_,
            "filters": {
                key: data.get(key)
                for key in (
                    "q",
                    "trace_id",
                    "span_id",
                    "service",
                    "domain",
                    "route",
                    "request_id",
                    "event_id",
                    "business_key",
                    "error",
                    "start_ts",
                    "end_ts",
                )
                if data.get(key) not in (None, "")
            },
        },
        "total": total,
        "items": [_normalize_trace_hit(hit) for hit in hits],
    }


def search_business_logs(params):
    return log_search.search_logs(params)


def correlate_business_context(params):
    data = params if isinstance(params, dict) else {}
    context = infer_business_context(data)
    size = max(1, min(int(data.get("size") or 50), 200))

    log_filters = {
        key: data.get(key)
        for key in (
            "q",
            "cluster",
            "namespace",
            "pod",
            "workload_name",
            "trace_id",
            "span_id",
            "request_id",
            "event_id",
            "tenant_id",
            "user_id",
            "order_id",
            "error_code",
            "route",
            "version",
            "start_ts",
            "end_ts",
        )
        if data.get(key) not in (None, "")
    }
    if data.get("service"):
        log_filters["service"] = data.get("service")
    elif context["backend_service"]:
        log_filters["service"] = context["backend_service"]
    if data.get("domain"):
        log_filters["domain"] = data.get("domain")
    elif context["domains"]:
        log_filters["domain"] = context["domains"][0]
    if context["business_key"]:
        log_filters.setdefault("business_key", context["business_key"])
    log_filters["size"] = size

    trace_filters = {
        key: data.get(key)
        for key in (
            "q",
            "trace_id",
            "span_id",
            "request_id",
            "event_id",
            "route",
            "start_ts",
            "end_ts",
        )
        if data.get(key) not in (None, "")
    }
    if data.get("service"):
        trace_filters["service"] = data.get("service")
    elif context["backend_service"]:
        trace_filters["service"] = context["backend_service"]
    if data.get("domain"):
        trace_filters["domain"] = data.get("domain")
    elif context["domains"]:
        trace_filters["domain"] = context["domains"][0]
    if context["business_key"]:
        trace_filters.setdefault("business_key", context["business_key"])
    trace_filters["size"] = size

    payload = {
        "mode": "read_only_business_correlation",
        "safety": {
            "server_commands": "not_allowed",
            "kubernetes_mutations": "not_allowed",
        },
        "business_context": context,
        "logs": None,
        "traces": None,
        "errors": [],
    }
    try:
        payload["logs"] = search_business_logs(log_filters)
    except Exception as exc:
        payload["errors"].append({"source": "business_logs", "message": str(exc)})
    try:
        payload["traces"] = search_traces(trace_filters)
    except Exception as exc:
        payload["errors"].append({"source": "traces", "message": str(exc)})
    return payload
