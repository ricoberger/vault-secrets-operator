{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "vault-secrets-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "vault-secrets-operator.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "vault-secrets-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "vault-secrets-operator.labels" -}}
app.kubernetes.io/name: {{ include "vault-secrets-operator.name" . }}
helm.sh/chart: {{ include "vault-secrets-operator.chart" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Values.podLabels }}
{{ toYaml .Values.podLabels }}
{{- end }}
{{- end -}}

{{/*
matchLabels
*/}}
{{- define "vault-secrets-operator.matchLabels" -}}
app.kubernetes.io/name: {{ include "vault-secrets-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Additional pod annotations
*/}}
{{- define "vault-secrets-operator.annotations" -}}
{{- if .Values.podAnnotations }}
{{- toYaml .Values.podAnnotations }}
{{- end }}
{{- end -}}

{{/*
Additional test-connection pod annotations
*/}}
{{- define "vault-secrets-operator.testPodAnnotations" -}}
{{- if .Values.testPodAnnotations }}
{{- toYaml .Values.testPodAnnotations }}
{{- end }}
{{- end }}

{{/*
Additional test-connection pod labels
*/}}
{{- define "vault-secrets-operator.testPodLabels" -}}
{{- if .Values.testPodLabels }}
{{- toYaml .Values.testPodLabels }}
{{- end }}
{{- end }}

{{/*
Create the name of the service account to use.
*/}}
{{- define "vault-secrets-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{ default (include "vault-secrets-operator.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Additional containers to add to the deployment
*/}}
{{- define "vault-secrets-operator.additionalContainers" -}}
{{- end -}}

{{/*
initContainers to add to the deployment. Example:
- name: my-init-container
  image: busy-box
  command:
    - sh
    - -c
    - my-command; my-other-command
*/}}
{{- define "vault-secrets-operator.initContainers" -}}
{{- end -}}

{{/*
Helper function for checking if a property is defined
*/}}
{{- define "vault-secrets-operator.imageRef" -}}
{{- if .Values.image.digest -}}
    @{{ .Values.image.digest }}
{{- else -}}
    :{{ .Values.image.tag }}
{{- end -}}
{{- end -}}