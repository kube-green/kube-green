package sleepinfo

import (
	"context"

	"github.com/kube-green/kube-green/internal/controller/sleepinfo/cronjobs"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/deployments"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/jsonpatch"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"
)

type Resources struct {
	deployments      resource.Resource
	cronjobs         resource.Resource
	genericResources resource.Resource
}

func NewResources(ctx context.Context, resourceClient resource.ResourceClient, namespace string, sleepInfoData SleepInfoData) (Resources, error) {
	if err := resourceClient.IsClientValid(); err != nil {
		return Resources{}, err
	}
	deployResource, err := deployments.NewResource(ctx, resourceClient, namespace, sleepInfoData.OriginalDeploymentsReplicas)
	if err != nil {
		resourceClient.Log.Error(err, "fails to init deployments")
		return Resources{}, err
	}
	cronJobResource, err := cronjobs.NewResource(ctx, resourceClient, namespace, sleepInfoData.OriginalCronJobStatus)
	if err != nil {
		resourceClient.Log.Error(err, "fails to init cronjobs")
		return Resources{}, err
	}
	// TODO: what happen if Deploy and CronJob are managed also by jsonpatch?
	genericResources, err := jsonpatch.NewResources(ctx, resourceClient, namespace, sleepInfoData.OriginalGenericResourceInfo)
	if err != nil {
		resourceClient.Log.Error(err, "fails to init jsonpatch resources")
		return Resources{}, err
	}

	return Resources{
		deployments:      deployResource,
		cronjobs:         cronJobResource,
		genericResources: genericResources,
	}, nil
}

func (r Resources) hasResources() bool {
	return r.deployments.HasResource() || r.cronjobs.HasResource() || r.genericResources.HasResource()
}

func (r Resources) sleep(ctx context.Context) error {
	if err := r.deployments.Sleep(ctx); err != nil {
		return err
	}
	if err := r.cronjobs.Sleep(ctx); err != nil {
		return err
	}
	return r.genericResources.Sleep(ctx)
}

func (r Resources) wakeUp(ctx context.Context) error {
	if err := r.deployments.WakeUp(ctx); err != nil {
		return err
	}
	if err := r.cronjobs.WakeUp(ctx); err != nil {
		return err
	}
	return r.genericResources.WakeUp(ctx)
}

func (r Resources) getOriginalResourceInfoToSave() (map[string][]byte, error) {
	newData := make(map[string][]byte)

	originalDeploymentInfo, err := r.deployments.GetOriginalInfoToSave()
	if err != nil {
		return nil, err
	}
	if originalDeploymentInfo != nil {
		newData[replicasBeforeSleepKey] = originalDeploymentInfo
	}

	originalCronJobStatus, err := r.cronjobs.GetOriginalInfoToSave()
	if err != nil {
		return nil, err
	}
	if originalCronJobStatus != nil {
		newData[originalCronjobStatusKey] = originalCronJobStatus
	}

	genericResourcePatch, err := r.genericResources.GetOriginalInfoToSave()
	if err != nil {
		return nil, err
	}
	if genericResourcePatch != nil {
		newData[originalJSONPatchDataKey] = genericResourcePatch
	}

	return newData, nil
}

func setOriginalResourceInfoToRestoreInSleepInfo(data map[string][]byte, sleepInfoData *SleepInfoData) error {
	var err error
	if sleepInfoData.OriginalDeploymentsReplicas, err = deployments.GetOriginalInfoToRestore(data[replicasBeforeSleepKey]); err != nil {
		return err
	}

	if sleepInfoData.OriginalCronJobStatus, err = cronjobs.GetOriginalInfoToRestore(data[originalCronjobStatusKey]); err != nil {
		return err
	}

	if sleepInfoData.OriginalGenericResourceInfo, err = jsonpatch.GetOriginalInfoToRestore(data[originalJSONPatchDataKey]); err != nil {
		return err
	}

	return nil
}
