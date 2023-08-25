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
helm upgrade kube-green \
--namespace=kube-green \
--create-namespace=true \
./charts/kube-green --install 
```


# Deploy Kube-Green Helm Chart with Terraform

This example show how to use [Terraform Helm Chart Provider](https://developer.hashicorp.com/terraform/tutorials/kubernetes/helm-provider) to deploy `Kube-Green` on Kubernetes Clusters. 

## Prerequisites 
*   [Terraform Install](https://developer.hashicorp.com/terraform/tutorials/aws-get-started/install-cli)
*   [Helm Provider Credentials Setup](https://developer.hashicorp.com/terraform/tutorials/kubernetes/helm-provider#review-the-helm-configuration)

## Installation

We need to install `cert-manager` as dependency before `kube-green` installation. To provision the both resources in same terraform run, you can declare helm release from cert_manager as dependency from kube-green helm helease using [depends_on](https://developer.hashicorp.com/terraform/language/meta-arguments/depends_on) meta-argument. 

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


After configuration `helm_releases`, you can run terraform cli to provisioning kube-green installation properly. 

```hcl
terraform plan 
terraform apply --auto-approve
```