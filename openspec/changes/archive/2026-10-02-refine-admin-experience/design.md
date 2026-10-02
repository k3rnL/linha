## Context

The existing operational read model and bounded metric families support the new
views. Current UI totals cannot be derived from paginated lists. Lease records
have only a random client ID, and the JVM client does not send a hostname.

## Goals / Non-Goals

Provide an immediate operational summary and clear resource history. Keep the
full Prometheus export for diagnostics while simplifying the default dashboard.
Worker execution, durable acknowledgement, retry, cancellation, result storage,
owner isolation and stale-attempt fencing keep their existing semantics.

## Decisions

- Add a read-only `/v1/admin/overview` repository transaction using a consistent
  database timestamp, complete aggregates and five-item detail lists. Bound it
  with a timeout and index current/recent work; do not derive totals from page 1
  or depend on a Prometheus installation. Recent outcomes use terminal update
  time; processing uses completed attempt durations, excluding queue wait.
- Add optional `hostname` to ensure/attach/lease observation contracts. Schema 10
  adds a lease column with an empty legacy default and overview indexes. SDK
  discovery uses HOSTNAME, then the JVM host resolver, with an empty fallback;
  it runs once per client and never changes the UUID used for identity.
- Add server-side `state=active` (non-DEAD) and exact instance-state filters so
  history pagination cannot hide a current driver. History is lazy and collapsed;
  draining instances remain current but their allocation is marked last known.
- Use a small set of primary Grafana panels with explicit current/selected-period
  labels, independent timing charts and resource requests/limits. Preserve all
  dimensions before deduplicating; derive range counts from durable counters.
  Hide diagnostics inside a collapsed row rather than remove their metrics.
- Preserve unknowns: no-execution durations are null, collection errors show an
  error, and incomplete Spark scopes do not silently sum only healthy contexts.

## Risks / Trade-offs

- Host metadata is client-reported → treat it as display information, escape it,
  bound it, and never use it for authorization or metric labels.
- Large retained history → bounded recent/current indexed queries and five-item
  detail lists; test totals beyond list size and query timeout behavior.
- Prometheus range counts are estimates from scrapes → label the time window and
  keep exact persisted job inspection in the console. No fabricated zero on outage.
- Existing clients lack hostnames → explicit fallback until the client SDK is
  upgraded/re-attached. Server upgrade must precede upgraded SDKs.

## Migration Plan

Deploy schema 10 with the server/UI image, upgrade JVM clients to report hostnames,
and upgrade the chart/import dashboard version 2 under the existing dashboard UID.
Old clients continue to work. Schema rollback is not automatic; older servers
reject the new schema. Historical validation evidence remains labelled as such.
