package sleepinfo

import (
	"context"
	"testing"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/internal/testutil"
	"github.com/kube-green/kube-green/controllers/sleepinfo/metrics"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestSleepInfoController(t *testing.T) {
	const (
		sleepInfoName = "default-sleep"
		mockNow       = "2021-03-23T20:01:20.555Z"
	)
	testLogger := zap.New(zap.UseDevMode(true))

	zeroDeployments := features.New("with zero deployments").
		WithSetup("create SleepInfo", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			createSleepInfoCRD(ctx, t, c, sleepInfoName, nil)

			return ctx
		}).
		Assess("is requeued if not sleep time", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepInfoReconciler := SleepInfoReconciler{
				Clock: mockClock{
					now: mockNow,
				},
				Client:  newControllerRuntimeClient(t, c),
				Log:     testLogger,
				Metrics: metrics.SetupMetricsOrDie("kube_green"),
			}

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
			return WithAssertOperation(ctx, AssertOperation{
				sleepInfoName: sleepInfoName,
				reconciler:    sleepInfoReconciler,
				req:           req,
			})
		}).
		Assess("sleep", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			sleepScheduleTime := "2021-03-23T20:05:59.000Z"
			assertOperations := GetAssertOperation(t, ctx)

			sleepInfoReconciler := SleepInfoReconciler{
				Clock: mockClock{
					now: sleepScheduleTime,
				},
				Client:  newControllerRuntimeClient(t, c),
				Log:     testLogger,
				Metrics: assertOperations.reconciler.Metrics,
			}

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

			return WithAssertOperation(ctx, AssertOperation{
				sleepInfoName: sleepInfoName,
				reconciler:    sleepInfoReconciler,
				req:           assertOperations.req,
			})
		}).
		Assess("WAKE_UP is skipped", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			assertOperations := GetAssertOperation(t, ctx)
			lastSleepScheduleTime := assertOperations.reconciler.Clock.Now().Format(time.RFC3339)

			sleepInfoReconciler := SleepInfoReconciler{
				Clock: mockClock{
					now: "2021-03-23T20:07:00.000Z",
				},
				Client:  newControllerRuntimeClient(t, c),
				Log:     testLogger,
				Metrics: assertOperations.reconciler.Metrics,
			}
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
			sleepInfoReconciler := SleepInfoReconciler{
				Clock: mockClock{
					now: mockNow,
				},
				Client:  newControllerRuntimeClient(t, c),
				Log:     testLogger,
				Metrics: metrics.SetupMetricsOrDie("kube_green"),
			}

			result, err := sleepInfoReconciler.Reconcile(ctx, req)
			require.NoError(t, err)
			require.Empty(t, result)

			return ctx
		}).
		Feature()

	testenv.TestInParallel(t, zeroDeployments, notExistentResource)
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
				createSleepInfoCRD(ctx, t, c, sleepInfoName, sleepInfo)

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sleepInfoName,
						Namespace: c.Namespace(),
					},
				}
				sleepInfoReconciler := SleepInfoReconciler{
					Clock: mockClock{
						now: mockNow,
					},
					Client:  newControllerRuntimeClient(t, c),
					Log:     testLogger,
					Metrics: metrics.SetupMetricsOrDie("kube_green"),
				}

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
				createSleepInfoCRD(ctx, t, c, sleepInfoName, sleepInfo)

				sleepInfoReconciler := SleepInfoReconciler{
					Clock: mockClock{
						now: mockNow,
					},
					Client:  newControllerRuntimeClient(t, c),
					Log:     testLogger,
					Metrics: metrics.SetupMetricsOrDie("kube_green"),
				}
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
				createSleepInfoCRD(ctx, t, c, sleepInfoName, sleepInfo)

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sleepInfoName,
						Namespace: c.Namespace(),
					},
				}
				sleepInfoReconciler := SleepInfoReconciler{
					Clock: mockClock{
						now: mockNow,
					},
					Client:  newControllerRuntimeClient(t, c),
					Log:     testLogger,
					Metrics: metrics.SetupMetricsOrDie("kube_green"),
				}
				result, err := sleepInfoReconciler.Reconcile(ctx, req)
				require.EqualError(t, err, "empty weekdays from SleepInfo configuration")
				require.Empty(t, result)

				return ctx
			},
		},
	}.Build("invalid resources").Feature()

	testenv.TestInParallel(t, invalid)
}
