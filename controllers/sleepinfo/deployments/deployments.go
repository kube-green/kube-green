package deployments

import (
	"context"
	"encoding/json"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/sleepinfo/metrics"
	"github.com/kube-green/kube-green/controllers/sleepinfo/resource"
	"github.com/prometheus/client_golang/prometheus"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const resourceType = "deployment"

type deployments struct {
	resource.ResourceClient
	OriginalReplicas map[string]int32

	data          []appsv1.Deployment
	metricsClient metrics.Metrics
	namespace     string
}

func NewResource(ctx context.Context, res resource.ResourceClient, namespace string, originalReplicas map[string]int32, metricsClient metrics.Metrics) (deployments, error) {
	if originalReplicas == nil {
		originalReplicas = make(map[string]int32)
	}
	d := deployments{
		ResourceClient:   res,
		OriginalReplicas: originalReplicas,
		metricsClient:    metricsClient,
		namespace:        namespace,
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
	sleepReplicas := float64(0)
	numberOfDeploymentSleeped := float64(0)
	for _, deployment := range d.data {
		deployment := deployment

		deploymentReplicas := *deployment.Spec.Replicas
		if deploymentReplicas == 0 {
			continue
		}
		d.OriginalReplicas[deployment.Name] = deploymentReplicas

		sleepReplicas += float64(deploymentReplicas)
		numberOfDeploymentSleeped += 1

		newDeploy := deployment.DeepCopy()
		*newDeploy.Spec.Replicas = 0

		if err := d.Patch(ctx, &deployment, newDeploy); err != nil {
			return err
		}
	}

	d.metricsClient.SleepWorkloadTotal.With(prometheus.Labels{
		"resource_type": resourceType,
		"namespace":     d.namespace,
	}).Add(numberOfDeploymentSleeped)

	return nil
}

func (d deployments) WakeUp(ctx context.Context) error {
	wakeUpReplicas := float64(0)

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
		wakeUpReplicas += float64(replica)

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
