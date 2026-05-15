from __future__ import annotations

import argparse
import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any
from urllib.parse import parse_qs, urlparse

from opspilot import __version__
from opspilot.core.k8s import K8sClient, K8sError, LogRequest
from opspilot.core.response import envelope, error_envelope


def make_handler(k8s: K8sClient) -> type[BaseHTTPRequestHandler]:
    class OpsPilotHandler(BaseHTTPRequestHandler):
        server_version = f"opspilot-core/{__version__}"

        def do_GET(self) -> None:  # noqa: N802
            parsed = urlparse(self.path)
            query = {key: values[-1] for key, values in parse_qs(parsed.query).items()}
            try:
                status, body = route_get(parsed.path, query, k8s)
            except K8sError as exc:
                status, body = error_envelope("K8S_ERROR", str(exc), 502)
            except ValueError as exc:
                status, body = error_envelope("BAD_REQUEST", str(exc), 400)
            except Exception as exc:  # noqa: BLE001
                status, body = error_envelope("INTERNAL_ERROR", str(exc), 500)
            self._send_json(status, body)

        def log_message(self, fmt: str, *args: Any) -> None:
            if os.environ.get("OPSPILOT_ACCESS_LOG", "1") != "0":
                super().log_message(fmt, *args)

        def _send_json(self, status: int, body: dict[str, Any]) -> None:
            data = json.dumps(body, ensure_ascii=False, indent=2).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

    return OpsPilotHandler


def route_get(path: str, query: dict[str, str], k8s: K8sClient) -> tuple[int, dict[str, Any]]:
    if path == "/api/health":
        return 200, envelope({"version": __version__, "kubernetes": k8s.health()})
    if path == "/api/inventory/overview":
        return 200, envelope(k8s.inventory_overview(limit=_int(query, "limit", 10)))
    if path == "/api/k8s/pods":
        return 200, envelope(
            k8s.list_pods(
                namespace=query.get("namespace", ""),
                status=query.get("status", ""),
                q=query.get("q", ""),
                limit=_int(query, "limit", 100),
            )
        )
    if path == "/api/k8s/logs/pod":
        req = LogRequest(
            namespace=_required(query, "namespace"),
            pod=_required(query, "pod"),
            container=query.get("container", ""),
            tail_lines=_int(query, "tail_lines", _int(query, "tail", 300)),
            since_seconds=_int(query, "since_seconds", _int(query, "since", 1800)),
            limit_bytes=_int(query, "limit_bytes", 1024 * 1024),
            previous=_bool(query.get("previous", "false")),
            timestamps=_bool(query.get("timestamps", "false")),
        )
        return 200, envelope(k8s.read_pod_log(req))
    if path == "/api/context/pod":
        return 200, envelope(k8s.pod_context(_required(query, "namespace"), _required(query, "pod")))
    if path == "/api/diagnose/pod":
        return 200, envelope(k8s.diagnose_pod(_required(query, "namespace"), _required(query, "pod")))
    status, body = error_envelope("NOT_FOUND", f"unknown endpoint: {path}", 404)
    return status, body


def _required(query: dict[str, str], name: str) -> str:
    value = query.get(name, "").strip()
    if not value:
        raise ValueError(f"{name} is required")
    return value


def _int(query: dict[str, str], name: str, default: int) -> int:
    value = query.get(name)
    if value in {None, ""}:
        return default
    try:
        return int(str(value))
    except ValueError as exc:
        raise ValueError(f"{name} must be an integer") from exc


def _bool(value: str) -> bool:
    return str(value).lower() in {"1", "true", "yes", "y", "on"}


def serve(host: str, port: int) -> None:
    k8s = K8sClient()
    httpd = ThreadingHTTPServer((host, port), make_handler(k8s))
    print(f"opspilot-core listening on http://{host}:{port}")
    httpd.serve_forever()


def main() -> None:
    parser = argparse.ArgumentParser(description="Run OpsPilot Core API")
    parser.add_argument("--host", default=os.environ.get("OPSPILOT_HOST", "0.0.0.0"))
    parser.add_argument("--port", type=int, default=int(os.environ.get("OPSPILOT_PORT", "18080")))
    args = parser.parse_args()
    serve(args.host, args.port)
