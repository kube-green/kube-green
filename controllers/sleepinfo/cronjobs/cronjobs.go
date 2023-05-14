package cronjobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/sleepinfo/resource"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrFetchingCronJobs = errors.New("error fetching cronjobs")
)

type OriginalSuspendStatus map[string]bool
type cronjobs struct {
	resource.ResourceClient
	data                  []unstructured.Unstructured
	OriginalSuspendStatus OriginalSuspendStatus
	areToSuspend          bool
}

func NewResource(ctx context.Context, res resource.ResourceClient, namespace string, originalSuspendStatus map[string]bool) (resource.Resource, error) {
	c := cronjobs{
		ResourceClient:        res,
		OriginalSuspendStatus: originalSuspendStatus,
		areToSuspend:          res.SleepInfo.IsCronjobsToSuspend(),
		data:                  []unstructured.Unstructured{},
	}
	if !c.areToSuspend {
		return c, nil
	}
	if err := c.fetch(ctx, namespace); err != nil {
		return cronjobs{}, fmt.Errorf("%w: %s", ErrFetchingCronJobs, err)
	}

	return c, nil
}

func (c cronjobs) HasResource() bool {
	return len(c.data) > 0
}

func getSuspendStatus(cronjob unstructured.Unstructured) (bool, bool, error) {
	return unstructured.NestedBool(cronjob.Object, "spec", "suspend")
}

func (c cronjobs) Sleep(ctx context.Context) error {
	for _, cronjob := range c.data {
		cronjobSuspended, found, err := getSuspendStatus(cronjob)
		if err != nil {
			return err
		}
		if found && cronjobSuspended {
			continue
		}
		newCronJob := cronjob.DeepCopy()
		unstructured.RemoveNestedField(newCronJob.Object, "metadata", "resourceVersion")
		if err = unstructured.SetNestedField(newCronJob.Object, true, "spec", "suspend"); err != nil {
			return err
		}

		if err := c.SSAPatch(ctx, newCronJob); err != nil {
			return err
		}
	}
	return nil
}

func (c cronjobs) WakeUp(ctx context.Context) error {
	for _, cronjob := range c.data {
		cronjob := cronjob

		cjLogger := c.Log.WithValues("cronjob", cronjob.GetName(), "namespace", cronjob.GetNamespace())
		cronjobSuspended, found, err := getSuspendStatus(cronjob)
		if err != nil {
			cjLogger.Info("fails to read suspend status")
			return err
		}
		if !found || !cronjobSuspended {
			cjLogger.Info("cronjob is not suspended during wake up")
			continue
		}

		status, ok := c.OriginalSuspendStatus[cronjob.GetName()]
		if !ok || status {
			cjLogger.Info("original cron job info not correctly set")
			continue
		}

		newCronJob := cronjob.DeepCopy()
		unstructured.RemoveNestedField(newCronJob.Object, "spec", "suspend")

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
		cronJobSuspended, found, err := getSuspendStatus(cronJob)
		if err != nil {
			return nil, err
		}
		if found && cronJobSuspended {
			continue
		}
		cronJobsStatus = append(cronJobsStatus, OriginalCronJobStatus{
			Name: cronJob.GetName(),
		})
	}
	return json.Marshal(cronJobsStatus)
}

func (c *cronjobs) fetch(ctx context.Context, namespace string) error {
	var err error
	c.data, err = c.getListByNamespace(ctx, namespace)
	c.Log.V(1).WithValues("number of cron jobs", len(c.data), "namespace", namespace).Info("cron jobs in namespace")
	return err
}

func (c cronjobs) getListByNamespace(ctx context.Context, namespace string) ([]unstructured.Unstructured, error) {
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}

	excludeRef := c.ResourceClient.SleepInfo.GetExcludeRef()
	cronJobsToExclude := getCronJobNameToExclude(excludeRef)
	cronJobLabelsToExclude := getCronJobLabelsToExclude(excludeRef)
	fieldsSelector := []string{}
	for _, cronJobToExclude := range cronJobsToExclude {
		fieldsSelector = append(fieldsSelector, fmt.Sprintf("metadata.name!=%s", cronJobToExclude))
	}
	if len(fieldsSelector) > 0 {
		fSel, err := fields.ParseSelector(strings.Join(fieldsSelector, ","))
		if err != nil {
			return nil, err
		}
		listOptions.FieldSelector = fSel
	}

	if cronJobLabelsToExclude != nil {
		labelSelector, err := labels.Parse(strings.Join(cronJobLabelsToExclude, ","))
		if err != nil {
			return nil, err
		}
		listOptions.LabelSelector = labelSelector
	}

	restMapping, err := c.Client.RESTMapper().RESTMapping(schema.GroupKind{
		Group: "batch",
		Kind:  "CronJob",
	})
	if err != nil {
		return nil, err
	}

	cronjobs := unstructured.UnstructuredList{}
	cronjobs.SetGroupVersionKind(restMapping.GroupVersionKind)

	if err := c.Client.List(ctx, &cronjobs, listOptions); err != nil {
		return cronjobs.Items, client.IgnoreNotFound(err)
	}
	return cronjobs.Items, nil
}

func getCronJobNameToExclude(excludeRef []kubegreenv1alpha1.ExcludeRef) []string {
	cronJobsToExclude := []string{}
	for _, exclude := range excludeRef {
		if exclude.Kind == "CronJob" && exclude.Name != "" {
			cronJobsToExclude = append(cronJobsToExclude, exclude.Name)
		}
	}
	return cronJobsToExclude
}

func getCronJobLabelsToExclude(excludeRef []kubegreenv1alpha1.ExcludeRef) []string {
	labelsToExclude := []string{}
	for _, exclude := range excludeRef {
		for k, v := range exclude.MatchLabels {
			labelsToExclude = append(labelsToExclude, fmt.Sprintf("%s!=%s", k, v))
		}
	}
	return labelsToExclude
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
