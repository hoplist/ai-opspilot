#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import argparse
import json

from auto_inspection import config
from auto_inspection import incident_store
from auto_inspection import opensearch_client


def _logs_template():
    return {
        "index_patterns": [getattr(config, "OPENSEARCH_INDEX_LOGS", "logs-k8s-*")],
        "template": {
            "settings": {
                "number_of_shards": 1,
                "number_of_replicas": 0,
            },
            "mappings": {
                "dynamic": True,
                "properties": {
                    "@timestamp": {"type": "date"},
                    "cluster": {"type": "keyword"},
                    "namespace": {"type": "keyword"},
                    "pod": {"type": "keyword"},
                    "container": {"type": "keyword"},
                    "node": {"type": "keyword"},
                    "service": {"type": "keyword"},
                    "biz_line": {"type": "keyword"},
                    "trace_id": {"type": "keyword"},
                    "span_id": {"type": "keyword"},
                    "parent_span_id": {"type": "keyword"},
                    "request_id": {"type": "keyword"},
                    "event_id": {"type": "keyword"},
                    "tenant_id": {"type": "keyword"},
                    "user_id": {"type": "keyword"},
                    "order_id": {"type": "keyword"},
                    "error_code": {"type": "keyword"},
                    "route": {"type": "keyword"},
                    "domain": {"type": "keyword"},
                    "host": {"type": "keyword"},
                    "version": {"type": "keyword"},
                    "frontend_service": {"type": "keyword"},
                    "backend_service": {"type": "keyword"},
                    "business_key": {"type": "keyword"},
                    "workload_kind": {"type": "keyword"},
                    "workload_name": {"type": "keyword"},
                    "severity": {"type": "keyword"},
                    "logger": {"type": "keyword"},
                    "stream": {"type": "keyword"},
                    "stack_language": {"type": "keyword"},
                    "exception_type": {"type": "keyword"},
                    "exception_message": {
                        "type": "text",
                        "fields": {
                            "keyword": {"type": "keyword", "ignore_above": 2048}
                        },
                    },
                    "message": {
                        "type": "text",
                        "fields": {
                            "keyword": {"type": "keyword", "ignore_above": 2048}
                        },
                    },
                    "message_normalized": {
                        "type": "text",
                        "fields": {
                            "keyword": {"type": "keyword", "ignore_above": 2048}
                        },
                    },
                    "log": {
                        "type": "text",
                        "fields": {
                            "keyword": {"type": "keyword", "ignore_above": 2048}
                        },
                    },
                    "exception": {
                        "properties": {
                            "type": {"type": "keyword"},
                            "message": {
                                "type": "text",
                                "fields": {
                                    "keyword": {"type": "keyword", "ignore_above": 2048}
                                },
                            },
                            "language": {"type": "keyword"},
                        },
                    },
                    "kubernetes": {"type": "object", "dynamic": True},
                    "log_processed": {"type": "object", "dynamic": True},
                },
            },
        },
    }


def _events_template():
    return {
        "index_patterns": [getattr(config, "OPENSEARCH_INDEX_EVENTS", "events-k8s-*")],
        "template": {
            "settings": {
                "number_of_shards": 1,
                "number_of_replicas": 0,
            },
            "mappings": {
                "dynamic": True,
                "properties": {
                    "@timestamp": {"type": "date"},
                    "cluster": {"type": "keyword"},
                    "namespace": {"type": "keyword"},
                    "pod": {"type": "keyword"},
                    "object_name": {"type": "keyword"},
                    "object_kind": {"type": "keyword"},
                    "reason": {"type": "keyword"},
                    "type": {"type": "keyword"},
                    "reporting_component": {"type": "keyword"},
                    "service": {"type": "keyword"},
                    "message": {
                        "type": "text",
                        "fields": {
                            "keyword": {"type": "keyword", "ignore_above": 2048}
                        },
                    },
                    "metadata": {"type": "object", "dynamic": True},
                    "involvedObject": {"type": "object", "dynamic": True},
                },
            },
        },
    }


def _traces_template():
    return {
        "index_patterns": [getattr(config, "OPENSEARCH_INDEX_TRACES", "otel-traces-*")],
        "template": {
            "settings": {
                "number_of_shards": 1,
                "number_of_replicas": 0,
            },
            "mappings": {
                "dynamic": True,
                "properties": {
                    "@timestamp": {"type": "date"},
                    "trace_id": {"type": "keyword"},
                    "span_id": {"type": "keyword"},
                    "parent_span_id": {"type": "keyword"},
                    "service": {"type": "keyword"},
                    "service_name": {"type": "keyword"},
                    "span_name": {"type": "keyword"},
                    "span_kind": {"type": "keyword"},
                    "duration_ms": {"type": "float"},
                    "status_code": {"type": "keyword"},
                    "error": {"type": "boolean"},
                    "http_route": {"type": "keyword"},
                    "http_method": {"type": "keyword"},
                    "http_status_code": {"type": "integer"},
                    "rpc_method": {"type": "keyword"},
                    "db_system": {"type": "keyword"},
                    "db_operation": {"type": "keyword"},
                    "domain": {"type": "keyword"},
                    "route": {"type": "keyword"},
                    "request_id": {"type": "keyword"},
                    "event_id": {"type": "keyword"},
                    "business_key": {"type": "keyword"},
                    "resource": {"type": "object", "dynamic": True},
                    "attributes": {"type": "object", "dynamic": True},
                },
            },
        },
    }


def _investigations_template():
    return {
        "index_patterns": [getattr(config, "OPENSEARCH_INDEX_INVESTIGATIONS", "inspection-investigations-*")],
        "template": {
            "settings": {
                "number_of_shards": 1,
                "number_of_replicas": 0,
            },
            "mappings": {
                "dynamic": True,
                "properties": {
                    "investigation_id": {"type": "keyword"},
                    "generated_at": {
                        "type": "date",
                        "format": "yyyy-MM-dd HH:mm:ss||strict_date_optional_time||epoch_millis",
                    },
                    "request": {"type": "object", "dynamic": True},
                    "target": {"type": "object", "dynamic": True},
                    "analysis": {"type": "object", "dynamic": True},
                    "analysis_input": {"type": "text"},
                    "analysis_prompt": {"type": "text"},
                    "links": {"type": "object", "dynamic": True},
                    "storage": {"type": "object", "dynamic": True},
                },
            },
        },
    }


def _retention_policy(index_patterns, delete_after_days, description):
    return {
        "policy": {
            "description": description,
            "default_state": "hot",
            "states": [
                {
                    "name": "hot",
                    "actions": [],
                    "transitions": [
                        {
                            "state_name": "delete",
                            "conditions": {"min_index_age": f"{max(1, int(delete_after_days))}d"},
                        }
                    ],
                },
                {
                    "name": "delete",
                    "actions": [{"delete": {}}],
                    "transitions": [],
                },
            ],
            "ism_template": [
                {
                    "index_patterns": list(index_patterns),
                    "priority": 100,
                }
            ],
        }
    }


def _retention_policies():
    return {
        "logs": _retention_policy(
            [getattr(config, "OPENSEARCH_INDEX_LOGS", "logs-k8s-*")],
            getattr(config, "OPENSEARCH_RETENTION_LOGS_DAYS", 14),
            "Retention policy for Kubernetes logs shipped by Fluent Bit.",
        ),
        "events": _retention_policy(
            [getattr(config, "OPENSEARCH_INDEX_EVENTS", "events-k8s-*")],
            getattr(config, "OPENSEARCH_RETENTION_EVENTS_DAYS", 30),
            "Retention policy for Kubernetes events shipped by Fluent Bit.",
        ),
        "incidents": _retention_policy(
            [getattr(config, "OPENSEARCH_INDEX_INCIDENTS", "inspection-incidents-*")],
            getattr(config, "OPENSEARCH_RETENTION_INCIDENTS_DAYS", 60),
            "Retention policy for normalized incident documents.",
        ),
        "investigations": _retention_policy(
            [getattr(config, "OPENSEARCH_INDEX_INVESTIGATIONS", "inspection-investigations-*")],
            getattr(config, "OPENSEARCH_RETENTION_INVESTIGATIONS_DAYS", 90),
            "Retention policy for RCA investigation results.",
        ),
    }


def _snapshot_repository():
    return {
        "type": "fs",
        "settings": {
            "location": getattr(config, "OPENSEARCH_SNAPSHOT_PATH", "/usr/share/opensearch/data/snapshots"),
            "compress": True,
        },
    }


def bootstrap():
    if not opensearch_client.is_configured():
        raise RuntimeError("OPENSEARCH_URL is not configured.")
    policies = _retention_policies()
    result = {
        "logs": opensearch_client.put_index_template("auto-inspection-logs", _logs_template()),
        "events": opensearch_client.put_index_template("auto-inspection-events", _events_template()),
        "traces": opensearch_client.put_index_template("auto-inspection-traces", _traces_template()),
        "incidents": incident_store.ensure_template(),
        "investigations": opensearch_client.put_index_template("auto-inspection-investigations", _investigations_template()),
        "retention_policies": {
            "logs": opensearch_client.put_ism_policy("auto-inspection-logs-retention", policies["logs"]),
            "events": opensearch_client.put_ism_policy("auto-inspection-events-retention", policies["events"]),
            "incidents": opensearch_client.put_ism_policy("auto-inspection-incidents-retention", policies["incidents"]),
            "investigations": opensearch_client.put_ism_policy("auto-inspection-investigations-retention", policies["investigations"]),
        },
        "snapshot_repository": opensearch_client.put_snapshot_repository(
            getattr(config, "OPENSEARCH_SNAPSHOT_REPOSITORY", "auto-inspection-local-fs"),
            _snapshot_repository(),
        ),
    }
    return result


def main(argv=None):
    parser = argparse.ArgumentParser(description="Bootstrap OpenSearch index templates.")
    parser.parse_args(argv)
    result = bootstrap()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
