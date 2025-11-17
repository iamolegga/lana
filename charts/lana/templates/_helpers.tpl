{{/*
Expand the name of the chart.
*/}}
{{- define "lana.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Return the target Kubernetes version
*/}}
{{- define "lana.capabilities.kubeVersion" -}}
{{- if .Values.global }}
    {{- if .Values.global.kubeVersion }}
    {{- .Values.global.kubeVersion -}}
    {{- else }}
    {{- default .Capabilities.KubeVersion.Version .Values.kubeVersion -}}
    {{- end -}}
{{- else }}
{{- default .Capabilities.KubeVersion.Version .Values.kubeVersion -}}
{{- end -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "lana.fullname" -}}
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
{{- define "lana.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "lana.labels" -}}
helm.sh/chart: {{ include "lana.chart" . }}
{{ include "lana.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "lana.selectorLabels" -}}
app.kubernetes.io/name: {{ include "lana.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "lana.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "lana.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the proper Lana image name
*/}}
{{- define "lana.image" -}}
{{- $registryName := .Values.image.registry -}}
{{- $repositoryName := .Values.image.repository -}}
{{- $separator := ":" -}}
{{- $termination := .Values.image.tag | toString -}}
{{- if .Values.global }}
    {{- if .Values.global.imageRegistry }}
     {{- $registryName = .Values.global.imageRegistry -}}
    {{- end -}}
{{- end -}}
{{- if .Values.image.digest }}
    {{- $separator = "@" -}}
    {{- $termination = .Values.image.digest | toString -}}
{{- end -}}
{{- if $registryName }}
    {{- printf "%s/%s%s%s" $registryName $repositoryName $separator $termination -}}
{{- else -}}
    {{- printf "%s%s%s"  $repositoryName $separator $termination -}}
{{- end -}}
{{- end -}}

{{/*
Return the proper Docker Image Registry Secret Names
*/}}
{{- define "lana.imagePullSecrets" -}}
{{- $pullSecrets := list }}

{{- if .Values.global }}
  {{- range .Values.global.imagePullSecrets }}
    {{- $pullSecrets = append $pullSecrets . }}
  {{- end }}
{{- end }}

{{- range .Values.image.pullSecrets }}
  {{- $pullSecrets = append $pullSecrets . }}
{{- end }}

{{- if (not (empty $pullSecrets)) }}
imagePullSecrets:
{{- range $pullSecrets }}
  - name: {{ . }}
{{- end }}
{{- end }}
{{- end -}}

{{/*
Return the configmap name for Lana configuration
*/}}
{{- define "lana.configMapName" -}}
{{- if .Values.existingConfig.name }}
{{- .Values.existingConfig.name }}
{{- else }}
{{- include "lana.fullname" . }}
{{- end }}
{{- end }}

{{/*
Return true if a configmap should be created
*/}}
{{- define "lana.createConfigMap" -}}
{{- if and (not .Values.existingConfig.name) .Values.config }}
{{- true }}
{{- end }}
{{- end }}

{{/*
Return true if a secret should be created
*/}}
{{- define "lana.createSecret" -}}
{{- if and (not .Values.existingSecret.name) .Values.secrets }}
{{- true }}
{{- end }}
{{- end }}


{{/*
Get the Ingress TLS secret name
*/}}
{{- define "lana.ingress.tlsSecretName" -}}
{{- printf "%s-tls" .Values.ingress.hostname -}}
{{- end -}}

{{/*
Return podAnnotations
*/}}
{{- define "lana.podAnnotations" -}}
{{- if .Values.podAnnotations }}
{{ toYaml .Values.podAnnotations }}
{{- end }}
{{- if and (not .Values.existingConfig.name) .Values.config }}
checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
{{- end }}
{{- if and (not .Values.existingSecret.name) .Values.secrets }}
checksum/secret: {{ include (print $.Template.BasePath "/secret.yaml") . | sha256sum }}
{{- end }}
{{- end }}

{{/*
Return the appropriate apiVersion for HPA
*/}}
{{- define "lana.hpa.apiVersion" -}}
{{- if .Capabilities.APIVersions.Has "autoscaling/v2" -}}
{{- print "autoscaling/v2" -}}
{{- else -}}
{{- print "autoscaling/v2beta2" -}}
{{- end -}}
{{- end -}}

{{/*
Return the appropriate apiVersion for NetworkPolicy
*/}}
{{- define "lana.networkPolicy.apiVersion" -}}
{{- if .Capabilities.APIVersions.Has "networking.k8s.io/v1" -}}
{{- print "networking.k8s.io/v1" -}}
{{- else -}}
{{- print "networking.k8s.io/v1beta1" -}}
{{- end -}}
{{- end -}}

{{/*
Return the appropriate apiVersion for Ingress
*/}}
{{- define "lana.ingress.apiVersion" -}}
{{- if .Values.ingress.apiVersion -}}
{{- .Values.ingress.apiVersion -}}
{{- else if .Capabilities.APIVersions.Has "networking.k8s.io/v1" -}}
{{- print "networking.k8s.io/v1" -}}
{{- else if .Capabilities.APIVersions.Has "networking.k8s.io/v1beta1" -}}
{{- print "networking.k8s.io/v1beta1" -}}
{{- else -}}
{{- print "extensions/v1beta1" -}}
{{- end -}}
{{- end -}}

{{/*
Return true if cert-manager required annotations for TLS signed certificates are set in the Ingress annotations
Ref: https://cert-manager.io/docs/usage/ingress/#supported-annotations
*/}}
{{- define "lana.ingress.certManagerRequest" -}}
{{ if or (hasKey .Values.ingress.annotations "cert-manager.io/cluster-issuer") (hasKey .Values.ingress.annotations "cert-manager.io/issuer") }}
    {{- true -}}
{{- end -}}
{{- end -}}

{{/*
Return a soft podAffinity/podAntiAffinity definition
*/}}
{{- define "lana.affinities.pods.soft" -}}
preferredDuringSchedulingIgnoredDuringExecution:
  - weight: 1
    podAffinityTerm:
      labelSelector:
        matchLabels: {{- (include "lana.selectorLabels" .context) | nindent 10 }}
      topologyKey: kubernetes.io/hostname
{{- end -}}

{{/*
Return a hard podAffinity/podAntiAffinity definition
*/}}
{{- define "lana.affinities.pods.hard" -}}
requiredDuringSchedulingIgnoredDuringExecution:
  - labelSelector:
      matchLabels: {{- (include "lana.selectorLabels" .context) | nindent 8 }}
    topologyKey: kubernetes.io/hostname
{{- end -}}

{{/*
Return a podAffinity/podAntiAffinity definition
*/}}
{{- define "lana.affinities.pods" -}}
  {{- if eq .type "soft" }}
    {{- include "lana.affinities.pods.soft" . -}}
  {{- else if eq .type "hard" }}
    {{- include "lana.affinities.pods.hard" . -}}
  {{- end -}}
{{- end -}}

{{/*
Return a soft nodeAffinity definition
*/}}
{{- define "lana.affinities.nodes.soft" -}}
preferredDuringSchedulingIgnoredDuringExecution:
  - weight: 1
    preference:
      matchExpressions:
        - key: {{ .key }}
          operator: In
          values:
            {{- range .values }}
            - {{ . | quote }}
            {{- end }}
{{- end -}}

{{/*
Return a hard nodeAffinity definition
*/}}
{{- define "lana.affinities.nodes.hard" -}}
requiredDuringSchedulingIgnoredDuringExecution:
  nodeSelectorTerms:
    - matchExpressions:
        - key: {{ .key }}
          operator: In
          values:
            {{- range .values }}
            - {{ . | quote }}
            {{- end }}
{{- end -}}

{{/*
Return a nodeAffinity definition
*/}}
{{- define "lana.affinities.nodes" -}}
  {{- if eq .type "soft" }}
    {{- include "lana.affinities.nodes.soft" . -}}
  {{- else if eq .type "hard" }}
    {{- include "lana.affinities.nodes.hard" . -}}
  {{- end -}}
{{- end -}}

{{/*
Check if the Ingress API supports IngressClassName
*/}}
{{- define "lana.ingress.supportsIngressClassname" -}}
{{- if .Capabilities.APIVersions.Has "networking.k8s.io/v1/Ingress" -}}
  {{- print "true" -}}
{{- end -}}
{{- end -}}

{{/*
Check if the Ingress API supports pathType
*/}}
{{- define "lana.ingress.supportsPathType" -}}
{{- if .Capabilities.APIVersions.Has "networking.k8s.io/v1/Ingress" -}}
  {{- print "true" -}}
{{- end -}}
{{- end -}}

{{/*
Render Ingress backend
*/}}
{{- define "lana.ingress.backend" -}}
{{- $apiVersion := (include "lana.ingress.apiVersion" .context) -}}
{{- if or (eq $apiVersion "extensions/v1beta1") (eq $apiVersion "networking.k8s.io/v1beta1") -}}
serviceName: {{ .serviceName }}
servicePort: {{ .servicePort }}
{{- else -}}
service:
  name: {{ .serviceName }}
  port:
    name: {{ .servicePort }}
{{- end -}}
{{- end -}}
