# Install with Helm 

## Deploy cert-manager chart

```bash
helm repo add jetstack https://charts.jetstack.io

helm install \
  cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --version v1.12.0 \
  --set installCRDs=true
```

##  Install kube-green chart 

```bash
helm upgrade \
--namespace=kube-green \
--create-namespace=true \
./charts/kube-green --install 
```


### Deploy Locally with Terraform


```hcl
resource "helm_release" "cert_manager" {
    namespace        = "cert-manager"
    create_namespace = true

    name       = "cert-manager"
    repository = "https://charts.jetstack.io"
    chart      = "cert-manager"
    version    = "v1.12.0"

    set {
        name  = "installCRDs"
        value = true
    }   

    set {
        name  = "webhook.timeoutSeconds"
        value = 10
    }
}

```hcl 
resource "helm_release" "kube_green" {
    namespace           = "kube-green"
    create_namespace    = true

    name                = "kube-green"
    chart               = "./charts/kube-green"

    set {
        name    = "image.repository"
        value   = "kubegreen/kube-green"
    }

    set {
        name    = "image.tag"
        value   = "0.5.1"
    }

    depends_on = [
        helm_release.cert_manager
    ]
}
```