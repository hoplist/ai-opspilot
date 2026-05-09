#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import argparse
import json
import os
import sys

from auto_inspection.backend_client import BackendClient, DEFAULT_BASE_URL


def _add_common_arguments(parser):
    parser.add_argument("--namespace", required=True)
    parser.add_argument("--symptom", default="unknown")
    parser.add_argument("--q", default="")
    parser.add_argument("--range-hours", type=int, default=6)
    parser.add_argument("--size", type=int, default=30)
    parser.add_argument("--out", default="")


def _write_payload(payload, output_path):
    text = json.dumps(payload, ensure_ascii=False, indent=2)
    if output_path:
        with open(output_path, "w", encoding="utf-8") as handle:
            handle.write(text)
            handle.write("\n")
    else:
        sys.stdout.write(text)
        sys.stdout.write("\n")


def main(argv=None):
    parser = argparse.ArgumentParser(description="Fetch read-only auto_inspection Evidence Packs.")
    parser.add_argument(
        "--backend-url",
        default=os.environ.get("AUTO_INSPECTION_BACKEND_URL", DEFAULT_BASE_URL),
        help="Backend base URL. Defaults to AUTO_INSPECTION_BACKEND_URL or local backend.",
    )
    subparsers = parser.add_subparsers(dest="target_type", required=True)

    pod = subparsers.add_parser("pod", help="Fetch Pod Evidence Pack.")
    _add_common_arguments(pod)
    pod.add_argument("--pod", default="")
    pod.add_argument("--workload-name", default="")

    workload = subparsers.add_parser("workload", help="Fetch Workload Evidence Pack.")
    _add_common_arguments(workload)
    workload.add_argument("--workload-name", default="")
    workload.add_argument("--workload-kind", default="")
    workload.add_argument("--service", default="")

    args = parser.parse_args(argv)
    client = BackendClient(args.backend_url)
    params = {
        "namespace": args.namespace,
        "symptom": args.symptom,
        "q": args.q,
        "range_hours": args.range_hours,
        "size": args.size,
    }
    if args.target_type == "pod":
        params.update({"pod": args.pod, "workload_name": args.workload_name})
    else:
        params.update(
            {
                "workload_name": args.workload_name,
                "workload_kind": args.workload_kind,
                "service": args.service,
            }
        )
    payload = client.context_pack(args.target_type, **params)
    _write_payload(payload, args.out)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
