#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import datetime

from auto_inspection import config
from auto_inspection import opensearch_client


DEFAULT_EVENT_SOURCE_FIELDS = [
    "@timestamp",
    "cluster",
    "namespace",
    "message",
    "reason",
    "type",
    "action",
    "reporting_component",
    "note",
    "event_count",
    "first_timestamp",
    "last_timestamp",
    "regarding.kind",
    "regarding.name",
    "regarding.namespace",
    "metadata.namespace",
    "involvedObject.kind",
    "involvedObject.name",
    "involvedObject.namespace",
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
        "namespace": ["namespace", "regarding.namespace", "metadata.namespace", "involvedObject.namespace"],
        "pod": ["pod", "regarding.name", "involvedObject.name"],
        "reason": ["reason"],
        "type": ["type"],
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


def build_event_query(params):
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
                        "message^3",
                        "note^3",
                        "reason^2",
                        "type",
                        "action",
                        "reporting_component",
                        "regarding.name^2",
                        "involvedObject.name^2",
                        "metadata.namespace",
                        "regarding.kind",
                        "involvedObject.kind",
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


def normalize_event_hit(hit):
    source = (hit or {}).get("_source") or {}
    regarding = source.get("regarding") or {}
    metadata = source.get("metadata") or {}
    involved = source.get("involvedObject") or {}
    return {
        "id": hit.get("_id"),
        "index": hit.get("_index"),
        "score": hit.get("_score"),
        "timestamp": source.get("@timestamp"),
        "cluster": source.get("cluster"),
        "namespace": source.get("namespace") or regarding.get("namespace") or metadata.get("namespace") or involved.get("namespace"),
        "message": source.get("message") or source.get("note"),
        "reason": source.get("reason"),
        "type": source.get("type"),
        "action": source.get("action"),
        "reporting_component": source.get("reporting_component") or source.get("reportingComponent") or (source.get("source") or {}).get("component"),
        "regarding": {
            "kind": regarding.get("kind") or involved.get("kind"),
            "name": regarding.get("name") or involved.get("name"),
            "namespace": regarding.get("namespace") or involved.get("namespace"),
        },
        "event_count": source.get("event_count") or source.get("count"),
        "first_timestamp": source.get("first_timestamp") or source.get("firstTimestamp"),
        "last_timestamp": source.get("last_timestamp") or source.get("lastTimestamp"),
        "raw": source,
    }


def search_events(params):
    data = params if isinstance(params, dict) else {}
    size = max(1, min(int(data.get("size") or 50), 500))
    from_ = max(0, int(data.get("from") or 0))
    response = opensearch_client.search(
        getattr(config, "OPENSEARCH_INDEX_EVENTS", "events-k8s-*"),
        build_event_query(data),
        size=size,
        from_=from_,
        sort=[{"@timestamp": {"order": "desc"}}],
        source_includes=DEFAULT_EVENT_SOURCE_FIELDS,
        timeout=data.get("timeout"),
    )
    total, hits = opensearch_client.response_hits(response)
    return {
        "query": {
            "index": getattr(config, "OPENSEARCH_INDEX_EVENTS", "events-k8s-*"),
            "size": size,
            "from": from_,
            "filters": {
                key: data.get(key)
                for key in (
                    "q",
                    "cluster",
                    "namespace",
                    "pod",
                    "reason",
                    "type",
                    "start_ts",
                    "end_ts",
                )
                if data.get(key) not in (None, "")
            },
        },
        "total": total,
        "items": [normalize_event_hit(hit) for hit in hits],
    }
