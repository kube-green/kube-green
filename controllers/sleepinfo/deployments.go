package sleepinfo

import (
	"context"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SleepInfoReconciler) getDeploymentsList(ctx context.Context, namespace string, sleepInfo *kubegreenv1alpha1.SleepInfo) ([]appsv1.Deployment, error) {
	d := deployments{
		k8sClient: r.Client,
		sleepInfo: sleepInfo,
		log:       r.Log,
	}

	log := d.log.WithValues("namespace", namespace)

	deploymentList, err := d.getListByNamespace(ctx, namespace)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("deployments in namespace", "number of deployment", len(deploymentList))
	return d.filterExcludedDeployment(deploymentList), nil
}

type deployments struct {
	k8sClient client.Client
	sleepInfo *kubegreenv1alpha1.SleepInfo
	log       logr.Logger
}

func (d *deployments) getListByNamespace(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}
	deployments := appsv1.DeploymentList{}
	if err := d.k8sClient.List(ctx, &deployments, listOptions); err != nil {
		return deployments.Items, client.IgnoreNotFound(err)
	}
	return deployments.Items, nil
}

func (d *deployments) filterExcludedDeployment(deploymentList []appsv1.Deployment) []appsv1.Deployment {
	excludedDeploymentName := getExcludedDeploymentName(d.sleepInfo)
	filteredList := []appsv1.Deployment{}
	for _, deployment := range deploymentList {
		if !excludedDeploymentName[deployment.Name] {
			filteredList = append(filteredList, deployment)
		}
	}
	return filteredList
}

func getExcludedDeploymentName(sleepInfo *kubegreenv1alpha1.SleepInfo) map[string]bool {
	excludedDeploymentName := map[string]bool{}
	for _, exclusion := range sleepInfo.GetExcludeRef() {
		if exclusion.Kind == "Deployment" && exclusion.ApiVersion == "apps/v1" {
			excludedDeploymentName[exclusion.Name] = true
		}
	}
	return excludedDeploymentName
}
