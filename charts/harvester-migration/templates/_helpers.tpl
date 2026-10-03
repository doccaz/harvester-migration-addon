{{/* Full name of this release's resources. */}}
{{- define "harvester-migration.fullname" -}}
{{- if contains .Chart.Name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/* UI resources carry a -ui suffix so they never collide with the controller's. */}}
{{- define "harvester-migration.ui.fullname" -}}
{{- printf "%s-ui" (include "harvester-migration.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "harvester-migration.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "harvester-migration.ui.selectorLabels" -}}
app.kubernetes.io/name: harvester-migration-ui
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "harvester-migration.ui.labels" -}}
helm.sh/chart: {{ include "harvester-migration.chart" . }}
{{ include "harvester-migration.ui.selectorLabels" . }}
app.kubernetes.io/part-of: harvester-migration
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "harvester-migration.ui.serviceAccountName" -}}
{{- if .Values.ui.serviceAccount.create }}
{{- default (include "harvester-migration.ui.fullname" .) .Values.ui.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.ui.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "harvester-migration.ui.image" -}}
{{- printf "%s:%s" .Values.ui.image.repository (.Values.ui.image.tag | default .Chart.AppVersion) -}}
{{- end }}

{{- define "harvester-migration.exportClaimName" -}}
{{- default (printf "%s-exports" (include "harvester-migration.fullname" .)) .Values.export.storage.existingClaim -}}
{{- end -}}

{{/* Secret holding the CA that signs the user-token API endpoint. */}}
{{- define "harvester-migration.ui.caSecretName" -}}
{{- default (printf "%s-api-ca" (include "harvester-migration.ui.fullname" .)) .Values.ui.auth.ca.existingSecret -}}
{{- end -}}

{{- define "harvester-migration.exportTicketSecretName" -}}
{{ include "harvester-migration.ui.fullname" . }}-export-ticket-key
{{- end }}
