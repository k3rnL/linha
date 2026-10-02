## Why

Administrators currently need API calls and Kubernetes inspection to understand a context, inspect its requests, or repeat a failed execution. Linha needs an administration console that connects durable request history to live engine information and supports deliberate request creation, replay, and cancellation.

## What Changes

- Add a React, TypeScript, and Vite frontend with shadcn/ui, Tailwind CSS, and TanStack Query, built into the Go server and enabled through Helm.
- Make contexts the landing page, grouped by owner and logical name with immutable configuration versions, clients, instances, configuration, conditions, and associated requests.
- Show engine-specific detail through a common observation contract. For Spark, expose drivers, executors, application status, and existing SparkApplication ingress links without creating a UI proxy or an ingress.
- Add global and context-filtered request browsing, attempts, exception diagnostics, and primitive result inspection: bounded JSON previews, published local/S3 locations, file metadata, and paginated dataset parts.
- Add a shared create-request page. Replay prefills it with editable original parameters and an explicitly selected context version; submission creates a linked new durable request.
- Add administrator cancellation using existing request-scoped cancellation and attempt fencing.
- Introduce explicit server-side administrator authorization, browser OIDC login, and durable attribution of administrative mutations while preserving ordinary SDK owner isolation.
- Share internal operational observations with `add-platform-observability`; the UI does not require Prometheus or Grafana to run.

## Capabilities

### New Capabilities

- `admin-web-ui`: Optional embedded console, administrator access, navigation, context/request inspection, engine links, result presentation, and HA browser behavior.
- `administrative-request-management`: Audited cross-owner creation, edited replay, cancellation, and accepted-work lifetime independent of browser sessions.

### Modified Capabilities

- `durable-request-management`: Make the explicit administrative authorization boundary compatible with otherwise unchanged owner-scoped SDK observation and mutation.

## Impact

- New `web/` build and browser tests; Go static assets, authenticated admin APIs, OpenAPI, OIDC configuration, PostgreSQL migrations, and audit/provenance records.
- Helm UI/login configuration and Secret references; existing SDK transport and worker business code remain compatible.
- Depends on the internal observation contracts and persisted runtime snapshots in `add-platform-observability`; frontend/API implementation can proceed against those contracts before monitoring deployment is complete.
- No production deployment, dependency installation in the cluster, or implementation is included in this planning change.
