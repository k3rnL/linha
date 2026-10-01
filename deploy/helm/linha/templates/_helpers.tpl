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
