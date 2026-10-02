## 1. Helm ingress

- [x] 1.1 Add default-disabled UI ingress values and typed schema for class, annotations and TLS.
- [x] 1.2 Render a namespaced v1 Ingress using the canonical public URL host, fixed UI/admin Prefix routes and existing named service port; reject invalid UI/host/TLS combinations.

## 2. Verification and documentation

- [x] 2.1 Cover disabled ingress, HTTP/HTTPS/upstream TLS, annotations/class, public URL ports, invalid hosts/configuration and namespace/HA service routing in Helm tests.
- [x] 2.2 Document an HTTPS values example, controller/DNS/Secret prerequisites, callback/Origin/path preservation and external routing compatibility.
- [x] 2.3 Run Helm rendering/lint, relevant formatting and strict OpenSpec checks; record validation scope and mark verified tasks complete.
