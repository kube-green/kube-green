package controllers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SleepInfoReconciler) handleRestore(logger logr.Logger, ctx context.Context, deploymentList []appsv1.Deployment) error {
	logger.Info("handle restore operation", "number of deployments", len(deploymentList))
	err := r.restoreDeploymentReplicas(logger, ctx, deploymentList)
	if err != nil {
		logger.Error(err, "fails to update deployments")
		return err
	}

	return nil
}

func (r *SleepInfoReconciler) restoreDeploymentReplicas(logger logr.Logger, ctx context.Context, deployments []appsv1.Deployment) error {
	for _, deployment := range deployments {
		if *deployment.Spec.Replicas != 0 {
			logger.Info("replicas not 0 during restore", "deployment name", deployment.Name)
			return nil
		}
		d := deployment.DeepCopy()
		annotations := d.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		replicas, ok := annotations[replicasBeforeSleepAnnotation]
		if !ok {
			return fmt.Errorf("replicas annotation not set on deployment %s in namespace %s", deployment.Name, deployment.Namespace)
		}
		deploymentReplicasToRestore, err := strconv.Atoi(replicas)
		if err != nil {
			return fmt.Errorf("replicas in annotation %s is not correct: %s", replicasBeforeSleepAnnotation, err)
		}
		delete(annotations, replicasBeforeSleepAnnotation)
		d.SetAnnotations(annotations)

		*d.Spec.Replicas = int32(deploymentReplicasToRestore)
		if err := r.Client.Update(ctx, d); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil
			}
			return err
		}
	}
	return nil
}
