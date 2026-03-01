{{/*
Expand the name of the chart.
*/}}
{{- define "node-role-controller.name" -}}
node-role-controller
{{- end }}

{{/*
Create a default fully qualified app name.
Hardcoded to match existing resource names.
*/}}
{{- define "node-role-controller.fullname" -}}
node-role-controller
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "node-role-controller.chart" -}}
{{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "node-role-controller.labels" -}}
helm.sh/chart: {{ include "node-role-controller.chart" . }}
{{ include "node-role-controller.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "node-role-controller.selectorLabels" -}}
app: {{ include "node-role-controller.fullname" . }}
{{- end }}
