package cronjobs

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/davidebianchi/kube-green/controllers/internal/testutil"
	"github.com/davidebianchi/kube-green/controllers/sleepinfo/resource"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestCronJobs(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	namespace := "my-namespace"
	suspendTrue := true
	suspendFalse := false
	cronJob1 := GetMock(MockSpec{
		Name:      "cj1",
		Namespace: namespace,
	})
	cronJob2 := GetMock(MockSpec{
		Name:      "cj2",
		Namespace: namespace,
	})
	cronJobOtherNamespace := GetMock(MockSpec{
		Name:      "cjOtherNamespace",
		Namespace: "other-namespace",
	})
	suspendedCronJobs := GetMock(MockSpec{
		Name:      "cj-suspended",
		Namespace: namespace,
		Suspend:   &suspendTrue,
	})
	cronJobSuspendSetToFalseNotEmpty := GetMock(MockSpec{
		Name:      "cj-suspend-set-false",
		Namespace: namespace,
		Suspend:   &suspendFalse,
	})
	sleepInfo := &v1alpha1.SleepInfo{
		Spec: v1alpha1.SleepInfoSpec{
			SuspendCronjobs: true,
		},
	}

	getNewResource := func(t *testing.T, client client.Client, originalSuspendedCronJob map[string]bool) cronjobs {
		c, err := NewResource(context.Background(), resource.ResourceClient{
			Client:    client,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, namespace, originalSuspendedCronJob)
		require.NoError(t, err)

		return c
	}

	t.Run("NewResource", func(t *testing.T) {
		listCronJobsTests := []struct {
			name      string
			client    client.Client
			expected  []batchv1.CronJob
			sleepInfo *v1alpha1.SleepInfo
			throws    bool
		}{
			{
				name: "get list of cron jobs",
				client: fake.
					NewClientBuilder().
					WithRuntimeObjects([]runtime.Object{&cronJob1, &cronJob2, &cronJobOtherNamespace}...).
					Build(),
				expected:  []batchv1.CronJob{cronJob1, cronJob2},
				sleepInfo: sleepInfo,
			},
			{
				name:      "fails to list cron job",
				sleepInfo: sleepInfo,
				client: &testutil.PossiblyErroringFakeCtrlRuntimeClient{
					Client: fake.NewClientBuilder().Build(),
					ShouldError: func(method testutil.Method, obj runtime.Object) bool {
						return method == testutil.List
					},
				},
				throws: true,
			},
			{
				name: "empty list cron job",
				client: fake.
					NewClientBuilder().
					WithRuntimeObjects([]runtime.Object{&cronJobOtherNamespace}...).
					Build(),
				sleepInfo: sleepInfo,
				expected:  []batchv1.CronJob{},
			},
			{
				name: "disabled cronjob suspend",
				client: fake.
					NewClientBuilder().
					WithRuntimeObjects([]runtime.Object{&cronJob1, &cronJob2}...).
					Build(),
				sleepInfo: &v1alpha1.SleepInfo{},
				expected:  []batchv1.CronJob{},
			},
		}

		for _, test := range listCronJobsTests {
			t.Run(test.name, func(t *testing.T) {
				r := resource.ResourceClient{
					Client:    test.client,
					Log:       testLogger,
					SleepInfo: test.sleepInfo,
				}

				resource, err := NewResource(context.Background(), r, namespace, map[string]bool{})
				if test.throws {
					require.EqualError(t, err, "error during list")
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, test.expected, resource.data)
			})
		}
	})

	t.Run("HasResources", func(t *testing.T) {
		t.Run("without resource", func(t *testing.T) {
			c := getNewResource(t, fake.NewClientBuilder().Build(), nil)
			require.False(t, c.HasResource())
		})

		t.Run("with resource", func(t *testing.T) {
			c := getNewResource(t, fake.NewClientBuilder().WithRuntimeObjects(&cronJob1).Build(), nil)
			require.True(t, c.HasResource())
		})
	})

	t.Run("Sleep", func(t *testing.T) {
		t.Run("not throws if no data", func(t *testing.T) {
			c := getNewResource(t, fake.NewClientBuilder().Build(), nil)
			require.NoError(t, c.Sleep(context.Background()))
		})

		t.Run("suspend cronjobs", func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithRuntimeObjects(&cronJob1, &cronJob2, &suspendedCronJobs, &cronJobSuspendSetToFalseNotEmpty).Build()
			c := getNewResource(t, fakeClient, nil)
			require.NoError(t, c.Sleep(context.Background()))

			cjList, err := c.getListByNamespace(context.Background(), namespace)
			require.NoError(t, err)

			require.Equal(t, []batchv1.CronJob{
				suspendAndUpdateResourceVersion(t, cronJobSuspendSetToFalseNotEmpty),
				suspendedCronJobs,
				suspendAndUpdateResourceVersion(t, cronJob1),
				suspendAndUpdateResourceVersion(t, cronJob2),
			}, cjList)
		})

		t.Run("fails to suspend cronjobs", func(t *testing.T) {
			fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client: fake.NewClientBuilder().WithRuntimeObjects(&cronJob1, &cronJob2, &suspendedCronJobs).Build(),
				ShouldError: func(method testutil.Method, obj runtime.Object) bool {
					return method == testutil.Patch
				},
			}
			c := getNewResource(t, fakeClient, nil)
			require.EqualError(t, c.Sleep(context.Background()), "error during patch")
		})
	})

	t.Run("WakeUp", func(t *testing.T) {
		suspendedCronJob1 := convertCronJobToBeSuspended(t, cronJob1)
		suspendedCronJob2 := convertCronJobToBeSuspended(t, cronJob2)

		t.Run("not throws if no data", func(t *testing.T) {
			c := getNewResource(t, fake.NewClientBuilder().Build(), nil)
			require.NoError(t, c.WakeUp(context.Background()))
		})

		t.Run("wake up only cron job slept by controller", func(t *testing.T) {
			cronJobAddedToNamespaceAfterWakeUp := GetMock(MockSpec{
				Name:      "cronjob-added-to-namespace-after-wake-up",
				Schedule:  "0 0 0 * * *",
				Namespace: namespace,
			})
			fakeK8sClient := fake.
				NewClientBuilder().
				WithRuntimeObjects(
					&suspendedCronJob1,
					&suspendedCronJob2,
					&cronJobAddedToNamespaceAfterWakeUp,
					&suspendedCronJobs,
				).
				Build()

			c := getNewResource(t, fakeK8sClient, map[string]bool{
				cronJob1.Name:                         false,
				cronJob2.Name:                         false,
				cronJobSuspendSetToFalseNotEmpty.Name: false, // cron job deleted from namespace during sleep
			})
			require.NoError(t, c.WakeUp(context.Background()))

			cronJobList, err := c.getListByNamespace(context.Background(), namespace)
			require.NoError(t, err)
			require.Equal(t, []batchv1.CronJob{
				suspendedCronJobs,
				updateResourceVersion(t, cronJob1),
				updateResourceVersion(t, cronJob2),
				cronJobAddedToNamespaceAfterWakeUp,
			}, cronJobList)
		})

		t.Run("fails to wake up", func(t *testing.T) {
			fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client: fake.NewClientBuilder().WithRuntimeObjects(&suspendedCronJob1, &suspendedCronJob2, &suspendedCronJobs).Build(),
				ShouldError: func(method testutil.Method, obj runtime.Object) bool {
					return method == testutil.Patch
				},
			}
			c := getNewResource(t, fakeClient, map[string]bool{
				cronJob1.Name: false,
				cronJob2.Name: false,
			})
			require.EqualError(t, c.WakeUp(context.Background()), "error during patch")
		})
	})

	t.Run("GetOriginalInfoToSave", func(t *testing.T) {
		t.Run("without cron jobs", func(t *testing.T) {
			c := getNewResource(t, fake.NewClientBuilder().Build(), nil)
			res, err := c.GetOriginalInfoToSave()
			require.NoError(t, err)
			require.JSONEq(t, `[]`, string(res))
		})

		t.Run("with cron jobs", func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithRuntimeObjects(&cronJob1, &cronJob2, &suspendedCronJobs, &cronJobSuspendSetToFalseNotEmpty).
				Build()
			c := getNewResource(t, fakeClient, nil)
			res, err := c.GetOriginalInfoToSave()
			require.NoError(t, err)
			require.JSONEq(t, `[{"name":"cj-suspend-set-false","suspend":false},{"name":"cj1","suspend":false},{"name":"cj2","suspend":false}]`, string(res))
		})
	})

	t.Run("GetOriginalInfoToRestore", func(t *testing.T) {
		t.Run("if empty saved data, returns empty status", func(t *testing.T) {
			suspendedStatus, err := GetOriginalInfoToRestore(nil)
			require.NoError(t, err)
			require.Equal(t, OriginalSuspendStatus{}, suspendedStatus)
		})

		t.Run("throws if data is not a correct json", func(t *testing.T) {
			suspendedStatus, err := GetOriginalInfoToRestore([]byte("{"))
			require.Nil(t, suspendedStatus)
			require.EqualError(t, err, "unexpected end of JSON input")
		})

		t.Run("with empty data returns empty status", func(t *testing.T) {
			suspendedStatus, err := GetOriginalInfoToRestore([]byte("[]"))
			require.NoError(t, err)
			require.Equal(t, OriginalSuspendStatus{}, suspendedStatus)
		})

		t.Run("correctly returns data", func(t *testing.T) {
			savedData := []byte(`[{"name":"cj1","suspend":false},{"name":"cj2","suspend":true},{"name":"cj3","suspend":false},{"name":"","suspend":false}]`)
			suspendedStatus, err := GetOriginalInfoToRestore(savedData)
			require.NoError(t, err)
			require.Equal(t, OriginalSuspendStatus{
				"cj1": false,
				"cj2": true,
				"cj3": false,
			}, suspendedStatus)
		})
	})
}

func suspendAndUpdateResourceVersion(t *testing.T, cronJob batchv1.CronJob) batchv1.CronJob {
	return updateResourceVersion(t, convertCronJobToBeSuspended(t, cronJob))
}

func convertCronJobToBeSuspended(t *testing.T, cronJob batchv1.CronJob) batchv1.CronJob {
	suspendTrue := true
	newCronJob := cronJob.DeepCopy()
	newCronJob.Spec.Suspend = &suspendTrue
	return *newCronJob
}

func updateResourceVersion(t *testing.T, cronJob batchv1.CronJob) batchv1.CronJob {
	resourceVersion, err := strconv.Atoi(cronJob.ResourceVersion)
	require.NoError(t, err)
	newCronJob := cronJob.DeepCopy()
	newCronJob.ResourceVersion = fmt.Sprintf("%v", resourceVersion+1)
	return *newCronJob
}
