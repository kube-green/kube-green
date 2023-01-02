package sleepinfo

import (
	"context"
	"testing"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/sleepinfo/cronjobs"
	"github.com/kube-green/kube-green/controllers/sleepinfo/deployments"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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

// TODO: simplify setup with this function
func createSleepInfoCRD(t *testing.T, ctx context.Context, c *envconf.Config, sleepInfo *kubegreenv1alpha1.SleepInfo) kubegreenv1alpha1.SleepInfo {
	t.Helper()

	r, err := resources.New(c.Client().RESTConfig())
	require.NoError(t, err)
	kubegreenv1alpha1.AddToScheme(r.GetScheme())

	err = c.Client().Resources().Create(ctx, sleepInfo)
	require.NoError(t, err, "error creating SleepInfo")

	createdSleepInfo := &kubegreenv1alpha1.SleepInfo{}

	err = wait.For(conditions.New(c.Client().Resources(c.Namespace())).ResourceMatch(sleepInfo, func(object k8s.Object) bool {
		createdSleepInfo = object.(*kubegreenv1alpha1.SleepInfo)
		return true
	}), wait.WithTimeout(time.Second*10), wait.WithInterval(time.Millisecond*250))
	require.NoError(t, err)

	return *createdSleepInfo
}

func setupNamespaceWithResources2(t *testing.T, ctx context.Context, cfg *envconf.Config, sleepInfoToCreate *kubegreenv1alpha1.SleepInfo, reconciler SleepInfoReconciler, opts setupOptions) (ctrl.Request, originalResources) {
	t.Helper()

	sleepInfo := createSleepInfoCRD(t, ctx, cfg, sleepInfoToCreate)

	originalDeployments := upsertDeployments2(t, ctx, cfg, false)

	var originalCronJobs []unstructured.Unstructured
	if opts.insertCronjobs {
		originalCronJobs = upsertCronJobs2(t, ctx, cfg, false)
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
			require.Equal(t, *originalDeployments[i].Spec.Replicas, *deployment.Spec.Replicas)
		}
	})

	t.Run("cron jobs not suspended", func(t *testing.T) {
		cronJobsNotChanged := getCronJobList(t, ctx, cfg)
		for i, cj := range cronJobsNotChanged {
			require.Equal(t, isCronJobSuspended(originalCronJobs[i]), isCronJobSuspended(cj))
		}
	})

	return req, originalResources{
		deploymentList: originalDeployments,
		cronjobList:    originalCronJobs,

		sleepInfo: sleepInfo,
	}
}

func upsertDeployments2(t *testing.T, ctx context.Context, c *envconf.Config, updateIfAlreadyCreated bool) []appsv1.Deployment {
	t.Helper()

	k8sClient := newControllerRuntimeClient(t, c)
	namespace := c.Namespace()

	deployments := []appsv1.Deployment{
		deployments.GetMock(deployments.MockSpec{
			Name:      "service-1",
			Namespace: namespace,
			Replicas:  getPtr[int32](3),
		}),
		deployments.GetMock(deployments.MockSpec{
			Name:      "service-2",
			Namespace: namespace,
			Replicas:  getPtr[int32](1),
		}),
		deployments.GetMock(deployments.MockSpec{
			Name:      "zero-replicas",
			Namespace: namespace,
			Replicas:  getPtr[int32](0),
		}),
		deployments.GetMock(deployments.MockSpec{
			Name:      "zero-replicas-annotation",
			Namespace: namespace,
			Replicas:  getPtr[int32](0),
			PodAnnotations: map[string]string{
				lastScheduleKey: "2021-03-23T00:00:00.000Z",
			},
		}),
	}

	d := appsv1.DeploymentList{}
	if updateIfAlreadyCreated {
		err := k8sClient.List(ctx, &d)
		require.NoError(t, err)
	}

	for _, deployment := range deployments {
		if findDeployByName(d.Items, deployment.GetName()) != nil {
			deployment.SetManagedFields(nil)
			require.NoError(t, k8sClient.Patch(ctx, &deployment, client.Apply, &client.PatchOptions{
				FieldManager: "kube-green-test",
				Force:        getPtr(true),
			}))
		} else {
			err := k8sClient.Create(ctx, &deployment)
			require.NoError(t, err)
		}

		err := wait.For(conditions.New(c.Client().Resources()).ResourceMatch(deployment.DeepCopy(), func(object k8s.Object) bool {
			originalReplicas := getValueFromPtr(deployment.Spec.Replicas)
			actualDeployment, ok := object.(*appsv1.Deployment)
			require.True(t, ok)
			return originalReplicas == getValueFromPtr(actualDeployment.Spec.Replicas)
		}), wait.WithTimeout(time.Second*10), wait.WithInterval(time.Millisecond*250))
		require.NoError(t, err)
	}
	return deployments
}

// TODO: Use go client instead of controller-runtime
func upsertCronJobs2(t *testing.T, ctx context.Context, c *envconf.Config, updateIfAlreadyCreated bool) []unstructured.Unstructured {
	suspendTrue := true
	suspendFalse := false
	namespace := c.Namespace()

	k8sClient := newControllerRuntimeClient(t, c)

	restMapping, err := k8sClient.RESTMapper().RESTMapping(schema.GroupKind{
		Group: "batch",
		Kind:  "CronJob",
	})
	require.NoError(t, err)

	version := getCronJobAPIVersion2(restMapping)

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
		if obj := findResourceByName(objList.Items, cronJob.GetName()); obj != nil {
			patch := client.MergeFrom(obj)
			if err := k8sClient.Patch(ctx, &cronJob, patch); err != nil {
				require.NoError(t, err)
			}
		} else {
			require.NoError(t, k8sClient.Create(ctx, &cronJob))
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

func getDefaultSleepInfo(name, namespace string) *kubegreenv1alpha1.SleepInfo {
	return &kubegreenv1alpha1.SleepInfo{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SleepInfo",
			APIVersion: "kube-green.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: kubegreenv1alpha1.SleepInfoSpec{
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
	setupOpts, ok := ctx.Value(setupOptionsKey{}).(setupOptions)
	if !ok {
		return setupOptions{}
	}
	return setupOpts
}

// TODO: This function should be removed when e2e-framework > 0.0.8
func newControllerRuntimeClient(t *testing.T, c *envconf.Config) client.Client {
	t.Helper()
	r, err := resources.New(c.Client().RESTConfig())
	require.NoError(t, err)

	client, err := client.New(c.Client().RESTConfig(), client.Options{Scheme: r.GetScheme()})
	require.NoError(t, err)

	return client
}

type mockClock struct {
	now string
	t   *testing.T
}

// TODO: when remove gomega, remove also the panic else and make the t assertion
func (m mockClock) Now() time.Time {
	// if m.t == nil {
	// 	panic("testing.T not passed in mockClock")
	// }
	parsedTime, err := time.Parse(time.RFC3339, m.now)
	if m.t != nil {
		require.NoError(m.t, err)
	} else {
		if err != nil {
			panic(err.Error())
		}
	}
	return parsedTime
}

func getDeploymentList(t *testing.T, ctx context.Context, c *envconf.Config) []appsv1.Deployment {
	t.Helper()
	deployments := appsv1.DeploymentList{}
	require.NoError(t, c.Client().Resources(c.Namespace()).List(ctx, &deployments))
	return deployments.Items
}

func getCronJobList(t *testing.T, ctx context.Context, c *envconf.Config) []unstructured.Unstructured {
	k8sClient := newControllerRuntimeClient(t, c)

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

func assertAllReplicasSetToZero2(t *testing.T, actualDeployments []appsv1.Deployment, originalDeployments []appsv1.Deployment) {
	t.Helper()

	allReplicas := []int32{}
	for _, deployment := range actualDeployments {
		allReplicas = append(allReplicas, *deployment.Spec.Replicas)
	}
	for _, replicas := range allReplicas {
		require.Equal(t, replicas, int32(0))
	}
}

func assertAllCronJobsSuspended2(t *testing.T, actualCronJobs []unstructured.Unstructured, originalCronJobs []unstructured.Unstructured) {
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
			suspend: isCronJobSuspended(cronJob),
		})
	}
	for _, suspended := range allSuspended {
		require.True(t, suspended.suspend, suspended.name)
	}
}

func findDeployByName(deployments []appsv1.Deployment, nameToFind string) *appsv1.Deployment {
	for _, deployment := range deployments {
		if deployment.Name == nameToFind {
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

func getCronJobAPIVersion2(restMapping *meta.RESTMapping) string {
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

func getSuspendStatus(t *testing.T, cronjob unstructured.Unstructured) bool {
	suspend, _, err := unstructured.NestedBool(cronjob.Object, "spec", "suspend")
	require.NoError(t, err)
	t.Logf("suspend for %s is set: %v", cronjob.GetName(), suspend)
	return suspend
}

func getPtr[T any](item T) *T {
	return &item
}

func getValueFromPtr[T any](item *T) T {
	if item == nil {
		var r T
		return r
	}
	return *item
}
