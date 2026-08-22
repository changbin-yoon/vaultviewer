{{/*
Chart name, truncated and DNS-1123-safe.
*/}}
{{- define "vaultviewer.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully qualified app name — <release>-vaultviewer, or just the release name
when it already contains the chart name.
*/}}
{{- define "vaultviewer.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "vaultviewer.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "vaultviewer.labels" -}}
helm.sh/chart: {{ include "vaultviewer.chart" . }}
{{ include "vaultviewer.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "vaultviewer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "vaultviewer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "vaultviewer.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- .Values.serviceAccount.name | default (include "vaultviewer.fullname" .) -}}
{{- else -}}
{{- .Values.serviceAccount.name | default "default" -}}
{{- end -}}
{{- end -}}

{{- define "vaultviewer.clusterNamespace" -}}
{{- .Values.cluster.namespace | default .Release.Namespace -}}
{{- end -}}
