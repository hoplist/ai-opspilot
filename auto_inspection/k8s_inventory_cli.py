#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import argparse
import json
import os

from auto_inspection.backend_client import BackendClient, DEFAULT_BASE_URL


def _print_payload(payload):
    print(json.dumps(payload, ensure_ascii=False, indent=2))


def _compact_params(args, keys):
    params = {}
    for key in keys:
        value = getattr(args, key, None)
        if value not in (None, ""):
            params[key] = value
    return params


def _add_common_filters(parser, *, namespace=True, query=True, limit=True):
    if namespace:
        parser.add_argument("-n", "--namespace", default="", help="Kubernetes namespace. Empty/all means all namespaces.")
    if query:
        parser.add_argument("-q", "--q", default="", help="Fuzzy query string.")
    if limit:
        parser.add_argument("--limit", type=int, default=100, help="Maximum items to return.")


def build_parser():
    parser = argparse.ArgumentParser(description="Read-only Kubernetes inventory CLI for auto_inspection RCA backend.")
    parser.add_argument(
        "--backend-url",
        default=os.getenv("AUTO_INSPECTION_BACKEND_URL", DEFAULT_BASE_URL),
        help="RCA backend URL. Defaults to AUTO_INSPECTION_BACKEND_URL or local backend.",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    namespaces = subparsers.add_parser("namespaces", help="List namespaces.")
    _add_common_filters(namespaces, namespace=False)

    pods = subparsers.add_parser("pods", help="List pods.")
    _add_common_filters(pods)
    pods.add_argument("--status", default="", help="running, pending, failed, abnormal, crashloop, imagepull, not_ready, etc.")
    pods.add_argument("--node", default="", help="Filter by node name substring.")
    pods.add_argument("--owner-kind", default="", help="Filter by owner kind.")
    pods.add_argument("--owner-name", default="", help="Filter by owner name substring.")

    abnormal_pods = subparsers.add_parser("abnormal-pods", help="List abnormal pods.")
    _add_common_filters(abnormal_pods)

    workloads = subparsers.add_parser("workloads", help="List workloads.")
    _add_common_filters(workloads)
    workloads.add_argument("--kind", default="all", help="all, deployment, statefulset, daemonset, or replicaset.")

    services = subparsers.add_parser("services", help="List services.")
    _add_common_filters(services)
    services.add_argument("--type", default="", help="ClusterIP, NodePort, LoadBalancer, etc.")

    search = subparsers.add_parser("search", help="Fuzzy search namespaces, pods, workloads, and services.")
    _add_common_filters(search)
    search.add_argument("--kinds", default="pods,workloads,services,namespaces", help="Comma-separated kinds or all.")

    count = subparsers.add_parser("count", help="Count cluster resources.")
    _add_common_filters(count, query=False, limit=False)

    overview = subparsers.add_parser("overview", help="Cluster overview with counts, nodes, namespaces, and abnormal pods.")
    _add_common_filters(overview)

    return parser


def main(argv=None):
    parser = build_parser()
    args = parser.parse_args(argv)
    client = BackendClient(args.backend_url)

    if args.command == "namespaces":
        payload = client.list_namespaces(**_compact_params(args, ("q", "limit")))
    elif args.command == "pods":
        payload = client.list_pods(
            **_compact_params(args, ("namespace", "q", "status", "node", "owner_kind", "owner_name", "limit"))
        )
    elif args.command == "abnormal-pods":
        payload = client.list_abnormal_pods(**_compact_params(args, ("namespace", "q", "limit")))
    elif args.command == "workloads":
        payload = client.list_workloads(**_compact_params(args, ("namespace", "q", "kind", "limit")))
    elif args.command == "services":
        payload = client.list_services(**_compact_params(args, ("namespace", "q", "type", "limit")))
    elif args.command == "search":
        payload = client.search_k8s_resources(**_compact_params(args, ("namespace", "q", "kinds", "limit")))
    elif args.command == "count":
        payload = client.count_k8s_resources(**_compact_params(args, ("namespace",)))
    elif args.command == "overview":
        payload = client.cluster_overview(**_compact_params(args, ("namespace", "q", "limit")))
    else:
        parser.error(f"Unsupported command: {args.command}")
        return 2

    _print_payload(payload)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
