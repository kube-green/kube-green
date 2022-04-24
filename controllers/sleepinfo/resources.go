package sleepinfo

import (
	"context"

	"github.com/kube-green/kube-green/controllers/sleepinfo/cronjobs"
	"github.com/kube-green/kube-green/controllers/sleepinfo/deployments"
	"github.com/kube-green/kube-green/controllers/sleepinfo/metrics"
	"github.com/kube-green/kube-green/controllers/sleepinfo/resource"
)

type Resources struct {
	deployments resource.Resource
	cronjobs    resource.Resource
}

func NewResources(ctx context.Context, resourceClient resource.ResourceClient, namespace string, sleepInfoData SleepInfoData, metricsClient metrics.Metrics) (Resources, error) {
	if err := resourceClient.IsClientValid(); err != nil {
		return Resources{}, err
	}
	deployResource, err := deployments.NewResource(ctx, resourceClient, namespace, sleepInfoData.OriginalDeploymentsReplicas, metricsClient)
	if err != nil {
		resourceClient.Log.Error(err, "fails to init deployments")
		return Resources{}, err
	}
	cronJobResource, err := cronjobs.NewResource(ctx, resourceClient, namespace, sleepInfoData.OriginalCronJobStatus)
	if err != nil {
		resourceClient.Log.Error(err, "fails to init cronjobs")
		return Resources{}, err
	}

	return Resources{
		deployments: deployResource,
		cronjobs:    cronJobResource,
	}, nil
}

func (r Resources) hasResources() bool {
	return r.deployments.HasResource() || r.cronjobs.HasResource()
}

func (r Resources) sleep(ctx context.Context) error {
	if err := r.deployments.Sleep(ctx); err != nil {
		return err
	}
	if err := r.cronjobs.Sleep(ctx); err != nil {
		return err
	}
	return nil
}

func (r Resources) wakeUp(ctx context.Context) error {
	if err := r.deployments.WakeUp(ctx); err != nil {
		return err
	}
	if err := r.cronjobs.WakeUp(ctx); err != nil {
		return err
	}
	return nil
}

func (r Resources) getOriginalResourceInfoToSave() (map[string][]byte, error) {
	newData := make(map[string][]byte)

	originalDeploymentInfo, err := r.deployments.GetOriginalInfoToSave()
	if err != nil {
		return nil, err
	}
	newData[replicasBeforeSleepKey] = originalDeploymentInfo

	originalCronJobStatus, err := r.cronjobs.GetOriginalInfoToSave()
	if err != nil {
		return nil, err
	}
	if originalCronJobStatus != nil {
		newData[originalCronjobStatusKey] = originalCronJobStatus
	}
	return newData, nil
}

func setOriginalResourceInfoToRestoreInSleepInfo(data map[string][]byte, sleepInfoData *SleepInfoData) error {
	originalDeploymentsReplicasData, err := deployments.GetOriginalInfoToRestore(data[replicasBeforeSleepKey])
	if err != nil {
		return err
	}
	sleepInfoData.OriginalDeploymentsReplicas = originalDeploymentsReplicasData

	originalCronJobStatusData, err := cronjobs.GetOriginalInfoToRestore(data[originalCronjobStatusKey])
	if err != nil {
		return err
	}
	sleepInfoData.OriginalCronJobStatus = originalCronJobStatusData

	return nil
}
