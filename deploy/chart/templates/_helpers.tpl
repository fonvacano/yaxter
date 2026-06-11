{{/*
Expand the name of the chart.
*/}}
{{- define "yaxter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "yaxter.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Chart label (name + version).
*/}}
{{- define "yaxter.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "yaxter.labels" -}}
helm.sh/chart: {{ include "yaxter.chart" . }}
{{ include "yaxter.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels (stable — used in matchLabels).
*/}}
{{- define "yaxter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "yaxter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Full image reference (repository:tag).
*/}}
{{- define "yaxter.image" -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag }}
{{- end }}

{{/*
Name of the k8s Secret that holds app secrets (created by ExternalSecret or manually).
*/}}
{{- define "yaxter.secretName" -}}
{{- printf "%s-secrets" (include "yaxter.fullname" .) }}
{{- end }}

{{/*
Name of the api ConfigMap.
*/}}
{{- define "yaxter.apiConfigMapName" -}}
{{- printf "%s-api-config" (include "yaxter.fullname" .) }}
{{- end }}

{{/*
Name of the PgBouncer Service (used as POSTGRES_DSN host by api/worker).
*/}}
{{- define "yaxter.pgbouncerServiceName" -}}
{{- printf "%s-pgbouncer" (include "yaxter.fullname" .) }}
{{- end }}
