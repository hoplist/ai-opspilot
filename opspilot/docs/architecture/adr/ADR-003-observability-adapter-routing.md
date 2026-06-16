# ADR-003: Route observability queries through bounded adapters

## Status
Accepted

## Context

The environment can contain multiple clusters, multiple ES/OpenSearch
datasources, regional Kibana instances, APISIX logs, application logs, and
partial network topology. Global searches across all log stores are expensive
and can disturb production ES clusters.

## Decision

Use a datasource route resolver. Given service, domain, cluster, region, status,
and time window, OpsPilot returns an ordered, bounded query plan.

Kibana is treated as UI metadata only. Elasticsearch/OpenSearch is the query
target.

## Consequences

### Positive
- Queries use the shortest likely path first.
- Missing datasource evidence is explicit.
- Production ES risk is reduced by hard query limits.

### Negative
- Correct routing depends on service and datasource catalog quality.
- Unknown domains may still require manual mapping or explicit global search.

### Neutral
- Gateway metadata can be added later without blocking service-only inspection.

## Alternatives Considered

- Search every ES by default: rejected because it is noisy and risky.
- Require perfect APISIX trace correlation: rejected because current logs do not
  guarantee shared request IDs.
