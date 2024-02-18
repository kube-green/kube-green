package sleepinfo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/sleepinfo/cronjobs"
	"github.com/kube-green/kube-green/controllers/sleepinfo/deployments"
	"github.com/kube-green/kube-green/controllers/sleepinfo/internal/mocks"
	"github.com/kube-green/kube-green/controllers/sleepinfo/metrics"
	"github.com/kube-green/kube-green/internal/patcher"

	"github.com/go-logr/logr"
	promTestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestSleepInfoControllerReconciliation(t *testing.T) {
	const (
		sleepInfoName = "default-sleep"
		mockNow       = "2021-03-23T20:01:20.555Z"
		sleepTime     = "2021-03-23T20:05:59.000Z"
		wakeUpTime    = "2021-03-23T20:19:50.100Z"
		sleepTime2    = "2021-03-23T21:05:00.000Z"

		excludeLabelsKey   = "kube-green.dev/exclude"
		excludeLabelsValue = "true"
	)
	testLogger := zap.New(zap.UseDevMode(true))

	zeroDeployments := features.New("with zero deployments").
		WithSetup("create SleepInfo", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			t.Helper()
			createSleepInfoCRD(t, ctx, c, getDefaultSleepInfo(sleepInfoName, c.Namespace()))

			return ctx
		}).
		Assess("is requeued if not sleep time", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfoReconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sleepInfoName,
					Namespace: c.Namespace(),
				},
			}
			result, err := sleepInfoReconciler.Reconcile(ctx, req)
			require.NoError(t, err)
			require.Equal(t, ctrl.Result{
				RequeueAfter: sleepRequeue(mockNow),
			}, result)
			return withAssertOperation(ctx, AssertOperation{
				reconciler: sleepInfoReconciler,
				req:        req,
			})
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepScheduleTime := sleepTime
			assertOperations := getAssertOperation(t, ctx)

			sleepInfoReconciler := getSleepInfoReconciler(t, c, testLogger, sleepScheduleTime)
			sleepInfoReconciler.Metrics = assertOperations.reconciler.Metrics

			result, err := sleepInfoReconciler.Reconcile(ctx, assertOperations.req)
			require.NoError(t, err)
			deployments := getDeploymentList(t, ctx, c)
			require.Len(t, deployments, 0)
			require.Equal(t, ctrl.Result{
				// sleep is: now + next wake up + next sleep
				RequeueAfter: wakeUpRequeue(sleepScheduleTime) + sleepRequeue("2021-03-23T20:20:00.000Z"),
			}, result)

			secret, err := sleepInfoReconciler.getSecret(ctx, getSecretName(sleepInfoName), c.Namespace())
			require.NoError(t, err)
			require.NotNil(t, secret)
			require.Equal(t, map[string][]byte{
				lastScheduleKey: []byte(parseTime(t, sleepScheduleTime).Format(time.RFC3339)),
			}, secret.Data)

			sleepInfo, err := sleepInfoReconciler.getSleepInfo(ctx, assertOperations.req)
			require.NoError(t, err)
			require.Equal(t, kubegreenv1alpha1.SleepInfoStatus{
				LastScheduleTime: metav1.NewTime(parseTime(t, sleepScheduleTime).Local()),
			}, sleepInfo.Status)

			return withAssertOperation(ctx, AssertOperation{
				reconciler: sleepInfoReconciler,
				req:        assertOperations.req,
			})
		}).
		Assess("WAKE_UP is skipped", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assertOperations := getAssertOperation(t, ctx)
			lastSleepScheduleTime := assertOperations.reconciler.Clock.Now().Format(time.RFC3339)

			sleepInfoReconciler := getSleepInfoReconciler(t, c, testLogger, "2021-03-23T20:07:00.000Z")

			result, err := sleepInfoReconciler.Reconcile(ctx, assertOperations.req)
			require.NoError(t, err)
			deployments := getDeploymentList(t, ctx, c)
			require.Len(t, deployments, 0)
			require.Equal(t, ctrl.Result{
				RequeueAfter: sleepRequeue(sleepInfoReconciler.Now().Format(time.RFC3339)),
			}, result)

			sleepInfo, err := sleepInfoReconciler.getSleepInfo(ctx, assertOperations.req)
			require.NoError(t, err)
			require.Equal(t, kubegreenv1alpha1.SleepInfoStatus{
				LastScheduleTime: metav1.NewTime(parseTime(t, lastSleepScheduleTime).Local()),
			}, sleepInfo.Status)

			secret, err := sleepInfoReconciler.getSecret(ctx, getSecretName(sleepInfoName), c.Namespace())
			require.NoError(t, err)
			require.NotNil(t, secret)
			require.Equal(t, map[string][]byte{
				lastScheduleKey: []byte(parseTime(t, lastSleepScheduleTime).Format(time.RFC3339)),
			}, secret.Data)

			return ctx
		}).
		Feature()

	notExistentResource := features.New("not existent resource return without error").
		Assess("return empty", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "not-exists",
					Namespace: c.Namespace(),
				},
			}
			sleepInfoReconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)

			result, err := sleepInfoReconciler.Reconcile(ctx, req)
			require.NoError(t, err)
			require.Empty(t, result)

			return ctx
		}).
		Feature()

	notExistentNamespace := features.New("not existent namespace in request").
		Assess("does not trigger another reconcile loop", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfoReconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sleepInfoName,
					Namespace: "not-exists",
				},
			}
			result, err := sleepInfoReconciler.Reconcile(ctx, req)
			require.NoError(t, err)
			require.Empty(t, result)

			return ctx
		}).
		Feature()

	withDeployment := features.New("with deployments").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			return reconciliationSetup(t, ctx, c, mockNow, sleepInfo)
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assertOperation := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assertOperation.withScheduleAndExpectedSchedule(sleepTime).withRequeue(14*60+1))
			assertCorrectSleepOperation(t, ctx, c)

			return ctx
		}).
		Assess("wake up", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assertOperation := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assertOperation.withScheduleAndExpectedSchedule("2021-03-23T20:19:50.100Z").withRequeue(45*60+9.9))
			assertCorrectWakeUpOperation(t, ctx, c)

			return ctx
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assertOperation := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assertOperation.withScheduleAndExpectedSchedule("2021-03-23T21:05:00.000Z").withRequeue(15*60))
			assertCorrectSleepOperation(t, ctx, c)

			return ctx
		}).
		Feature()

	deployBetweenCycle := features.New("deploy between sleep and wake up").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			return reconciliationSetup(t, ctx, c, mockNow, sleepInfo)
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime).withRequeue(14*60+1))
			assertCorrectSleepOperation(t, ctx, c)
			return ctx
		}).
		Assess("redeploy before the wakeup", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			upsertDeployments(t, ctx, c, true)

			deployments := getDeploymentList(t, ctx, c)
			assert := getAssertOperation(t, ctx)
			for _, deployment := range deployments {
				originalDeployment := findDeployByName(assert.originalResources.deploymentList, deployment.Name)
				require.Equal(t, originalDeployment.Spec.Replicas, deployment.Spec.Replicas)
			}

			return ctx
		}).
		Assess("wake up", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule("2021-03-23T20:20:00.000Z").withRequeue(45*60))
			assertCorrectWakeUpOperation(t, ctx, c)
			return ctx
		}).Feature()

	changeSingleDeplymentBetweenCycle := features.New("change single deployment replicas between sleep and wake up").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			return reconciliationSetup(t, ctx, c, mockNow, sleepInfo)
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime).withRequeue(14*60+1))
			assertCorrectSleepOperation(t, ctx, c)

			return ctx
		}).
		Assess("redeploy a single deploy", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()
			deployments := getDeploymentList(t, ctx, c)

			deploymentToUpdate := deployments[0].DeepCopy()
			patch := client.MergeFrom(deploymentToUpdate)
			*deploymentToUpdate.Spec.Replicas = 0
			err := k8sClient.Patch(ctx, deploymentToUpdate, patch)
			require.NoError(t, err)

			updatedDeployment := appsv1.Deployment{}
			require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: deploymentToUpdate.Name, Namespace: c.Namespace()}, &updatedDeployment))
			require.Equal(t, int32(0), *updatedDeployment.Spec.Replicas)

			return ctx
		}).
		Assess("wake up", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule("2021-03-23T20:20:00.000Z").withRequeue(45*60))
			assertCorrectWakeUpOperation(t, ctx, c)

			return ctx
		}).
		Feature()

	twiceSleepOperationConsecutively := features.New("twice consecutive sleep operation").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			return reconciliationSetup(t, ctx, c, mockNow, sleepInfo)
		}).
		Assess("sleep #1", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime).withRequeue(14*60+1))
			assertCorrectSleepOperation(t, ctx, c)

			return ctx
		}).
		Assess("sleep #2", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withSchedule("2021-03-23T21:05:00.000Z").withExpectedSchedule("2021-03-23T20:05:59Z").withRequeue(15*60))
			assertCorrectSleepOperation(t, ctx, c)

			return ctx
		}).Feature()

	onlySleep := features.New("only sleep, wake up set to nil").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			sleepInfo.Spec.WakeUpTime = ""

			return reconciliationSetup(t, ctx, c, mockNow, sleepInfo)
		}).
		Assess("sleep #1", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime).withRequeue(59*60+1))
			assertCorrectSleepOperation(t, ctx, c)
			return ctx
		}).
		Assess("sleep #2", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule("2021-03-23T21:05:00.000Z").withRequeue(60*60))
			assertCorrectSleepOperation(t, ctx, c)
			return ctx
		}).
		Assess("deploy", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			upsertDeployments(t, ctx, c, true)

			deployments := getDeploymentList(t, ctx, c)
			assert := getAssertOperation(t, ctx)
			for _, deployment := range deployments {
				originalDeployment := findDeployByName(assert.originalResources.deploymentList, deployment.Name)
				require.Equal(t, originalDeployment.Spec.Replicas, deployment.Spec.Replicas)
			}

			return ctx
		}).
		Assess("sleep after deploy", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule("2021-03-23T22:04:00.000Z").withRequeue(61*60))
			assertCorrectSleepOperation(t, ctx, c)
			return ctx
		}).
		Feature()

	sleepInfoNotInNs := features.New("SleepInfo not present in namespace").
		Assess("reconciliation skip", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "not-exists",
					Namespace: c.Namespace(),
				},
			}
			sleepInfoReconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)

			result, err := sleepInfoReconciler.Reconcile(ctx, req)
			require.NoError(t, err)
			require.Empty(t, result)

			return ctx
		}).
		Feature()

	deployedWhenShouldBeTriggered := features.New("SleepInfo deployed when should be triggered").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			createSleepInfoCRD(t, ctx, c, getDefaultSleepInfo(sleepInfoName, c.Namespace()))
			deployments := upsertDeployments(t, ctx, c, false)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sleepInfoName,
					Namespace: c.Namespace(),
				},
			}

			return withAssertOperation(ctx, AssertOperation{
				req: req,
				originalResources: originalResources{
					deploymentList: deployments,
				},
				reconciler: getSleepInfoReconciler(t, c, testLogger, "2021-03-23T20:05:59.999Z"),
			})
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			result, err := assert.reconciler.Reconcile(ctx, assert.req)
			require.NoError(t, err)
			require.Equal(t, ctrl.Result{
				RequeueAfter: wakeUpRequeue("2021-03-23T20:05:59.999Z"),
			}, result)

			deployments := getDeploymentList(t, ctx, c)
			assertAllReplicasSetToZero(t, deployments)
			return ctx
		}).
		Feature()

	newDeploymentBeforeWakeUp := features.New("new deployment between sleep and wake up").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			return reconciliationSetup(t, ctx, c, mockNow, sleepInfo)
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime).nextWakeUp())
			assertCorrectSleepOperation(t, ctx, c)
			return ctx
		}).
		Assess("create new deployment", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			serviceNameToCreate := "new-service"
			deployToCreate := deployments.GetMock(deployments.MockSpec{
				Namespace: c.Namespace(),
				Name:      serviceNameToCreate,
				Replicas:  getPtr[int32](5),
				MatchLabels: map[string]string{
					"app": serviceNameToCreate,
				},
			})
			k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()
			err := k8sClient.Create(ctx, deployToCreate.DeepCopy())
			require.NoError(t, err)

			assert := getAssertOperation(t, ctx)
			assert.originalResources.deploymentList = append(assert.originalResources.deploymentList, deployToCreate)
			ctx = withAssertOperation(ctx, assert)
			deployments := getDeploymentList(t, ctx, c)
			for _, deployment := range deployments {
				if deployment.Name == serviceNameToCreate {
					require.Equal(t, int32(5), *deployment.Spec.Replicas)
					continue
				}
				require.Equal(t, int32(0), *deployment.Spec.Replicas, deployment.GetName())
			}

			return ctx
		}).
		Assess("wake up", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule("2021-03-23T20:20:00.000Z").nextSleep())
			assertCorrectWakeUpOperation(t, ctx, c)
			return ctx
		}).
		Feature()

	withDeploymentAndCronJobs := features.New("with Deployment and CronJob - deploy between sleep and wakeup").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			sleepInfo.Spec.SuspendCronjobs = true

			ctx = withSetupOptions(ctx, setupOptions{
				insertCronjobs: true,
			})
			return reconciliationSetup(t, ctx, c, mockNow, sleepInfo)
		}).
		Assess("sleep #1", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime).nextWakeUp())
			assertCorrectSleepOperation(t, ctx, c)
			return ctx
		}).
		Assess("re deploy", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			upsertDeployments(t, ctx, c, true)
			upsertCronJobs(t, ctx, c, true)

			assert := getAssertOperation(t, ctx)

			deployments := getDeploymentList(t, ctx, c)
			for _, deployment := range deployments {
				originalDeployment := findDeployByName(assert.originalResources.deploymentList, deployment.Name)
				require.Equal(t, originalDeployment.Spec.Replicas, deployment.Spec.Replicas)
			}

			cronJobs := getCronJobList(t, ctx, c)
			for _, cronJob := range cronJobs {
				originalCronJob := findResourceByName(assert.originalResources.cronjobList, cronJob.GetName())
				require.Equal(t, isCronJobSuspended(t, *originalCronJob), isCronJobSuspended(t, cronJob))
			}

			return ctx
		}).
		Assess("wake up", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(wakeUpTime).nextSleep())
			assertCorrectWakeUpOperation(t, ctx, c)
			return ctx
		}).
		Assess("sleep #2", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime2).nextWakeUp())
			assertCorrectSleepOperation(t, ctx, c)
			return ctx
		}).
		Feature()

	deleteSleepInfo := features.New("reconcile - delete SleepInfo").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			return reconciliationSetup(t, ctx, c, mockNow, sleepInfo)
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime).nextWakeUp())
			assertCorrectSleepOperation(t, ctx, c)
			return ctx
		}).
		Assess("check metrics", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			m := assert.reconciler.Metrics
			require.Equal(t, 1, promTestutil.CollectAndCount(m.CurrentSleepInfo))

			return ctx
		}).
		Assess("delete SleepInfo and check metrics", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			sleepInfo, err := assert.reconciler.getSleepInfo(ctx, assert.req)
			require.NoError(t, err)
			k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()

			err = k8sClient.Delete(ctx, sleepInfo)
			require.NoError(t, err)

			err = wait.For(
				conditions.New(c.Client().Resources()).ResourceDeleted(sleepInfo),
				wait.WithTimeout(time.Second*5),
				wait.WithInterval(time.Millisecond*100),
			)
			require.NoError(t, err)

			_, err = assert.reconciler.Reconcile(ctx, assert.req)
			require.NoError(t, err)

			m := assert.reconciler.Metrics
			require.Equal(t, 0, promTestutil.CollectAndCount(m.CurrentSleepInfo))

			return ctx
		}).
		Feature()

	withGenericResources := features.New("with generic resources").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			sleepInfo.Spec.Patches = []kubegreenv1alpha1.Patch{
				{
					Target: kubegreenv1alpha1.PatchTarget{
						Group: "apps",
						Kind:  "StatefulSet",
					},
					Patch: `
- op: add
  path: /spec/replicas
  value: 0
`,
				},
				{
					Target: kubegreenv1alpha1.PatchTarget{
						Group: "apps",
						Kind:  "ReplicaSet",
					},
					Patch: `
- path: /spec/replicas
  op: replace
  value: 0
`,
				},
			}
			sleepInfo.Spec.ExcludeRef = []kubegreenv1alpha1.ExcludeRef{
				{
					MatchLabels: map[string]string{
						excludeLabelsKey: excludeLabelsValue,
					},
				},
				{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "exclude-by-name",
				},
			}

			ctx = withSetupOptions(ctx, setupOptions{
				customResources: []unstructured.Unstructured{
					mocks.StatefulSet(mocks.StatefulSetOptions{
						Name:      "statefulset-1",
						Replicas:  getPtr[int32](1),
						Namespace: c.Namespace(),
					}).Unstructured(),
					mocks.StatefulSet(mocks.StatefulSetOptions{
						Name:      "statefulset-to-exclude-with-labels",
						Replicas:  getPtr[int32](1),
						Namespace: c.Namespace(),
						Labels: map[string]string{
							excludeLabelsKey: excludeLabelsValue,
						},
					}).Unstructured(),
					mocks.StatefulSet(mocks.StatefulSetOptions{
						Name:      "exclude-by-name",
						Replicas:  getPtr[int32](1),
						Namespace: c.Namespace(),
					}).Unstructured(),
				},
			})
			return reconciliationSetup(t, ctx, c, mockNow, sleepInfo)
		}).
		Assess("sleep #1", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx).withScheduleAndExpectedSchedule(sleepTime).nextWakeUp()
			ctx = withAssertOperation(ctx, assert)
			sleepInfoReconciler := nextReconciler(t, ctx)

			listGenericResources := getGenericResourcesMap(t, ctx, c, assert.originalResources.sleepInfo.GetPatches())

			t.Run("generic resources are correctly patched", func(t *testing.T) {
				for _, genericResource := range listGenericResources {
					for _, res := range genericResource.data {
						// resource to exclude by labels
						if res.GetLabels()[excludeLabelsKey] == excludeLabelsValue {
							continue
						}
						// resource to exclude by name
						if res.GetKind() == "StatefulSet" && res.GetAPIVersion() == "apps/v1" && res.GetName() == "exclude-by-name" {
							continue
						}

						patch := findPatchByTarget(assert.originalResources.sleepInfo.GetPatches(), res.GroupVersionKind())
						require.NotNil(t, patch)
						patcher, err := patcher.New(patch)
						require.NoError(t, err)
						new, err := json.Marshal(res.Object)
						require.NoError(t, err)
						output, err := patcher.Exec(new)
						require.NoError(t, err)
						require.Equal(t, output, new)
					}
				}
			})

			t.Run("secrets correctly created", func(t *testing.T) {
				secret, err := sleepInfoReconciler.getSecret(ctx, getSecretName(assert.originalResources.sleepInfo.GetName()), c.Namespace())
				require.NoError(t, err)
				secretData := secret.Data
				require.Equal(t, map[string][]byte{
					lastScheduleKey:          []byte(parseTime(t, assert.expectedScheduleTime).Truncate(time.Second).Format(time.RFC3339)),
					lastOperationKey:         []byte(sleepOperation),
					replicasBeforeSleepKey:   []byte(`[{"name":"service-1","replicas":3},{"name":"service-2","replicas":1}]`),
					originalJSONPatchDataKey: []byte(`{"StatefulSet.apps":{"statefulset-1":"{\"spec\":{\"replicas\":1}}"}}`),
				}, secretData)
			})

			return ctx
		}).
		Assess("wake up", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx).withScheduleAndExpectedSchedule(wakeUpTime).nextSleep()
			ctx = withAssertOperation(ctx, assert)
			sleepInfoReconciler := nextReconciler(t, ctx)

			listGenericResources := getGenericResourcesMap(t, ctx, c, assert.originalResources.sleepInfo.GetPatches())

			t.Run("generic resources are correctly patched", func(t *testing.T) {
				for _, genericResource := range listGenericResources {
					for _, res := range genericResource.data {
						originalRes := findResourceByName(assert.originalResources.genericResourcesMap.getResourceList(res.GroupVersionKind()), res.GetName())
						require.NotNil(t, originalRes, "resource not found: %s and type %s", res.GetName(), res.GroupVersionKind().String())
						originalSpec := originalRes.Object["spec"].(map[string]interface{})
						spec := res.Object["spec"].(map[string]interface{})
						require.Equal(t, originalSpec, spec)
					}
				}
			})

			t.Run("secrets correctly created", func(t *testing.T) {
				secret, err := sleepInfoReconciler.getSecret(ctx, getSecretName(assert.originalResources.sleepInfo.GetName()), c.Namespace())
				require.NoError(t, err)
				secretData := secret.Data
				require.Equal(t, map[string][]byte{
					lastScheduleKey:  []byte(parseTime(t, assert.expectedScheduleTime).Truncate(time.Second).Format(time.RFC3339)),
					lastOperationKey: []byte(wakeUpOperation),
				}, secretData)
			})

			return ctx
		}).
		Assess("sleep #2", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx).withScheduleAndExpectedSchedule(sleepTime2).nextWakeUp()
			ctx = withAssertOperation(ctx, assert)

			sleepInfoReconciler := nextReconciler(t, ctx)

			listGenericResources := getGenericResourcesMap(t, ctx, c, assert.originalResources.sleepInfo.GetPatches())

			t.Run("generic resources are correctly patched", func(t *testing.T) {
				for _, genericResource := range listGenericResources {
					for _, res := range genericResource.data {
						// resource to exclude by labels
						if res.GetLabels()[excludeLabelsKey] == excludeLabelsValue {
							continue
						}
						// resource to exclude by name
						if res.GetKind() == "StatefulSet" && res.GetAPIVersion() == "apps/v1" && res.GetName() == "exclude-by-name" {
							continue
						}

						patch := findPatchByTarget(assert.originalResources.sleepInfo.GetPatches(), res.GroupVersionKind())
						require.NotNil(t, patch)
						patcher, err := patcher.New(patch)
						require.NoError(t, err)
						new, err := json.Marshal(res.Object)
						require.NoError(t, err)
						output, err := patcher.Exec(new)
						require.NoError(t, err)
						require.Equal(t, output, new)
					}
				}
			})

			t.Run("secrets correctly created", func(t *testing.T) {
				secret, err := sleepInfoReconciler.getSecret(ctx, getSecretName(assert.originalResources.sleepInfo.GetName()), c.Namespace())
				require.NoError(t, err)
				secretData := secret.Data
				require.Equal(t, map[string][]byte{
					lastScheduleKey:          []byte(parseTime(t, assert.expectedScheduleTime).Truncate(time.Second).Format(time.RFC3339)),
					lastOperationKey:         []byte(sleepOperation),
					replicasBeforeSleepKey:   []byte(`[{"name":"service-1","replicas":3},{"name":"service-2","replicas":1}]`),
					originalJSONPatchDataKey: []byte(`{"StatefulSet.apps":{"statefulset-1":"{\"spec\":{\"replicas\":1}}"}}`),
				}, secretData)
			})
			return ctx
		}).
		Feature()

	testenv.TestInParallel(t,
		zeroDeployments,
		notExistentResource,
		notExistentNamespace,
		withDeployment,
		deployBetweenCycle,
		changeSingleDeplymentBetweenCycle,
		twiceSleepOperationConsecutively,
		onlySleep,
		sleepInfoNotInNs,
		deployedWhenShouldBeTriggered,
		newDeploymentBeforeWakeUp,
		withDeploymentAndCronJobs,
		deleteSleepInfo,
		withGenericResources,
	)
}

func TestDifferentSleepInfoConfiguration(t *testing.T) {
	const (
		sleepInfoName = "default-sleep"
		mockNow       = "2021-03-23T20:01:20.554Z"
		sleepTime     = "2021-03-23T20:05:59.000Z"
		wakeUpTime    = "2021-03-23T20:19:50.100Z"
		sleepTime2    = "2021-03-23T21:05:00.000Z"
	)

	table := []struct {
		name         string
		getSleepInfo func(t *testing.T, c *envconf.Config) *kubegreenv1alpha1.SleepInfo
		setupOptions setupOptions
	}{
		{
			name: "with deployments to exclude",
			getSleepInfo: func(t *testing.T, c *envconf.Config) *kubegreenv1alpha1.SleepInfo {
				sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
				sleepInfo.Spec.ExcludeRef = []kubegreenv1alpha1.ExcludeRef{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "service-1",
					},
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "zero-replicas",
					},
				}
				return sleepInfo
			},
		},
		{
			name: "suspend CronJobs",
			getSleepInfo: func(t *testing.T, c *envconf.Config) *kubegreenv1alpha1.SleepInfo {
				sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
				sleepInfo.Spec.SuspendCronjobs = true

				return sleepInfo
			},
			setupOptions: setupOptions{
				insertCronjobs: true,
			},
		},
		{
			name: "with only CronJob to suspend",
			getSleepInfo: func(t *testing.T, c *envconf.Config) *kubegreenv1alpha1.SleepInfo {
				sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
				sleepInfo.Spec.SuspendCronjobs = true
				sleepInfo.Spec.SuspendDeployments = getPtr(false)

				return sleepInfo
			},
			setupOptions: setupOptions{
				insertCronjobs: true,
			},
		},
		{
			name: "suspend CronJobs but CronJobs not present in namespace",
			getSleepInfo: func(t *testing.T, c *envconf.Config) *kubegreenv1alpha1.SleepInfo {
				sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
				sleepInfo.Spec.SuspendCronjobs = true

				return sleepInfo
			},
		},
		{
			name: "CronJobs in namespace but not to suspend",
			getSleepInfo: func(t *testing.T, c *envconf.Config) *kubegreenv1alpha1.SleepInfo {
				sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())

				return sleepInfo
			},
			setupOptions: setupOptions{
				insertCronjobs: true,
			},
		},
		{
			name: "exclude Deployment and CronJob",
			getSleepInfo: func(t *testing.T, c *envconf.Config) *kubegreenv1alpha1.SleepInfo {
				sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
				sleepInfo.Spec.SuspendCronjobs = true
				sleepInfo.Spec.ExcludeRef = []kubegreenv1alpha1.ExcludeRef{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "service-1",
					},
					{
						APIVersion: "batch/v1",
						Kind:       "CronJob",
						Name:       "cronjob-2",
					},
				}

				return sleepInfo
			},
			setupOptions: setupOptions{
				insertCronjobs: true,
			},
		},
	}

	featureList := []features.Feature{}
	for _, tableTest := range table {
		test := tableTest // necessary to ensure that the correct value is passed to the closure
		f := features.New(fmt.Sprintf("SleepInfo configuration %s", test.name)).
			Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				sleepInfo := test.getSleepInfo(t, c)

				ctx = withSetupOptions(ctx, test.setupOptions)
				ctx = reconciliationSetup(t, ctx, c, mockNow, sleepInfo)

				return ctx
			}).
			Assess("sleep #1", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				assert := getAssertOperation(t, ctx)
				ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime).nextWakeUp())
				assertCorrectSleepOperation(t, ctx, c)
				return ctx
			}).
			Assess("wake up", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				assert := getAssertOperation(t, ctx)
				ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(wakeUpTime).nextSleep())
				assertCorrectWakeUpOperation(t, ctx, c)
				return ctx
			}).
			Assess("sleep #2", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				assert := getAssertOperation(t, ctx)
				ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime2).nextWakeUp())
				assertCorrectSleepOperation(t, ctx, c)
				return ctx
			}).Feature()

		featureList = append(featureList, f)
	}

	featureList = append(featureList, features.New("SleepInfo configuration both Deployment and CronJob not to suspend").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			sleepInfo.Spec.SuspendCronjobs = false
			sleepInfo.Spec.SuspendDeployments = getPtr(false)

			ctx = withSetupOptions(ctx, setupOptions{
				insertCronjobs: true,
			})
			ctx = reconciliationSetup(t, ctx, c, mockNow, sleepInfo)

			return ctx
		}).
		Assess("sleep #1", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime).withRequeue(59*60+1))
			assertCorrectSleepOperation(t, ctx, c)
			return ctx
		}).
		Assess("sleep #2", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule(sleepTime2).withRequeue(60*60))
			assertCorrectSleepOperation(t, ctx, c)
			return ctx
		}).Feature())

	testenv.TestInParallel(t, featureList...)
}

func TestInvalidResource(t *testing.T) {
	const (
		mockNow = "2021-03-23T20:01:20.555Z"
	)
	testLogger := zap.New(zap.UseDevMode(true))

	invalid := features.Table{
		{
			Name: "not valid sleep schedule",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				sleepInfoName := envconf.RandomName("sleepinfo", 16)
				sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
				sleepInfo.Spec = kubegreenv1alpha1.SleepInfoSpec{
					Weekdays:  "1",
					SleepTime: "",
				}
				createSleepInfoCRD(t, ctx, c, sleepInfo)

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sleepInfoName,
						Namespace: c.Namespace(),
					},
				}
				sleepInfoReconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)

				result, err := sleepInfoReconciler.Reconcile(ctx, req)
				require.EqualError(t, err, "time should be of format HH:mm, actual: ")
				require.Empty(t, result)

				return ctx
			},
		},
		{
			Name: "not valid wake up schedule",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				sleepInfoName := envconf.RandomName("sleepinfo", 16)
				sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
				sleepInfo.Spec = kubegreenv1alpha1.SleepInfoSpec{
					Weekdays:   "1",
					SleepTime:  "*:*",
					WakeUpTime: "*",
				}
				createSleepInfoCRD(t, ctx, c, sleepInfo)

				sleepInfoReconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sleepInfoName,
						Namespace: c.Namespace(),
					},
				}
				result, err := sleepInfoReconciler.Reconcile(ctx, req)
				require.EqualError(t, err, "time should be of format HH:mm, actual: *")
				require.Empty(t, result)

				return ctx
			},
		},
		{
			Name: "not valid weekdays",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				sleepInfoName := envconf.RandomName("sleepinfo", 16)
				sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
				sleepInfo.Spec = kubegreenv1alpha1.SleepInfoSpec{
					Weekdays: "",
				}
				createSleepInfoCRD(t, ctx, c, sleepInfo)

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sleepInfoName,
						Namespace: c.Namespace(),
					},
				}
				sleepInfoReconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)

				result, err := sleepInfoReconciler.Reconcile(ctx, req)
				require.EqualError(t, err, "empty weekdays from SleepInfo configuration")
				require.Empty(t, result)

				return ctx
			},
		},
	}.Build("invalid resources").Feature()

	testenv.TestInParallel(t, invalid)
}

func reconciliationSetup(t *testing.T, ctx context.Context, c *envconf.Config, mockNow string, sleepInfo *kubegreenv1alpha1.SleepInfo) context.Context {
	t.Helper()

	testLogger := zap.New(zap.UseDevMode(true))

	reconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)
	opts := getSetupOptions(t, ctx)

	req, originalResources := setupNamespaceWithResources(t, ctx, c, sleepInfo, reconciler, opts)
	assertContextInfo := AssertOperation{
		req:                req,
		originalResources:  originalResources,
		reconciler:         reconciler,
		excludedDeployment: []string{},
		excludedCronJob:    []string{},
	}

	if excludeRef := sleepInfo.Spec.ExcludeRef; excludeRef != nil {
		for _, excluded := range excludeRef {
			if excluded.Kind == "Deployment" {
				assertContextInfo.excludedDeployment = append(assertContextInfo.excludedDeployment, excluded.Name)
			}
			if excluded.Kind == "CronJob" {
				assertContextInfo.excludedCronJob = append(assertContextInfo.excludedCronJob, excluded.Name)
			}
		}
	}

	return withAssertOperation(ctx, assertContextInfo)
}

func getSleepInfoReconciler(t *testing.T, c *envconf.Config, logger logr.Logger, now string) SleepInfoReconciler {
	k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()
	return SleepInfoReconciler{
		Clock: mockClock{
			now: now,
			t:   t,
		},
		Client:     k8sClient,
		Log:        logger,
		Metrics:    metrics.SetupMetricsOrDie("kube_green"),
		SleepDelta: 60,
	}
}

func assertCorrectSleepOperation(t *testing.T, ctx context.Context, cfg *envconf.Config) {
	assert := getAssertOperation(t, ctx)
	sleepInfoReconciler := nextReconciler(t, ctx)

	t.Run("replicas are set to 0 to all deployments set to sleep", func(t *testing.T) {
		deployments := getDeploymentList(t, ctx, cfg)
		if assert.originalResources.sleepInfo.IsDeploymentsToSuspend() {
			if len(assert.excludedDeployment) == 0 {
				assertAllReplicasSetToZero(t, deployments)
			} else {
				for _, deployment := range deployments {
					if contains(assert.excludedDeployment, deployment.Name) {
						originalDeployment := findDeployByName(assert.originalResources.deploymentList, deployment.GetName())
						require.Equal(t, originalDeployment.Spec.Replicas, deployment.Spec.Replicas)
						continue
					}
					require.Equal(t, int32(0), *deployment.Spec.Replicas)
				}
			}
		} else {
			for _, deployment := range deployments {
				originalDeployment := findDeployByName(assert.originalResources.deploymentList, deployment.GetName())
				require.Equal(t, originalDeployment.Spec.Replicas, deployment.Spec.Replicas)
			}
		}
	})

	t.Run("cron jobs are correctly suspended", func(t *testing.T) {
		cronJobs := getCronJobList(t, ctx, cfg)
		if assert.originalResources.sleepInfo.IsCronjobsToSuspend() {
			if len(assert.excludedCronJob) == 0 {
				assertAllCronJobsSuspended(t, cronJobs)
			} else {
				for _, cronJob := range cronJobs {
					originalCronJob := findResourceByName(assert.originalResources.cronjobList, cronJob.GetName())
					originalUnstructuredCronJob, err := runtime.DefaultUnstructuredConverter.ToUnstructured(originalCronJob)
					require.NoError(t, err)
					if contains(assert.excludedCronJob, cronJob.GetName()) {
						require.Equal(t, isSuspendedCronJob(t, unstructured.Unstructured{
							Object: originalUnstructuredCronJob,
						}), isSuspendedCronJob(t, cronJob))
						continue
					}
					require.True(t, isSuspendedCronJob(t, cronJob))
				}
			}
		} else {
			for _, cronJob := range cronJobs {
				originalCronJob := findResourceByName(assert.originalResources.cronjobList, cronJob.GetName())
				originalUnstructuredCronJob, err := runtime.DefaultUnstructuredConverter.ToUnstructured(originalCronJob)
				require.NoError(t, err)
				require.Equal(t, isSuspendedCronJob(t, unstructured.Unstructured{
					Object: originalUnstructuredCronJob,
				}), isSuspendedCronJob(t, cronJob))
			}
		}
	})

	t.Run("secret is correctly set", func(t *testing.T) {
		secret, err := sleepInfoReconciler.getSecret(ctx, getSecretName(assert.originalResources.sleepInfo.GetName()), cfg.Namespace())
		require.NoError(t, err)
		secretData := secret.Data

		if !assert.originalResources.sleepInfo.IsCronjobsToSuspend() && !assert.originalResources.sleepInfo.IsDeploymentsToSuspend() {
			require.Equal(t, map[string][]byte{
				lastScheduleKey: []byte(parseTime(t, assert.expectedScheduleTime).Truncate(time.Second).Format(time.RFC3339)),
			}, secretData)
			return
		}

		expectedSecretData := map[string][]byte{
			lastScheduleKey:  []byte(parseTime(t, assert.expectedScheduleTime).Truncate(time.Second).Format(time.RFC3339)),
			lastOperationKey: []byte(sleepOperation),
		}

		if assert.originalResources.sleepInfo.IsDeploymentsToSuspend() {
			type ExpectedReplicas struct {
				Name     string `json:"name"`
				Replicas int32  `json:"replicas"`
			}
			var originalReplicas []ExpectedReplicas
			for _, deployment := range assert.originalResources.deploymentList {
				if *deployment.Spec.Replicas == 0 || contains(assert.excludedDeployment, deployment.Name) {
					continue
				}
				originalReplicas = append(originalReplicas, ExpectedReplicas{
					Name:     deployment.Name,
					Replicas: *deployment.Spec.Replicas,
				})
			}
			expectedReplicas, err := json.Marshal(originalReplicas)
			require.NoError(t, err)

			expectedSecretData[replicasBeforeSleepKey] = expectedReplicas
		}

		if assert.originalResources.sleepInfo.IsCronjobsToSuspend() {
			originalStatus := []cronjobs.OriginalCronJobStatus{}
			for _, cronJob := range assert.originalResources.cronjobList {
				if isSuspendedCronJob(t, cronJob) {
					continue
				}
				if contains(assert.excludedCronJob, cronJob.GetName()) {
					continue
				}
				originalStatus = append(originalStatus, cronjobs.OriginalCronJobStatus{
					Name:    cronJob.GetName(),
					Suspend: false,
				})
			}
			expectedStatus, err := json.Marshal(originalStatus)
			require.NoError(t, err)

			expectedSecretData[originalCronjobStatusKey] = expectedStatus
		}

		require.Equal(t, expectedSecretData, secretData)
	})

	t.Run("sleepinfo status updated correctly", func(t *testing.T) {
		sleepInfo, err := sleepInfoReconciler.getSleepInfo(ctx, assert.req)
		require.NoError(t, err)

		operationType := sleepOperation

		if !sleepInfo.IsCronjobsToSuspend() && !sleepInfo.IsDeploymentsToSuspend() {
			operationType = ""
		}

		require.Equal(t, kubegreenv1alpha1.SleepInfoStatus{
			LastScheduleTime: metav1.NewTime(parseTime(t, assert.expectedScheduleTime).Local()),
			OperationType:    operationType,
		}, sleepInfo.Status)
	})

	t.Run("metrics correctly collected - quantitatively", func(*testing.T) {
		metrics := sleepInfoReconciler.Metrics

		require.Equal(t, 1, promTestutil.CollectAndCount(metrics.CurrentSleepInfo))
		expectedInfo := bytes.NewBufferString(fmt.Sprintf(`
		# HELP kube_green_current_sleepinfo Info about SleepInfo resource
		# TYPE kube_green_current_sleepinfo gauge
		kube_green_current_sleepinfo{name="%s",namespace="%s"} 1
`, assert.originalResources.sleepInfo.GetName(), cfg.Namespace()))
		require.NoError(t, promTestutil.CollectAndCompare(metrics.CurrentSleepInfo, expectedInfo))
	})
}

func assertCorrectWakeUpOperation(t *testing.T, ctx context.Context, cfg *envconf.Config) {
	assert := getAssertOperation(t, ctx)
	sleepInfoReconciler := nextReconciler(t, ctx)

	t.Run("deployment replicas correctly waked up", func(t *testing.T) {
		deployments := getDeploymentList(t, ctx, cfg)
		for _, deployment := range deployments {
			originalDeployment := findDeployByName(assert.originalResources.deploymentList, deployment.Name)
			require.Equal(t, originalDeployment.Spec.Replicas, deployment.Spec.Replicas)
		}
	})

	t.Run("cron jobs correctly waked up", func(t *testing.T) {
		cronJobs := getCronJobList(t, ctx, cfg)
		for _, cronJob := range cronJobs {
			originalCronJob := findResourceByName(assert.originalResources.cronjobList, cronJob.GetName())
			originalUnstructuredCronJob, err := runtime.DefaultUnstructuredConverter.ToUnstructured(originalCronJob)
			require.NoError(t, err)
			require.Equal(t, isSuspendedCronJob(t, unstructured.Unstructured{
				Object: originalUnstructuredCronJob,
			}), isSuspendedCronJob(t, cronJob))
		}
	})

	t.Run("secret is correctly set", func(t *testing.T) {
		secret, err := sleepInfoReconciler.getSecret(ctx, getSecretName(assert.originalResources.sleepInfo.GetName()), cfg.Namespace())
		require.NoError(t, err)
		secretData := secret.Data
		require.Equal(t, map[string][]byte{
			lastScheduleKey:  []byte(parseTime(t, assert.expectedScheduleTime).Truncate(time.Second).Format(time.RFC3339)),
			lastOperationKey: []byte(wakeUpOperation),
		}, secretData)
	})

	t.Run("status correctly updated", func(t *testing.T) {
		sleepInfo, err := sleepInfoReconciler.getSleepInfo(ctx, assert.req)
		require.NoError(t, err)
		require.Equal(t, kubegreenv1alpha1.SleepInfoStatus{
			LastScheduleTime: metav1.NewTime(parseTime(t, assert.expectedScheduleTime).Round(time.Second).Local()),
			OperationType:    wakeUpOperation,
		}, sleepInfo.Status)
	})

	t.Run("metrics correctly collected - quantitatively", func(t *testing.T) {
		metrics := sleepInfoReconciler.Metrics

		require.Equal(t, 1, promTestutil.CollectAndCount(metrics.CurrentSleepInfo))
	})
}

func nextReconciler(t *testing.T, ctx context.Context) SleepInfoReconciler {
	assert := getAssertOperation(t, ctx)

	sleepInfoReconciler := SleepInfoReconciler{
		Clock: mockClock{
			now: assert.scheduleTime,
			t:   t,
		},
		Client:     assert.reconciler.Client,
		Log:        assert.reconciler.Log,
		Metrics:    assert.reconciler.Metrics,
		SleepDelta: 60,
	}
	result, err := sleepInfoReconciler.Reconcile(ctx, assert.req)
	require.NoError(t, err)

	require.Equal(t, ctrl.Result{
		RequeueAfter: assert.expectedNextRequeue,
	}, result, "requeue correctly")

	return sleepInfoReconciler
}

type AssertOperation struct {
	req        ctrl.Request
	reconciler SleepInfoReconciler

	scheduleTime string
	// optional - default is equal to scheduleTime
	expectedScheduleTime string
	expectedNextRequeue  time.Duration

	originalResources originalResources

	excludedDeployment []string
	excludedCronJob    []string
}

func (a AssertOperation) withSchedule(schedule string) AssertOperation {
	a.scheduleTime = schedule
	if a.expectedScheduleTime == "" {
		a.expectedScheduleTime = schedule
	}
	return a
}

func (a AssertOperation) withScheduleAndExpectedSchedule(schedule string) AssertOperation {
	a.scheduleTime = schedule
	a.expectedScheduleTime = schedule
	return a
}

func (a AssertOperation) withExpectedSchedule(schedule string) AssertOperation {
	a.expectedScheduleTime = schedule
	return a
}

func (a AssertOperation) withRequeue(requeue float64) AssertOperation {
	a.expectedNextRequeue = time.Duration(requeue*1000) * time.Millisecond
	return a
}

func (a AssertOperation) nextWakeUp() AssertOperation {
	a.expectedNextRequeue = wakeUpRequeue(a.scheduleTime)
	return a
}

func (a AssertOperation) nextSleep() AssertOperation {
	a.expectedNextRequeue = sleepRequeue(a.scheduleTime)
	return a
}
