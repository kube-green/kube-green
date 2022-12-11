package sleepinfo

import (
	"context"
	"testing"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/sleepinfo/metrics"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestSleepInfoController(t *testing.T) {
	const (
		sleepInfoName = "default-sleep"
		mockNow       = "2021-03-23T20:01:20.555Z"
	)
	testLogger := zap.New(zap.UseDevMode(true))

	reconcile := features.New("with zero deployments").
		WithSetup("create SleepInfo", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			r, err := resources.New(c.Client().RESTConfig())
			require.NoError(t, err)
			kubegreenv1alpha1.AddToScheme(r.GetScheme())
			createSleepInfoCRD(ctx, t, c, sleepInfoName, setupOptions{})

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

			t.Run("secret saved", func(t *testing.T) {
				secret, err := sleepInfoReconciler.getSecret(ctx, getSecretName(sleepInfoName), c.Namespace())
				require.NoError(t, err)
				require.NotNil(t, secret)
				require.Equal(t, map[string][]byte{
					lastScheduleKey: []byte(parseTime(t, sleepScheduleTime).Format(time.RFC3339)),
				}, secret.Data)
			})

			t.Run("status updated", func(t *testing.T) {
				sleepInfo, err := sleepInfoReconciler.getSleepInfo(ctx, assertOperations.req)
				require.NoError(t, err)
				require.Equal(t, kubegreenv1alpha1.SleepInfoStatus{
					LastScheduleTime: metav1.NewTime(parseTime(t, sleepScheduleTime).Local()),
				}, sleepInfo.Status)
			})

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

			t.Run("sleepinfo status updated correctly", func(t *testing.T) {
				sleepInfo, err := sleepInfoReconciler.getSleepInfo(ctx, assertOperations.req)
				require.NoError(t, err)
				require.Equal(t, kubegreenv1alpha1.SleepInfoStatus{
					LastScheduleTime: metav1.NewTime(parseTime(t, lastSleepScheduleTime).Local()),
				}, sleepInfo.Status)
			})

			t.Run("without deployments, in secret is written only the last schedule", func(t *testing.T) {
				secret, err := sleepInfoReconciler.getSecret(ctx, getSecretName(sleepInfoName), c.Namespace())
				require.NoError(t, err)
				require.NotNil(t, secret)
				require.Equal(t, map[string][]byte{
					lastScheduleKey: []byte(parseTime(t, lastSleepScheduleTime).Format(time.RFC3339)),
				}, secret.Data)
			})

			return ctx
		}).
		Feature()

	testenv.Test(t, reconcile)
}
