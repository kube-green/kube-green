package controllers

import (
	"context"
	"strconv"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO: handle double subsequent sleep --> replicas annotation must not be set to 0
func (r *SleepInfoReconciler) handleSleep(logger logr.Logger, ctx context.Context, deploymentList []appsv1.Deployment) error {
	logger.Info("handle sleep operation", "number of deployments", len(deploymentList))
	err := r.updateDeploymentsWithZeroReplicas(ctx, deploymentList)
	if err != nil {
		logger.Error(err, "fails to update deployments")
		return err
	}

	return nil
}

func (r *SleepInfoReconciler) updateDeploymentsWithZeroReplicas(ctx context.Context, deployments []appsv1.Deployment) error {
	for _, deployment := range deployments {
		// handle replicas in secret instead of annotations
		currentDeploymentReplicas := strconv.Itoa(int(*deployment.Spec.Replicas))
		d := deployment.DeepCopy()
		annotations := d.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[replicasBeforeSleepAnnotation] = currentDeploymentReplicas
		d.SetAnnotations(annotations)

		*d.Spec.Replicas = 0
		if err := r.Client.Update(ctx, d); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil
			}
			return err
		}
	}
	return nil
}