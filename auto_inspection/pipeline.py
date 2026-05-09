#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import argparse
import importlib
import sys
import time


STEPS = [
    ("targets", "auto_inspection.discover_targets", "main"),
    ("baseline", "auto_inspection.baseline_builder", "main"),
    ("anomaly", "auto_inspection.baseline_anomaly", "main"),
    ("health", "auto_inspection.health_profile", "main"),
    ("merge", "auto_inspection.anomaly_merge", "main"),
    ("lifecycle", "auto_inspection.event_lifecycle", "main"),
    ("escalation", "auto_inspection.event_escalation", "main"),
    ("runbook", "auto_inspection.runbook_attach", "main"),
    ("report", "auto_inspection.weekly_inspection", "main"),
]


def list_steps():
    for name, mod, func in STEPS:
        print(f"{name}: {mod}.{func}")


def resolve_steps(args):
    names = [name for name, _, _ in STEPS]

    if args.list:
        list_steps()
        return []

    selected = names

    if args.steps:
        selected = [s.strip() for s in args.steps.split(",") if s.strip()]
    else:
        start_idx = names.index(args.from_step) if args.from_step else 0
        end_idx = names.index(args.to_step) if args.to_step else len(names) - 1
        if start_idx > end_idx:
            raise ValueError("--from must be before --to")
        selected = names[start_idx:end_idx + 1]

    if args.skip:
        skip_set = {s.strip() for s in args.skip.split(",") if s.strip()}
        selected = [s for s in selected if s not in skip_set]

    unknown = [s for s in selected if s not in names]
    if unknown:
        raise ValueError(f"Unknown steps: {', '.join(unknown)}")

    return selected


def run_steps(selected, continue_on_error=False):
    run_steps_with_results(selected, continue_on_error=continue_on_error)


def run_steps_with_results(selected, continue_on_error=False):
    results = []
    for name, mod, func in STEPS:
        if name not in selected:
            continue
        started_at = time.time()
        try:
            module = importlib.import_module(mod)
            getattr(module, func)()
            results.append(
                {
                    "step": name,
                    "module": mod,
                    "function": func,
                    "status": "ok",
                    "duration_seconds": round(time.time() - started_at, 3),
                }
            )
        except Exception as exc:
            results.append(
                {
                    "step": name,
                    "module": mod,
                    "function": func,
                    "status": "error",
                    "duration_seconds": round(time.time() - started_at, 3),
                    "error": str(exc),
                }
            )
            print(f"[ERROR] step={name} module={mod}: {exc}", file=sys.stderr)
            if not continue_on_error:
                raise
    return results


def main(argv=None):
    parser = argparse.ArgumentParser(description="Run auto inspection pipeline.")
    parser.add_argument("--list", action="store_true", help="List available steps and exit.")
    parser.add_argument("--steps", help="Comma-separated steps to run.")
    parser.add_argument("--from", dest="from_step", help="Start step name (inclusive).")
    parser.add_argument("--to", dest="to_step", help="End step name (inclusive).")
    parser.add_argument("--skip", help="Comma-separated steps to skip.")
    parser.add_argument(
        "--continue-on-error",
        action="store_true",
        help="Continue running remaining steps on error.",
    )
    args = parser.parse_args(argv)

    selected = resolve_steps(args)
    if not selected:
        return 0

    run_steps(selected, continue_on_error=args.continue_on_error)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
