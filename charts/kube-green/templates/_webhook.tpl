{{- define "kube-green.webhook.name" -}}
{{- printf "%s-webhook" (include "kube-green.name" .) -}}
{{- end -}}

{{- define "kube-green.webhook.serviceAccount.name" -}}
{{- include  "kube-green.webhook.name" . -}}
{{- end -}}

{{- define "kube-green.webhook.service.name" -}}
kube-green-webhook-service
{{- end -}}
