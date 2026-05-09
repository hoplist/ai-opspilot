# RCA Product Overview

## Positioning

This project is a Kubernetes-focused, log-first AI RCA platform that unifies:

- Kubernetes container logs
- Kubernetes events
- Prometheus metric context
- AI investigation outputs

into a searchable, explainable, and reusable troubleshooting product.

## Core Capabilities

### 1. Unified Evidence Aggregation

The platform brings together:

- normalized logs from OpenSearch
- Kubernetes events from OpenSearch
- incidents and investigations from OpenSearch
- Pod / Node / kube-state metrics from Prometheus
- Kubernetes API fallback state and container details

### 2. AI Investigation and RCA

The backend assembles a single investigation input from
"logs + K8s events + Prometheus context" and produces:

- one-line summary
- ranked root causes
- supporting evidence / counter evidence
- timeline
- next actions
- items that still need human confirmation

### 3. Single Source of Truth for Current Problem Events

Current problem events now use OpenSearch `inspection-incidents-*` as the
primary source of truth.

Local JSON artifacts are treated only as:

- cache
- fallback
- exportable intermediate artifacts

### 4. Investigation Target Recommendation

The recommendation list combines:

- incident risk level
- runbook availability
- investigation history count
- restart total / restart increase
- strong signals such as OOMKilled, CrashLoopBackOff, and NotReady
- current source fingerprint scoped incidents and investigations

### 5. Visualization and Operator Workbench

The stack currently exposes two main entry points:

- OpenSearch Dashboards
  for Discover, saved searches, and dashboard visualizations
- local `dashboard-rca`
  for running investigations, reviewing summary cards, and opening logs/events/dashboard links

## Main Index Families

- `logs-k8s-*`
- `events-k8s-*`
- `inspection-incidents-*`
- `inspection-investigations-*`

## Main Normalized Fields

- `cluster`
- `namespace`
- `pod`
- `container`
- `node`
- `service`
- `severity`
- `logger`
- `message`
- `message_normalized`
- `exception_type`
- `exception_message`
- `stack_language`

## Typical Usage

### Scenario 1: On-call Triage

1. Open `Current Incidents`
2. Pick a high-risk Pod
3. Click `Run RCA`
4. Review summary cards and ranked root causes
5. Open logs, events, or dashboard directly

### Scenario 2: Postmortem Review

1. Open `Investigations - Recent`
2. Filter by namespace or object
3. Review historical investigation output and evidence snapshots

### Scenario 3: Codex / MCP Driven Operation

The stack is already wired for MCP and Skill-based use, so the next layer can
call:

- log search
- event search
- incident listing
- one-click investigation

## Current Delivery Scope

Already implemented in the current stack:

- log normalization and structured exception extraction
- incidents as the single operational truth source
- enriched investigation target ranking
- RCA summary cards
- upgraded dashboards and charts
- OpenSearch retention / snapshot / disk watermark baseline

## Storage Model

Investigation storage is now split into hot and cold layers:

- hot metadata store:
  MySQL in the shared deployment, used for recent investigation lists and fast reads
- cold archive store:
  MinIO object storage, used for full archived payloads
- search store:
  OpenSearch investigation documents for retrieval and analytics

Recommended next steps:

- scheduled snapshots
- OpenSearch authentication and security hardening
- more granular log parsers
- RCA page text and interaction polishing
