package deployments

import (
	"context"
	"encoding/json"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/sleepinfo/resource"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type deployments struct {
	resource.ResourceClient
	data             []appsv1.Deployment
	OriginalReplicas map[string]int32
	areToSuspend     bool
}

func NewResource(ctx context.Context, res resource.ResourceClient, namespace string, originalReplicas map[string]int32) (resource.Resource, error) {
	d := deployments{
		ResourceClient:   res,
		OriginalReplicas: originalReplicas,
		data:             []appsv1.Deployment{},
		areToSuspend:     res.SleepInfo.IsDeploymentsToSuspend(),
	}
	if !d.areToSuspend {
		return d, nil
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
		deployment := deployment

		deploymentReplicas := *deployment.Spec.Replicas
		if deploymentReplicas == 0 {
			continue
		}
		newDeploy := deployment.DeepCopy()
 		annotations := newDeploy.GetAnnotations()
 		annotations["autoscaling.keda.sh/paused-replicas"] = "0"
  		newDeploy.SetAnnotations(annotations)
		*newDeploy.Spec.Replicas = 0

		if err := d.Patch(ctx, &deployment, newDeploy); err != nil {
			return err
		}
	}
	return nil
}

func (d deployments) WakeUp(ctx context.Context) error {
	for _, deployment := range d.data {
		deployment := deployment

		deployLogger := d.Log.WithValues("deployment", deployment.Name, "namespace", deployment.Namespace)
		if *deployment.Spec.Replicas != 0 {
			deployLogger.Info("replicas not 0 during wake up")
			continue
		}

		replica, ok := d.OriginalReplicas[deployment.Name]
		if !ok {
			deployLogger.Info("original deploy info not correctly set")
			continue
		}

		newDeploy := deployment.DeepCopy()
 		annotations := newDeploy.GetAnnotations()
 		delete(annotations, "autoscaling.keda.sh/paused-replicas")
 		newDeploy.SetAnnotations(annotations)
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
	filteredList := []appsv1.Deployment{}
	for _, deployment := range deploymentList {
		if !shouldExcludeDeployment(deployment, d.SleepInfo) {
			filteredList = append(filteredList, deployment)
		}
	}
	return filteredList
}

func shouldExcludeDeployment(deployment appsv1.Deployment, sleepInfo *kubegreenv1alpha1.SleepInfo) bool {
	for _, exclusion := range sleepInfo.GetExcludeRef() {
		if exclusion.Kind == "Deployment" && exclusion.APIVersion == "apps/v1" && exclusion.Name != "" && deployment.Name == exclusion.Name {
			return true
		}
		if labelMatch(deployment.Labels, exclusion.MatchLabels) {
			return true
		}
	}

	return false
}

func labelMatch(labels, matchLabels map[string]string) bool {
	if len(matchLabels) == 0 {
		return false
	}

	matched := true
	for key, value := range matchLabels {
		v, ok := labels[key]
		if !ok || v != value {
			matched = false
			break
		}
	}

	return matched
}

type OriginalReplicas struct {
	Name     string `json:"name"`
	Replicas int32  `json:"replicas"`
}

func (d deployments) GetOriginalInfoToSave() ([]byte, error) {
	if !d.areToSuspend {
		return nil, nil
	}
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
