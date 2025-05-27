{{- define "kube-green.metrics.service.name" -}}
kube-green-controller-manager-metrics-service
{{- end -}}

{{- define "kube-green.metrics.secret.name" -}}
metrics-server-cert
{{- end -}}

{{- define "kube-green.metrics.name" -}}
{{- printf "%s-metrics" (include "kube-green.name" .) -}}
{{- end -}}

{{- define "kube-green.metrics.serviceAccount.name" -}}
{{- include  "kube-green.metrics.name" . -}}
{{- end -}}

{{- define "kube-green.metrics.serviceMonitor.name" -}}
kube-green-controller-manager-metrics-servicemonitor
{{- end -}}
