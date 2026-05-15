from __future__ import annotations

import json
import os
import ssl
import subprocess
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any


DEFAULT_TAIL_LINES = 300
DEFAULT_SINCE_SECONDS = 1800
DEFAULT_LIMIT_BYTES = 1024 * 1024
MAX_TAIL_LINES = 1000
MAX_SINCE_SECONDS = 86400
MAX_LIMIT_BYTES = 5 * 1024 * 1024


class K8sError(RuntimeError):
    pass


@dataclass
class LogRequest:
    namespace: str
    pod: str
    container: str = ""
    tail_lines: int = DEFAULT_TAIL_LINES
    since_seconds: int = DEFAULT_SINCE_SECONDS
    limit_bytes: int = DEFAULT_LIMIT_BYTES
    previous: bool = False
    timestamps: bool = False


class K8sClient:
    def __init__(self, kubectl: str | None = None) -> None:
        self.kubectl = kubectl or os.environ.get("OPSPILOT_KUBECTL", "kubectl")
        self.token_path = os.environ.get(
            "OPSPILOT_SERVICEACCOUNT_TOKEN",
            "/var/run/secrets/kubernetes.io/serviceaccount/token",
        )
        self.ca_path = os.environ.get(
            "OPSPILOT_SERVICEACCOUNT_CA",
            "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
        )
        self.host = os.environ.get("KUBERNETES_SERVICE_HOST", "")
        self.port = os.environ.get("KUBERNETES_SERVICE_PORT", "443")
        self.mode = "in-cluster" if self.host and os.path.exists(self.token_path) else "kubectl"

    def health(self) -> dict[str, Any]:
        return {
            "mode": self.mode,
            "kubectl": self.kubectl if self.mode == "kubectl" else None,
            "in_cluster_host": self.host if self.mode == "in-cluster" else None,
        }

    def inventory_overview(self, limit: int = 10) -> dict[str, Any]:
        warnings: list[str] = []
        result: dict[str, Any] = {
            "clusters": [],
            "counts": {},
            "top_abnormal_pods": [],
        }
        for name, path, args in [
            ("namespace_count", "/api/v1/namespaces", ["get", "namespaces", "-o", "json"]),
            ("node_count", "/api/v1/nodes", ["get", "nodes", "-o", "json"]),
            ("pod_count", "/api/v1/pods", ["get", "pods", "-A", "-o", "json"]),
            ("service_count", "/api/v1/services", ["get", "services", "-A", "-o", "json"]),
            ("deployment_count", "/apis/apps/v1/deployments", ["get", "deployments", "-A", "-o", "json"]),
            ("statefulset_count", "/apis/apps/v1/statefulsets", ["get", "statefulsets", "-A", "-o", "json"]),
            ("daemonset_count", "/apis/apps/v1/daemonsets", ["get", "daemonsets", "-A", "-o", "json"]),
        ]:
            try:
                payload = self._json(path, args)
                result["counts"][name] = len(payload.get("items", []))
            except Exception as exc:  # noqa: BLE001
                result["counts"][name] = None
                warnings.append(f"{name}: {exc}")
        try:
            pods = self.list_pods(status="abnormal", limit=limit)
            result["counts"]["abnormal_pod_count"] = pods["item_count"]
            result["top_abnormal_pods"] = pods["items"]
        except Exception as exc:  # noqa: BLE001
            result["counts"]["abnormal_pod_count"] = None
            warnings.append(f"abnormal_pods: {exc}")
        result["warnings"] = warnings
        return result

    def list_pods(
        self,
        namespace: str = "",
        status: str = "",
        q: str = "",
        limit: int = 100,
    ) -> dict[str, Any]:
        path = f"/api/v1/namespaces/{quote(namespace)}/pods" if namespace else "/api/v1/pods"
        args = ["get", "pods"]
        if namespace:
            args += ["-n", namespace]
        else:
            args.append("-A")
        args += ["-o", "json"]
        payload = self._json(path, args)
        items = [_pod_summary(item) for item in payload.get("items", [])]
        if status:
            items = [item for item in items if _matches_status(item, status)]
        if q:
            items = [item for item in items if _matches_query(item, q)]
        total = len(items)
        return {
            "items": items[: max(0, limit)],
            "item_count": min(total, max(0, limit)),
            "total_count": total,
            "truncated": total > limit,
        }

    def get_pod(self, namespace: str, pod: str) -> dict[str, Any]:
        if not namespace or not pod:
            raise K8sError("namespace and pod are required")
        path = f"/api/v1/namespaces/{quote(namespace)}/pods/{quote(pod)}"
        args = ["get", "pod", pod, "-n", namespace, "-o", "json"]
        return self._json(path, args)

    def list_events(self, namespace: str, involved_name: str = "", limit: int = 50) -> dict[str, Any]:
        if not namespace:
            raise K8sError("namespace is required")
        query = ""
        args = ["get", "events", "-n", namespace, "-o", "json"]
        if involved_name:
            selector = f"involvedObject.name={involved_name}"
            query = "?" + urllib.parse.urlencode({"fieldSelector": selector})
            args = ["get", "events", "-n", namespace, "--field-selector", selector, "-o", "json"]
        payload = self._json(f"/api/v1/namespaces/{quote(namespace)}/events{query}", args)
        events = [_event_summary(item) for item in payload.get("items", [])]
        events.sort(key=lambda item: item.get("last_timestamp") or item.get("event_time") or "")
        events.reverse()
        return {
            "items": events[: max(0, limit)],
            "item_count": min(len(events), max(0, limit)),
            "total_count": len(events),
            "truncated": len(events) > limit,
        }

    def read_pod_log(self, request: LogRequest) -> dict[str, Any]:
        request = _bounded_log_request(request)
        if not request.namespace or not request.pod:
            raise K8sError("namespace and pod are required")
        if self.mode == "in-cluster":
            params: dict[str, Any] = {
                "tailLines": request.tail_lines,
                "sinceSeconds": request.since_seconds,
                "limitBytes": request.limit_bytes,
                "previous": str(request.previous).lower(),
                "timestamps": str(request.timestamps).lower(),
            }
            if request.container:
                params["container"] = request.container
            query = urllib.parse.urlencode(params)
            text = self._raw(
                f"/api/v1/namespaces/{quote(request.namespace)}/pods/{quote(request.pod)}/log?{query}"
            )
        else:
            args = [
                "logs",
                "-n",
                request.namespace,
                request.pod,
                f"--tail={request.tail_lines}",
                f"--since={request.since_seconds}s",
                f"--limit-bytes={request.limit_bytes}",
            ]
            if request.container:
                args += ["-c", request.container]
            if request.previous:
                args.append("--previous")
            if request.timestamps:
                args.append("--timestamps")
            text = self._kubectl_text(args)
        truncated = False
        if len(text.encode("utf-8", errors="replace")) > request.limit_bytes:
            encoded = text.encode("utf-8", errors="replace")[: request.limit_bytes]
            text = encoded.decode("utf-8", errors="replace")
            truncated = True
        return {
            "namespace": request.namespace,
            "pod": request.pod,
            "container": request.container,
            "previous": request.previous,
            "tail_lines": request.tail_lines,
            "since_seconds": request.since_seconds,
            "limit_bytes": request.limit_bytes,
            "truncated": truncated,
            "text": text,
        }

    def pod_context(self, namespace: str, pod: str) -> dict[str, Any]:
        warnings: list[str] = []
        raw_pod = self.get_pod(namespace, pod)
        summary = _pod_summary(raw_pod)
        events: dict[str, Any] = {"items": [], "item_count": 0, "total_count": 0, "truncated": False}
        logs: list[dict[str, Any]] = []
        try:
            events = self.list_events(namespace, pod, limit=20)
        except Exception as exc:  # noqa: BLE001
            warnings.append(f"events: {exc}")
        containers = summary.get("containers") or []
        selected_container = containers[0].get("name", "") if containers else ""
        for previous in [False, True] if summary.get("restart_count", 0) > 0 else [False]:
            try:
                logs.append(
                    self.read_pod_log(
                        LogRequest(
                            namespace=namespace,
                            pod=pod,
                            container=selected_container,
                            tail_lines=120,
                            since_seconds=1800,
                            limit_bytes=256 * 1024,
                            previous=previous,
                        )
                    )
                )
            except Exception as exc:  # noqa: BLE001
                warnings.append(f"logs previous={previous}: {exc}")
        return {
            "target": {
                "type": "pod",
                "namespace": namespace,
                "name": pod,
                "cluster": os.environ.get("OPSPILOT_CLUSTER", "default"),
            },
            "summary": summary,
            "evidence": {
                "inventory": {"pod": summary},
                "metrics": [],
                "events": events.get("items", []),
                "logs": logs,
                "release": {},
            },
            "warnings": warnings,
        }

    def diagnose_pod(self, namespace: str, pod: str) -> dict[str, Any]:
        context = self.pod_context(namespace, pod)
        summary = context.get("summary", {})
        findings: list[str] = []
        if summary.get("waiting_reasons"):
            findings.append("Pod has waiting containers: " + ", ".join(summary["waiting_reasons"]))
        if summary.get("restart_count", 0) > 0:
            findings.append(f"Pod has container restarts: {summary['restart_count']}")
        if not summary.get("ready"):
            findings.append("Pod is not ready")
        if not findings:
            findings.append("No obvious pod-level failure signal found in MVP evidence")
        context["diagnosis"] = {
            "findings": findings,
            "confidence": "low" if len(findings) == 1 and findings[0].startswith("No obvious") else "medium",
            "next_steps": [
                "Review Kubernetes events",
                "Review current and previous short-window pod logs",
                "Check Prometheus metrics once metrics adapter is enabled",
            ],
        }
        return context

    def _json(self, path: str, kubectl_args: list[str]) -> dict[str, Any]:
        if self.mode == "in-cluster":
            return json.loads(self._raw(path) or "{}")
        text = self._kubectl_text(kubectl_args)
        return json.loads(text or "{}")

    def _raw(self, path: str) -> str:
        token = _read_file(self.token_path).strip()
        if not token:
            raise K8sError("service account token is empty")
        url = f"https://{self.host}:{self.port}{path}"
        context = ssl.create_default_context(cafile=self.ca_path if os.path.exists(self.ca_path) else None)
        req = urllib.request.Request(url, headers={"Authorization": f"Bearer {token}"})
        try:
            with urllib.request.urlopen(req, context=context, timeout=10) as resp:
                return resp.read().decode("utf-8", errors="replace")
        except urllib.error.HTTPError as exc:
            detail = exc.read().decode("utf-8", errors="replace")
            raise K8sError(f"kubernetes api {exc.code}: {detail[:500]}") from exc
        except urllib.error.URLError as exc:
            raise K8sError(f"kubernetes api error: {exc}") from exc

    def _kubectl_text(self, args: list[str]) -> str:
        cmd = [self.kubectl, *args]
        try:
            proc = subprocess.run(cmd, check=False, capture_output=True, text=True, timeout=20)
        except FileNotFoundError as exc:
            raise K8sError(f"kubectl not found: {self.kubectl}") from exc
        except subprocess.TimeoutExpired as exc:
            raise K8sError(f"kubectl timeout: {' '.join(cmd)}") from exc
        if proc.returncode != 0:
            message = (proc.stderr or proc.stdout or "").strip()
            raise K8sError(f"kubectl failed: {message}")
        return proc.stdout


def quote(value: str) -> str:
    return urllib.parse.quote(value, safe="")


def _read_file(path: str) -> str:
    with open(path, "r", encoding="utf-8") as fh:
        return fh.read()


def _bounded_log_request(request: LogRequest) -> LogRequest:
    request.tail_lines = min(max(int(request.tail_lines or DEFAULT_TAIL_LINES), 1), MAX_TAIL_LINES)
    request.since_seconds = min(max(int(request.since_seconds or DEFAULT_SINCE_SECONDS), 1), MAX_SINCE_SECONDS)
    request.limit_bytes = min(max(int(request.limit_bytes or DEFAULT_LIMIT_BYTES), 1), MAX_LIMIT_BYTES)
    return request


def _pod_summary(item: dict[str, Any]) -> dict[str, Any]:
    meta = item.get("metadata", {})
    status = item.get("status", {})
    spec = item.get("spec", {})
    container_statuses = status.get("containerStatuses", []) or []
    conditions = {cond.get("type"): cond.get("status") for cond in status.get("conditions", []) or []}
    containers = []
    waiting_reasons: list[str] = []
    restart_count = 0
    for container in container_statuses:
        state = container.get("state", {}) or {}
        waiting = state.get("waiting") or {}
        if waiting.get("reason"):
            waiting_reasons.append(waiting["reason"])
        restart_count += int(container.get("restartCount") or 0)
        containers.append(
            {
                "name": container.get("name", ""),
                "ready": bool(container.get("ready")),
                "restart_count": int(container.get("restartCount") or 0),
                "image": container.get("image", ""),
                "image_id": container.get("imageID", ""),
                "state": next(iter(state.keys()), "") if state else "",
                "waiting_reason": waiting.get("reason", ""),
            }
        )
    owners = meta.get("ownerReferences", []) or []
    owner = owners[0] if owners else {}
    ready = conditions.get("Ready") == "True"
    phase = status.get("phase", "")
    return {
        "namespace": meta.get("namespace", ""),
        "name": meta.get("name", ""),
        "phase": phase,
        "ready": ready,
        "status": "Ready" if ready and phase == "Running" else phase or "Unknown",
        "node": spec.get("nodeName", ""),
        "pod_ip": status.get("podIP", ""),
        "host_ip": status.get("hostIP", ""),
        "restart_count": restart_count,
        "waiting_reasons": waiting_reasons,
        "owner_kind": owner.get("kind", ""),
        "owner_name": owner.get("name", ""),
        "labels": meta.get("labels", {}) or {},
        "containers": containers,
        "start_time": status.get("startTime", ""),
    }


def _event_summary(item: dict[str, Any]) -> dict[str, Any]:
    involved = item.get("involvedObject", {}) or {}
    return {
        "namespace": item.get("metadata", {}).get("namespace", ""),
        "name": item.get("metadata", {}).get("name", ""),
        "type": item.get("type", ""),
        "reason": item.get("reason", ""),
        "message": item.get("message", ""),
        "involved_kind": involved.get("kind", ""),
        "involved_name": involved.get("name", ""),
        "count": item.get("count", 0),
        "first_timestamp": item.get("firstTimestamp", ""),
        "last_timestamp": item.get("lastTimestamp", ""),
        "event_time": item.get("eventTime", ""),
    }


def _matches_status(item: dict[str, Any], status: str) -> bool:
    wanted = status.lower()
    phase = str(item.get("phase", "")).lower()
    ready = bool(item.get("ready"))
    waiting = [str(x).lower() for x in item.get("waiting_reasons", [])]
    if wanted in {"all", "*"}:
        return True
    if wanted == "running":
        return phase == "running"
    if wanted == "pending":
        return phase == "pending"
    if wanted == "failed":
        return phase == "failed"
    if wanted in {"not_ready", "not-ready"}:
        return not ready
    if wanted == "abnormal":
        return phase not in {"running", "succeeded"} or not ready or bool(waiting)
    if wanted == "crashloop":
        return any("crashloop" in reason for reason in waiting)
    if wanted == "imagepull":
        return any("imagepull" in reason or "errimagepull" in reason for reason in waiting)
    return wanted in phase or any(wanted in reason for reason in waiting)


def _matches_query(item: dict[str, Any], query: str) -> bool:
    haystack = json.dumps(item, ensure_ascii=False).lower()
    return all(token in haystack for token in query.lower().split())
