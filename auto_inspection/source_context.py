#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import hashlib
import json
import os

from auto_inspection import config
from auto_inspection import opensearch_client
from auto_inspection.paths import project_path


def prometheus_source_metadata():
    urls = list(getattr(config, "PROMETHEUS_URLS", []) or [])
    clusters = list(getattr(config, "PROMETHEUS_CLUSTERS", []) or [])
    source_id = str(getattr(config, "SOURCE_ID", "") or "").strip()
    if not source_id:
        if clusters:
            source_id = "prometheus-" + "-".join(sorted(set(clusters)))
        else:
            source_id = "prometheus-default"
    return {
        "type": "prometheus",
        "source_id": source_id,
        "urls": urls,
        "clusters": clusters,
        "primary_url": urls[0] if urls else "",
    }


def source_fingerprint():
    meta = prometheus_source_metadata()
    basis = {
        "type": meta.get("type"),
        "source_id": meta.get("source_id"),
        "clusters": meta.get("clusters") or [],
    }
    payload = json.dumps(basis, sort_keys=True, ensure_ascii=False)
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()[:16]


def source_metadata():
    data = prometheus_source_metadata()
    data["fingerprint"] = source_fingerprint()
    return data


def _resolved_state_path():
    path = getattr(config, "SOURCE_STATE_FILE", "data/source_state.json") or "data/source_state.json"
    return project_path(path)


def load_source_state():
    path = _resolved_state_path()
    if not os.path.exists(path):
        return {}
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
        return data if isinstance(data, dict) else {}
    except (OSError, json.JSONDecodeError):
        return {}


def save_source_state():
    path = _resolved_state_path()
    os.makedirs(os.path.dirname(path), exist_ok=True)
    data = source_metadata()
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
    return data


def has_source_changed():
    current = source_fingerprint()
    saved = load_source_state().get("fingerprint")
    return bool(saved and saved != current)


def cleanup_local_event_artifacts():
    paths = [
        "data/targets.json",
        "data/anomalies.json",
        "data/health_profiles.json",
        "data/events.json",
        "data/events_lifecycle.json",
        "data/events_history.json",
        "data/events_escalated.json",
        "data/events_with_runbook.json",
    ]
    for relative in paths:
        path = project_path(relative)
        if os.path.exists(path):
            try:
                os.remove(path)
            except OSError:
                pass

    baseline_dir = project_path("data", "baseline")
    if os.path.isdir(baseline_dir):
        for name in os.listdir(baseline_dir):
            path = os.path.join(baseline_dir, name)
            if os.path.isfile(path):
                try:
                    os.remove(path)
                except OSError:
                    pass


def cleanup_opensearch_incidents():
    if not opensearch_client.is_configured():
        return None
    index_pattern = getattr(config, "OPENSEARCH_INDEX_INCIDENTS", "") or ""
    if not index_pattern:
        return None
    return opensearch_client.delete_by_query(index_pattern, {"match_all": {}})


def ensure_current_source_state():
    changed = has_source_changed()
    if changed:
      cleanup_local_event_artifacts()
      try:
          cleanup_opensearch_incidents()
      except Exception:
          pass
    return save_source_state(), changed
