package deployments

import (
	"context"
	"encoding/json"
	"fmt"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/davidebianchi/kube-green/controllers/sleepinfo/resource"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type deployments struct {
	resource.ResourceClient
	data             []appsv1.Deployment
	OriginalReplicas map[string]int32
}

func NewResource(ctx context.Context, res resource.ResourceClient, namespace string, originalReplicas map[string]int32) (deployments, error) {
	d := deployments{
		ResourceClient:   res,
		OriginalReplicas: originalReplicas,
	}
	if err := d.fetch(ctx, namespace); err != nil {
		return deployments{}, err
	}

	return d, nil
}

func (d deployments) HasResource() bool {
	return len(d.data) > 0
}

func (d deployments) Sleep(ctx context.Context) error {
	for _, deployment := range d.data {
		deploymentReplicas := *deployment.Spec.Replicas
		if deploymentReplicas == 0 {
			continue
		}
		newDeploy := deployment.DeepCopy()
		*newDeploy.Spec.Replicas = 0

		if err := d.Patch(ctx, &deployment, newDeploy); err != nil {
			return err
		}
	}
	return nil
}

func (d deployments) WakeUp(ctx context.Context) error {
	for _, deployment := range d.data {
		if *deployment.Spec.Replicas != 0 {
			d.Log.Info("replicas not 0 during wake up", "deployment name", deployment.Name)
			continue
		}

		replica, ok := d.OriginalReplicas[deployment.Name]
		if !ok {
			d.Log.Info(fmt.Sprintf("replicas info not set on secret in namespace %s for deployment %s", deployment.Namespace, deployment.Name))
			continue
		}

		newDeploy := deployment.DeepCopy()
		*newDeploy.Spec.Replicas = replica

		if err := d.Patch(ctx, &deployment, newDeploy); err != nil {
			return err
		}
	}
	return nil
}

func (d *deployments) fetch(ctx context.Context, namespace string) error {
	log := d.Log.WithValues("namespace", namespace)

	deploymentList, err := d.getListByNamespace(ctx, namespace)
	if err != nil {
		return err
	}
	log.V(1).Info("deployments in namespace", "number of deployment", len(deploymentList))
	d.data = d.filterExcludedDeployment(deploymentList)
	return nil
}

func (d deployments) getListByNamespace(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}
	deployments := appsv1.DeploymentList{}
	if err := d.Client.List(ctx, &deployments, listOptions); err != nil {
		return deployments.Items, client.IgnoreNotFound(err)
	}
	return deployments.Items, nil
}

func (d deployments) filterExcludedDeployment(deploymentList []appsv1.Deployment) []appsv1.Deployment {
	excludedDeploymentName := getExcludedDeploymentName(d.SleepInfo)
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

type OriginalReplicas struct {
	Name     string `json:"name"`
	Replicas int32  `json:"replicas"`
}

func (d deployments) GetOriginalInfoToSave() ([]byte, error) {
	originalDeploymentsReplicas := []OriginalReplicas{}
	for _, deployment := range d.data {
		originalReplicas := *deployment.Spec.Replicas
		if replica, ok := d.OriginalReplicas[deployment.Name]; ok {
			if ok && replica != 0 {
				originalReplicas = replica
			}
		}
		if originalReplicas == 0 {
			continue
		}
		originalDeploymentsReplicas = append(originalDeploymentsReplicas, OriginalReplicas{
			Name:     deployment.Name,
			Replicas: originalReplicas,
		})
	}
	return json.Marshal(originalDeploymentsReplicas)
}

func GetOriginalInfoToRestore(data []byte) (map[string]int32, error) {
	if data == nil {
		return map[string]int32{}, nil
	}
	originalDeploymentsReplicas := []OriginalReplicas{}
	originalDeploymentsReplicasData := map[string]int32{}
	if err := json.Unmarshal(data, &originalDeploymentsReplicas); err != nil {
		return nil, err
	}
	for _, replicaInfo := range originalDeploymentsReplicas {
		if replicaInfo.Name != "" {
			originalDeploymentsReplicasData[replicaInfo.Name] = replicaInfo.Replicas
		}
	}
	return originalDeploymentsReplicasData, nil
}
