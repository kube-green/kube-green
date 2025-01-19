# kube-green

![Version: 0.7.0](https://img.shields.io/badge/Version-0.7.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.7.0](https://img.shields.io/badge/AppVersion-0.7.0-informational?style=flat-square)

kube-green helm chart

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity rules for pod assignment. |
| certManager.enabled | bool | `true` | If cert-manager is enabled, the configurations will use it to manage the needed certificates. |
| crds.enabled | bool | `true` |  |
| crds.keep | bool | `true` |  |
| fullnameOverride | string | `""` |  |
| imagePullSecrets | list | `[]` | List of secrets used to access private image repositories. |
| jobsCert.enabled | bool | `false` | If enabled, the certificates will be managed by a custom Job, without the integration with the cert-manager. |
| jobsCert.image.pullPolicy | string | `"IfNotPresent"` |  |
| jobsCert.image.registry | string | `"registry.k8s.io"` |  |
| jobsCert.image.repository | string | `"ingress-nginx/kube-webhook-certgen"` |  |
| jobsCert.image.tag | string | `"v20221220-controller-v1.5.1-58-g787ea74b6"` |  |
| manager.image.pullPolicy | string | `"IfNotPresent"` | Defines the image pull policy. Avoids pulling the image if it's already present. |
| manager.image.repository | string | `"kubegreen/kube-green"` | The Docker image repository for the kube-green manager application. |
| manager.image.tag | string | `"0.7.0"` | The specific image tag of the kube-green manager to use. |
| manager.metrics.enabled | bool | `true` | If enabled, the manager will expose metrics. |
| manager.metrics.port | int | `8443` | The address to bind the metrics server. |
| manager.metrics.secure | bool | `true` | If true, the metrics server will use a secure connection via HTTPS. Set it to false to use HTTP instead. The certificate will be created in a secret called "metrics-server-cert". |
| manager.resources.limits.cpu | string | `"400m"` | Maximum CPU allowed. |
| manager.resources.limits.memory | string | `"400Mi"` | Maximum memory allowed. |
| manager.resources.requests.cpu | string | `"100m"` | Requested CPU to guarantee for the pod. |
| manager.resources.requests.memory | string | `"50Mi"` | Requested memory to guarantee for the pod. |
| manager.securityContext.allowPrivilegeEscalation | bool | `false` | Prevents the pod from gaining additional privileges. Set to false for security. |
| manager.securityContext.capabilities.drop[0] | string | `"ALL"` | Drops all Linux capabilities for the pod, enhancing security. |
| nameOverride | string | `""` |  |
| nodeSelector | object | `{}` | Node labels for pod assignment. |
| podAnnotations | object | `{}` | Annotations to add to each pod. |
| podSecurityContext | object | `{}` | Security settings that apply to all containers in the pod. |
| priorityClassName | string | `""` | Priority class name for the pods. |
| rbac.customClusterRole.enabled | bool | `false` | If true, the custom ClusterRole is enabled. |
| rbac.customClusterRole.name | string | `"kube-green-manager-role-custom-aggregate"` | The name of the custom ClusterRole to aggregate with the default role managed by the chart. |
| rbac.customClusterRole.rules | list | `[]` | Rules to add to the custom ClusterRole. |
| serviceAccount.annotations | object | `{}` | Annotations to add to the service account if created. |
| serviceAccount.create | bool | `true` | Specifies whether a service account should be created for the application. |
| serviceAccount.name | string | `"kube-green-controller-manager"` | The name of the service account to use. If not set and create is true, a name is generated using the fullname template. |
| tolerations | list | `[]` | Tolerations for pod scheduling. |
| topologySpreadConstraints | list | `[]` | Topology spread constraints for pod placement. |

