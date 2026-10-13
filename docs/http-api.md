# HTTP API reference

The base protocol is JSON over HTTP. Typed SDK calls wrap the same envelopes.
The full [OpenAPI document](https://github.com/k3rnL/linha/blob/main/api/openapi.json)
contains request/response schemas and bounds; these tables are a lookup guide.

## Public routes

Authenticated requests use `Authorization: Bearer <access-token>`. Ownership is
derived from that token, never from caller-supplied JSON.

| Method / route | Purpose |
| --- | --- |
| `POST /v1/contexts:ensure` | Derive a backend version and acquire a client lease |
| `GET /v1/contexts/{context}` | Context status |
| `POST /v1/contexts/{context}/clients` | Attach to a persisted version |
| `POST /v1/contexts/{context}/clients/{lease}/renew` | Renew a live lease |
| `DELETE /v1/contexts/{context}/clients/{lease}` | Release a lease idempotently |
| `POST /v1/contexts/{context}/jobs` | Persist a new job; requires `X-Linha-Client-Lease` |
| `GET /v1/jobs` | Owner-scoped job list with bounded pagination |
| `GET /v1/jobs/{job}` | Status, progress and final failure |
| `GET /v1/jobs/{job}/attempts` | Persisted attempts |
| `POST /v1/jobs/{job}:cancel` | Request cancellation |
| `GET /v1/jobs/{job}/result` | Published result metadata |
| `GET /v1/jobs/{job}/files/{output}` | Stream an authorized file |
| `GET /v1/jobs/{job}/parts` | Paginated dataset part metadata |
| `GET /v1/jobs/{job}/part?path=...` | Stream an authorized dataset part |

Ensure accepts `name`, `clientId`, optional `hostname` and `spec`; the response
includes `id`, `version` and `clientLease`. Polling does not renew a lease.

A job envelope is:

```json
{
  "handler": "count-rows",
  "version": 1,
  "payload": {"size": 1000},
  "idempotencyKey": "request-42",
  "retry": {"maxAttempts": 1, "backoffSeconds": 1}
}
```

A raw worker registers the same handler/version and maps the payload to business
code. Duplicate capability registration is rejected. Classes and executable
closures do not travel through this envelope.

## State and errors

Job states are `QUEUED`, `RETRYING`, `RUNNING`, `CANCELLING`, `SUCCEEDED`, `FAILED`
and `CANCELLED`. Retrying work can be in backoff. Final result retrieval requires
successful publication and an unexpired retained payload.

Errors use this envelope:

```json
{"error":{"code":"FORBIDDEN","message":"administrator role required"}}
```

| Code / condition | Meaning |
| --- | --- |
| `UNAUTHORIZED` | Missing or invalid credentials |
| `FORBIDDEN` | Valid identity lacks the required admin action |
| `CLIENT_LEASE_EXPIRED` | Attach again to acquire a new context lease |
| Idempotency conflict | Same owner/name/key with different request or policy |
| `SCHEMA_MISMATCH` | Typed result descriptor differs from the retained descriptor |
| `RESULT_EXPIRED` | Payload retention ended; status metadata remains |
| `DESTINATION_CHANGED` | Stored destination identity no longer matches configuration |
| `DATASET_UNSUPPORTED` | Destination cannot safely delegate a distributed write |
| `STAGING_LIMIT_EXCEEDED` | Concurrent staging reservation budget exhausted |

Foreign job identifiers are not a way to access another owner's history.
See [failure diagnosis](job-failures.md) for exceptions and traces.

## Worker and administrator routes

Worker registration, heartbeat, claim, renew, output allocation/upload,
part registration/sealing, completion and failure routes use managed worker
identity plus current-attempt fencing. They are runtime protocol operations,
not application OAuth requests.

`/v1/admin/*` is a separate cross-owner inspection/action surface. It requires
viewer/operator membership from the browser issuer. Mutations require operator
access; cookie mutations also require origin/CSRF checks. The
[console guide](admin-ui.md) covers setup.

## Operational routes

| Route | Response purpose |
| --- | --- |
| `/livez` | Process liveness |
| `/readyz` | Database/schema and configured local storage readiness |
| `/metrics` | Prometheus exposition when enabled |
| `/ui/` | Administrator console when enabled |

There is no `/health` Prometheus route. Scrapes do not acquire context leases.
