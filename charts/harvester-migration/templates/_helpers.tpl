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

{{/*
Non-empty when the bundled controller's minor version differs from the cluster's
Harvester minor. Harvester ships its own controller per release, so a controller
from another minor is untested against that cluster's CRDs and APIs (the 0.3.0
chart shipped a 1.9 controller to a 1.8.2 lab). The cluster version comes from the
server-version Setting; controller.bundledVersion mirrors the Chart.yaml dependency
(hack/check-version-warning.sh keeps them equal); controller.clusterVersionOverride replaces it where the
Setting cannot be read (and in tests, since lookup is empty under `helm template`).
*/}}
{{- define "harvester-migration.controllerVersionWarning" -}}
{{- if .Values.controller.enabled -}}
{{- $cluster := .Values.controller.clusterVersionOverride | default "" -}}
{{- if not $cluster -}}
{{- $s := lookup "harvesterhci.io/v1beta1" "Setting" "" "server-version" -}}
{{- if and $s $s.value -}}{{- $cluster = $s.value -}}{{- end -}}
{{- end -}}
{{- $bundled := .Values.controller.bundledVersion | default "" -}}
{{- /* Only major.minor matters, and a version that does not start with one (a dev build such as "master-head") is skipped rather than failing the install. */ -}}
{{- $cm := regexFind "^v?[0-9]+\\.[0-9]+" $cluster | trimPrefix "v" -}}
{{- $bm := regexFind "^v?[0-9]+\\.[0-9]+" $bundled | trimPrefix "v" -}}
{{- if and $cm $bm (ne $cm $bm) -}}
This chart bundles VM Import Controller {{ $bundled }}, but the cluster runs Harvester {{ $cluster }}.
Use the chart release built for Harvester {{ $cm }}, or set controller.enabled=false to keep the built-in
add-on's controller. See docs/support-matrix.md.
{{- end -}}
{{- end -}}
{{- end -}}

