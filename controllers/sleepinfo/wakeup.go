package controllers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SleepInfoReconciler) handleWakeUp(logger logr.Logger, ctx context.Context, deploymentList []appsv1.Deployment) error {
	logger.Info("handle wake up operation", "number of deployments", len(deploymentList))
	err := r.wakeUpDeploymentReplicas(logger, ctx, deploymentList)
	if err != nil {
		logger.Error(err, "fails to update deployments")
		return err
	}

	return nil
}

func (r *SleepInfoReconciler) wakeUpDeploymentReplicas(logger logr.Logger, ctx context.Context, deployments []appsv1.Deployment) error {
	for _, deployment := range deployments {
		if *deployment.Spec.Replicas != 0 {
			logger.Info("replicas not 0 during wake up", "deployment name", deployment.Name)
			return nil
		}
		d := deployment.DeepCopy()
		annotations := d.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		replicas, ok := annotations[replicasBeforeSleepAnnotation]
		if !ok {
			logger.Info(fmt.Sprintf("replicas annotation not set on deployment %s in namespace %s", deployment.Name, deployment.Namespace))
			continue
		}
		deploymentReplicasToWakeUp, err := strconv.Atoi(replicas)
		if err != nil {
			logger.Error(err, fmt.Sprintf("replicas in annotation %s is not correct", replicasBeforeSleepAnnotation))
			continue
		}
		delete(annotations, replicasBeforeSleepAnnotation)
		d.SetAnnotations(annotations)

		*d.Spec.Replicas = int32(deploymentReplicasToWakeUp)
		if err := r.Client.Update(ctx, d); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil
			}
			return err
		}
	}
	return nil
}
