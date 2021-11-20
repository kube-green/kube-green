package cronjobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/davidebianchi/kube-green/controllers/sleepinfo/resource"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OriginalSuspendStatus map[string]bool
type cronjobs struct {
	resource.ResourceClient
	data                  []batchv1.CronJob
	OriginalSuspendStatus OriginalSuspendStatus
	areToSuspend          bool
}

func NewResource(ctx context.Context, res resource.ResourceClient, namespace string, originalSuspendStatus map[string]bool) (cronjobs, error) {
	d := cronjobs{
		ResourceClient:        res,
		OriginalSuspendStatus: originalSuspendStatus,
		areToSuspend:          res.SleepInfo.IsCronjobsToSuspend(),
		data:                  []batchv1.CronJob{},
	}
	if !d.areToSuspend {
		return d, nil
	}
	if err := d.fetch(ctx, namespace); err != nil {
		return cronjobs{}, err
	}

	return d, nil
}

func (d cronjobs) HasResource() bool {
	return len(d.data) > 0
}

var suspendTrue = true

func (c cronjobs) Sleep(ctx context.Context) error {
	for _, cronjob := range c.data {
		cronjobSuspended := cronjob.Spec.Suspend
		if cronjobSuspended != nil && *cronjobSuspended {
			continue
		}
		newCronJob := cronjob.DeepCopy()
		newCronJob.Spec.Suspend = &suspendTrue

		if err := c.Patch(ctx, &cronjob, newCronJob); err != nil {
			return err
		}
	}
	return nil
}

func (c cronjobs) WakeUp(ctx context.Context) error {
	for _, cronjob := range c.data {
		cronjobSuspended := cronjob.Spec.Suspend
		if cronjobSuspended == nil || !*cronjobSuspended {
			c.Log.Info("cronjob is not suspended during wake up", "cronjob", cronjob.Name)
			continue
		}

		status, ok := c.OriginalSuspendStatus[cronjob.Name]
		if !ok || status {
			c.Log.Info(fmt.Sprintf("replicas info not set on secret in namespace %s for deployment %s", cronjob.Namespace, cronjob.Name))
			continue
		}

		newCronJob := cronjob.DeepCopy()
		newCronJob.Spec.Suspend = nil

		if err := c.Patch(ctx, &cronjob, newCronJob); err != nil {
			return err
		}
	}
	return nil
}

type OriginalCronJobStatus struct {
	Name    string `json:"name"`
	Suspend bool   `json:"suspend"`
}

func (c cronjobs) GetOriginalInfoToSave() ([]byte, error) {
	if !c.areToSuspend {
		return nil, nil
	}
	cronJobsStatus := []OriginalCronJobStatus{}
	for _, cronJob := range c.data {
		if cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend {
			continue
		}
		cronJobsStatus = append(cronJobsStatus, OriginalCronJobStatus{
			Name: cronJob.Name,
		})
	}
	return json.Marshal(cronJobsStatus)
}

func (c *cronjobs) fetch(ctx context.Context, namespace string) error {
	var err error
	c.data, err = c.getListByNamespace(ctx, namespace)
	return err
}

func (c cronjobs) getListByNamespace(ctx context.Context, namespace string) ([]batchv1.CronJob, error) {
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}
	cronjobs := batchv1.CronJobList{}
	if err := c.Client.List(ctx, &cronjobs, listOptions); err != nil {
		return cronjobs.Items, client.IgnoreNotFound(err)
	}
	return cronjobs.Items, nil
}

func GetOriginalInfoToRestore(savedData []byte) (OriginalSuspendStatus, error) {
	if savedData == nil {
		return OriginalSuspendStatus{}, nil
	}
	originalSuspendedCronJob := []OriginalCronJobStatus{}
	if savedData != nil {
		if err := json.Unmarshal(savedData, &originalSuspendedCronJob); err != nil {
			return nil, err
		}
	}
	originalSuspendedCronjobData := map[string]bool{}
	for _, cronJob := range originalSuspendedCronJob {
		if cronJob.Name != "" {
			originalSuspendedCronjobData[cronJob.Name] = cronJob.Suspend
		}
	}
	return originalSuspendedCronjobData, nil
}
