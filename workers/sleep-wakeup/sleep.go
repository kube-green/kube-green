package sleepwakeup

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (c Client) sleepDeployments(ctx context.Context) error {
	deploymentList, err := c.listDeployments(ctx, c.namespaceName)
	if err != nil {
		return fmt.Errorf("fails to list deployment for namespace %s: %s", c.namespaceName, err.Error())
	}
	sleepInfoData, err := c.getSleepInfoData(ctx)
	if err == nil || apierrors.IsNotFound(err) {
		if len(sleepInfoData.OriginalDeploymentsReplicas) > 0 {
			return nil
		}
	}

	originalDeploymentReplicas := []OriginalDeploymentReplicas{}
	for _, deployment := range deploymentList.Items {
		if *deployment.Spec.Replicas == 0 {
			continue
		}
		err := c.updateReplicas(ctx, deployment.Name, 0)
		if err != nil {
			fmt.Printf("fails to update deployment %s: %s", deployment.Name, err)
			continue
		}
		originalDeploymentReplicas = append(originalDeploymentReplicas, OriginalDeploymentReplicas{
			Name:     deployment.Name,
			Replicas: *deployment.Spec.Replicas,
		})
	}

	return c.upsertSecret(ctx, originalDeploymentReplicas)
}
