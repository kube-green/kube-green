package sleepinfo

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SleepInfoReconciler) handleSleep(logger logr.Logger, ctx context.Context, resources Resources) error {
	logger.Info("handle sleep operation", "number of deployments", len(resources.Deployments))
	err := r.updateDeploymentsWithZeroReplicas(ctx, resources.Deployments)
	if err != nil {
		logger.Error(err, "fails to update deployments")
		return err
	}

	return nil
}

func (r *SleepInfoReconciler) updateDeploymentsWithZeroReplicas(ctx context.Context, deployments []appsv1.Deployment) error {
	for _, deployment := range deployments {
		deploymentReplicas := *deployment.Spec.Replicas
		if deploymentReplicas == 0 {
			continue
		}
		d := deployment.DeepCopy()
		*d.Spec.Replicas = 0

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
