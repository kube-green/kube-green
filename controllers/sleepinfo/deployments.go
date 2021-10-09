package sleepinfo

import (
	"context"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SleepInfoReconciler) getDeploymentsByNamespace(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}
	deployments := appsv1.DeploymentList{}
	if err := r.Client.List(ctx, &deployments, listOptions); err != nil {
		return deployments.Items, client.IgnoreNotFound(err)
	}
	return deployments.Items, nil
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

func filterExcludedDeployment(deploymentList []appsv1.Deployment, sleepInfo *kubegreenv1alpha1.SleepInfo) []appsv1.Deployment {
	excludedDeploymentName := getExcludedDeploymentName(sleepInfo)
	filteredList := []appsv1.Deployment{}
	for _, deployment := range deploymentList {
		if !excludedDeploymentName[deployment.Name] {
			filteredList = append(filteredList, deployment)
		}
	}
	return filteredList
}
