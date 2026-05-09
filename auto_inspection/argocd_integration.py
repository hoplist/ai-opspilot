#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import os
import time
from urllib.parse import quote

import requests


DEFAULT_TIMEOUT = int(os.environ.get("ARGOCD_TIMEOUT", "20") or 20)


def _env(name, default=""):
    return str(os.environ.get(name, default) or "").strip()


def _config():
    server = _env("ARGOCD_SERVER") or _env("ARGOCD_BASE_URL")
    token = _env("ARGOCD_TOKEN") or _env("ARGOCD_AUTH_TOKEN")
    verify_tls = _env("ARGOCD_VERIFY_TLS", "false").lower() in {"1", "true", "yes", "on"}
    return {
        "server": server.rstrip("/"),
        "token": token,
        "token_configured": bool(token),
        "verify_tls": verify_tls,
        "timeout": DEFAULT_TIMEOUT,
    }


def _safety():
    return {
        "server_commands": "not_allowed",
        "kubernetes_mutations": "not_allowed",
        "argocd_mutations": "not_allowed",
    }


def _not_configured(mode, request):
    cfg = _config()
    return {
        "mode": mode,
        "configured": False,
        "safety": _safety(),
        "request": request,
        "config": {
            "server": cfg["server"],
            "token_configured": cfg["token_configured"],
            "verify_tls": cfg["verify_tls"],
        },
        "errors": [
            {
                "source": "argocd",
                "message": "ARGOCD_SERVER and ARGOCD_TOKEN are required for Argo CD API queries.",
            }
        ],
    }


def _request(path, params=None):
    cfg = _config()
    if not cfg["server"] or not cfg["token"]:
        raise RuntimeError("ARGOCD_SERVER and ARGOCD_TOKEN are required")
    session = requests.Session()
    session.trust_env = False
    response = session.get(
        f"{cfg['server']}{path}",
        params={key: value for key, value in (params or {}).items() if value not in ("", None)},
        timeout=cfg["timeout"],
        verify=cfg["verify_tls"],
        headers={"Authorization": f"Bearer {cfg['token']}", "Accept": "application/json"},
        proxies={"http": None, "https": None},
    )
    response.raise_for_status()
    return response.json() if response.text else {}


def _app_name(params):
    data = params if isinstance(params, dict) else {}
    return str(data.get("app_name") or data.get("application") or data.get("name") or "").strip()


def _summary(app):
    metadata = app.get("metadata") or {}
    spec = app.get("spec") or {}
    status = app.get("status") or {}
    sync = status.get("sync") or {}
    health = status.get("health") or {}
    operation = status.get("operationState") or {}
    source = spec.get("source") or {}
    destination = spec.get("destination") or {}
    return {
        "name": metadata.get("name"),
        "namespace": metadata.get("namespace"),
        "project": spec.get("project"),
        "repo_url": source.get("repoURL"),
        "path": source.get("path"),
        "target_revision": source.get("targetRevision"),
        "destination": {
            "server": destination.get("server"),
            "namespace": destination.get("namespace"),
        },
        "sync": {
            "status": sync.get("status"),
            "revision": sync.get("revision"),
            "compared_to": sync.get("comparedTo"),
        },
        "health": {"status": health.get("status"), "message": health.get("message")},
        "operation": {
            "phase": operation.get("phase"),
            "message": operation.get("message"),
            "started_at": operation.get("startedAt"),
            "finished_at": operation.get("finishedAt"),
        },
        "images": (status.get("summary") or {}).get("images") or [],
        "resources": status.get("resources") or [],
    }


def app_status(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_argocd_app_status"
    cfg = _config()
    if not cfg["server"] or not cfg["token"]:
        return _not_configured(mode, data)

    started = time.time()
    name = _app_name(data)
    if name:
        app = _request(f"/api/v1/applications/{quote(name, safe='')}", {"refresh": data.get("refresh", "")})
        applications = [_summary(app)]
    else:
        payload = _request("/api/v1/applications")
        applications = [_summary(item) for item in payload.get("items", [])]
    return {
        "mode": mode,
        "configured": True,
        "safety": _safety(),
        "request": data,
        "applications": applications,
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3), "item_count": len(applications)},
    }


def app_history(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_argocd_app_history"
    name = _app_name(data)
    if not name:
        raise ValueError("app_name is required")
    cfg = _config()
    if not cfg["server"] or not cfg["token"]:
        return _not_configured(mode, data)

    started = time.time()
    app = _request(f"/api/v1/applications/{quote(name, safe='')}")
    status = app.get("status") or {}
    history = status.get("history") or []
    try:
        limit = max(1, min(int(data.get("limit") or 20), 100))
    except (TypeError, ValueError):
        limit = 20
    items = list(reversed(history))[:limit]
    latest = (items[0] or {}).get("revision") if items else (status.get("sync") or {}).get("revision")
    return {
        "mode": mode,
        "configured": True,
        "safety": _safety(),
        "request": data,
        "application": _summary(app),
        "history": items,
        "latest_revision": latest,
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3), "item_count": len(items)},
    }


def diff_summary(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_argocd_diff_summary"
    name = _app_name(data)
    if not name:
        raise ValueError("app_name is required")
    cfg = _config()
    if not cfg["server"] or not cfg["token"]:
        return _not_configured(mode, data)

    started = time.time()
    app = _request(f"/api/v1/applications/{quote(name, safe='')}", {"refresh": data.get("refresh", "")})
    summary = _summary(app)
    resources = summary.get("resources") or []
    counts = {}
    for item in resources:
        status = str(item.get("status") or "Unknown")
        counts[status] = counts.get(status, 0) + 1
    return {
        "mode": mode,
        "configured": True,
        "safety": _safety(),
        "request": data,
        "application": summary,
        "sync_status": (summary.get("sync") or {}).get("status"),
        "git_revision": (summary.get("sync") or {}).get("revision"),
        "resource_status_counts": counts,
        "out_of_sync_resources": [
            item for item in resources if str(item.get("status") or "").lower() == "outofsync"
        ],
        "notes": [
            "This is a read-only diff summary based on Argo CD application resource status.",
            "It does not execute sync, rollback, refresh mutation, or kubectl operations.",
        ],
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3)},
    }
