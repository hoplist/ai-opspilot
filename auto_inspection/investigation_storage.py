#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import datetime
import io
import json
import os
import sqlite3

from auto_inspection import config
from auto_inspection.paths import project_path


HOT_STORE_TABLE = "investigation_metadata"


def _json_text(value):
    return json.dumps(value or {}, ensure_ascii=False)


def _hot_store_driver():
    return str(getattr(config, "INVESTIGATION_HOT_STORE_DRIVER", "") or "").strip().lower()


def _cold_store_driver():
    return str(getattr(config, "INVESTIGATION_COLD_STORE_DRIVER", "") or "").strip().lower()


def hot_store_enabled():
    driver = _hot_store_driver()
    return driver in {"sqlite", "mysql"}


def cold_store_enabled():
    if _cold_store_driver() != "minio":
        return False
    return bool(
        str(getattr(config, "MINIO_ENDPOINT", "") or "").strip()
        and str(getattr(config, "MINIO_ACCESS_KEY", "") or "").strip()
        and str(getattr(config, "MINIO_SECRET_KEY", "") or "").strip()
        and str(getattr(config, "MINIO_BUCKET", "") or "").strip()
    )


def _sqlite_path():
    path = getattr(config, "INVESTIGATION_SQLITE_PATH", "data/investigation_hot.db") or "data/investigation_hot.db"
    return project_path(path)


def _connect_sqlite():
    path = _sqlite_path()
    os.makedirs(os.path.dirname(path), exist_ok=True)
    return sqlite3.connect(path)


def _connect_mysql():
    import pymysql

    return pymysql.connect(
        host=getattr(config, "INVESTIGATION_MYSQL_HOST", "") or "",
        port=int(getattr(config, "INVESTIGATION_MYSQL_PORT", 3306) or 3306),
        user=getattr(config, "INVESTIGATION_MYSQL_USER", "") or "",
        password=getattr(config, "INVESTIGATION_MYSQL_PASSWORD", "") or "",
        database=getattr(config, "INVESTIGATION_MYSQL_DATABASE", "auto_inspection") or "auto_inspection",
        charset="utf8mb4",
        autocommit=True,
    )


def _connect_hot_store():
    driver = _hot_store_driver()
    if driver == "sqlite":
        return driver, _connect_sqlite()
    if driver == "mysql":
        return driver, _connect_mysql()
    return "", None


def ensure_hot_store():
    if not hot_store_enabled():
        return {"enabled": False}
    driver, conn = _connect_hot_store()
    if conn is None:
        return {"enabled": False}
    try:
        if driver == "sqlite":
            conn.execute(
                f"""
                CREATE TABLE IF NOT EXISTS {HOT_STORE_TABLE} (
                    investigation_id TEXT PRIMARY KEY,
                    generated_at TEXT,
                    namespace TEXT,
                    pod TEXT,
                    workload_name TEXT,
                    summary TEXT,
                    status TEXT,
                    logs_count INTEGER,
                    events_count INTEGER,
                    dashboards_links_json TEXT,
                    local_path TEXT,
                    cold_store_driver TEXT,
                    cold_bucket TEXT,
                    cold_object_key TEXT
                )
                """
            )
            conn.commit()
        elif driver == "mysql":
            with conn.cursor() as cur:
                cur.execute(
                    f"""
                    CREATE TABLE IF NOT EXISTS {HOT_STORE_TABLE} (
                        investigation_id VARCHAR(128) PRIMARY KEY,
                        generated_at VARCHAR(32),
                        namespace VARCHAR(128),
                        pod VARCHAR(256),
                        workload_name VARCHAR(256),
                        summary TEXT,
                        status VARCHAR(32),
                        logs_count INT,
                        events_count INT,
                        dashboards_links_json LONGTEXT,
                        local_path TEXT,
                        cold_store_driver VARCHAR(32),
                        cold_bucket VARCHAR(255),
                        cold_object_key TEXT
                    ) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci
                    """
                )
        return {"enabled": True, "driver": driver}
    finally:
        conn.close()


def _metadata_values(payload, *, local_path="", archive=None):
    request = payload.get("request") or {}
    analysis = payload.get("analysis") or {}
    evidence = payload.get("evidence") or {}
    archive = archive or {}
    return {
        "investigation_id": payload.get("investigation_id") or "",
        "generated_at": payload.get("generated_at") or "",
        "namespace": request.get("namespace") or "",
        "pod": request.get("pod") or "",
        "workload_name": request.get("workload_name") or "",
        "summary": analysis.get("summary") or "",
        "status": (payload.get("meta") or {}).get("status") or "ok",
        "logs_count": len(evidence.get("logs") or []),
        "events_count": len(evidence.get("events") or []),
        "dashboards_links_json": _json_text((payload.get("links") or {}).get("dashboards") or {}),
        "local_path": local_path or "",
        "cold_store_driver": archive.get("driver") or "",
        "cold_bucket": archive.get("bucket") or "",
        "cold_object_key": archive.get("object_key") or "",
    }


def save_investigation_metadata(payload, *, local_path="", archive=None):
    if not hot_store_enabled():
        return {"stored": False, "reason": "hot_store_disabled"}
    ensure_hot_store()
    driver, conn = _connect_hot_store()
    values = _metadata_values(payload, local_path=local_path, archive=archive)
    try:
        if driver == "sqlite":
            conn.execute(
                f"""
                INSERT OR REPLACE INTO {HOT_STORE_TABLE} (
                    investigation_id, generated_at, namespace, pod, workload_name,
                    summary, status, logs_count, events_count, dashboards_links_json,
                    local_path, cold_store_driver, cold_bucket, cold_object_key
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    values["investigation_id"],
                    values["generated_at"],
                    values["namespace"],
                    values["pod"],
                    values["workload_name"],
                    values["summary"],
                    values["status"],
                    values["logs_count"],
                    values["events_count"],
                    values["dashboards_links_json"],
                    values["local_path"],
                    values["cold_store_driver"],
                    values["cold_bucket"],
                    values["cold_object_key"],
                ),
            )
            conn.commit()
        elif driver == "mysql":
            with conn.cursor() as cur:
                cur.execute(
                    f"""
                    INSERT INTO {HOT_STORE_TABLE} (
                        investigation_id, generated_at, namespace, pod, workload_name,
                        summary, status, logs_count, events_count, dashboards_links_json,
                        local_path, cold_store_driver, cold_bucket, cold_object_key
                    ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
                    ON DUPLICATE KEY UPDATE
                        generated_at=VALUES(generated_at),
                        namespace=VALUES(namespace),
                        pod=VALUES(pod),
                        workload_name=VALUES(workload_name),
                        summary=VALUES(summary),
                        status=VALUES(status),
                        logs_count=VALUES(logs_count),
                        events_count=VALUES(events_count),
                        dashboards_links_json=VALUES(dashboards_links_json),
                        local_path=VALUES(local_path),
                        cold_store_driver=VALUES(cold_store_driver),
                        cold_bucket=VALUES(cold_bucket),
                        cold_object_key=VALUES(cold_object_key)
                    """,
                    (
                        values["investigation_id"],
                        values["generated_at"],
                        values["namespace"],
                        values["pod"],
                        values["workload_name"],
                        values["summary"],
                        values["status"],
                        values["logs_count"],
                        values["events_count"],
                        values["dashboards_links_json"],
                        values["local_path"],
                        values["cold_store_driver"],
                        values["cold_bucket"],
                        values["cold_object_key"],
                    ),
                )
        return {"stored": True, "driver": driver}
    finally:
        conn.close()


def _summary_row_to_item(row):
    return {
        "investigation_id": row["investigation_id"] or "",
        "generated_at": row["generated_at"] or "",
        "namespace": row["namespace"] or "",
        "pod": row["pod"] or "",
        "workload_name": row["workload_name"] or "",
        "summary": row["summary"] or "",
        "logs_count": int(row["logs_count"] or 0),
        "events_count": int(row["events_count"] or 0),
        "logs_source": "",
        "events_source": "",
        "use_ai": False,
        "dashboards_links": json.loads(row["dashboards_links_json"] or "{}"),
    }


def list_recent_investigations(limit=20):
    if not hot_store_enabled():
        return None
    ensure_hot_store()
    driver, conn = _connect_hot_store()
    try:
        rows = []
        if driver == "sqlite":
            conn.row_factory = sqlite3.Row
            cur = conn.execute(
                f"""
                SELECT investigation_id, generated_at, namespace, pod, workload_name,
                       summary, logs_count, events_count, dashboards_links_json
                FROM {HOT_STORE_TABLE}
                ORDER BY generated_at DESC
                LIMIT ?
                """,
                (max(1, int(limit or 20)),),
            )
            rows = [dict(item) for item in cur.fetchall()]
        elif driver == "mysql":
            with conn.cursor() as cur:
                cur.execute(
                    f"""
                    SELECT investigation_id, generated_at, namespace, pod, workload_name,
                           summary, logs_count, events_count, dashboards_links_json
                    FROM {HOT_STORE_TABLE}
                    ORDER BY generated_at DESC
                    LIMIT %s
                    """,
                    (max(1, int(limit or 20)),),
                )
                columns = [col[0] for col in cur.description or []]
                rows = [dict(zip(columns, row)) for row in cur.fetchall()]
        return [_summary_row_to_item(row) for row in rows]
    finally:
        conn.close()


def load_investigation_pointer(investigation_id):
    if not hot_store_enabled():
        return None
    ensure_hot_store()
    driver, conn = _connect_hot_store()
    try:
        row = None
        if driver == "sqlite":
            conn.row_factory = sqlite3.Row
            if investigation_id == "latest":
                cur = conn.execute(
                    f"""
                    SELECT *
                    FROM {HOT_STORE_TABLE}
                    ORDER BY generated_at DESC
                    LIMIT 1
                    """
                )
            else:
                cur = conn.execute(
                    f"""
                    SELECT *
                    FROM {HOT_STORE_TABLE}
                    WHERE investigation_id = ?
                    LIMIT 1
                    """,
                    (investigation_id,),
                )
            item = cur.fetchone()
            row = dict(item) if item else None
        elif driver == "mysql":
            with conn.cursor() as cur:
                if investigation_id == "latest":
                    cur.execute(
                        f"""
                        SELECT *
                        FROM {HOT_STORE_TABLE}
                        ORDER BY generated_at DESC
                        LIMIT 1
                        """
                    )
                else:
                    cur.execute(
                        f"""
                        SELECT *
                        FROM {HOT_STORE_TABLE}
                        WHERE investigation_id = %s
                        LIMIT 1
                        """,
                        (investigation_id,),
                    )
                item = cur.fetchone()
                if item and cur.description:
                    columns = [col[0] for col in cur.description]
                    row = dict(zip(columns, item))
        return row
    finally:
        conn.close()


def _minio_client():
    from minio import Minio

    endpoint = str(getattr(config, "MINIO_ENDPOINT", "") or "").strip()
    return Minio(
        endpoint,
        access_key=str(getattr(config, "MINIO_ACCESS_KEY", "") or "").strip(),
        secret_key=str(getattr(config, "MINIO_SECRET_KEY", "") or "").strip(),
        secure=bool(getattr(config, "MINIO_SECURE", False)),
    )


def _archive_object_key(investigation_id):
    now = datetime.datetime.utcnow()
    prefix = str(getattr(config, "MINIO_PREFIX", "investigations") or "investigations").strip("/ ")
    return f"{prefix}/{now.strftime('%Y/%m/%d')}/{investigation_id}.json"


def archive_investigation_payload(investigation_id, payload):
    if not cold_store_enabled():
        return {"stored": False, "reason": "cold_store_disabled"}

    client = _minio_client()
    bucket = str(getattr(config, "MINIO_BUCKET", "auto-inspection-archive") or "auto-inspection-archive").strip()
    if not client.bucket_exists(bucket):
        client.make_bucket(bucket)
    content = json.dumps(payload, ensure_ascii=False, indent=2).encode("utf-8")
    object_key = _archive_object_key(investigation_id)
    client.put_object(
        bucket,
        object_key,
        io.BytesIO(content),
        len(content),
        content_type="application/json",
    )
    return {
        "stored": True,
        "driver": "minio",
        "bucket": bucket,
        "object_key": object_key,
        "size_bytes": len(content),
    }


def load_investigation_archive(pointer):
    if not pointer or (pointer.get("cold_store_driver") or "") != "minio":
        return None
    if not cold_store_enabled():
        return None
    client = _minio_client()
    bucket = pointer.get("cold_bucket") or ""
    object_key = pointer.get("cold_object_key") or ""
    if not bucket or not object_key:
        return None
    response = client.get_object(bucket, object_key)
    try:
        raw = response.read()
        return json.loads(raw.decode("utf-8"))
    finally:
        response.close()
        response.release_conn()
