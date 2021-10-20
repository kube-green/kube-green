package sleepinfo

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SleepInfoReconciler) handleWakeUp(logger logr.Logger, ctx context.Context, deploymentList []appsv1.Deployment, sleepInfoData SleepInfoData) error {
	logger.Info("handle wake up operation", "number of deployments", len(deploymentList))
	err := r.wakeUpDeploymentReplicas(logger, ctx, deploymentList, sleepInfoData)
	if err != nil {
		return err
	}

	return nil
}

func (r *SleepInfoReconciler) wakeUpDeploymentReplicas(logger logr.Logger, ctx context.Context, deployments []appsv1.Deployment, sleepInfoData SleepInfoData) error {
	for _, deployment := range deployments {
		if *deployment.Spec.Replicas != 0 {
			logger.Info("replicas not 0 during wake up", "deployment name", deployment.Name)
			return nil
		}

		replica, ok := sleepInfoData.OriginalDeploymentsReplicas[deployment.Name]
		if !ok {
			logger.Info(fmt.Sprintf("replicas info not set on secret in namespace %s for deployment %s", deployment.Namespace, deployment.Name))
			continue
		}

		d := deployment.DeepCopy()
		*d.Spec.Replicas = replica

		patch := client.MergeFrom(deployment.DeepCopy())
		if err := r.Client.Patch(ctx, d, patch); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil
			}
			return err
		}
	}
	return nil
}
