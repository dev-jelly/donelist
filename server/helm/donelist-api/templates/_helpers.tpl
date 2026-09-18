{{/*
Expand the name of the chart.
*/}}
{{- define "donelist-api.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "donelist-api.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "donelist-api.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "donelist-api.labels" -}}
helm.sh/chart: {{ include "donelist-api.chart" . }}
{{ include "donelist-api.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
environment: {{ .Values.global.environment }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "donelist-api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "donelist-api.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "donelist-api.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "donelist-api.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Database URL
*/}}
{{- define "donelist-api.databaseURL" -}}
postgresql://{{ .Values.env.DB_USER }}:$(DB_PASSWORD)@{{ .Values.env.DB_HOST }}:{{ .Values.env.DB_PORT }}/{{ .Values.env.DB_NAME }}?sslmode={{ .Values.env.DB_SSLMODE }}
{{- end }}

{{/*
Redis URL
*/}}
{{- define "donelist-api.redisURL" -}}
redis://:$(REDIS_PASSWORD)@{{ .Values.env.REDIS_HOST }}:{{ .Values.env.REDIS_PORT }}/{{ .Values.env.REDIS_DB }}
{{- end }}
