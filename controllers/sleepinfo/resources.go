package sleepinfo

import (
	"context"

	"github.com/davidebianchi/kube-green/controllers/sleepinfo/deployments"
	"github.com/davidebianchi/kube-green/controllers/sleepinfo/resource"
)

type Resources struct {
	deployments resource.Resource
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

	return Resources{
		deployments: deployResource,
	}, nil
}

func (r Resources) hasResources() bool {
	return r.deployments.HasResource()
}

func (r Resources) sleep(ctx context.Context) error {
	if err := r.deployments.Sleep(ctx); err != nil {
		return err
	}
	return nil
}

func (r Resources) wakeUp(ctx context.Context) error {
	if err := r.deployments.WakeUp(ctx); err != nil {
		return err
	}
	return nil
}

func (r Resources) getOriginalResourceInfoToSave(sleepInfoData SleepInfoData) (map[string][]byte, error) {
	newData := make(map[string][]byte)

	originalDeploymentInfo, err := r.deployments.GetOriginalInfoToSave()
	if err != nil {
		return nil, err
	}
	newData[replicasBeforeSleepKey] = originalDeploymentInfo
	return newData, nil
}
