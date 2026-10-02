{{- define "linha.name" -}}{{ .Release.Name }}-linha{{- end -}}

{{- define "linha.validate" -}}
{{- if or (hasKey .Values "mode") (hasKey .Values "deploymentMode") (hasKey .Values "existingSecret") (hasKey .Values "oidc") -}}
{{- fail "Legacy mode/deploymentMode/existingSecret/oidc values were removed: use database.postgres, security, and s3.secretRef; see docs/operations.md" -}}
{{- end -}}
{{- if and .Values.security.enabled .Values.security.oidc.enabled -}}
{{- $_ := required "security.oidc.issuer is required when OIDC is enabled" .Values.security.oidc.issuer -}}
{{- $_ := required "security.oidc.audience is required when OIDC is enabled" .Values.security.oidc.audience -}}
{{- end -}}
{{- range $key := list "host" "database" "secretRef" "usernameKey" "passwordKey" -}}
{{- $_ := required (printf "database.postgres.%s is required" $key) (index $.Values.database.postgres $key) -}}
{{- end -}}
{{- end -}}

{{- define "linha.observability.validate" -}}
{{- if .Values.ui.enabled -}}
{{- $_ := required "ui.publicURL is required when UI is enabled (absolute URL ending /ui/)" .Values.ui.publicURL -}}
{{- if not (regexMatch "^https?://[^/?#]+/ui/$" .Values.ui.publicURL) -}}{{- fail "ui.publicURL must be an absolute http(s) URL ending /ui/" -}}{{- end -}}
{{- if .Values.security.enabled -}}
{{- if not .Values.security.oidc.enabled -}}{{- fail "UI requires security.oidc.enabled when security is enabled" -}}{{- end -}}
{{- $_ := required "ui.oidc.clientId is required when UI security is enabled" .Values.ui.oidc.clientId -}}
{{- end -}}
{{- end -}}
{{- range $labels := list .Values.metrics.serviceMonitor.labels .Values.metrics.grafanaDashboard.labels -}}
{{- range $key,$value := $labels -}}
{{- if has $key (list "app.kubernetes.io/name" "app.kubernetes.io/instance" "app.kubernetes.io/component" "linha_deployment" "cluster") -}}{{- fail (printf "monitoring label %s is reserved" $key) -}}{{- end -}}
{{- end -}}
{{- end -}}
{{- if .Values.metrics.serviceMonitor.enabled -}}
{{- if not .Values.metrics.enabled -}}{{- fail "ServiceMonitor requires metrics.enabled=true" -}}{{- end -}}
{{- if not (.Capabilities.APIVersions.Has "monitoring.coreos.com/v1/ServiceMonitor") -}}{{- fail "ServiceMonitor CRD is required; offline template: --api-versions monitoring.coreos.com/v1/ServiceMonitor" -}}{{- end -}}
{{- $interval := trimSuffix "s" .Values.metrics.serviceMonitor.interval | atoi -}}
{{- $timeout := trimSuffix "s" .Values.metrics.serviceMonitor.scrapeTimeout | atoi -}}
{{- if gt $timeout $interval -}}{{- fail "ServiceMonitor scrapeTimeout must not exceed interval" -}}{{- end -}}
{{- end -}}
{{- if .Values.metrics.grafanaDashboard.enabled -}}
{{- if not (.Capabilities.APIVersions.Has "grafana.integreatly.org/v1beta1/GrafanaDashboard") -}}{{- fail "GrafanaDashboard CRD is required; offline template: --api-versions grafana.integreatly.org/v1beta1/GrafanaDashboard" -}}{{- end -}}
{{- $_ := required "metrics.grafanaDashboard.instanceSelector is required" .Values.metrics.grafanaDashboard.instanceSelector -}}
{{- end -}}
{{- end -}}

{{- define "linha.ui.ingress.host" -}}
{{- $url := urlParse .Values.ui.publicURL -}}
{{- regexReplaceAll ":[0-9]+$" (get $url "host") "" -}}
{{- end -}}

{{- define "linha.ui.ingress.validate" -}}
{{- if not .Values.ui.enabled -}}{{- fail "ui.ingress.enabled requires ui.enabled=true" -}}{{- end -}}
{{- if not (regexMatch "^https?://[a-z0-9.-]+(:[0-9]+)?/ui/$" .Values.ui.publicURL) -}}
{{- fail "ui.publicURL must contain a concrete DNS host for UI ingress (no credentials, wildcard or IP literals)" -}}
{{- end -}}
{{- $host := include "linha.ui.ingress.host" . -}}
{{- if or (gt (len $host) 253) (not (regexMatch "^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?([.][a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?)*$" $host)) (regexMatch "^([0-9]{1,3}[.]){3}[0-9]{1,3}$" $host) -}}
{{- fail "ui.publicURL must contain a valid DNS host for UI ingress, not an IP literal" -}}
{{- end -}}
{{- if .Values.ui.ingress.tls.enabled -}}
{{- if not (hasPrefix "https://" .Values.ui.publicURL) -}}{{- fail "ui.ingress.tls.enabled requires an https ui.publicURL" -}}{{- end -}}
{{- $_ := required "ui.ingress.tls.secretName is required when ingress TLS is enabled" .Values.ui.ingress.tls.secretName -}}
{{- end -}}
{{- end -}}
