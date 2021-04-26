package controllers

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
		logger.Error(err, "fails to update deployments")
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
		d := deployment.DeepCopy()

		replica, ok := sleepInfoData.OriginalDeploymentsReplicas[deployment.Name]
		if !ok {
			logger.Info(fmt.Sprintf("replicas info not set on secret in namespace %s for deployment %s", deployment.Namespace, deployment.Name))
			continue
		}

		*d.Spec.Replicas = replica
		// TODO: change update with patch
		if err := r.Client.Update(ctx, d); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil
			}
			return err
		}
	}
	return nil
}
