#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import argparse
import json
import queue
import threading
import time
import uuid
from collections import deque
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse

from auto_inspection.backend_client import BackendClient


PROTOCOL_VERSION = "2025-06-18"
SERVER_NAME = "auto-inspection-mcp"
SERVER_VERSION = "0.1.0"
LOCALHOST_NAMES = {"127.0.0.1", "::1", "localhost"}
SSE_KEEPALIVE_SECONDS = 15
SESSION_IDLE_SECONDS = 3600
MAX_PENDING_EVENTS = 200
DEFAULT_LOG_LEVEL = "info"
LOG_LEVEL_RANK = {
    "debug": 7,
    "info": 6,
    "notice": 5,
    "warning": 4,
    "error": 3,
    "critical": 2,
    "alert": 1,
    "emergency": 0,
}


class SessionState:
    def __init__(self, session_id, protocol_version, client_key):
        self.session_id = session_id
        self.protocol_version = protocol_version
        self.client_key = client_key
        self.client_ready = False
        self.logging_level = None
        self.closed = False
        self.created_at = time.monotonic()
        self.last_seen = time.monotonic()
        self._lock = threading.Lock()
        self._stream_queues = {}
        self._stream_order = []
        self._next_stream_index = 0
        self._next_event_number = 1
        self._pending_events = deque(maxlen=MAX_PENDING_EVENTS)

    def attach_stream(self):
        stream_queue = queue.Queue()
        with self._lock:
            if self.closed:
                raise RuntimeError("Session is closed.")
            self.touch()
            stream_id = uuid.uuid4().hex
            self._stream_queues[stream_id] = stream_queue
            self._stream_order.append(stream_id)
            pending_events = list(self._pending_events)
            self._pending_events.clear()
        for event in pending_events:
            stream_queue.put(event)
        return stream_id, stream_queue

    def detach_stream(self, stream_id):
        with self._lock:
            self.touch()
            self._stream_queues.pop(stream_id, None)
            self._stream_order = [item for item in self._stream_order if item != stream_id]
            if self._stream_order:
                self._next_stream_index %= len(self._stream_order)
            else:
                self._next_stream_index = 0

    def enqueue(self, payload):
        with self._lock:
            if self.closed:
                return
            self.touch()
            event_id = f"{self.session_id}-{self._next_event_number}"
            self._next_event_number += 1
            target_queue = self._pick_stream_queue_locked()
            event = (event_id, payload)
            if target_queue is None:
                self._pending_events.append(event)
                return
        target_queue.put(event)

    def terminate(self):
        with self._lock:
            self.closed = True
            queues = list(self._stream_queues.values())
            self._stream_queues.clear()
            self._stream_order.clear()
            self._pending_events.clear()
        for item in queues:
            item.put(None)

    def touch(self):
        self.last_seen = time.monotonic()

    def _pick_stream_queue_locked(self):
        while self._stream_order:
            index = self._next_stream_index % len(self._stream_order)
            stream_id = self._stream_order[index]
            stream_queue = self._stream_queues.get(stream_id)
            self._next_stream_index = (index + 1) % len(self._stream_order)
            if stream_queue is not None:
                return stream_queue
            self._stream_order.pop(index)
        self._next_stream_index = 0
        return None


class SessionManager:
    def __init__(self):
        self._lock = threading.Lock()
        self._sessions = {}

    def create(self, protocol_version, client_key):
        session = SessionState(uuid.uuid4().hex, protocol_version, client_key)
        with self._lock:
            self._cleanup_expired_locked()
            self._sessions[session.session_id] = session
        return session

    def get(self, session_id):
        with self._lock:
            self._cleanup_expired_locked()
            session = self._sessions.get(session_id)
            if session is not None:
                session.touch()
            return session

    def remove(self, session_id):
        with self._lock:
            self._cleanup_expired_locked()
            session = self._sessions.pop(session_id, None)
        if session is not None:
            session.terminate()
        return session

    def get_latest_for_client(self, client_key):
        with self._lock:
            self._cleanup_expired_locked()
            candidates = [
                session
                for session in self._sessions.values()
                if session.client_key == client_key and not session.closed
            ]
            if not candidates:
                return None
            session = max(candidates, key=lambda item: item.created_at)
            session.touch()
            return session

    def _cleanup_expired_locked(self):
        now = time.monotonic()
        expired_ids = [
            session_id
            for session_id, session in self._sessions.items()
            if now - session.last_seen > SESSION_IDLE_SECONDS
        ]
        for session_id in expired_ids:
            session = self._sessions.pop(session_id, None)
            if session is not None:
                session.terminate()


SESSION_MANAGER = SessionManager()


TOOLS = [
    {
        "name": "health",
        "description": "Check whether the local auto_inspection backend is healthy.",
        "inputSchema": {
            "type": "object",
            "properties": {},
            "additionalProperties": False,
        },
    },
    {
        "name": "health_details",
        "description": "Check backend dependency health including OpenSearch and Prometheus.",
        "inputSchema": {
            "type": "object",
            "properties": {},
            "additionalProperties": False,
        },
    },
    {
        "name": "search_logs",
        "description": "Search Kubernetes logs from OpenSearch through the local auto_inspection backend.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "workload_name": {"type": "string"},
                "q": {"type": "string"},
                "size": {"type": "integer", "minimum": 1, "maximum": 200},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "search_business_logs",
        "description": "Search business-correlated logs by service, domain, trace_id, request_id, tenant/user/order id, route, version, or error_code. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "workload_name": {"type": "string"},
                "service": {"type": "string"},
                "biz_line": {"type": "string"},
                "business_key": {"type": "string"},
                "frontend_service": {"type": "string"},
                "backend_service": {"type": "string"},
                "domain": {"type": "string"},
                "route": {"type": "string"},
                "version": {"type": "string"},
                "trace_id": {"type": "string"},
                "span_id": {"type": "string"},
                "request_id": {"type": "string"},
                "event_id": {"type": "string"},
                "tenant_id": {"type": "string"},
                "user_id": {"type": "string"},
                "order_id": {"type": "string"},
                "error_code": {"type": "string"},
                "q": {"type": "string"},
                "size": {"type": "integer", "minimum": 1, "maximum": 200},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "search_traces",
        "description": "Search OpenTelemetry trace spans from the configured OpenSearch trace index. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "trace_id": {"type": "string"},
                "span_id": {"type": "string"},
                "service": {"type": "string"},
                "domain": {"type": "string"},
                "route": {"type": "string"},
                "request_id": {"type": "string"},
                "event_id": {"type": "string"},
                "business_key": {"type": "string"},
                "error": {"type": "boolean"},
                "q": {"type": "string"},
                "size": {"type": "integer", "minimum": 1, "maximum": 200},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "correlate_business_context",
        "description": "Infer and correlate backend service, frontend service, domain, logs, and traces for a business system such as workflow-server/workflow-web/workflow.tpo.xzoa.com. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "workload_name": {"type": "string"},
                "service": {"type": "string"},
                "backend_service": {"type": "string"},
                "frontend_service": {"type": "string"},
                "business_key": {"type": "string"},
                "domain": {"type": "string"},
                "route": {"type": "string"},
                "version": {"type": "string"},
                "trace_id": {"type": "string"},
                "span_id": {"type": "string"},
                "request_id": {"type": "string"},
                "event_id": {"type": "string"},
                "tenant_id": {"type": "string"},
                "user_id": {"type": "string"},
                "order_id": {"type": "string"},
                "error_code": {"type": "string"},
                "q": {"type": "string"},
                "size": {"type": "integer", "minimum": 1, "maximum": 200},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "get_context_pack",
        "description": "Return a read-only Evidence Pack for a pod, workload, service, incident, or namespace by aggregating logs, events, incidents, resource trends, business context, and release metadata. Source failures are reported in the response.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "target_type": {"type": "string", "enum": ["pod", "workload", "service", "incident", "namespace"]},
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "workload_name": {"type": "string"},
                "workload_kind": {"type": "string"},
                "service": {"type": "string"},
                "app_name": {"type": "string"},
                "application": {"type": "string"},
                "ref": {"type": "string"},
                "branch": {"type": "string"},
                "sha": {"type": "string"},
                "commit": {"type": "string"},
                "revision": {"type": "string"},
                "project_id": {"type": "string"},
                "project": {"type": "string"},
                "project_path": {"type": "string"},
                "incident_id": {"type": "string"},
                "symptom": {"type": "string"},
                "q": {"type": "string"},
                "size": {"type": "integer", "minimum": 1, "maximum": 200},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "required": ["target_type", "namespace"],
            "additionalProperties": False,
        },
    },
    {
        "name": "search_events",
        "description": "Search Kubernetes events from OpenSearch through the local auto_inspection backend.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "q": {"type": "string"},
                "size": {"type": "integer", "minimum": 1, "maximum": 200},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "investigate",
        "description": "Run RCA investigation against a namespace/pod or workload using the local auto_inspection backend.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "workload_name": {"type": "string"},
                "question": {"type": "string"},
                "query": {"type": "string"},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
                "use_ai": {"type": "boolean"},
            },
            "required": ["namespace"],
            "additionalProperties": False,
        },
    },
    {
        "name": "list_investigations",
        "description": "List recent investigations from the local auto_inspection backend.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "list_targets",
        "description": "List recommended investigation targets from the local auto_inspection backend.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "get_investigation",
        "description": "Fetch a single investigation by id, or use 'latest'.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "investigation_id": {"type": "string"},
            },
            "required": ["investigation_id"],
            "additionalProperties": False,
        },
    },
    {
        "name": "list_incidents",
        "description": "List current incident records from OpenSearch incidents index or local event artifacts.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "search_incidents",
        "description": "Search current incidents by query text, namespace, or pod.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "q": {"type": "string"},
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "node_resources",
        "description": "Query machine or node remaining CPU, memory, and disk capacity from Prometheus resource data.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "instance": {"type": "string"},
                "metric": {"type": "string"},
                "zone": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "list_namespaces",
        "description": "List Kubernetes namespaces through the read-only RCA backend, with optional fuzzy query.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "q": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 500},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "list_pods",
        "description": "List Kubernetes Pods through the read-only RCA backend. Supports namespace, fuzzy query, node, owner, and status filters.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "q": {"type": "string"},
                "status": {
                    "type": "string",
                    "description": "Phase or symptom filter such as running, pending, failed, abnormal, crashloop, imagepull, not_ready.",
                },
                "node": {"type": "string"},
                "owner_kind": {"type": "string"},
                "owner_name": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 500},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "list_abnormal_pods",
        "description": "List non-healthy Kubernetes Pods through the read-only RCA backend, including waiting reasons, restarts, owner, and node.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "q": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 500},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "list_workloads",
        "description": "List Kubernetes Deployments, StatefulSets, DaemonSets, and ReplicaSets through the read-only RCA backend.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "q": {"type": "string"},
                "kind": {"type": "string", "description": "all, deployment, statefulset, daemonset, or replicaset."},
                "limit": {"type": "integer", "minimum": 1, "maximum": 500},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "list_services",
        "description": "List Kubernetes Services through the read-only RCA backend, with optional namespace, fuzzy query, and service type filter.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "q": {"type": "string"},
                "type": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 500},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "search_k8s_resources",
        "description": "Fuzzy search Kubernetes namespaces, Pods, workloads, and Services through the read-only RCA backend.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "q": {"type": "string"},
                "kinds": {"type": "string", "description": "Comma-separated kinds such as pods,workloads,services,namespaces or all."},
                "limit": {"type": "integer", "minimum": 1, "maximum": 500},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "count_k8s_resources",
        "description": "Count Kubernetes namespaces, nodes, Pods, workloads, Services, and abnormal Pods through the read-only RCA backend.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "cluster_overview",
        "description": "Read a compact Kubernetes cluster overview with resource counts, nodes, namespaces, and top abnormal Pods. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "q": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 500},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "diagnose_pod",
        "description": "Build a read-only evidence bundle for a pod/workload symptom by combining RCA, logs, events, incidents, and resource summaries. This tool must not execute commands on servers or mutate Kubernetes resources.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "workload_name": {"type": "string"},
                "symptom": {
                    "type": "string",
                    "description": "Known symptom such as oom, crashloop, probe, pending, imagepull, latency, error, or unknown.",
                },
                "q": {"type": "string"},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
                "size": {"type": "integer", "minimum": 1, "maximum": 200},
                "use_ai": {"type": "boolean"},
            },
            "required": ["namespace"],
            "additionalProperties": False,
        },
    },
    {
        "name": "release_for_workload",
        "description": "Read Kubernetes workload release metadata such as images, revision, Helm annotations, and owner details. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "workload_name": {"type": "string"},
                "workload_kind": {"type": "string"},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "required": ["namespace"],
            "additionalProperties": False,
        },
    },
    {
        "name": "release_recent_changes",
        "description": "List recent read-only Kubernetes release/change metadata from workloads and ConfigMaps in a namespace.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "workload_name": {"type": "string"},
                "service": {"type": "string"},
                "include_configmaps": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 200},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "required": ["namespace"],
            "additionalProperties": False,
        },
    },
    {
        "name": "correlate_change_with_incident",
        "description": "Correlate incident time window with read-only workload release metadata and recent ConfigMap/workload changes.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "workload_name": {"type": "string"},
                "workload_kind": {"type": "string"},
                "service": {"type": "string"},
                "include_configmaps": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 200},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "required": ["namespace"],
            "additionalProperties": False,
        },
    },
    {
        "name": "argocd_app_status",
        "description": "Read Argo CD application status, health, sync state, Git revision, images, and resource summary. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "app_name": {"type": "string"},
                "application": {"type": "string"},
                "refresh": {"type": "string"},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "argocd_app_history",
        "description": "Read Argo CD application sync/deployment history and latest Git revision. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "app_name": {"type": "string"},
                "application": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "required": ["app_name"],
            "additionalProperties": False,
        },
    },
    {
        "name": "argocd_diff_summary",
        "description": "Read Argo CD application resource sync/diff summary and out-of-sync resources. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "app_name": {"type": "string"},
                "application": {"type": "string"},
                "refresh": {"type": "string"},
            },
            "required": ["app_name"],
            "additionalProperties": False,
        },
    },
    {
        "name": "gitlab_recent_commits",
        "description": "Read recent GitLab commits for the GitOps project. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "project_id": {"type": "string"},
                "project": {"type": "string"},
                "project_path": {"type": "string"},
                "ref": {"type": "string"},
                "branch": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
                "since": {"type": "string"},
                "until": {"type": "string"},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "gitlab_commit_detail",
        "description": "Read GitLab commit metadata and stats for a specific revision. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "project_id": {"type": "string"},
                "project": {"type": "string"},
                "project_path": {"type": "string"},
                "sha": {"type": "string"},
                "commit": {"type": "string"},
                "revision": {"type": "string"},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "gitlab_pipeline_status",
        "description": "Read GitLab pipeline status by ref, sha, or status. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "project_id": {"type": "string"},
                "project": {"type": "string"},
                "project_path": {"type": "string"},
                "ref": {"type": "string"},
                "branch": {"type": "string"},
                "sha": {"type": "string"},
                "commit": {"type": "string"},
                "revision": {"type": "string"},
                "status": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "gitlab_release_context",
        "description": "Read a combined GitLab + Argo CD release context for a GitOps revision. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "project_id": {"type": "string"},
                "project": {"type": "string"},
                "project_path": {"type": "string"},
                "app_name": {"type": "string"},
                "application": {"type": "string"},
                "ref": {"type": "string"},
                "branch": {"type": "string"},
                "sha": {"type": "string"},
                "commit": {"type": "string"},
                "revision": {"type": "string"},
                "history_limit": {"type": "integer", "minimum": 1, "maximum": 100},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "gitlab_merge_requests",
        "description": "Read GitLab merge requests by commit, branch, state, or search text. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "project_id": {"type": "string"},
                "project": {"type": "string"},
                "project_path": {"type": "string"},
                "sha": {"type": "string"},
                "commit": {"type": "string"},
                "revision": {"type": "string"},
                "state": {"type": "string"},
                "source_branch": {"type": "string"},
                "target_branch": {"type": "string"},
                "ref": {"type": "string"},
                "branch": {"type": "string"},
                "search": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "gitlab_tags",
        "description": "Read GitLab repository tags and tag commit metadata. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "project_id": {"type": "string"},
                "project": {"type": "string"},
                "project_path": {"type": "string"},
                "tag": {"type": "string"},
                "tag_name": {"type": "string"},
                "search": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "gitlab_artifacts",
        "description": "Read GitLab job and artifact metadata by ref, commit, or job name. Does not download artifact contents. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "project_id": {"type": "string"},
                "project": {"type": "string"},
                "project_path": {"type": "string"},
                "ref": {"type": "string"},
                "branch": {"type": "string"},
                "sha": {"type": "string"},
                "commit": {"type": "string"},
                "revision": {"type": "string"},
                "scope": {"type": "string"},
                "job": {"type": "string"},
                "job_name": {"type": "string"},
                "with_artifacts": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "gitlab_image_digest_context",
        "description": "Read GitLab registry tag digest metadata and correlate it with Argo CD or Kubernetes workload images. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "project_id": {"type": "string"},
                "project": {"type": "string"},
                "project_path": {"type": "string"},
                "app_name": {"type": "string"},
                "application": {"type": "string"},
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "workload_name": {"type": "string"},
                "workload_kind": {"type": "string"},
                "image": {"type": "string"},
                "registry_limit": {"type": "integer", "minimum": 1, "maximum": 100},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "service_red_metrics",
        "description": "Query Beyla/OpenTelemetry service RED metrics from Prometheus for read-only service call evidence.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "service": {"type": "string"},
                "service_name": {"type": "string"},
                "namespace": {"type": "string"},
                "route": {"type": "string"},
                "http_route": {"type": "string"},
                "rate_window": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "runtime_events_context",
        "description": "Search Falco runtime security and process events from OpenSearch logs. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "container": {"type": "string"},
                "rule": {"type": "string"},
                "priority": {"type": "string"},
                "q": {"type": "string"},
                "size": {"type": "integer", "minimum": 1, "maximum": 200},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "profile_hotspots",
        "description": "Query Pyroscope/Alloy eBPF profiling metadata and hot stack nodes for read-only performance RCA.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "service": {"type": "string"},
                "service_name": {"type": "string"},
                "namespace": {"type": "string"},
                "pod": {"type": "string"},
                "container": {"type": "string"},
                "node": {"type": "string"},
                "label_selector": {"type": "string"},
                "profile_type": {"type": "string"},
                "limit": {"type": "integer", "minimum": 1, "maximum": 100},
                "max_nodes": {"type": "integer", "minimum": 1, "maximum": 2048},
                "range_hours": {"type": "integer", "minimum": 1, "maximum": 168},
            },
            "additionalProperties": False,
        },
    },
]

RESOURCE_TEMPLATES = [
    {
        "uriTemplate": "pod://{cluster}/{namespace}/{pod}",
        "name": "Pod Evidence Pack",
        "description": "Read a pod Evidence Pack.",
        "mimeType": "application/json",
    },
    {
        "uriTemplate": "workload://{cluster}/{namespace}/{kind}/{name}",
        "name": "Workload Evidence Pack",
        "description": "Read a workload Evidence Pack.",
        "mimeType": "application/json",
    },
    {
        "uriTemplate": "incident://{incident_id}",
        "name": "Incident Evidence Pack",
        "description": "Read an incident Evidence Pack.",
        "mimeType": "application/json",
    },
]


def _resource_payload_from_uri(client, uri):
    parsed = urlparse(str(uri or ""))
    scheme = parsed.scheme.lower()
    parts = [item for item in parsed.path.strip("/").split("/") if item]
    host = parsed.netloc
    if scheme == "pod":
        if len(parts) < 2:
            raise ValueError("pod URI must be pod://<cluster>/<namespace>/<pod>")
        return client.context_pack_resource("pod", cluster=host, namespace=parts[0], pod=parts[1])
    if scheme == "workload":
        if len(parts) < 3:
            raise ValueError("workload URI must be workload://<cluster>/<namespace>/<kind>/<name>")
        return client.context_pack_resource("workload", cluster=host, namespace=parts[0], workload_kind=parts[1], workload_name=parts[2])
    if scheme == "incident":
        incident_id = host or (parts[0] if parts else "")
        if not incident_id:
            raise ValueError("incident URI must be incident://<incident_id>")
        return client.context_pack_resource("incident", incident_id=incident_id)
    raise ValueError(f"Unsupported resource URI scheme: {scheme}")


SYMPTOM_QUERY_HINTS = {
    "oom": "OOMKilled OR out of memory OR memory limit OR exit_code=137",
    "memory": "OOMKilled OR out of memory OR memory limit OR exit_code=137",
    "crashloop": "CrashLoopBackOff OR BackOff OR exit_code OR exception OR error",
    "probe": "Readiness probe failed OR Liveness probe failed OR Unhealthy OR timeout",
    "pending": "FailedScheduling OR Pending OR insufficient OR taint OR node selector",
    "imagepull": "ImagePullBackOff OR ErrImagePull OR pull image OR registry",
    "latency": "timeout OR latency OR slow OR deadline exceeded OR connection reset",
    "error": "error OR exception OR failed OR fatal OR panic",
    "unknown": "error OR warning OR failed OR exception OR BackOff",
}


def _symptom_query(symptom, q):
    explicit = str(q or "").strip()
    if explicit:
        return explicit
    key = str(symptom or "unknown").strip().lower()
    return SYMPTOM_QUERY_HINTS.get(key, SYMPTOM_QUERY_HINTS["unknown"])


def _with_window_arguments(arguments):
    data = dict(arguments or {})
    if "size" in data:
        data["size"] = max(1, min(int(data.get("size") or 20), 200))
    if "range_hours" in data:
        data["range_hours"] = max(1, min(int(data.get("range_hours") or 6), 168))
    return {key: value for key, value in data.items() if value not in (None, "")}


def _shorten_investigation(payload):
    if not isinstance(payload, dict):
        return payload
    analysis = payload.get("analysis") or {}
    evidence = payload.get("evidence") or {}
    target = payload.get("target") or {}
    storage = payload.get("storage") or {}
    return {
        "investigation_id": payload.get("investigation_id"),
        "generated_at": payload.get("generated_at"),
        "summary": analysis.get("summary"),
        "root_cause": analysis.get("root_cause"),
        "actions": analysis.get("actions"),
        "need_human_check": analysis.get("need_human_check"),
        "timeline": analysis.get("timeline"),
        "logs_count": len(evidence.get("logs") or []),
        "events_count": len(evidence.get("events") or []),
        "target_pods": target.get("pod_names"),
        "links": (payload.get("links") or {}).get("dashboards"),
        "storage": {
            "opensearch": (storage.get("opensearch") or {}).get("indexed"),
            "hot_store": (storage.get("hot_store") or {}).get("stored"),
            "cold_store": (storage.get("cold_store") or {}).get("stored"),
        },
    }


def _diagnose_pod_payload(client, arguments):
    namespace = str(arguments.get("namespace", "") or "").strip()
    pod = str(arguments.get("pod", "") or "").strip()
    workload_name = str(arguments.get("workload_name", "") or "").strip()
    symptom = str(arguments.get("symptom", "unknown") or "unknown").strip().lower()
    range_hours = int(arguments.get("range_hours", 6) or 6)
    size = int(arguments.get("size", 50) or 50)
    use_ai = bool(arguments.get("use_ai", False))
    query = _symptom_query(symptom, arguments.get("q", ""))

    payload = {
        "mode": "read_only_evidence_bundle",
        "safety": {
            "server_commands": "not_allowed",
            "kubernetes_mutations": "not_allowed",
            "notes": "This tool only reads RCA backend data sources and may create an investigation record through the backend.",
        },
        "request": {
            "namespace": namespace,
            "pod": pod,
            "workload_name": workload_name,
            "symptom": symptom,
            "query": query,
            "range_hours": range_hours,
            "size": size,
            "use_ai": use_ai,
        },
        "health": None,
        "investigation": None,
        "logs": None,
        "events": None,
        "incidents": None,
        "resources": None,
        "errors": [],
    }

    try:
        payload["health"] = client.health_details()
    except Exception as exc:
        payload["errors"].append({"source": "health_details", "message": str(exc)})

    try:
        payload["investigation"] = _shorten_investigation(
            client.investigate(
                namespace=namespace,
                pod=pod,
                workload_name=workload_name,
                question=f"Diagnose {namespace}/{pod or workload_name or '-'} symptom={symptom}",
                query=query,
                range_hours=range_hours,
                use_ai=use_ai,
            )
        )
    except Exception as exc:
        payload["errors"].append({"source": "investigate", "message": str(exc)})

    try:
        payload["logs"] = client.search_logs(
            namespace=namespace,
            pod=pod,
            workload_name=workload_name,
            q=query,
            size=size,
            range_hours=range_hours,
        )
    except Exception as exc:
        payload["errors"].append({"source": "search_logs", "message": str(exc)})

    try:
        payload["events"] = client.search_events(
            namespace=namespace,
            pod=pod,
            q=query,
            size=size,
            range_hours=range_hours,
        )
    except Exception as exc:
        payload["errors"].append({"source": "search_events", "message": str(exc)})

    try:
        payload["incidents"] = client.search_incidents(q=query, namespace=namespace, pod=pod, limit=20)
    except Exception as exc:
        payload["errors"].append({"source": "search_incidents", "message": str(exc)})

    try:
        payload["resources"] = _resource_filter_payload(
            client.resources(range_hours=range_hours),
            {"limit": 20, "range_hours": range_hours},
        )
    except Exception as exc:
        payload["errors"].append({"source": "node_resources", "message": str(exc)})

    return payload


def _resource_filter_payload(payload, arguments):
    items = (payload or {}).get("items") or []
    instance_text = str((arguments or {}).get("instance", "") or "").strip().lower()
    metric_text = str((arguments or {}).get("metric", "") or "").strip().lower()
    zone_text = str((arguments or {}).get("zone", "") or "").strip().lower()
    limit = max(1, min(int((arguments or {}).get("limit", 20) or 20), 100))

    filtered = []
    for item in items:
        if item.get("metric") == "pod":
            continue
        hay = " ".join(
            [
                str(item.get("instance") or ""),
                str(item.get("group") or ""),
                str(item.get("cluster") or ""),
            ]
        ).lower()
        if instance_text and instance_text not in hay:
            continue
        if metric_text and metric_text != str(item.get("metric") or "").lower():
            continue
        if zone_text:
            zone_value = str(item.get("remaining_zone") or item.get("usage_zone") or "").lower()
            if zone_text != zone_value:
                continue
        filtered.append(item)

    grouped = {}
    for item in filtered:
        entry = grouped.setdefault(
            item.get("instance") or "-",
            {
                "instance": item.get("instance") or "-",
                "cluster": item.get("cluster") or "-",
                "group": item.get("group") or "-",
                "cpu_usage": None,
                "cpu_remaining": None,
                "mem_usage": None,
                "mem_remaining": None,
                "disk_usage": None,
                "disk_remaining": None,
                "disk_avail_bytes": None,
                "disk_size_bytes": None,
                "zones": set(),
            },
        )
        metric = str(item.get("metric") or "").lower()
        if metric == "cpu":
            entry["cpu_usage"] = item.get("usage_current")
            entry["cpu_remaining"] = item.get("remaining_current")
        elif metric == "mem":
            entry["mem_usage"] = item.get("usage_current")
            entry["mem_remaining"] = item.get("remaining_current")
        elif metric == "disk":
            entry["disk_usage"] = item.get("usage_current")
            entry["disk_remaining"] = item.get("remaining_current")
            entry["disk_avail_bytes"] = item.get("avail_bytes")
            entry["disk_size_bytes"] = item.get("size_bytes")
        if item.get("remaining_zone"):
            entry["zones"].add(item.get("remaining_zone"))

    summaries = []
    for entry in grouped.values():
        entry["zones"] = sorted(entry["zones"])
        summaries.append(entry)

    summaries.sort(
        key=lambda item: (
            max(
                [value for value in (item.get("cpu_usage"), item.get("mem_usage"), item.get("disk_usage")) if isinstance(value, (int, float))]
                or [0]
            ),
            item.get("instance") or "",
        ),
        reverse=True,
    )
    return {
        "range_hours": int((arguments or {}).get("range_hours", 24) or 24),
        "items": summaries[:limit],
    }


class MCPHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server_version = "auto-inspection-mcp"

    def do_GET(self):
        if not self._validate_origin():
            return
        if self.path.rstrip("/") == "/mcp/health":
            self._send_json(
                {
                    "service": SERVER_NAME,
                    "status": "ok",
                    "version": SERVER_VERSION,
                    "protocolVersion": PROTOCOL_VERSION,
                }
            )
            return
        if self.path.rstrip("/") == "/mcp":
            session = self._resolve_session()
            if session is None:
                self._serve_probe_stream()
                return
            if not self._validate_session_protocol_header(session):
                return
            self._serve_sse_stream(session)
            return
        self._send_json({"error": "Not found"}, status=404)

    def do_POST(self):
        if not self._validate_origin():
            return
        if self.path.rstrip("/") != "/mcp":
            self._send_json({"error": "Not found"}, status=404)
            return
        content_type = str(self.headers.get("Content-Type") or "").lower()
        if "application/json" not in content_type:
            self._send_json({"error": "Unsupported media type"}, status=415)
            return

        try:
            request = self._read_json()
            session = None
            if request.get("method") != "initialize":
                session = self._resolve_session()
                if session is None:
                    self._send_json({"error": "Session not found"}, status=404)
                    return
            if self._is_jsonrpc_response(request):
                self._send_empty(status=202)
                return
            if not self._validate_protocol_header(request, session):
                return
            response, headers = self._handle_rpc(request, session)
            if response is None:
                self._send_empty(status=202, headers=headers)
                return
            self._send_json(response, headers=headers)
        except Exception as exc:
            self._send_json(
                {
                    "jsonrpc": "2.0",
                    "id": None,
                    "error": {
                        "code": -32000,
                        "message": str(exc),
                    },
                },
                status=500,
            )

    def do_DELETE(self):
        if not self._validate_origin():
            return
        if self.path.rstrip("/") != "/mcp":
            self._send_json({"error": "Not found"}, status=404)
            return
        session = self._resolve_session()
        if session is None:
            self._send_json({"error": "Session not found"}, status=404)
            return
        if not self._validate_session_protocol_header(session):
            return
        SESSION_MANAGER.remove(session.session_id)
        self._send_empty(status=204)

    def _read_json(self):
        length = int(self.headers.get("Content-Length", "0") or 0)
        raw = self.rfile.read(length) if length > 0 else b"{}"
        payload = json.loads(raw.decode("utf-8"))
        if not isinstance(payload, dict):
            raise ValueError("JSON-RPC request must be an object.")
        return payload

    def _send_json(self, payload, status=200, headers=None):
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        if headers:
            for key, value in headers.items():
                self.send_header(key, value)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def _send_empty(self, status=202, headers=None):
        self.send_response(status)
        if headers:
            for key, value in headers.items():
                self.send_header(key, value)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def _serve_sse_stream(self, session):
        stream_id, stream_queue = session.attach_stream()
        self.close_connection = True
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "keep-alive")
        self.send_header("X-Accel-Buffering", "no")
        self.end_headers()
        try:
            self._write_sse_comment("stream-open")
            while True:
                try:
                    event = stream_queue.get(timeout=SSE_KEEPALIVE_SECONDS)
                except queue.Empty:
                    self._write_sse_comment("keepalive")
                    continue
                if event is None:
                    return
                event_id, payload = event
                self._write_sse_event(event_id, payload)
        except (BrokenPipeError, ConnectionError, OSError):
            return
        finally:
            session.detach_stream(stream_id)

    def _serve_probe_stream(self):
        self.close_connection = True
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "keep-alive")
        self.send_header("X-Accel-Buffering", "no")
        self.end_headers()
        try:
            self._write_sse_comment("stream-open")
            while True:
                time.sleep(SSE_KEEPALIVE_SECONDS)
                self._write_sse_comment("keepalive")
        except (BrokenPipeError, ConnectionError, OSError):
            return

    def _write_sse_comment(self, text):
        self.wfile.write(f": {text}\n\n".encode("utf-8"))
        self.wfile.flush()

    def _write_sse_event(self, event_id, payload):
        self.wfile.write(f"id: {event_id}\n".encode("utf-8"))
        data = json.dumps(payload, ensure_ascii=False)
        for line in data.splitlines() or [""]:
            self.wfile.write(f"data: {line}\n".encode("utf-8"))
        self.wfile.write(b"\n")
        self.wfile.flush()

    def _validate_origin(self):
        origin = str(self.headers.get("Origin") or "").strip()
        if not origin:
            return True
        origin_host = self._extract_hostname(origin)
        request_host = self._extract_hostname(self.headers.get("Host") or "")
        if origin_host in LOCALHOST_NAMES or (origin_host and origin_host == request_host):
            return True
        self._send_json({"error": "Forbidden origin"}, status=403)
        return False

    def _validate_protocol_header(self, request, session):
        if request.get("method") == "initialize":
            return True
        version = str(self.headers.get("MCP-Protocol-Version") or "").strip()
        expected_version = str((session.protocol_version if session is not None else PROTOCOL_VERSION) or PROTOCOL_VERSION)
        if version and version != expected_version:
            self._send_json(
                {
                    "error": "Unsupported MCP protocol version",
                    "supported": [expected_version],
                    "received": version,
                },
                status=400,
            )
            return False
        return True

    def _validate_session_protocol_header(self, session):
        version = str(self.headers.get("MCP-Protocol-Version") or "").strip()
        expected_version = str((session.protocol_version if session is not None else PROTOCOL_VERSION) or PROTOCOL_VERSION)
        if version and version != expected_version:
            self._send_json(
                {
                    "error": "Unsupported MCP protocol version",
                    "supported": [expected_version],
                    "received": version,
                },
                status=400,
            )
            return False
        return True

    def _resolve_session(self):
        session_id = str(self.headers.get("Mcp-Session-Id") or "").strip()
        if session_id:
            return self._require_session(session_id)
        return SESSION_MANAGER.get_latest_for_client(self._client_key())

    def _require_session(self, session_id):
        session = SESSION_MANAGER.get(session_id)
        return session

    def _client_key(self):
        return str((self.client_address or [""])[0] or "")

    @staticmethod
    def _extract_hostname(value):
        text = str(value or "").strip()
        if not text:
            return ""
        parsed = urlparse(text if "://" in text else f"//{text}")
        return str(parsed.hostname or "").lower()

    @staticmethod
    def _is_jsonrpc_response(request):
        return "method" not in request and ("result" in request or "error" in request)

    def _success(self, request_id, result):
        return {"jsonrpc": "2.0", "id": request_id, "result": result}

    def _error(self, request_id, code, message):
        return {"jsonrpc": "2.0", "id": request_id, "error": {"code": code, "message": message}}

    def _handle_rpc(self, request, session):
        request_id = request.get("id") if "id" in request else None
        method = request.get("method")
        params = request.get("params") or {}

        if method == "notifications/initialized":
            session.client_ready = True
            self._emit_log(
                session,
                "info",
                {
                    "event": "session_ready",
                    "sessionId": session.session_id,
                },
            )
            return None, {}

        if "id" not in request:
            return None, {}

        if method == "initialize":
            session = SESSION_MANAGER.create(PROTOCOL_VERSION, self._client_key())
            self._emit_log(
                session,
                "info",
                {
                    "event": "session_created",
                    "sessionId": session.session_id,
                },
            )
            return self._success(
                request_id,
                {
                    "protocolVersion": PROTOCOL_VERSION,
                    "capabilities": {
                        "tools": {"listChanged": False},
                        "resources": {"subscribe": False, "listChanged": False},
                        "logging": {},
                    },
                    "serverInfo": {"name": SERVER_NAME, "version": SERVER_VERSION},
                    "instructions": "Use the tools to search logs, search events, list targets, or run investigations against the local auto_inspection backend.",
                },
            ), {"Mcp-Session-Id": session.session_id}

        if method == "ping":
            return self._success(request_id, {}), {}

        if session is not None and not session.client_ready:
            return self._error(request_id, -32002, "Session not initialized"), {}

        if method == "logging/setLevel":
            level = str(params.get("level") or "").strip().lower()
            if level not in LOG_LEVEL_RANK:
                return self._error(request_id, -32602, f"Unsupported log level: {level}"), {}
            session.logging_level = level
            self._emit_log(
                session,
                "info",
                {
                    "event": "logging_level_set",
                    "level": level,
                },
            )
            return self._success(request_id, {}), {}

        if method == "tools/list":
            return self._success(request_id, {"tools": TOOLS}), {}

        if method == "tools/call":
            return self._handle_tool_call(request_id, params, session), {}

        if method in {"resources/templates/list", "resourceTemplates/list"}:
            return self._success(request_id, {"resourceTemplates": RESOURCE_TEMPLATES}), {}

        if method == "resources/list":
            return self._success(
                request_id,
                {
                    "resources": [
                        {
                            "uri": "incident://latest",
                            "name": "Latest incidents context",
                            "description": "Read a compact incident Evidence Pack for recent incidents.",
                            "mimeType": "application/json",
                        }
                    ]
                },
            ), {}

        if method == "resources/read":
            uri = params.get("uri")
            try:
                payload = _resource_payload_from_uri(BackendClient(), uri)
            except Exception as exc:
                return self._error(request_id, -32602, str(exc)), {}
            return self._success(
                request_id,
                {
                    "contents": [
                        {
                            "uri": uri,
                            "mimeType": "application/json",
                            "text": json.dumps(payload, ensure_ascii=False, indent=2),
                        }
                    ]
                },
            ), {}

        return self._error(request_id, -32601, f"Method not found: {method}"), {}

    def _tool_result(self, request_id, payload):
        return self._success(
            request_id,
            {
                "content": [
                    {
                        "type": "text",
                        "text": json.dumps(payload, ensure_ascii=False, indent=2),
                    }
                ],
                "structuredContent": payload,
                "isError": False,
            },
        )

    def _handle_tool_call(self, request_id, params, session):
        name = params.get("name")
        arguments = params.get("arguments") or {}
        client = BackendClient()

        self._emit_log(
            session,
            "info",
            {
                "event": "tool_call_started",
                "tool": name,
            },
        )

        try:
            if name == "health":
                payload = client.health()
            elif name == "health_details":
                payload = client.health_details()
            elif name == "search_logs":
                payload = client.search_logs(
                    namespace=arguments.get("namespace", ""),
                    pod=arguments.get("pod", ""),
                    workload_name=arguments.get("workload_name", ""),
                    q=arguments.get("q", ""),
                    size=int(arguments.get("size", 20) or 20),
                    range_hours=int(arguments.get("range_hours", 6) or 6),
                )
            elif name == "search_business_logs":
                payload = client.search_business_logs(**_with_window_arguments(arguments))
            elif name == "search_traces":
                payload = client.search_traces(**_with_window_arguments(arguments))
            elif name == "correlate_business_context":
                payload = client.correlate_business_context(**_with_window_arguments(arguments))
            elif name == "get_context_pack":
                target_type = str(arguments.get("target_type", "") or "").strip().lower()
                context_arguments = {key: value for key, value in arguments.items() if key != "target_type"}
                payload = client.context_pack(target_type, **context_arguments)
            elif name == "search_events":
                payload = client.search_events(
                    namespace=arguments.get("namespace", ""),
                    pod=arguments.get("pod", ""),
                    q=arguments.get("q", ""),
                    size=int(arguments.get("size", 20) or 20),
                    range_hours=int(arguments.get("range_hours", 6) or 6),
                )
            elif name == "investigate":
                payload = client.investigate(
                    namespace=arguments.get("namespace", ""),
                    pod=arguments.get("pod", ""),
                    workload_name=arguments.get("workload_name", ""),
                    question=arguments.get("question", ""),
                    query=arguments.get("query", ""),
                    range_hours=int(arguments.get("range_hours", 6) or 6),
                    use_ai=bool(arguments.get("use_ai", False)),
                )
            elif name == "list_investigations":
                payload = client.list_investigations(limit=int(arguments.get("limit", 10) or 10))
            elif name == "list_targets":
                payload = client.list_targets(limit=int(arguments.get("limit", 10) or 10))
            elif name == "get_investigation":
                payload = client.get_investigation(arguments["investigation_id"])
            elif name == "list_incidents":
                payload = client.list_incidents(limit=int(arguments.get("limit", 10) or 10))
            elif name == "search_incidents":
                payload = client.search_incidents(
                    q=arguments.get("q", ""),
                    namespace=arguments.get("namespace", ""),
                    pod=arguments.get("pod", ""),
                    limit=int(arguments.get("limit", 20) or 20),
                )
            elif name == "node_resources":
                payload = _resource_filter_payload(
                    client.resources(range_hours=int(arguments.get("range_hours", 24) or 24)),
                    arguments,
                )
            elif name == "list_namespaces":
                payload = client.list_namespaces(**_with_window_arguments(arguments))
            elif name == "list_pods":
                payload = client.list_pods(**_with_window_arguments(arguments))
            elif name == "list_abnormal_pods":
                payload = client.list_abnormal_pods(**_with_window_arguments(arguments))
            elif name == "list_workloads":
                payload = client.list_workloads(**_with_window_arguments(arguments))
            elif name == "list_services":
                payload = client.list_services(**_with_window_arguments(arguments))
            elif name == "search_k8s_resources":
                payload = client.search_k8s_resources(**_with_window_arguments(arguments))
            elif name == "count_k8s_resources":
                payload = client.count_k8s_resources(**_with_window_arguments(arguments))
            elif name == "cluster_overview":
                payload = client.cluster_overview(**_with_window_arguments(arguments))
            elif name == "diagnose_pod":
                payload = _diagnose_pod_payload(client, arguments)
            elif name == "release_for_workload":
                payload = client.release_for_workload(**_with_window_arguments(arguments))
            elif name == "release_recent_changes":
                payload = client.release_recent_changes(**_with_window_arguments(arguments))
            elif name == "correlate_change_with_incident":
                payload = client.correlate_change_with_incident(**_with_window_arguments(arguments))
            elif name == "argocd_app_status":
                payload = client.argocd_app_status(**_with_window_arguments(arguments))
            elif name == "argocd_app_history":
                payload = client.argocd_app_history(**_with_window_arguments(arguments))
            elif name == "argocd_diff_summary":
                payload = client.argocd_diff_summary(**_with_window_arguments(arguments))
            elif name == "gitlab_recent_commits":
                payload = client.gitlab_recent_commits(**_with_window_arguments(arguments))
            elif name == "gitlab_commit_detail":
                payload = client.gitlab_commit_detail(**_with_window_arguments(arguments))
            elif name == "gitlab_pipeline_status":
                payload = client.gitlab_pipeline_status(**_with_window_arguments(arguments))
            elif name == "gitlab_release_context":
                payload = client.gitlab_release_context(**_with_window_arguments(arguments))
            elif name == "gitlab_merge_requests":
                payload = client.gitlab_merge_requests(**_with_window_arguments(arguments))
            elif name == "gitlab_tags":
                payload = client.gitlab_tags(**_with_window_arguments(arguments))
            elif name == "gitlab_artifacts":
                payload = client.gitlab_artifacts(**_with_window_arguments(arguments))
            elif name == "gitlab_image_digest_context":
                payload = client.gitlab_image_digest_context(**_with_window_arguments(arguments))
            elif name == "service_red_metrics":
                payload = client.service_red_metrics(**_with_window_arguments(arguments))
            elif name == "runtime_events_context":
                payload = client.runtime_events_context(**_with_window_arguments(arguments))
            elif name == "profile_hotspots":
                payload = client.profile_hotspots(**_with_window_arguments(arguments))
            else:
                return self._error(request_id, -32602, f"Unknown tool: {name}")
        except Exception as exc:
            self._emit_log(
                session,
                "error",
                {
                    "event": "tool_call_failed",
                    "tool": name,
                    "message": str(exc),
                },
            )
            return self._success(
                request_id,
                {
                    "content": [{"type": "text", "text": str(exc)}],
                    "structuredContent": {"error": str(exc)},
                    "isError": True,
                },
            )

        self._emit_log(
            session,
            "info",
            {
                "event": "tool_call_finished",
                "tool": name,
            },
        )
        return self._tool_result(request_id, payload)

    def _emit_log(self, session, level, data):
        if session is None:
            return
        normalized_level = str(level or DEFAULT_LOG_LEVEL).strip().lower()
        if normalized_level not in LOG_LEVEL_RANK:
            normalized_level = DEFAULT_LOG_LEVEL
        threshold = str(session.logging_level or DEFAULT_LOG_LEVEL).strip().lower()
        if threshold not in LOG_LEVEL_RANK:
            threshold = DEFAULT_LOG_LEVEL
        if LOG_LEVEL_RANK[normalized_level] > LOG_LEVEL_RANK[threshold]:
            return
        session.enqueue(
            {
                "jsonrpc": "2.0",
                "method": "notifications/message",
                "params": {
                    "level": normalized_level,
                    "logger": SERVER_NAME,
                    "data": data,
                },
            }
        )


def main(argv=None):
    parser = argparse.ArgumentParser(description="Run auto_inspection MCP HTTP server.")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", default=18081, type=int)
    args = parser.parse_args(argv)

    server = ThreadingHTTPServer((args.host, args.port), MCPHandler)
    server.daemon_threads = True
    print(f"[OK] auto_inspection MCP listening at http://{args.host}:{args.port}/mcp")
    server.serve_forever()


if __name__ == "__main__":
    raise SystemExit(main())
