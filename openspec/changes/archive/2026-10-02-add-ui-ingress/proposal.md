## Why

Administrators currently have to create their own ingress to reach Linha's console.
The Helm chart should optionally publish the UI and its same-origin admin API,
including login callbacks and downloads, with consistent host and TLS settings.

## What Changes

- Add default-disabled `ui.ingress` settings for ingress class, annotations and TLS.
- Infer one canonical ingress host from the existing `ui.publicURL`.
- Route `/ui` and `/v1/admin` prefixes to the existing named HTTP service port without rewriting paths.
- Validate UI/ingress/TLS combinations and document a usable HTTPS configuration.

## Capabilities

### New Capabilities

- `admin-ui-ingress`: Optional Helm-managed, same-origin ingress for the administrative console.

### Modified Capabilities

None.

## Impact

Changes affect the Helm chart's values/schema/templates, its rendering tests and
administrator setup documentation. Existing UI/OIDC authorization, SDK and worker
endpoints, Spark UI ingress ownership, RBAC, database schemas and container images
are unchanged. An ingress controller, DNS and the referenced TLS Secret remain
operator-managed dependencies.
