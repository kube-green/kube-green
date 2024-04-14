package sleepinfo

import (
	"context"
	"testing"
	"time"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/cronjobs"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/internal/mocks"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var deploymentGVK = schema.GroupVersionKind{
	Group:   "apps",
	Version: "v1",
	Kind:    "Deployment",
}

type setupOptions struct {
	insertCronjobs  bool
	customResources []unstructured.Unstructured
}

type originalResources struct {
	deploymentList      []unstructured.Unstructured
	cronjobList         []unstructured.Unstructured
	genericResourcesMap resourceMap

	sleepInfo v1alpha1.SleepInfo
}

func createSleepInfoCRD(t *testing.T, ctx context.Context, c *envconf.Config, sleepInfo *v1alpha1.SleepInfo) v1alpha1.SleepInfo {
	t.Helper()

	r, err := resources.New(c.Client().RESTConfig())
	require.NoError(t, err)
	err = v1alpha1.AddToScheme(r.GetScheme())
	require.NoError(t, err)

	err = c.Client().Resources().Create(ctx, sleepInfo)
	require.NoError(t, err, "error creating SleepInfo")

	createdSleepInfo := &v1alpha1.SleepInfo{}

	err = wait.For(conditions.New(c.Client().Resources(c.Namespace())).ResourceMatch(sleepInfo, func(object k8s.Object) bool {
		createdSleepInfo = object.(*v1alpha1.SleepInfo)
		return true
	}), wait.WithTimeout(time.Second*10), wait.WithInterval(time.Millisecond*250))
	require.NoError(t, err)

	return *createdSleepInfo
}

func setupNamespaceWithResources(t *testing.T, ctx context.Context, cfg *envconf.Config, sleepInfoToCreate *v1alpha1.SleepInfo, reconciler SleepInfoReconciler, opts setupOptions) (ctrl.Request, originalResources) {
	t.Helper()

	sleepInfo := createSleepInfoCRD(t, ctx, cfg, sleepInfoToCreate)

	originalDeployments := upsertDeployments(t, ctx, cfg, false)

	var originalCronJobs []unstructured.Unstructured
	if opts.insertCronjobs {
		originalCronJobs = upsertCronJobs(t, ctx, cfg, false)
	}

	r := resourceMap{}
	if len(opts.customResources) > 0 {
		r = r.upsert(t, ctx, cfg, opts.customResources)
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      sleepInfoToCreate.GetName(),
			Namespace: cfg.Namespace(),
		},
	}
	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{
		RequeueAfter: sleepRequeue(reconciler.Now().Format(time.RFC3339Nano)),
	}, result)

	t.Run("replicas not changed", func(t *testing.T) {
		deploymentsNotChanged := getDeploymentList(t, ctx, cfg)
		for i, deployment := range deploymentsNotChanged {
			require.Equal(t, deploymentReplicas(t, originalDeployments[i]), deploymentReplicas(t, deployment))
		}
	})

	t.Run("cron jobs not suspended", func(t *testing.T) {
		cronJobsNotChanged := getCronJobList(t, ctx, cfg)
		for i, cj := range cronJobsNotChanged {
			require.Equal(t, isCronJobSuspended(t, originalCronJobs[i]), isCronJobSuspended(t, cj))
		}
	})

	return req, originalResources{
		deploymentList:      originalDeployments,
		cronjobList:         originalCronJobs,
		genericResourcesMap: r,

		sleepInfo: sleepInfo,
	}
}

func upsertDeployments(t *testing.T, ctx context.Context, c *envconf.Config, updateIfAlreadyCreated bool) []unstructured.Unstructured {
	t.Helper()

	k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()

	namespace := c.Namespace()

	deployments := []unstructured.Unstructured{
		mocks.Deployment(mocks.DeploymentOptions{
			Name:      "service-1",
			Namespace: namespace,
			Replicas:  getPtr[int32](3),
		}).Unstructured(),
		mocks.Deployment(mocks.DeploymentOptions{
			Name:      "service-2",
			Namespace: namespace,
			Replicas:  getPtr[int32](1),
		}).Unstructured(),
		mocks.Deployment(mocks.DeploymentOptions{
			Name:      "zero-replicas",
			Namespace: namespace,
			Replicas:  getPtr[int32](0),
		}).Unstructured(),
		mocks.Deployment(mocks.DeploymentOptions{
			Name:      "zero-replicas-annotation",
			Namespace: namespace,
			Replicas:  getPtr[int32](0),
			PodAnnotations: map[string]string{
				lastScheduleKey: "2021-03-23T00:00:00.000Z",
			},
		}).Unstructured(),
	}

	d := unstructured.UnstructuredList{}
	d.SetGroupVersionKind(deploymentGVK)
	if updateIfAlreadyCreated {
		err := k8sClient.List(ctx, &d)
		require.NoError(t, err)
	}

	for _, deployment := range deployments {
		var deploy = deployment

		if findDeployByName(d.Items, deploy.GetName()) != nil {
			deploy.SetManagedFields(nil)
			require.NoError(t, k8sClient.Patch(ctx, &deploy, client.Apply, &client.PatchOptions{
				FieldManager: "kube-green-test",
				Force:        getPtr(true),
			}))
		} else {
			err := k8sClient.Create(ctx, &deploy)
			require.NoError(t, err)
		}

		err := wait.For(conditions.New(c.Client().Resources()).ResourceMatch(deployment.DeepCopy(), func(object k8s.Object) bool {
			originalReplicas := deploymentReplicas(t, deploy)
			actualObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
			require.NoError(t, err)
			return originalReplicas == deploymentReplicas(t, unstructured.Unstructured{Object: actualObj})
		}), wait.WithTimeout(time.Second*10), wait.WithInterval(time.Millisecond*250))
		require.NoError(t, err)
	}
	return deployments
}

func upsertCronJobs(t *testing.T, ctx context.Context, c *envconf.Config, updateIfAlreadyCreated bool) []unstructured.Unstructured {
	suspendTrue := true
	suspendFalse := false
	namespace := c.Namespace()

	k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()

	restMapping, err := k8sClient.RESTMapper().RESTMapping(schema.GroupKind{
		Group: "batch",
		Kind:  "CronJob",
	})
	require.NoError(t, err)

	version := getCronJobAPIVersion(restMapping)

	cronJobs := []unstructured.Unstructured{
		cronjobs.GetMock(cronjobs.MockSpec{
			Name:      "cronjob-1",
			Namespace: namespace,
			Version:   version,
		}),
		cronjobs.GetMock(cronjobs.MockSpec{
			Name:      "cronjob-2",
			Namespace: namespace,
			Suspend:   &suspendFalse,
			Version:   version,
		}),
		cronjobs.GetMock(cronjobs.MockSpec{
			Name:      "cronjob-suspended",
			Namespace: namespace,
			Suspend:   &suspendTrue,
			Version:   version,
		}),
	}
	objList := unstructured.UnstructuredList{}
	if updateIfAlreadyCreated {
		objList.SetGroupVersionKind(restMapping.GroupVersionKind)

		err := k8sClient.List(ctx, &objList)
		require.NoError(t, err)
	}
	for _, cronJob := range cronJobs {
		var cron = cronJob
		if obj := findResourceByName(objList.Items, cron.GetName()); obj != nil {
			patch := client.MergeFrom(obj)
			if err := k8sClient.Patch(ctx, &cron, patch); err != nil {
				require.NoError(t, err)
			}
		} else {
			require.NoError(t, k8sClient.Create(ctx, &cron))
		}

		err := wait.For(conditions.New(c.Client().Resources(c.Namespace())).ResourceMatch(cronJob.DeepCopy(), func(object k8s.Object) bool {
			originalSuspend := getSuspendStatus(t, cronJob)
			actualObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
			require.NoError(t, err)
			actualSuspend := getSuspendStatus(t, unstructured.Unstructured{Object: actualObj})
			return originalSuspend == actualSuspend
		}), wait.WithTimeout(time.Second*10), wait.WithInterval(time.Millisecond*250))
		require.NoError(t, err)
	}
	return cronJobs
}

func getDefaultSleepInfo(name, namespace string) *v1alpha1.SleepInfo {
	return &v1alpha1.SleepInfo{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SleepInfo",
			APIVersion: "kube-green.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.SleepInfoSpec{
			Weekdays:   "*",
			SleepTime:  "*:05", // at minute 5
			WakeUpTime: "*:20", // at minute 20
		},
	}
}

type assertOperationKey struct{}

func withAssertOperation(ctx context.Context, assert AssertOperation) context.Context {
	return context.WithValue(ctx, assertOperationKey{}, assert)
}

func getAssertOperation(t *testing.T, ctx context.Context) AssertOperation {
	assertOperation, ok := ctx.Value(assertOperationKey{}).(AssertOperation)
	if !ok {
		t.Fatal("fail to get AssertOperation from context")
	}
	return assertOperation
}

type setupOptionsKey struct{}

func withSetupOptions(ctx context.Context, setup setupOptions) context.Context {
	return context.WithValue(ctx, setupOptionsKey{}, setup)
}

func getSetupOptions(t *testing.T, ctx context.Context) setupOptions {
	t.Helper()
	setupOpts, ok := ctx.Value(setupOptionsKey{}).(setupOptions)
	if !ok {
		return setupOptions{}
	}
	return setupOpts
}

type mockClock struct {
	now string
	t   *testing.T
}

func (m mockClock) Now() time.Time {
	if m.t == nil {
		panic("testing.T not passed in mockClock")
	}
	parsedTime, err := time.Parse(time.RFC3339, m.now)
	require.NoError(m.t, err)
	return parsedTime
}

func getDeploymentList(t *testing.T, ctx context.Context, c *envconf.Config) []unstructured.Unstructured {
	t.Helper()
	deployments := unstructured.UnstructuredList{}
	deployments.SetGroupVersionKind(deploymentGVK)
	require.NoError(t, c.Client().Resources(c.Namespace()).List(ctx, &deployments))
	return deployments.Items
}

func getCronJobList(t *testing.T, ctx context.Context, c *envconf.Config) []unstructured.Unstructured {
	k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()
	restMapping, err := k8sClient.RESTMapper().RESTMapping(schema.GroupKind{
		Group: "batch",
		Kind:  "CronJob",
	})
	require.NoError(t, err)

	u := unstructured.UnstructuredList{}
	u.SetGroupVersionKind(restMapping.GroupVersionKind)

	err = k8sClient.List(ctx, &u, &client.ListOptions{
		Namespace: c.Namespace(),
	})
	require.NoError(t, err)

	return u.Items
}

type resourceList struct {
	data []unstructured.Unstructured
}
type resourceMap map[string]resourceList

func getGenericResourcesMap(t *testing.T, ctx context.Context, c *envconf.Config, patches []v1alpha1.Patch) resourceMap {
	k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()

	m := resourceMap{}

	for _, patch := range patches {
		restMapping, err := k8sClient.RESTMapper().RESTMapping(schema.GroupKind{
			Group: patch.Target.Group,
			Kind:  patch.Target.Kind,
		})
		if meta.IsNoMatchError(err) {
			continue
		}
		require.NoError(t, err)

		u := unstructured.UnstructuredList{}
		u.SetGroupVersionKind(restMapping.GroupVersionKind)

		err = k8sClient.List(ctx, &u, &client.ListOptions{
			Namespace: c.Namespace(),
		})
		require.NoError(t, err)

		m[m.key(restMapping.GroupVersionKind)] = resourceList{
			data: u.Items,
		}
	}

	return m
}

func (r resourceMap) upsert(t *testing.T, ctx context.Context, c *envconf.Config, resources []unstructured.Unstructured) resourceMap {
	k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()

	for _, resource := range resources {
		resource := resource
		gvk := resource.GetObjectKind().GroupVersionKind()

		if obj := findResourceByName(r.getResourceList(gvk), resource.GetName()); obj != nil {
			patch := client.MergeFrom(obj)
			if err := k8sClient.Patch(ctx, &resource, patch); err != nil {
				require.NoError(t, err)
			}
		} else {
			require.NoError(t, k8sClient.Create(ctx, &resource))
		}

		r[r.key(gvk)] = resourceList{
			data: append(r.getResourceList(gvk), resource),
		}

		err := wait.For(conditions.New(c.Client().Resources(c.Namespace())).ResourceMatch(resource.DeepCopy(), func(object k8s.Object) bool {
			actualObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
			require.NoError(t, err)
			return resource.GetResourceVersion() == actualObj["metadata"].(map[string]interface{})["resourceVersion"]
		}), wait.WithTimeout(time.Second*10), wait.WithInterval(time.Millisecond*250))
		require.NoError(t, err)
	}

	return r
}

func (r resourceMap) key(gvk schema.GroupVersionKind) string {
	return gvk.String()
}

func (r resourceMap) getResourceList(gvk schema.GroupVersionKind) []unstructured.Unstructured {
	if r[r.key(gvk)].data == nil {
		return []unstructured.Unstructured{}
	}
	return r[r.key(gvk)].data
}

func parseTime(t *testing.T, mockNowRaw string) time.Time {
	t.Helper()

	now, err := time.Parse(time.RFC3339, mockNowRaw)
	require.NoError(t, err)
	return now
}

func sleepRequeue(now string) time.Duration {
	parsedTime, _ := time.Parse(time.RFC3339, now)
	hour := parsedTime.Hour()
	if parsedTime.Minute() > 5 {
		hour += 1
	}
	return time.Duration((time.Date(2021, time.March, 23, hour, 5, 0, 0, time.UTC).UnixNano() - parsedTime.UnixNano()))
}

func wakeUpRequeue(now string) time.Duration {
	parsedTime, _ := time.Parse(time.RFC3339, now)
	hour := parsedTime.Hour()
	if parsedTime.Minute() > 20 {
		hour += 1
	}
	return time.Duration((time.Date(2021, time.March, 23, hour, 20, 0, 0, time.UTC).UnixNano() - parsedTime.UnixNano()))
}

func isSuspendedCronJob(t *testing.T, cronJob unstructured.Unstructured) bool {
	t.Helper()
	suspend, found, err := unstructured.NestedBool(cronJob.Object, "spec", "suspend")
	require.NoError(t, err, "cronJob suspend error")
	if !found {
		return false
	}
	return suspend
}

func assertAllReplicasSetToZero(t *testing.T, actualDeployments []unstructured.Unstructured) {
	t.Helper()

	allReplicas := []int32{}
	for _, deployment := range actualDeployments {
		allReplicas = append(allReplicas, deploymentReplicas(t, deployment))
	}
	for _, replicas := range allReplicas {
		require.Equal(t, replicas, int32(0))
	}
}

func assertAllCronJobsSuspended(t *testing.T, actualCronJobs []unstructured.Unstructured) {
	t.Helper()

	allSuspended := []struct {
		name    string
		suspend bool
	}{}
	for _, cronJob := range actualCronJobs {
		allSuspended = append(allSuspended, struct {
			name    string
			suspend bool
		}{
			name:    cronJob.GetName(),
			suspend: isCronJobSuspended(t, cronJob),
		})
	}
	for _, suspended := range allSuspended {
		require.True(t, suspended.suspend, suspended.name)
	}
}

func findDeployByName(deployments []unstructured.Unstructured, nameToFind string) *unstructured.Unstructured {
	for _, deployment := range deployments {
		if deployment.GetName() == nameToFind {
			return deployment.DeepCopy()
		}
	}
	return nil
}

func contains(s []string, v string) bool {
	for _, a := range s {
		if a == v {
			return true
		}
	}
	return false
}

func getCronJobAPIVersion(restMapping *meta.RESTMapping) string {
	return restMapping.GroupVersionKind.Version
}

func findResourceByName(resources []unstructured.Unstructured, nameToFind string) *unstructured.Unstructured {
	for _, resource := range resources {
		if resource.GetName() == nameToFind {
			return resource.DeepCopy()
		}
	}
	return nil
}

func findPatchByTarget(patches []v1alpha1.Patch, target schema.GroupVersionKind) []byte {
	for _, patch := range patches {
		if patch.Target.Group == target.Group && patch.Target.Kind == target.Kind {
			return []byte(patch.Patch)
		}
	}
	return nil
}

func getSuspendStatus(t *testing.T, cronjob unstructured.Unstructured) bool {
	suspend, _, err := unstructured.NestedBool(cronjob.Object, "spec", "suspend")
	require.NoError(t, err)
	t.Logf("suspend for %s is set: %v", cronjob.GetName(), suspend)
	return suspend
}

func getPtr[T any](item T) *T {
	return &item
}

func isCronJobSuspended(t *testing.T, cronJob unstructured.Unstructured) bool {
	suspend, found, err := unstructured.NestedBool(cronJob.Object, "spec", "suspend")
	require.NoError(t, err)
	if !found {
		return false
	}
	return suspend
}

func deploymentReplicas(t *testing.T, d unstructured.Unstructured) int32 {
	v, _, err := unstructured.NestedInt64(d.Object, "spec", "replicas")
	require.NoError(t, err)
	return int32(v)
}
