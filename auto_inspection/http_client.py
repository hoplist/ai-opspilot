#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import time
import requests

_SESSION = requests.Session()
_SESSION.trust_env = False


def request_data(
    method,
    url,
    *,
    params=None,
    payload=None,
    timeout=30,
    retries=3,
    backoff_seconds=0.5,
    expect_json=True,
):
    retries = max(1, retries)
    last_exc = None
    for attempt in range(retries):
        try:
            resp = _SESSION.request(
                method,
                url,
                params=params,
                json=payload,
                timeout=timeout,
                proxies={"http": None, "https": None},
            )
            resp.raise_for_status()
            if expect_json:
                return resp.json()
            if not resp.text:
                return {}
            try:
                return resp.json()
            except ValueError:
                return {"text": resp.text}
        except Exception as exc:
            last_exc = exc
            if attempt < retries - 1:
                time.sleep(backoff_seconds * (2 ** attempt))
    raise last_exc


def request_json(
    method,
    url,
    *,
    params=None,
    payload=None,
    timeout=30,
    retries=3,
    backoff_seconds=0.5,
):
    return request_data(
        method,
        url,
        params=params,
        payload=payload,
        timeout=timeout,
        retries=retries,
        backoff_seconds=backoff_seconds,
        expect_json=True,
    )
