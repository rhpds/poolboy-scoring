{{/*
Chart name, truncated to 63 chars (K8s label limit).
*/}}
{{- define "poolboyScoring.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fully qualified app name. Uses fullnameOverride if set,
otherwise combines release name + chart name.
*/}}
{{- define "poolboyScoring.fullname" -}}
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
Chart label value: "chartname-version"
*/}}
{{- define "poolboyScoring.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to every resource.
*/}}
{{- define "poolboyScoring.labels" -}}
helm.sh/chart: {{ include "poolboyScoring.chart" . }}
{{ include "poolboyScoring.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app: {{ include "poolboyScoring.name" . }}
{{- end }}

{{/*
Selector labels — used in Deployment matchLabels and Service selector.
Must be immutable after first deploy.
*/}}
{{- define "poolboyScoring.selectorLabels" -}}
app.kubernetes.io/name: {{ include "poolboyScoring.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
ServiceAccount name — uses values override or falls back to fullname.
*/}}
{{- define "poolboyScoring.serviceAccountName" -}}
{{- if .Values.serviceAccount.name }}
{{- .Values.serviceAccount.name }}
{{- else }}
{{- include "poolboyScoring.fullname" . }}
{{- end }}
{{- end }}

{{/*
Namespace name — uses values override or release namespace.
*/}}
{{- define "poolboyScoring.namespaceName" -}}
{{- default .Release.Namespace .Values.namespace.name }}
{{- end }}

{{/*
Container image URI. Logic:
  version == "main"  → :latest
  tagOverride == "-"  → no tag (use digest elsewhere)
  tagOverride set     → use it
  default             → use .Values.version
*/}}
{{- define "poolboyScoring.image" -}}
{{- if eq .Values.version "main" }}
{{- printf "%s:latest" .Values.image.repository }}
{{- else if eq (default "" .Values.image.tagOverride) "-" }}
{{- .Values.image.repository }}
{{- else if .Values.image.tagOverride }}
{{- printf "%s:%s" .Values.image.repository .Values.image.tagOverride }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository .Values.version }}
{{- end }}
{{- end }}
