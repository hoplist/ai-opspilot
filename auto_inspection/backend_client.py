#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import requests


DEFAULT_BASE_URL = "http://127.0.0.1:18080"


class BackendClient:
    def __init__(self, base_url=DEFAULT_BASE_URL):
        self.base_url = str(base_url or DEFAULT_BASE_URL).rstrip("/")
        self._session = requests.Session()
        self._session.trust_env = False

    def request(self, method, path, *, params=None, payload=None, timeout=120):
        url = f"{self.base_url}{path}"
        response = self._session.request(
            method,
            url,
            params=params,
            json=payload,
            timeout=timeout,
            proxies={"http": None, "https": None},
            headers={"Content-Type": "application/json"},
        )
        response.raise_for_status()
        return response.json()

    def health(self):
        return self.request("GET", "/api/health", timeout=30)

    def health_details(self):
        return self.request("GET", "/api/health/details", timeout=30)

    def list_namespaces(self, **params):
        return self.request("GET", "/api/k8s/namespaces", params=params, timeout=60)

    def list_pods(self, **params):
        return self.request("GET", "/api/k8s/pods", params=params, timeout=60)

    def list_abnormal_pods(self, **params):
        return self.request("GET", "/api/k8s/pods/abnormal", params=params, timeout=60)

    def list_workloads(self, **params):
        return self.request("GET", "/api/k8s/workloads", params=params, timeout=60)

    def list_services(self, **params):
        return self.request("GET", "/api/k8s/services", params=params, timeout=60)

    def search_k8s_resources(self, **params):
        return self.request("GET", "/api/k8s/resources/search", params=params, timeout=60)

    def count_k8s_resources(self, **params):
        return self.request("GET", "/api/k8s/resources/count", params=params, timeout=90)

    def cluster_overview(self, **params):
        return self.request("GET", "/api/k8s/cluster/overview", params=params, timeout=90)

    def search_logs(self, *, namespace="", pod="", workload_name="", q="", size=20, range_hours=6):
        return self.request(
            "GET",
            "/api/search/logs",
            params={
                "namespace": namespace,
                "pod": pod,
                "workload_name": workload_name,
                "q": q,
                "size": size,
                "range_hours": range_hours,
            },
        )

    def search_business_logs(self, **params):
        return self.request("GET", "/api/search/business-logs", params=params, timeout=60)

    def search_traces(self, **params):
        return self.request("GET", "/api/traces/search", params=params, timeout=60)

    def correlate_business_context(self, **params):
        return self.request("GET", "/api/business/correlate", params=params, timeout=90)

    def context_pack(self, target_type, **params):
        target_type = str(target_type or "").strip().lower()
        return self.request("GET", f"/api/context/{target_type}", params=params, timeout=120)

    def context_pack_resource(self, target_type, **params):
        target_type = str(target_type or "").strip().lower()
        url = f"{self.base_url}/api/context/{target_type}"
        response = self._session.get(
            url,
            params=params,
            timeout=120,
            proxies={"http": None, "https": None},
            headers={"Content-Type": "application/json"},
        )
        try:
            payload = response.json()
        except ValueError:
            response.raise_for_status()
            return {}
        if response.status_code >= 400 and not isinstance(payload, dict):
            response.raise_for_status()
        return payload

    def snapshot_index(self, **params):
        return self.request("GET", "/api/snapshot-index", params=params, timeout=120)

    def release_for_workload(self, **params):
        return self.request("GET", "/api/releases/workload", params=params, timeout=60)

    def release_recent_changes(self, **params):
        return self.request("GET", "/api/releases/recent-changes", params=params, timeout=60)

    def correlate_change_with_incident(self, **params):
        return self.request("GET", "/api/releases/correlate", params=params, timeout=90)

    def argocd_app_status(self, **params):
        return self.request("GET", "/api/argocd/app-status", params=params, timeout=60)

    def argocd_app_history(self, **params):
        return self.request("GET", "/api/argocd/app-history", params=params, timeout=60)

    def argocd_diff_summary(self, **params):
        return self.request("GET", "/api/argocd/diff-summary", params=params, timeout=60)

    def gitlab_recent_commits(self, **params):
        return self.request("GET", "/api/gitlab/recent-commits", params=params, timeout=60)

    def gitlab_commit_detail(self, **params):
        return self.request("GET", "/api/gitlab/commit-detail", params=params, timeout=60)

    def gitlab_pipeline_status(self, **params):
        return self.request("GET", "/api/gitlab/pipeline-status", params=params, timeout=60)

    def gitlab_release_context(self, **params):
        return self.request("GET", "/api/gitlab/release-context", params=params, timeout=90)

    def gitlab_merge_requests(self, **params):
        return self.request("GET", "/api/gitlab/merge-requests", params=params, timeout=60)

    def gitlab_tags(self, **params):
        return self.request("GET", "/api/gitlab/tags", params=params, timeout=60)

    def gitlab_artifacts(self, **params):
        return self.request("GET", "/api/gitlab/artifacts", params=params, timeout=60)

    def gitlab_image_digest_context(self, **params):
        return self.request("GET", "/api/gitlab/image-digest-context", params=params, timeout=90)

    def service_red_metrics(self, **params):
        return self.request("GET", "/api/observability/service-red-metrics", params=params, timeout=60)

    def runtime_events_context(self, **params):
        return self.request("GET", "/api/observability/runtime-events", params=params, timeout=60)

    def profile_hotspots(self, **params):
        return self.request("GET", "/api/observability/profile-hotspots", params=params, timeout=90)

    def search_events(self, *, namespace="", pod="", q="", size=20, range_hours=6):
        return self.request(
            "GET",
            "/api/search/events",
            params={
                "namespace": namespace,
                "pod": pod,
                "q": q,
                "size": size,
                "range_hours": range_hours,
            },
        )

    def investigate(
        self,
        *,
        namespace,
        pod="",
        workload_name="",
        question="",
        query="",
        range_hours=6,
        use_ai=False,
    ):
        return self.request(
            "POST",
            "/api/investigate",
            payload={
                "namespace": namespace,
                "pod": pod,
                "workload_name": workload_name,
                "question": question,
                "query": query,
                "range_hours": range_hours,
                "use_ai": use_ai,
            },
        )

    def list_investigations(self, *, limit=10):
        return self.request("GET", "/api/investigations", params={"limit": limit}, timeout=30)

    def list_targets(self, *, limit=10):
        return self.request("GET", "/api/investigation-targets", params={"limit": limit}, timeout=30)

    def get_investigation(self, investigation_id):
        return self.request("GET", f"/api/investigations/{investigation_id}", timeout=30)

    def list_incidents(self, *, limit=10):
        return self.request("GET", "/api/incidents/list", params={"limit": limit}, timeout=30)

    def search_incidents(self, *, q="", namespace="", pod="", limit=20):
        return self.request(
            "GET",
            "/api/incidents/search",
            params={
                "q": q,
                "namespace": namespace,
                "pod": pod,
                "limit": limit,
            },
            timeout=30,
        )

    def resources(self, *, range_hours=24):
        return self.request(
            "GET",
            "/api/resources",
            params={
                "range_hours": range_hours,
            },
            timeout=60,
        )
