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

{{/* Allow KubeVersion to be overridden. */}}
{{- define "kubeVersion" -}}
  {{- default .Capabilities.KubeVersion.Version -}}
{{- end -}}

{{/* set k8s scheduler init job Version */}}
{{/* see https://github.com/kubernetes/kubernetes/pull/105424 */}}
{{- define "init.job.version" -}}
  {{- if and (.Capabilities.APIVersions.Has "networking.k8s.io/v1") (semverCompare ">= 1.23-0" (include "kubeVersion" .)) -}}
      {{- print "new" -}}
  {{- else -}}
    {{- print "old" -}}
  {{- end -}}
{{- end -}}