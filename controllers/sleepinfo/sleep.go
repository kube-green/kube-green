package controllers

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SleepInfoReconciler) updateDeploymentsWithZeroReplicas(ctx context.Context, deployments []appsv1.Deployment, now time.Time) error {
	for _, deployment := range deployments {
		if *deployment.Spec.Replicas == 0 {
			continue
		}
		d := deployment.DeepCopy()
		*d.Spec.Replicas = 0
		annotations := d.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[lastScheduledAnnotation] = now.Format(time.RFC3339)
		d.SetAnnotations(annotations)
		if err := r.Client.Update(ctx, d); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil
			}
			return err
		}
	}
	return nil
}
