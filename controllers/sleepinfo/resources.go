package sleepinfo

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
)

type Resources struct {
	Deployments []appsv1.Deployment
	CronJobs    []batchv1.CronJob
}

func (r Resources) hasResources() bool {
	return len(r.CronJobs) != 0 || len(r.Deployments) != 0
}
