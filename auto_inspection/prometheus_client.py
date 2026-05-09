#!/usr/bin/env python3
# -*- coding: utf-8 -*-

from auto_inspection import config
from auto_inspection.http_client import request_json


def query_instant(promql, *, url=None, timeout=20):
    url = url or config.PROMETHEUS_URL
    data = request_json(
        "GET",
        f"{url}/api/v1/query",
        params={"query": promql},
        timeout=timeout,
        retries=config.REQUEST_RETRIES,
        backoff_seconds=config.REQUEST_BACKOFF_SECONDS,
    )
    if data.get("status") != "success":
        raise RuntimeError(data)
    return data["data"]["result"]


def query_range(promql, start, end, step, *, url=None, timeout=30):
    url = url or config.PROMETHEUS_URL
    data = request_json(
        "GET",
        f"{url}/api/v1/query_range",
        params={
            "query": promql,
            "start": start,
            "end": end,
            "step": step,
        },
        timeout=timeout,
        retries=config.REQUEST_RETRIES,
        backoff_seconds=config.REQUEST_BACKOFF_SECONDS,
    )
    if data.get("status") != "success":
        raise RuntimeError(data)
    return data["data"]["result"]


def label_values(label, *, url=None, timeout=15):
    url = url or config.PROMETHEUS_URL
    data = request_json(
        "GET",
        f"{url}/api/v1/label/{label}/values",
        timeout=timeout,
        retries=config.REQUEST_RETRIES,
        backoff_seconds=config.REQUEST_BACKOFF_SECONDS,
    )
    if data.get("status") != "success":
        raise RuntimeError(data)
    return data["data"]


def active_targets(*, url=None, timeout=20):
    url = url or config.PROMETHEUS_URL
    data = request_json(
        "GET",
        f"{url}/api/v1/targets",
        timeout=timeout,
        retries=config.REQUEST_RETRIES,
        backoff_seconds=config.REQUEST_BACKOFF_SECONDS,
    )
    if data.get("status") != "success":
        raise RuntimeError(data)
    payload = data.get("data") or {}
    return payload.get("activeTargets") or []
