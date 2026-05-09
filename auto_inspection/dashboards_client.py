#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import json

import requests

from auto_inspection import config


_SESSION = requests.Session()
_SESSION.trust_env = False


def is_configured():
    return bool((getattr(config, "OPENSEARCH_DASHBOARDS_URL", "") or "").strip())


def base_url():
    url = (getattr(config, "OPENSEARCH_DASHBOARDS_URL", "") or "").strip()
    if not url:
        raise RuntimeError("OPENSEARCH_DASHBOARDS_URL is not configured.")
    return url.rstrip("/")


def request(method, path, *, payload=None, timeout=30):
    url = f"{base_url()}/{path.lstrip('/')}"
    response = _SESSION.request(
        method,
        url,
        json=payload,
        timeout=timeout,
        proxies={"http": None, "https": None},
        headers={"osd-xsrf": "true", "Content-Type": "application/json"},
    )
    response.raise_for_status()
    return response.json()


def build_saved_search_payload(*, title, description, index_id, query, filters=None, columns=None, sort=None):
    return {
        "attributes": {
            "title": title,
            "description": description,
            "hits": 0,
            "version": 1,
            "columns": columns or [],
            "sort": sort or [["@timestamp", "desc"]],
            "kibanaSavedObjectMeta": {
                "searchSourceJSON": json.dumps(
                    {
                        "query": {
                            "query": query,
                            "language": "kuery",
                        },
                        "filter": filters or [],
                        "indexRefName": "kibanaSavedObjectMeta.searchSourceJSON.index",
                    }
                )
            },
        },
        "references": [
            {
                "id": index_id,
                "name": "kibanaSavedObjectMeta.searchSourceJSON.index",
                "type": "index-pattern",
            }
        ],
    }


def upsert_saved_search(object_id, payload):
    return request(
        "POST",
        f"api/saved_objects/search/{object_id}?overwrite=true",
        payload=payload,
    )


def discover_saved_search_url(object_id):
    return f"{base_url()}/app/discover#/view/{object_id}"


def dashboards_view_url(object_id):
    return f"{base_url()}/app/dashboards#/view/{object_id}"
