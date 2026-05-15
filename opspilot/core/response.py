from __future__ import annotations

from datetime import datetime, timezone
from typing import Any


BACKEND_NAME = "opspilot-core"


def now_iso() -> str:
    return datetime.now(timezone.utc).astimezone().isoformat(timespec="seconds")


def envelope(data: Any = None, warnings: list[str] | None = None) -> dict[str, Any]:
    return {
        "ok": True,
        "data": data if data is not None else {},
        "warnings": warnings or [],
        "source": {
            "backend": BACKEND_NAME,
            "time": now_iso(),
        },
    }


def error_envelope(code: str, message: str, status: int = 500) -> tuple[int, dict[str, Any]]:
    return status, {
        "ok": False,
        "error": {
            "code": code,
            "message": message,
        },
        "warnings": [],
        "source": {
            "backend": BACKEND_NAME,
            "time": now_iso(),
        },
    }
