#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import re

from auto_inspection import config
from auto_inspection import dashboards_client
from auto_inspection import opensearch_client
from auto_inspection import source_context


INCIDENT_TEMPLATE_NAME = "auto-inspection-incidents"


def _resolve_write_index(pattern):
    pattern = (pattern or "").strip()
    if not pattern:
        return ""
    if "*" not in pattern:
        return pattern
    from datetime import datetime

    return pattern.replace("*", datetime.now().strftime("%Y.%m.%d"))


def _incident_index_template(index_pattern):
    return {
        "index_patterns": [index_pattern],
        "template": {
            "settings": {
                "number_of_shards": 1,
                "number_of_replicas": 0,
            },
            "mappings": {
                "dynamic": True,
                "properties": {
                    "source": {"type": "object", "dynamic": True},
                    "generated_at": {
                        "type": "date",
                        "format": "yyyy-MM-dd HH:mm:ss||strict_date_optional_time||epoch_millis",
                    },
                    "event_key": {"type": "keyword"},
                    "instance": {"type": "keyword"},
                    "namespace": {"type": "keyword"},
                    "pod": {"type": "keyword"},
                    "workload_name": {"type": "keyword"},
                    "service": {"type": "keyword"},
                    "node": {"type": "keyword"},
                    "risk_level": {"type": "keyword"},
                    "final_risk_level": {"type": "keyword"},
                    "dominant_risk": {"type": "keyword"},
                    "lifecycle": {"type": "keyword"},
                    "runbook": {"type": "object", "dynamic": True},
                    "signals": {"type": "keyword"},
                    "escalation_reasons": {"type": "keyword"},
                },
            },
        },
    }


def ensure_template():
    if not opensearch_client.is_configured():
        return None
    pattern = getattr(config, "OPENSEARCH_INDEX_INCIDENTS", "") or ""
    if not pattern:
        return None
    return opensearch_client.put_index_template(
        INCIDENT_TEMPLATE_NAME,
        _incident_index_template(pattern),
    )


def sync_events(events_payload):
    if not opensearch_client.is_configured():
        return {"indexed": False, "reason": "opensearch_not_configured"}

    pattern = getattr(config, "OPENSEARCH_INDEX_INCIDENTS", "") or ""
    if not pattern:
        return {"indexed": False, "reason": "index_pattern_missing"}

    ensure_template()
    index_name = _resolve_write_index(pattern)
    source = source_context.source_metadata()
    generated_at = events_payload.get("generated_at")
    count = 0

    for event in events_payload.get("events") or []:
        document = {
            **event,
            "generated_at": generated_at,
            "source": source,
        }
        document_id = f"{source['fingerprint']}::{event.get('event_key') or event.get('instance') or count}"
        opensearch_client.index_document(index_name, document, document_id=document_id, refresh=True)
        count += 1

    return {
        "indexed": True,
        "index": index_name,
        "count": count,
        "source_fingerprint": source["fingerprint"],
    }


def _list_local_events(limit=20, include_links=True):
    try:
        from auto_inspection.paths import project_path
        path = project_path("data", "events_with_runbook.json")
        with open(path, "r", encoding="utf-8") as f:
            payload = __import__("json").load(f)
    except Exception:
        return []
    events = payload.get("events") or []
    events.sort(
        key=lambda item: (
            _risk_score(item.get("final_risk_level") or item.get("risk_level")),
            item.get("generated_at") or item.get("last_seen") or "",
        ),
        reverse=True,
    )
    return _enrich_incident_items(events[:limit], include_links=include_links)


def _risk_score(level):
    mapping = {"critical": 120, "high": 90, "medium": 55, "low": 30, "info": 10}
    return mapping.get(str(level or "").lower(), 40)


def _current_source_filter():
    return {"term": {"source.fingerprint": source_context.source_fingerprint()}}


def _incident_sort():
    return [
        {"risk_score": {"order": "desc", "missing": "_last"}},
        {"generated_at": {"order": "desc", "missing": "_last"}},
    ]


def _saved_object_token(value):
    token = re.sub(r"[^a-zA-Z0-9_-]+", "-", str(value or "").strip()).strip("-").lower()
    return token or "item"


def _incident_dashboards_links(item):
    if not dashboards_client.is_configured():
        return {}

    links = {
        "overview_dashboard": dashboards_client.dashboards_view_url("dashboard-auto-inspection-overview"),
    }
    namespace = str(item.get("namespace") or "").strip()
    pod = str(item.get("pod") or "").strip()
    workload_name = str(item.get("workload_name") or "").strip()
    if not namespace or not (pod or workload_name):
        return links

    target = pod or workload_name
    scope = f"{source_context.source_fingerprint()}-{namespace}-{target}"
    logs_id = f"incident-{_saved_object_token(scope)}-logs"
    events_id = f"incident-{_saved_object_token(scope)}-events"

    logs_query_parts = [f'namespace:"{namespace}"']
    events_query_parts = [f'namespace:"{namespace}"']
    if pod:
        logs_query_parts.append(f'pod:"{pod}"')
        events_query_parts.append(f'pod:"{pod}" or object_name:"{pod}"')
    else:
        logs_query_parts.append(f'pod:*{workload_name}* or service:"{workload_name}"')
        events_query_parts.append(f'object_name:*{workload_name}*')

    dashboards_client.upsert_saved_search(
        logs_id,
        dashboards_client.build_saved_search_payload(
            title=f"Incident Logs - {namespace}/{target}",
            description="Generated from auto_inspection incidents view",
            index_id="logs-k8s-data-view",
            query=" and ".join(logs_query_parts),
            columns=["namespace", "pod", "container", "severity", "message"],
            sort=[["@timestamp", "desc"]],
        ),
    )
    dashboards_client.upsert_saved_search(
        events_id,
        dashboards_client.build_saved_search_payload(
            title=f"Incident Events - {namespace}/{target}",
            description="Generated from auto_inspection incidents view",
            index_id="events-k8s-data-view",
            query=" and ".join(events_query_parts),
            columns=["namespace", "object_kind", "object_name", "reason", "message"],
            sort=[["@timestamp", "desc"]],
        ),
    )
    links["logs"] = dashboards_client.discover_saved_search_url(logs_id)
    links["events"] = dashboards_client.discover_saved_search_url(events_id)
    return links


def _enrich_incident_items(items, *, include_links=True):
    enriched = []
    for item in items or []:
        incident = dict(item or {})
        incident["risk_score"] = int(
            incident.get("risk_score")
            or _risk_score(incident.get("final_risk_level") or incident.get("risk_level"))
        )
        incident["investigation_supported"] = bool(
            incident.get("namespace") and (incident.get("pod") or incident.get("workload_name"))
        )
        if include_links:
            links = dict((incident.get("links") or {}).get("dashboards") or {})
            generated_links = _incident_dashboards_links(incident)
            if generated_links:
                links.update(generated_links)
            if links:
                incident["links"] = {"dashboards": links}
        enriched.append(incident)
    return enriched


def list_incidents(limit=20, include_links=True):
    if not opensearch_client.is_configured():
        return {
            "source": "local-json-cache",
            "source_fingerprint": source_context.source_fingerprint(),
            "items": _list_local_events(limit=limit, include_links=include_links),
        }

    index = getattr(config, "OPENSEARCH_INDEX_INCIDENTS", "") or ""
    if not index:
        return {
            "source": "local-json-cache",
            "source_fingerprint": source_context.source_fingerprint(),
            "items": _list_local_events(limit=limit, include_links=include_links),
        }

    response = opensearch_client.search(
        index,
        {"bool": {"filter": [_current_source_filter()]}},
        size=limit,
        sort=_incident_sort(),
    )
    _, hits = opensearch_client.response_hits(response)
    return {
        "source": "opensearch",
        "source_fingerprint": source_context.source_fingerprint(),
        "items": _enrich_incident_items([((item or {}).get("_source") or {}) for item in hits], include_links=include_links),
    }


def search_incidents(*, q="", namespace="", pod="", limit=20, include_links=True):
    if not opensearch_client.is_configured():
        items = _list_local_events(limit=200, include_links=False)
        q_text = str(q or "").strip().lower()
        namespace_text = str(namespace or "").strip().lower()
        pod_text = str(pod or "").strip().lower()
        filtered = []
        for item in items:
            hay = " ".join(
                [
                    str(item.get("instance") or ""),
                    str(item.get("namespace") or ""),
                    str(item.get("pod") or ""),
                    str(item.get("workload_name") or ""),
                    str(item.get("service") or ""),
                    str(item.get("dominant_risk") or ""),
                    str(item.get("final_risk_level") or ""),
                    str((item.get("runbook") or {}).get("title") or ""),
                ]
            ).lower()
            if q_text and q_text not in hay:
                continue
            if namespace_text and namespace_text not in hay:
                continue
            if pod_text and pod_text not in hay:
                continue
            filtered.append(item)
        return {
            "source": "local-json-cache",
            "source_fingerprint": source_context.source_fingerprint(),
            "items": _enrich_incident_items(filtered[:limit], include_links=include_links),
        }

    should = []
    q_text = str(q or "").strip()
    if q_text:
        should.append(
            {
                "simple_query_string": {
                    "query": q_text,
                    "fields": [
                        "instance^3",
                        "pod^3",
                        "workload_name^3",
                        "dominant_risk^2",
                        "final_risk_level",
                        "runbook.title^2",
                        "escalation_reasons",
                    ],
                    "default_operator": "and",
                }
            }
        )

    filters = []
    namespace_text = str(namespace or "").strip()
    if namespace_text:
        filters.append({"term": {"namespace": namespace_text}})
    pod_text = str(pod or "").strip()
    if pod_text:
        filters.append({"term": {"pod": pod_text}})
    filters.append(_current_source_filter())

    query = {"bool": {"must": should or [{"match_all": {}}]}}
    if filters:
        query["bool"]["filter"] = filters

    response = opensearch_client.search(
        getattr(config, "OPENSEARCH_INDEX_INCIDENTS", ""),
        query,
        size=limit,
        sort=_incident_sort(),
    )
    _, hits = opensearch_client.response_hits(response)
    return {
        "source": "opensearch",
        "source_fingerprint": source_context.source_fingerprint(),
        "items": _enrich_incident_items([((item or {}).get("_source") or {}) for item in hits], include_links=include_links),
    }
