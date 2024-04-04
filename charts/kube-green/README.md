# kube-green

![Version: 0.5.2](https://img.shields.io/badge/Version-0.5.2-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.5.2](https://img.shields.io/badge/AppVersion-0.5.2-informational?style=flat-square)

kube-green helm chart

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| imagePullSecrets | list | `[]` |  |
| kubeRbacProxy.image.pullPolicy | string | `"IfNotPresent"` |  |
| kubeRbacProxy.image.repository | string | `"gcr.io/kubebuilder/kube-rbac-proxy"` |  |
| kubeRbacProxy.image.tag | string | `"v0.15.0"` |  |
| labels | object | `{}` |  |
| manager.image.pullPolicy | string | `"IfNotPresent"` |  |
| manager.image.repository | string | `"kubegreen/kube-green"` |  |
| manager.image.tag | string | `"0.5.2"` |  |
| manager.logtostderr | bool | `true` |  |
| manager.resources.limits.cpu | string | `"400m"` |  |
| manager.resources.limits.memory | string | `"400Mi"` |  |
| manager.resources.requests.cpu | string | `"100m"` |  |
| manager.resources.requests.memory | string | `"50Mi"` |  |
| manager.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| manager.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| manager.verbosity | int | `0` |  |
| nodeSelector | object | `{}` |  |
| podAnnotations | object | `{}` |  |
| podSecurityContext | object | `{}` |  |
| priorityClassName | string | `""` |  |
| resources.limits.cpu | string | `"400m"` |  |
| resources.limits.memory | string | `"400Mi"` |  |
| resources.requests.cpu | string | `"100m"` |  |
| resources.requests.memory | string | `"50Mi"` |  |
| service.port | int | `80` |  |
| service.type | string | `"ClusterIP"` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `"kube-green-controller-manager"` |  |
| tolerations | list | `[]` |  |
| topologySpreadConstraints | list | `[]` |  |

