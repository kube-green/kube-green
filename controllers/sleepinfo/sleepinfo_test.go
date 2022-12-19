package sleepinfo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/internal/testutil"
	"github.com/kube-green/kube-green/controllers/sleepinfo/cronjobs"
	"github.com/kube-green/kube-green/controllers/sleepinfo/metrics"
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
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestSleepInfoControllerReconciliation(t *testing.T) {
	const (
		sleepInfoName = "default-sleep"
		mockNow       = "2021-03-23T20:01:20.555Z"
	)
	testLogger := zap.New(zap.UseDevMode(true))

	zeroDeployments := features.New("with zero deployments").
		WithSetup("create SleepInfo", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			createSleepInfoCRD(t, ctx, c, getDefaultSleepInfo(sleepInfoName, c.Namespace()))

			return ctx
		}).
		Assess("is requeue d if not sleep time", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
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
				sleepInfoName: sleepInfoName,
				reconciler:    sleepInfoReconciler,
				req:           req,
			})
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepScheduleTime := "2021-03-23T20:05:59.000Z"
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
				sleepInfoName: sleepInfoName,
				reconciler:    sleepInfoReconciler,
				req:           assertOperations.req,
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
					Namespace: testutil.RandString(32),
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
			ctx = withAssertOperation(ctx, assertOperation.withScheduleAndExpectedSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60+1))
			assertCorrectSleepOperation2(t, ctx, c)

			return ctx
		}).
		Assess("wake up", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assertOperation := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assertOperation.withScheduleAndExpectedSchedule("2021-03-23T20:19:50.100Z").withRequeue(45*60+9.9))
			assertCorrectWakeUpOperation2(t, ctx, c)

			return ctx
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assertOperation := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assertOperation.withScheduleAndExpectedSchedule("2021-03-23T21:05:00.000Z").withRequeue(15*60))
			assertCorrectSleepOperation2(t, ctx, c)

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
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60+1))
			assertCorrectSleepOperation2(t, ctx, c)
			return ctx
		}).
		Assess("redeploy before the wakeup", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			upsertDeployments2(t, ctx, c, true)

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
			assertCorrectWakeUpOperation2(t, ctx, c)
			return ctx
		}).Feature()

	changeSingleDeplymentBetweenCycle := features.New("change single deployment replicas between sleep and wake up").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			return reconciliationSetup(t, ctx, c, mockNow, sleepInfo)
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60+1))
			assertCorrectSleepOperation2(t, ctx, c)

			return ctx
		}).
		Assess("redeploy a single deploy", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			k8sClient := newControllerRuntimeClient(t, c)
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
			assertCorrectWakeUpOperation2(t, ctx, c)

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
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60+1))
			assertCorrectSleepOperation2(t, ctx, c)

			return ctx
		}).
		Assess("sleep #2", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withSchedule("2021-03-23T21:05:00.000Z").withExpectedSchedule("2021-03-23T20:05:59Z").withRequeue(15*60))
			assertCorrectSleepOperation2(t, ctx, c)

			return ctx
		}).Feature()

	onlySleep := features.New("only sleep, wake up set to nil").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			ctx = withSetupOptions(ctx, setupOptions{
				unsetWakeUpTime: true,
			})
			sleepInfo := getDefaultSleepInfo(sleepInfoName, c.Namespace())
			sleepInfo.Spec.WakeUpTime = ""

			return reconciliationSetup(t, ctx, c, mockNow, sleepInfo)
		}).
		Assess("sleep #1", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule("2021-03-23T20:05:59.000Z").withRequeue(59*60+1))
			assertCorrectSleepOperation2(t, ctx, c)
			return ctx
		}).
		Assess("sleep #2", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assert := getAssertOperation(t, ctx)
			ctx = withAssertOperation(ctx, assert.withScheduleAndExpectedSchedule("2021-03-23T21:05:00.000Z").withRequeue(60*60))
			assertCorrectSleepOperation2(t, ctx, c)
			return ctx
		}).
		Assess("deploy", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			upsertDeployments2(t, ctx, c, true)

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
			assertCorrectSleepOperation2(t, ctx, c)
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
	)
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
	testLogger := zap.New(zap.UseDevMode(true))

	reconciler := getSleepInfoReconciler(t, c, testLogger, mockNow)

	req, originalResources := setupNamespaceWithResources2(t, ctx, c, sleepInfo, reconciler, getSetupOptions(t, ctx))
	assertContextInfo := AssertOperation{
		testLogger:        testLogger,
		ctx:               ctx,
		req:               req,
		namespace:         c.Namespace(),
		sleepInfoName:     sleepInfo.GetName(),
		originalResources: originalResources,
		reconciler:        reconciler,
	}

	return withAssertOperation(ctx, assertContextInfo)
}

func getSleepInfoReconciler(t *testing.T, c *envconf.Config, logger logr.Logger, now string) SleepInfoReconciler {
	return SleepInfoReconciler{
		Clock: mockClock{
			now: now,
			t:   t,
		},
		Client:  newControllerRuntimeClient(t, c),
		Log:     logger,
		Metrics: metrics.SetupMetricsOrDie("kube_green"),
	}
}

func assertCorrectSleepOperation2(t *testing.T, ctx context.Context, cfg *envconf.Config) {
	assert := getAssertOperation(t, ctx)

	sleepInfoReconciler := SleepInfoReconciler{
		Clock: mockClock{
			now: assert.scheduleTime,
		},
		Client:  assert.reconciler.Client,
		Log:     assert.testLogger,
		Metrics: assert.reconciler.Metrics,
	}
	result, err := sleepInfoReconciler.Reconcile(assert.ctx, assert.req)
	require.NoError(t, err)

	t.Run("replicas are set to 0 to all deployments set to sleep", func(t *testing.T) {
		deployments := getDeploymentList(t, ctx, cfg)
		if assert.originalResources.sleepInfo.IsDeploymentsToSuspend() {
			if len(assert.excludedDeployment) == 0 {
				assertAllReplicasSetToZero2(t, deployments, assert.originalResources.deploymentList)
			} else {
				for _, deployment := range deployments {
					if contains(assert.excludedDeployment, deployment.Name) {
						originalDeployment := findDeployByName(assert.originalResources.deploymentList, deployment.GetName())
						require.Equal(t, originalDeployment.Spec.Replicas, deployment.Spec.Replicas)
						continue
					}
					require.Equal(t, 0, *deployment.Spec.Replicas)
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
		cronJobs := getCronJobList(t, assert.ctx, cfg)
		if assert.originalResources.sleepInfo.IsCronjobsToSuspend() {
			if len(assert.excludedCronJob) == 0 {
				assertAllCronJobsSuspended2(t, cronJobs, assert.originalResources.cronjobList)
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
		secret, err := sleepInfoReconciler.getSecret(assert.ctx, getSecretName(assert.sleepInfoName), assert.namespace)
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
			var expectedReplicas = []byte{}
			expectedReplicas, err = json.Marshal(originalReplicas)
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
		sleepInfo, err := sleepInfoReconciler.getSleepInfo(assert.ctx, assert.req)
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

	t.Run("is requeued after correct duration to wake up", func(t *testing.T) {
		require.Equal(t, ctrl.Result{
			RequeueAfter: assert.expectedNextRequeue,
		}, result)
	})

	t.Run("metrics correctly collected - quantitatively", func(*testing.T) {
		metrics := sleepInfoReconciler.Metrics

		require.Equal(t, 1, promTestutil.CollectAndCount(metrics.CurrentSleepInfo))
		expectedInfo := bytes.NewBufferString(fmt.Sprintf(`
		# HELP kube_green_current_sleepinfo Info about SleepInfo resource
		# TYPE kube_green_current_sleepinfo gauge
		kube_green_current_sleepinfo{name="%s",namespace="%s"} 1
`, assert.sleepInfoName, assert.namespace))
		require.NoError(t, promTestutil.CollectAndCompare(metrics.CurrentSleepInfo, expectedInfo))
	})
}

func assertCorrectWakeUpOperation2(t *testing.T, ctx context.Context, cfg *envconf.Config) {
	assert := getAssertOperation(t, ctx)

	sleepInfoReconciler := SleepInfoReconciler{
		Clock: mockClock{
			now: assert.scheduleTime,
		},
		Client:  assert.reconciler.Client,
		Log:     assert.testLogger,
		Metrics: assert.reconciler.Metrics,
	}

	result, err := sleepInfoReconciler.Reconcile(assert.ctx, assert.req)
	require.NoError(t, err)

	t.Run("deployment replicas correctly waked up", func(t *testing.T) {
		deployments := getDeploymentList(t, assert.ctx, cfg)
		for _, deployment := range deployments {
			originalDeployment := findDeployByName(assert.originalResources.deploymentList, deployment.Name)
			require.Equal(t, originalDeployment.Spec.Replicas, deployment.Spec.Replicas)
		}
	})

	t.Run("cron jobs correctly waked up", func(t *testing.T) {
		cronJobs := getCronJobList(t, assert.ctx, cfg)
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
		secret, err := sleepInfoReconciler.getSecret(assert.ctx, getSecretName(assert.sleepInfoName), assert.namespace)
		require.NoError(t, err)
		secretData := secret.Data
		require.Equal(t, map[string][]byte{
			lastScheduleKey:  []byte(parseTime(t, assert.expectedScheduleTime).Truncate(time.Second).Format(time.RFC3339)),
			lastOperationKey: []byte(wakeUpOperation),
		}, secretData)
	})

	t.Run("status correctly updated", func(t *testing.T) {
		sleepInfo, err := sleepInfoReconciler.getSleepInfo(assert.ctx, assert.req)
		require.NoError(t, err)
		require.Equal(t, kubegreenv1alpha1.SleepInfoStatus{
			LastScheduleTime: metav1.NewTime(parseTime(t, assert.expectedScheduleTime).Round(time.Second).Local()),
			OperationType:    wakeUpOperation,
		}, sleepInfo.Status)
	})

	t.Run("is requeued after correct duration to sleep", func(t *testing.T) {
		require.Equal(t, ctrl.Result{
			RequeueAfter: assert.expectedNextRequeue,
		}, result)
	})

	t.Run("metrics correctly collected - quantitatively", func(t *testing.T) {
		metrics := sleepInfoReconciler.Metrics

		require.Equal(t, 1, promTestutil.CollectAndCount(metrics.CurrentSleepInfo))
	})
}
