package sleepwakeup

import (
	"context"
	"fmt"
)

func (c Client) wakeUpDeployments(ctx context.Context) error {
	sleepInfoData, err := c.getSleepInfoData(ctx)
	if err != nil {
		return err
	}
	if sleepInfoData.OriginalDeploymentsReplicas == nil {
		return fmt.Errorf("original deployment replicas not found in secret %s in namespace %s", c.secretName, c.namespaceName)
	}
	for _, originalDeploymentReplica := range sleepInfoData.OriginalDeploymentsReplicas {
		scale, err := c.getReplicas(ctx, originalDeploymentReplica.Name)
		if err != nil {
			return err
		}
		if scale.Spec.Replicas != 0 {
			fmt.Printf("Skip wake up. replicas not 0 during wake up, deployment name: %s", originalDeploymentReplica.Name)
			continue
		}
		err = c.updateReplicas(ctx, originalDeploymentReplica.Name, originalDeploymentReplica.Replicas)
		if err != nil {
			fmt.Printf("fails to update deployment %s scaling: %s", originalDeploymentReplica.Name, err.Error())
		}
	}
	return c.upsertSecret(ctx, nil)
}
