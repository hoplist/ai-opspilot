#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import os
import time
from urllib.parse import quote, urlparse

import requests

from auto_inspection import argocd_integration
from auto_inspection import release_changes


DEFAULT_TIMEOUT = int(os.environ.get("GITLAB_TIMEOUT", "20") or 20)
DEFAULT_URL = "http://192.168.48.206:8929"
DEFAULT_PROJECT = "platform/gitops-manifests"


def _env(name, default=""):
    return str(os.environ.get(name, default) or "").strip()


def _config():
    url = _env("GITLAB_URL") or _env("GITLAB_BASE_URL") or DEFAULT_URL
    token = _env("GITLAB_TOKEN") or _env("GITLAB_PRIVATE_TOKEN")
    project = _env("GITLAB_PROJECT_ID") or _env("GITLAB_PROJECT_PATH") or DEFAULT_PROJECT
    verify_tls = _env("GITLAB_VERIFY_TLS", "false").lower() in {"1", "true", "yes", "on"}
    return {
        "url": url.rstrip("/"),
        "token": token,
        "token_configured": bool(token),
        "project": project,
        "verify_tls": verify_tls,
        "timeout": DEFAULT_TIMEOUT,
    }


def _safety():
    return {
        "server_commands": "not_allowed",
        "kubernetes_mutations": "not_allowed",
        "gitlab_mutations": "not_allowed",
    }


def _project(params):
    data = params if isinstance(params, dict) else {}
    return str(data.get("project_id") or data.get("project") or data.get("project_path") or "").strip() or _config()["project"]


def _not_configured(mode, request):
    cfg = _config()
    return {
        "mode": mode,
        "configured": False,
        "safety": _safety(),
        "request": request,
        "config": {
            "url": cfg["url"],
            "project": cfg["project"],
            "token_configured": cfg["token_configured"],
            "verify_tls": cfg["verify_tls"],
        },
        "errors": [
            {
                "source": "gitlab",
                "message": "GITLAB_TOKEN or GITLAB_PRIVATE_TOKEN is required for GitLab API queries.",
            }
        ],
    }


def _request(path, params=None):
    cfg = _config()
    if not cfg["url"] or not cfg["token"]:
        raise RuntimeError("GITLAB_URL and GITLAB_TOKEN are required")
    session = requests.Session()
    session.trust_env = False
    response = session.get(
        f"{cfg['url']}{path}",
        params={key: value for key, value in (params or {}).items() if value not in ("", None)},
        timeout=cfg["timeout"],
        verify=cfg["verify_tls"],
        headers={"PRIVATE-TOKEN": cfg["token"], "Accept": "application/json"},
        proxies={"http": None, "https": None},
    )
    response.raise_for_status()
    return response.json() if response.text else {}


def _project_path(project):
    return f"/api/v4/projects/{quote(str(project or '').strip(), safe='')}"


def _safe_int(value, default, minimum=1, maximum=100):
    try:
        number = int(value)
    except (TypeError, ValueError):
        number = default
    return max(minimum, min(number, maximum))


def _commit_summary(item):
    return {
        "id": item.get("id"),
        "short_id": item.get("short_id"),
        "title": item.get("title"),
        "message": item.get("message"),
        "author_name": item.get("author_name"),
        "author_email": item.get("author_email"),
        "authored_date": item.get("authored_date"),
        "committer_name": item.get("committer_name"),
        "committer_email": item.get("committer_email"),
        "committed_date": item.get("committed_date"),
        "created_at": item.get("created_at"),
        "web_url": item.get("web_url"),
        "parent_ids": item.get("parent_ids") or [],
    }


def _pipeline_summary(item):
    return {
        "id": item.get("id"),
        "iid": item.get("iid"),
        "project_id": item.get("project_id"),
        "sha": item.get("sha"),
        "ref": item.get("ref"),
        "status": item.get("status"),
        "source": item.get("source"),
        "created_at": item.get("created_at"),
        "updated_at": item.get("updated_at"),
        "web_url": item.get("web_url"),
    }


def _merge_request_summary(item):
    return {
        "id": item.get("id"),
        "iid": item.get("iid"),
        "project_id": item.get("project_id"),
        "title": item.get("title"),
        "state": item.get("state"),
        "merge_status": item.get("merge_status"),
        "detailed_merge_status": item.get("detailed_merge_status"),
        "source_branch": item.get("source_branch"),
        "target_branch": item.get("target_branch"),
        "sha": item.get("sha"),
        "merge_commit_sha": item.get("merge_commit_sha"),
        "squash_commit_sha": item.get("squash_commit_sha"),
        "author": (item.get("author") or {}).get("username"),
        "created_at": item.get("created_at"),
        "updated_at": item.get("updated_at"),
        "merged_at": item.get("merged_at"),
        "web_url": item.get("web_url"),
    }


def _tag_summary(item):
    commit = item.get("commit") or {}
    return {
        "name": item.get("name"),
        "target": item.get("target"),
        "message": item.get("message"),
        "protected": item.get("protected"),
        "created_at": item.get("created_at"),
        "release": item.get("release"),
        "commit": _commit_summary(commit) if commit else None,
    }


def _job_summary(item):
    artifacts = item.get("artifacts") or []
    artifact_file = item.get("artifacts_file") or {}
    return {
        "id": item.get("id"),
        "name": item.get("name"),
        "stage": item.get("stage"),
        "status": item.get("status"),
        "ref": item.get("ref"),
        "tag": item.get("tag"),
        "allow_failure": item.get("allow_failure"),
        "created_at": item.get("created_at"),
        "started_at": item.get("started_at"),
        "finished_at": item.get("finished_at"),
        "duration": item.get("duration"),
        "queued_duration": item.get("queued_duration"),
        "web_url": item.get("web_url"),
        "commit": _commit_summary(item.get("commit") or {}) if item.get("commit") else None,
        "pipeline": item.get("pipeline"),
        "artifact_file": {
            "filename": artifact_file.get("filename"),
            "size": artifact_file.get("size"),
        },
        "artifacts": [
            {
                "file_type": artifact.get("file_type"),
                "filename": artifact.get("filename"),
                "size": artifact.get("size"),
            }
            for artifact in artifacts
        ],
    }


def _registry_repository_summary(item):
    return {
        "id": item.get("id"),
        "name": item.get("name"),
        "path": item.get("path"),
        "location": item.get("location"),
        "created_at": item.get("created_at"),
        "cleanup_policy_started_at": item.get("cleanup_policy_started_at"),
    }


def _registry_tag_summary(item):
    return {
        "name": item.get("name"),
        "path": item.get("path"),
        "location": item.get("location"),
        "revision": item.get("revision"),
        "digest": item.get("digest"),
        "short_revision": item.get("short_revision"),
        "created_at": item.get("created_at"),
        "total_size": item.get("total_size"),
    }


def _image_parts(image):
    text = str(image or "").strip()
    if not text:
        return {"image": "", "repository": "", "tag": "", "digest": ""}
    digest = ""
    base = text
    if "@" in base:
        base, digest = base.rsplit("@", 1)
    tag = ""
    last_slash = base.rfind("/")
    last_colon = base.rfind(":")
    if last_colon > last_slash:
        tag = base[last_colon + 1 :]
        repository = base[:last_colon]
    else:
        repository = base
    return {"image": text, "repository": repository, "tag": tag or "latest", "digest": digest}


def _repo_tail(value):
    text = str(value or "").strip().lower()
    if not text:
        return ""
    parsed = urlparse(text if "://" in text else f"//{text}")
    path = parsed.path or text
    return path.strip("/").lower()


def _tag_matches_image(repository, image):
    repo_tail = _repo_tail(repository.get("location") or repository.get("path") or repository.get("name"))
    image_tail = _repo_tail((image or {}).get("repository"))
    return bool(repo_tail and image_tail and (image_tail.endswith(repo_tail) or repo_tail.endswith(image_tail)))


def recent_commits(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_gitlab_recent_commits"
    cfg = _config()
    if not cfg["url"] or not cfg["token"]:
        return _not_configured(mode, data)

    started = time.time()
    project = _project(data)
    limit = _safe_int(data.get("limit") or data.get("per_page"), 10, 1, 100)
    api_params = {
        "ref_name": data.get("ref") or data.get("branch") or data.get("ref_name") or "main",
        "per_page": limit,
    }
    if data.get("since"):
        api_params["since"] = data.get("since")
    if data.get("until"):
        api_params["until"] = data.get("until")
    items = _request(f"{_project_path(project)}/repository/commits", api_params)
    commits = [_commit_summary(item) for item in (items if isinstance(items, list) else [])]
    return {
        "mode": mode,
        "configured": True,
        "safety": _safety(),
        "request": data,
        "project": project,
        "commits": commits,
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3), "item_count": len(commits)},
    }


def commit_detail(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_gitlab_commit_detail"
    commit_sha = str(data.get("sha") or data.get("commit") or data.get("revision") or "").strip()
    if not commit_sha:
        raise ValueError("sha is required")
    cfg = _config()
    if not cfg["url"] or not cfg["token"]:
        return _not_configured(mode, data)

    started = time.time()
    project = _project(data)
    commit = _request(f"{_project_path(project)}/repository/commits/{quote(commit_sha, safe='')}")
    stats = commit.get("stats") or {}
    return {
        "mode": mode,
        "configured": True,
        "safety": _safety(),
        "request": data,
        "project": project,
        "commit": _commit_summary(commit),
        "stats": {
            "additions": stats.get("additions"),
            "deletions": stats.get("deletions"),
            "total": stats.get("total"),
        },
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3)},
    }


def pipeline_status(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_gitlab_pipeline_status"
    cfg = _config()
    if not cfg["url"] or not cfg["token"]:
        return _not_configured(mode, data)

    started = time.time()
    project = _project(data)
    limit = _safe_int(data.get("limit") or data.get("per_page"), 10, 1, 100)
    api_params = {
        "ref": data.get("ref") or data.get("branch") or data.get("ref_name") or "",
        "sha": data.get("sha") or data.get("commit") or data.get("revision") or "",
        "status": data.get("status") or "",
        "per_page": limit,
    }
    items = _request(f"{_project_path(project)}/pipelines", api_params)
    pipelines = [_pipeline_summary(item) for item in (items if isinstance(items, list) else [])]
    return {
        "mode": mode,
        "configured": True,
        "safety": _safety(),
        "request": data,
        "project": project,
        "pipelines": pipelines,
        "latest_pipeline": pipelines[0] if pipelines else None,
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3), "item_count": len(pipelines)},
    }


def merge_requests(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_gitlab_merge_requests"
    cfg = _config()
    if not cfg["url"] or not cfg["token"]:
        return _not_configured(mode, data)

    started = time.time()
    project = _project(data)
    limit = _safe_int(data.get("limit") or data.get("per_page"), 10, 1, 100)
    sha = str(data.get("sha") or data.get("commit") or data.get("revision") or "").strip()
    if sha:
        items = _request(f"{_project_path(project)}/repository/commits/{quote(sha, safe='')}/merge_requests", {"per_page": limit})
    else:
        api_params = {
            "state": data.get("state") or "all",
            "source_branch": data.get("source_branch") or "",
            "target_branch": data.get("target_branch") or data.get("ref") or data.get("branch") or "",
            "search": data.get("search") or "",
            "order_by": data.get("order_by") or "updated_at",
            "sort": data.get("sort") or "desc",
            "per_page": limit,
        }
        items = _request(f"{_project_path(project)}/merge_requests", api_params)
    merge_requests_items = [_merge_request_summary(item) for item in (items if isinstance(items, list) else [])]
    return {
        "mode": mode,
        "configured": True,
        "safety": _safety(),
        "request": data,
        "project": project,
        "merge_requests": merge_requests_items,
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3), "item_count": len(merge_requests_items)},
    }


def tags(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_gitlab_tags"
    cfg = _config()
    if not cfg["url"] or not cfg["token"]:
        return _not_configured(mode, data)

    started = time.time()
    project = _project(data)
    tag_name = str(data.get("tag") or data.get("tag_name") or data.get("name") or "").strip()
    if tag_name:
        payload = _request(f"{_project_path(project)}/repository/tags/{quote(tag_name, safe='')}")
        items = [_tag_summary(payload)] if isinstance(payload, dict) else []
    else:
        limit = _safe_int(data.get("limit") or data.get("per_page"), 20, 1, 100)
        payload = _request(
            f"{_project_path(project)}/repository/tags",
            {"search": data.get("search") or "", "order_by": data.get("order_by") or "updated", "sort": data.get("sort") or "desc", "per_page": limit},
        )
        items = [_tag_summary(item) for item in (payload if isinstance(payload, list) else [])]
    return {
        "mode": mode,
        "configured": True,
        "safety": _safety(),
        "request": data,
        "project": project,
        "tags": items,
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3), "item_count": len(items)},
    }


def artifacts(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_gitlab_artifacts"
    cfg = _config()
    if not cfg["url"] or not cfg["token"]:
        return _not_configured(mode, data)

    started = time.time()
    project = _project(data)
    limit = _safe_int(data.get("limit") or data.get("per_page"), 20, 1, 100)
    api_params = {
        "scope[]": data.get("scope") or "",
        "per_page": limit,
    }
    jobs = _request(f"{_project_path(project)}/jobs", api_params)
    filtered = []
    ref = str(data.get("ref") or data.get("branch") or "").strip()
    sha = str(data.get("sha") or data.get("commit") or data.get("revision") or "").strip()
    job_name = str(data.get("job") or data.get("job_name") or "").strip().lower()
    for item in jobs if isinstance(jobs, list) else []:
        commit = item.get("commit") or {}
        if ref and ref != str(item.get("ref") or ""):
            continue
        if sha and sha != str(commit.get("id") or ""):
            continue
        if job_name and job_name not in str(item.get("name") or "").lower():
            continue
        artifact_file = item.get("artifacts_file") or {}
        if data.get("with_artifacts") and not artifact_file and not item.get("artifacts"):
            continue
        filtered.append(_job_summary(item))
    return {
        "mode": mode,
        "configured": True,
        "safety": _safety(),
        "request": data,
        "project": project,
        "jobs": filtered,
        "artifact_jobs": [item for item in filtered if item.get("artifact_file", {}).get("filename") or item.get("artifacts")],
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3), "item_count": len(filtered)},
    }


def registry_repositories(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_gitlab_registry_repositories"
    cfg = _config()
    if not cfg["url"] or not cfg["token"]:
        return _not_configured(mode, data)

    started = time.time()
    project = _project(data)
    limit = _safe_int(data.get("limit") or data.get("per_page"), 20, 1, 100)
    payload = _request(f"{_project_path(project)}/registry/repositories", {"tags": data.get("tags") or "true", "per_page": limit})
    repos = [_registry_repository_summary(item) for item in (payload if isinstance(payload, list) else [])]
    return {
        "mode": mode,
        "configured": True,
        "safety": _safety(),
        "request": data,
        "project": project,
        "repositories": repos,
        "meta": {"status": "ok", "query_seconds": round(time.time() - started, 3), "item_count": len(repos)},
    }


def image_digest_context(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_image_digest_context"
    cfg = _config()
    if not cfg["url"] or not cfg["token"]:
        return _not_configured(mode, data)

    started = time.time()
    errors = []
    project = _project(data)
    images = []
    app_name = str(data.get("app_name") or data.get("application") or "").strip()
    namespace = str(data.get("namespace") or "").strip()
    workload_name = str(data.get("workload_name") or "").strip()
    workload_kind = str(data.get("workload_kind") or "").strip()
    pod = str(data.get("pod") or "").strip()

    if app_name:
        try:
            status = argocd_integration.app_status({"app_name": app_name})
            for image in ((_first_application(status).get("images") or []) if isinstance(status, dict) else []):
                images.append({"source": "argocd", **_image_parts(image)})
        except Exception as exc:
            errors.append({"source": "argocd_images", "message": str(exc)})
    if namespace and (workload_name or pod):
        try:
            release = release_changes.release_for_workload(
                {
                    "namespace": namespace,
                    "workload_name": workload_name,
                    "workload_kind": workload_kind,
                    "pod": pod,
                }
            )
            for container in (((release.get("workload") or {}).get("images")) or []):
                images.append({"source": "kubernetes_workload", "container": container.get("name"), **_image_parts(container.get("image"))})
        except Exception as exc:
            errors.append({"source": "kubernetes_workload_images", "message": str(exc)})
    explicit_image = str(data.get("image") or "").strip()
    if explicit_image:
        images.append({"source": "request", **_image_parts(explicit_image)})

    unique = []
    seen = set()
    for image in images:
        key = image.get("image")
        if key and key not in seen:
            unique.append(image)
            seen.add(key)

    try:
        repos_payload = registry_repositories({**data, "project_id": project, "limit": data.get("registry_limit") or data.get("limit") or 50})
        errors.extend(_collect_payload_errors("gitlab_registry_repositories", repos_payload))
        repositories = repos_payload.get("repositories") or []
    except Exception as exc:
        repositories = []
        errors.append({"source": "gitlab_registry_repositories", "message": str(exc)})
    matches = []
    for image in unique:
        image_matches = []
        for repository in repositories:
            if not _tag_matches_image(repository, image):
                continue
            try:
                tag = _request(
                    f"{_project_path(project)}/registry/repositories/{repository['id']}/tags/{quote(image.get('tag') or 'latest', safe='')}"
                )
                image_matches.append({"repository": repository, "tag": _registry_tag_summary(tag)})
            except Exception as exc:
                image_matches.append({"repository": repository, "tag": None, "error": str(exc)})
        matches.append({"image": image, "registry_matches": image_matches})

    return {
        "mode": mode,
        "configured": True,
        "safety": _safety(),
        "request": data,
        "project": project,
        "images": unique,
        "registry_repositories": repositories,
        "matches": matches,
        "errors": errors,
        "meta": {
            "status": "ok" if not errors else "partial",
            "query_seconds": round(time.time() - started, 3),
            "image_count": len(unique),
            "match_count": sum(len(item.get("registry_matches") or []) for item in matches),
        },
    }


def _first_application(payload):
    if not isinstance(payload, dict):
        return {}
    applications = payload.get("applications") or []
    return applications[0] if applications else {}


def _is_configured(payload):
    return isinstance(payload, dict) and bool(payload.get("configured"))


def _collect_payload_errors(source, payload):
    if not isinstance(payload, dict):
        return []
    return [
        {"source": source, "message": item.get("message") or str(item)}
        for item in (payload.get("errors") or [])
    ]


def release_context(params):
    data = params if isinstance(params, dict) else {}
    mode = "read_only_gitlab_argocd_release_context"
    started = time.time()
    errors = []
    app_name = str(data.get("app_name") or data.get("application") or "").strip()
    sha = str(data.get("sha") or data.get("commit") or data.get("revision") or "").strip()
    ref = str(data.get("ref") or data.get("branch") or data.get("ref_name") or "main").strip()

    argocd_status = None
    argocd_history = None
    argocd_diff = None
    if app_name:
        try:
            argocd_status = argocd_integration.app_status({"app_name": app_name})
            errors.extend(_collect_payload_errors("argocd_status", argocd_status))
            apps = argocd_status.get("applications") or []
            if not sha and apps:
                sha = ((apps[0].get("sync") or {}).get("revision") or "").strip()
        except Exception as exc:
            errors.append({"source": "argocd_status", "message": str(exc)})
        try:
            argocd_history = argocd_integration.app_history({"app_name": app_name, "limit": data.get("history_limit") or 10})
            errors.extend(_collect_payload_errors("argocd_history", argocd_history))
        except Exception as exc:
            errors.append({"source": "argocd_history", "message": str(exc)})
        try:
            argocd_diff = argocd_integration.diff_summary({"app_name": app_name})
            errors.extend(_collect_payload_errors("argocd_diff", argocd_diff))
        except Exception as exc:
            errors.append({"source": "argocd_diff", "message": str(exc)})

    commit = None
    pipelines = None
    merge_request_payload = None
    tag_payload = None
    artifact_payload = None
    image_digest_payload = None
    if sha:
        try:
            commit = commit_detail({**data, "sha": sha})
            errors.extend(_collect_payload_errors("gitlab_commit", commit))
        except Exception as exc:
            errors.append({"source": "gitlab_commit", "message": str(exc)})
        try:
            pipelines = pipeline_status({**data, "sha": sha, "ref": ref})
            errors.extend(_collect_payload_errors("gitlab_pipelines", pipelines))
        except Exception as exc:
            errors.append({"source": "gitlab_pipelines", "message": str(exc)})
        try:
            merge_request_payload = merge_requests({**data, "sha": sha, "limit": data.get("mr_limit") or data.get("limit") or 10})
            errors.extend(_collect_payload_errors("gitlab_merge_requests", merge_request_payload))
        except Exception as exc:
            errors.append({"source": "gitlab_merge_requests", "message": str(exc)})
        try:
            artifact_payload = artifacts({**data, "sha": sha, "ref": ref, "limit": data.get("artifact_limit") or data.get("limit") or 10})
            errors.extend(_collect_payload_errors("gitlab_artifacts", artifact_payload))
        except Exception as exc:
            errors.append({"source": "gitlab_artifacts", "message": str(exc)})
    else:
        try:
            commits = recent_commits({**data, "ref": ref, "limit": 1})
            errors.extend(_collect_payload_errors("gitlab_recent_commits", commits))
            latest = (commits.get("commits") or [{}])[0]
            sha = latest.get("id") or ""
            commit = {"mode": "read_only_gitlab_commit_detail", "configured": commits.get("configured"), "project": commits.get("project"), "commit": latest}
        except Exception as exc:
            errors.append({"source": "gitlab_recent_commits", "message": str(exc)})
    try:
        tag_payload = tags({**data, "limit": data.get("tag_limit") or data.get("limit") or 10})
        errors.extend(_collect_payload_errors("gitlab_tags", tag_payload))
    except Exception as exc:
        errors.append({"source": "gitlab_tags", "message": str(exc)})
    try:
        image_digest_payload = image_digest_context({**data, "sha": sha, "ref": ref})
        errors.extend(_collect_payload_errors("image_digest_context", image_digest_payload))
    except Exception as exc:
        errors.append({"source": "image_digest_context", "message": str(exc)})

    configured = any(
        _is_configured(item)
        for item in (argocd_status, argocd_history, argocd_diff, commit, pipelines, merge_request_payload, tag_payload, artifact_payload, image_digest_payload)
    )
    status = "ok" if configured and not errors else "partial"
    if errors and not configured:
        status = "error"
    return {
        "mode": mode,
        "configured": configured,
        "safety": _safety(),
        "request": data,
        "release": {
            "app_name": app_name,
            "ref": ref,
            "revision": sha,
            "argocd_sync_status": (_first_application(argocd_status).get("sync") or {}).get("status"),
            "argocd_health_status": (_first_application(argocd_status).get("health") or {}).get("status"),
            "pipeline_status": ((pipelines or {}).get("latest_pipeline") or {}).get("status") if isinstance(pipelines, dict) else None,
        },
        "argocd": {
            "status": argocd_status,
            "history": argocd_history,
            "diff": argocd_diff,
        },
        "gitlab": {
            "commit": commit,
            "pipelines": pipelines,
            "merge_requests": merge_request_payload,
            "tags": tag_payload,
            "artifacts": artifact_payload,
            "image_digest_context": image_digest_payload,
        },
        "errors": errors,
        "meta": {"status": status, "query_seconds": round(time.time() - started, 3)},
    }
