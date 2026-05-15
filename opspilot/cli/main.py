from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any


DEFAULT_BACKEND = "http://127.0.0.1:18080"


def main(argv: list[str] | None = None) -> None:
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        result = dispatch(args)
    except CliError as exc:
        print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False, indent=2), file=sys.stderr)
        raise SystemExit(1) from exc
    if isinstance(result, str):
        print(result)
    else:
        print(json.dumps(result, ensure_ascii=False, indent=2))


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="opspilot", description="OpsPilot deterministic CLI")
    parser.add_argument("--backend-url", default=os.environ.get("OPSPILOT_BACKEND_URL", DEFAULT_BACKEND))
    sub = parser.add_subparsers(dest="group", required=True)

    sub.add_parser("schema", help="Print CLI schema")

    inventory = sub.add_parser("inventory", help="Inventory commands")
    inv_sub = inventory.add_subparsers(dest="command", required=True)
    overview = inv_sub.add_parser("overview", help="Return inventory overview")
    overview.add_argument("--limit", type=int, default=10)

    k8s = sub.add_parser("k8s", help="Kubernetes commands")
    k8s_sub = k8s.add_subparsers(dest="command", required=True)
    pods = k8s_sub.add_parser("pods", help="List pods")
    pods.add_argument("-n", "--namespace", default="")
    pods.add_argument("--status", default="")
    pods.add_argument("-q", default="")
    pods.add_argument("--limit", type=int, default=100)
    logs = k8s_sub.add_parser("logs", help="Read Kubernetes logs")
    logs_sub = logs.add_subparsers(dest="logs_command", required=True)
    pod_logs = logs_sub.add_parser("pod", help="Read Pod logs")
    _add_pod_ref_args(pod_logs)
    pod_logs.add_argument("-c", "--container", default="")
    pod_logs.add_argument("--tail", "--tail-lines", dest="tail_lines", type=int, default=300)
    pod_logs.add_argument("--since", "--since-seconds", dest="since_seconds", type=int, default=1800)
    pod_logs.add_argument("--limit-bytes", type=int, default=1024 * 1024)
    pod_logs.add_argument("--previous", action="store_true")
    pod_logs.add_argument("--timestamps", action="store_true")

    context = sub.add_parser("context", help="Context commands")
    context_sub = context.add_subparsers(dest="command", required=True)
    context_pod = context_sub.add_parser("pod", help="Return Pod Evidence Pack")
    _add_pod_ref_args(context_pod)

    diagnose = sub.add_parser("diagnose", help="Diagnose commands")
    diagnose_sub = diagnose.add_subparsers(dest="command", required=True)
    diagnose_pod = diagnose_sub.add_parser("pod", help="Diagnose Pod")
    _add_pod_ref_args(diagnose_pod)
    return parser


def _add_pod_ref_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("-n", "--namespace", required=True)
    parser.add_argument("--pod", required=True)


def dispatch(args: argparse.Namespace) -> Any:
    if args.group == "schema":
        return _schema_text()
    if args.group == "inventory" and args.command == "overview":
        return _get(args.backend_url, "/api/inventory/overview", {"limit": args.limit})
    if args.group == "k8s" and args.command == "pods":
        return _get(
            args.backend_url,
            "/api/k8s/pods",
            {
                "namespace": args.namespace,
                "status": args.status,
                "q": args.q,
                "limit": args.limit,
            },
        )
    if args.group == "k8s" and args.command == "logs" and args.logs_command == "pod":
        return _get(
            args.backend_url,
            "/api/k8s/logs/pod",
            {
                "namespace": args.namespace,
                "pod": args.pod,
                "container": args.container,
                "tail_lines": args.tail_lines,
                "since_seconds": args.since_seconds,
                "limit_bytes": args.limit_bytes,
                "previous": str(args.previous).lower(),
                "timestamps": str(args.timestamps).lower(),
            },
        )
    if args.group == "context" and args.command == "pod":
        return _get(args.backend_url, "/api/context/pod", {"namespace": args.namespace, "pod": args.pod})
    if args.group == "diagnose" and args.command == "pod":
        return _get(args.backend_url, "/api/diagnose/pod", {"namespace": args.namespace, "pod": args.pod})
    raise CliError("unsupported command")


def _schema_text() -> str:
    path = Path(__file__).resolve().parents[1] / "contracts" / "cli-schema.json"
    return path.read_text(encoding="utf-8")


def _get(base_url: str, path: str, query: dict[str, Any]) -> dict[str, Any]:
    clean = {key: str(value) for key, value in query.items() if value not in {None, ""}}
    url = base_url.rstrip("/") + path
    if clean:
        url += "?" + urllib.parse.urlencode(clean)
    req = urllib.request.Request(url)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise CliError(f"backend returned {exc.code}: {body[:500]}") from exc
    except urllib.error.URLError as exc:
        raise CliError(f"backend unavailable: {exc}") from exc


class CliError(RuntimeError):
    pass
