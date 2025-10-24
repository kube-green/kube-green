# Install with Helm

## Using cert-manager

To use kube-green, it is necessary to have a valid certificate for the domain that will be used by the WebHook.
It is possible to manage it using the cert-manager, or configuring the certificate manually. [Read here](https://kube-green.dev/docs/advanced/webhook-cert-management/) for more information.

By default, this chart is configured to use the cert-manager to manage the certificate for the WebHook, and it is enabled by the values `.certManager.enabled`. It is possible to set it to *false*, and configure `.jobsCert.enabled` to *true* to manage manually the certificates, using a job which generate them and change correctly the WebHook configuration.

## Install with kube-green chart

To successfully install kube-green, in the cluster must be installed a cert-manager.
If it is not already installed, [check the cert-manager installation guide](https://cert-manager.io/docs/installation/).

To install kube-green using the helm-chart (inside the `kube-green` namespace), clone the kube-green repository and run this command:

```bash
helm upgrade kube-green \
--namespace=kube-green \
--create-namespace=true \
./charts/kube-green --install
```

## Deploy Kube-Green Helm Chart with Terraform

This example show how to use [Terraform Helm Chart Provider](https://developer.hashicorp.com/terraform/tutorials/kubernetes/helm-provider) to deploy `kube-green` on Kubernetes Clusters. 

### Prerequisites

* [Terraform Install](https://developer.hashicorp.com/terraform/tutorials/aws-get-started/install-cli)
* [Helm Provider Credentials Setup](https://developer.hashicorp.com/terraform/tutorials/kubernetes/helm-provider#review-the-helm-configuration)

### Installation

We need to install `cert-manager` as dependency before `kube-green` installation. To provision the both resources in same terraform run, you can declare helm release from cert_manager as dependency from kube-green helm release using [depends_on](https://developer.hashicorp.com/terraform/language/meta-arguments/depends_on) meta-argument.

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
```

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

After the configuration of the `helm_release`, you can run terraform cli to provisioning kube-green installation properly.

```hcl
terraform plan 
terraform apply --auto-approve
```
