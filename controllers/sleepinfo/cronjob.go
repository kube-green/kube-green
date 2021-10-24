package sleepinfo

import (
	"context"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SleepInfoReconciler) getCronJobList(ctx context.Context, namespace string, sleepInfo *kubegreenv1alpha1.SleepInfo) ([]batchv1.CronJob, error) {
	c := cronJobs{
		k8sClient: r.Client,
	}

	cronJobList, err := c.getListByNamespace(ctx, namespace)
	if err != nil {
		return nil, err
	}

	return cronJobList, nil
}

type cronJobs struct {
	k8sClient client.Client
}

func (c cronJobs) getListByNamespace(ctx context.Context, namespace string) ([]batchv1.CronJob, error) {
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}
	cronJobs := batchv1.CronJobList{}
	if err := c.k8sClient.List(ctx, &cronJobs, listOptions); err != nil {
		return cronJobs.Items, client.IgnoreNotFound(err)
	}
	return cronJobs.Items, nil
}
