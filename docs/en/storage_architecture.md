# RCA Storage Architecture

## Goal

Investigation storage is now split into hot and cold layers so that:

- frequently read metadata stays fast
- full archived payloads move to cheaper long-term storage
- search and analytics stay in OpenSearch

## Current Storage Layers

### 1. Hot Storage

Hot investigation metadata is used for:

- recent investigation lists
- frequent summary lookups
- RCA workbench history cards
- recommendation ranking inputs

Supported drivers:

- SQLite
- MySQL

Current shared deployment:

- MySQL
- database: `auto_inspection`
- table: `investigation_metadata`

## 2. Cold Storage

Cold storage keeps the full archived investigation payloads.

Current driver:

- MinIO

Current bucket:

- `auto-inspection-archive`

Object path layout:

- `investigations/YYYY/MM/DD/<investigation_id>.json`

## 3. Search Layer

OpenSearch still stores investigation documents for:

- search
- aggregation
- dashboard visualizations

Index family:

- `inspection-investigations-*`

## 4. Write Path

Each completed investigation now writes to:

1. local JSON snapshot
2. OpenSearch investigation index
3. hot metadata store
4. cold archive store

## 5. Read Path

Read priority:

1. local file cache
2. hot store pointer
3. cold archive object

Recent investigation list priority:

1. hot metadata store
2. local filesystem fallback

## 6. Why This Split Works

### Hot Store

Best for:

- low-latency reads
- compact structured data
- history lists and summary cards

### Cold Store

Best for:

- full JSON payloads
- long retention
- replay and export

### OpenSearch

Best for:

- search
- correlation
- charts
- text-oriented evidence retrieval

## 7. Shared Deployment

The shared RCA service currently uses:

- MySQL at `mysql-31326`
- MinIO in namespace `observability`

## 8. Recommended Next Improvements

- move MySQL credentials and MinIO credentials fully into dedicated secrets
- add migration support for investigation metadata schema
- add archive lifecycle policy in MinIO
- add a recovery endpoint that can rehydrate a full investigation from cold storage
