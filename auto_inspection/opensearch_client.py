#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import json
from urllib.parse import quote

import requests

from auto_inspection import config


class OpenSearchError(RuntimeError):
    pass


_SESSION = requests.Session()
_SESSION.trust_env = False


def is_configured():
    return bool((getattr(config, "OPENSEARCH_URL", "") or "").strip())


def _base_url():
    return (getattr(config, "OPENSEARCH_URL", "") or "").strip().rstrip("/")


def _auth():
    username = getattr(config, "OPENSEARCH_USERNAME", "") or ""
    password = getattr(config, "OPENSEARCH_PASSWORD", "") or ""
    if username:
        return (username, password)
    return None


def _request(method, path, *, payload=None, params=None, timeout=None):
    if not is_configured():
        raise OpenSearchError("OPENSEARCH_URL is not configured.")

    url = f"{_base_url()}/{path.lstrip('/')}"
    timeout = timeout or getattr(config, "OPENSEARCH_TIMEOUT", 30)

    try:
        response = _SESSION.request(
            method,
            url,
            json=payload,
            params=params,
            timeout=timeout,
            auth=_auth(),
            verify=bool(getattr(config, "OPENSEARCH_VERIFY_SSL", True)),
            proxies={"http": None, "https": None},
            headers={"Content-Type": "application/json"},
        )
    except Exception as exc:
        raise OpenSearchError(str(exc)) from exc

    if response.status_code >= 400:
        text = response.text.strip()
        if len(text) > 500:
            text = text[:500] + "..."
        raise OpenSearchError(
            f"OpenSearch request failed: status={response.status_code} body={text}"
        )

    if not response.text:
        return {}
    try:
        return response.json()
    except ValueError as exc:
        raise OpenSearchError(f"Invalid JSON response from OpenSearch: {exc}") from exc


def search(
    index,
    query,
    *,
    size=50,
    from_=0,
    sort=None,
    source_includes=None,
    timeout=None,
):
    body = {
        "query": query or {"match_all": {}},
        "size": max(1, int(size or 50)),
        "from": max(0, int(from_ or 0)),
    }
    if sort:
        body["sort"] = sort
    if source_includes:
        body["_source"] = {"includes": list(source_includes)}
    return _request("POST", f"{index}/_search", payload=body, timeout=timeout)


def index_document(index, document, *, document_id=None, refresh=False, timeout=None):
    params = {}
    if refresh:
        params["refresh"] = "true"
    if document_id:
        encoded_id = quote(str(document_id), safe="")
        return _request(
            "PUT",
            f"{index}/_doc/{encoded_id}",
            payload=document,
            params=params,
            timeout=timeout,
        )
    return _request(
        "POST",
        f"{index}/_doc",
        payload=document,
        params=params,
        timeout=timeout,
    )


def put_index_template(name, body, *, timeout=None):
    return _request(
        "PUT",
        f"_index_template/{name}",
        payload=body,
        timeout=timeout,
    )


def put_ism_policy(name, body, *, timeout=None):
    params = None
    try:
        existing = _request(
            "GET",
            f"_plugins/_ism/policies/{name}",
            timeout=timeout,
        )
        seq_no = existing.get("_seq_no")
        primary_term = existing.get("_primary_term")
        if seq_no is not None and primary_term is not None:
            params = {"if_seq_no": seq_no, "if_primary_term": primary_term}
    except OpenSearchError as exc:
        if "status=404" not in str(exc):
            raise
    return _request(
        "PUT",
        f"_plugins/_ism/policies/{name}",
        payload=body,
        params=params,
        timeout=timeout,
    )


def put_snapshot_repository(name, body, *, timeout=None):
    return _request(
        "PUT",
        f"_snapshot/{name}",
        payload=body,
        timeout=timeout,
    )


def delete_by_query(index, query, *, timeout=None):
    return _request(
        "POST",
        f"{index}/_delete_by_query",
        payload={"query": query or {"match_all": {}}},
        timeout=timeout,
    )


def ping():
    return _request("GET", "/", timeout=min(getattr(config, "OPENSEARCH_TIMEOUT", 30), 10))


def response_hits(payload):
    hits = (((payload or {}).get("hits") or {}).get("hits")) or []
    total = (((payload or {}).get("hits") or {}).get("total")) or {}
    if isinstance(total, dict):
        total_value = int(total.get("value") or 0)
    else:
        total_value = int(total or 0)
    return total_value, hits


def safe_json(value):
    try:
        return json.loads(json.dumps(value))
    except (TypeError, ValueError):
        return value
