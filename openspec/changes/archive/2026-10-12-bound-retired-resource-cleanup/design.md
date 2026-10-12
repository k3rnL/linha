## Context

The controller polls every second. Its retired-intent cleanup currently reads every DEAD instance, observes it, and calls Stop even on NotFound. Legacy Stop issues an additional Pod DELETE and Service GET; SparkApplication Stop performs another application GET. Stopped contexts remain in this loop indefinitely. See proposal.md for production evidence.

Late creates are possible after ambiguous API responses or controller failover. Removing all cleanup for DEAD instances would leak them. Context attachment already changes STOPPED to STARTING and clears reconcile_after. Instance retirement is irreversible; replacement capacity always has a fresh instance ID.

## Goals / Non-Goals

**Goals:** Bound missing-resource API load; reclaim late roots and legacy Services; preserve context ownership and resource identity; keep request handling independent of Kubernetes.

**Non-Goals:** Rewrite active resource observation with watches, purge historical database rows, change SDK contracts, or deploy automatically to production. Job acknowledgement, retry policies, attempt fencing, owner access, and result publication are unchanged.

## Decisions

- Extend the internal engine adapter with inventory and UID-guarded resource deletion. Resource descriptors carry context ID, instance ID, durable instance kind, adapter resource kind and UID. The controller matches durable DEAD intents; Kubernetes naming and labels remain inside the adapter.
- Sweep existing managed roots once per minute per server, starting on its first tick. Advance the local deadline even on failure. List Pods, Services and SparkApplications using namespace and managed-by/context/instance label selectors, paginating responses. Exact root names exclude Spark executor/driver children. An unavailable SparkApplication collection (404) is empty; other errors remain observable. Existing Helm list/delete permissions suffice.
- Exclude STOPPED contexts from one-second capacity reconciliation and remove its per-DEAD-instance loop. The independent sweep also covers stopped contexts. Reactivation uses the existing durable attach/ensure transaction. Ordinary active reconciliation runs before cleanup, and cleanup errors do not alter a healthy context's provisioning status.
- Before each deletion, atomically claim/renew the existing context reconciliation lease only when the listed context/instance/kind matches a durable DEAD intent. Keep the lease check adjacent to the network call; never hold a database transaction over Kubernetes I/O. Delete with the discovered UID precondition; SparkApplication deletion retains foreground propagation. Concurrent cleanup can only target an immutable retired intent and cannot touch replacement capacity with a different instance ID. A stale worker cannot reactivate a DEAD intent.
- Inventory state is ephemeral. On server restart a fresh sweep discovers late resources, while PostgreSQL remains authoritative for ownership and retirement. No schema migration is required. Returning NotFound during a deletion race is success; UID conflicts are retried from a fresh inventory on a later sweep.
- Prefer bounded inventory over per-instance backoff (still scales with historical rows) or a new watcher/cache (more recovery and lifecycle complexity for this fix). Active resource polling is unchanged.

## Risks / Trade-offs

- Orphan removal may wait up to the next successful minute sweep; immediate normal draining remains unchanged.
- Every HA replica performs the bounded inventory. Context leases serialize deletion; list traffic scales with replicas and currently existing resources, not historical instances.
- Slow API calls may delay a tick during a sweep; cleanup uses an overall timeout and always runs after active reconciliation. Errors remain visible through existing loop and Kubernetes metrics and warning logs.
- Large inventories require multiple pages; every page uses the same namespace/label filter and incomplete inventories fail safely.

## Migration Plan

Release a server image and roll all replicas. Mixed old/new replicas remain compatible but old replicas can continue the noisy cleanup until replaced. No database cleanup or client changes are required; rollback uses the previous server image. Cluster investigation for this change is read-only.
