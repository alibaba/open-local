{{/* vim: set filetype=mustache: */}}

{{/* Expand the name of the chart.*/}}

{{/* labels for helm resources */}}
{{- define "local.labels" -}}
labels:
  app.kubernetes.io/managed-by: "{{ .Release.Service }}"
  app.kubernetes.io/name: "{{ .Values.name }}"
  app.kubernetes.io/version: "{{ .Chart.AppVersion }}"
  helm.sh/chart: "{{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}"
  {{- if .Values.customLabels }}
{{ toYaml .Values.customLabels | indent 2 -}}
  {{- end }}
{{- end -}}
