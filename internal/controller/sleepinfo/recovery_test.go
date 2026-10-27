package sleepinfo

import (
	"context"
	"errors"
	"testing"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/internal/mocks"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/metrics"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestRestoreSecretFailureDoesNotSleep verifies that failed restore-data writes
// prevent workload and status changes, and that a later retry can sleep and wake.
func TestRestoreSecretFailureDoesNotSleep(t *testing.T) {
	for _, existingSecret := range []bool{false, true} {
		for _, hasWorkloads := range []bool{false, true} {
			name := "create"
			if existingSecret {
				name = "update"
			}
			if !hasWorkloads {
				name += " without workloads"
			}
			t.Run(name, func(t *testing.T) {
				ctx := t.Context()
				r, c, req, deployments := newRecoveryReconciler(t)
				if !hasWorkloads {
					for _, deployment := range deployments {
						require.NoError(t, c.Delete(ctx, deployment))
					}
					deployments = nil
				}
				if existingSecret {
					require.NoError(t, c.Create(ctx, &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: getSecretName(req.Name), Namespace: req.Namespace},
						Data: map[string][]byte{
							lastScheduleKey:  []byte("2021-03-23T19:20:00Z"),
							lastOperationKey: []byte(wakeUpOperation),
						},
					}))
				}
				c.ShouldError = func(method testutil.Method, obj runtime.Object) bool {
					_, secret := obj.(*corev1.Secret)
					return secret && (method == testutil.Create || method == testutil.Update)
				}
				_, err := r.Reconcile(ctx, req)
				require.Error(t, err)
				for _, deployment := range deployments {
					assertDeploymentReplicas(t, ctx, c, deployment, 2)
				}
				sleepInfo := &v1alpha1.SleepInfo{}
				require.NoError(t, c.Get(ctx, req.NamespacedName, sleepInfo))
				require.Empty(t, sleepInfo.Status.OperationType)
				require.True(t, sleepInfo.Status.LastScheduleTime.IsZero())

				c.ShouldError = nil
				_, err = r.Reconcile(ctx, req)
				require.NoError(t, err)
				for _, deployment := range deployments {
					assertDeploymentReplicas(t, ctx, c, deployment, 0)
				}
				r.Clock = mockClock{now: "2021-03-23T20:20:00Z", t: t}
				_, err = r.Reconcile(ctx, req)
				require.NoError(t, err)
				for _, deployment := range deployments {
					assertDeploymentReplicas(t, ctx, c, deployment, 2)
				}
			})
		}
	}
}

// TestPendingSleepRecovery checks that persisted state survives interrupted sleep,
// list failures, and controller restarts without losing the original restore data.
func TestPendingSleepRecovery(t *testing.T) {
	for _, tc := range []struct {
		name      string
		failApply bool
		retryAt   string
		wantAwake bool
	}{
		{"partial sleep", true, "2021-03-23T20:07:00Z", false},
		{"completion save failure", false, "2021-03-23T20:07:00Z", false},
		{"partial sleep past wake time", true, "2021-03-23T20:23:00Z", true},
		{"completion save failure past wake time", false, "2021-03-23T20:23:00Z", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			r, c, req, deployments := newRecoveryReconciler(t)
			applyCount := 0
			c.ShouldError = func(method testutil.Method, obj runtime.Object) bool {
				if method == testutil.Apply {
					applyCount++
					return tc.failApply && applyCount == 2
				}
				_, secret := obj.(*corev1.Secret)
				return !tc.failApply && secret && method == testutil.Update
			}
			_, err := r.Reconcile(ctx, req)
			require.Error(t, err)
			require.Equal(t, 2, applyCount)
			secret := &corev1.Secret{}
			key := client.ObjectKey{Name: getSecretName(req.Name), Namespace: req.Namespace}
			require.NoError(t, c.Get(ctx, key, secret))
			require.Equal(t, sleepOperation, string(secret.Data[pendingSleepKey]))
			originalRestoreData := string(secret.Data[originalJSONPatchDataKey])
			require.JSONEq(t, `{"Deployment.apps":{"first":"{\"spec\":{\"replicas\":2}}","second":"{\"spec\":{\"replicas\":2}}"}}`, originalRestoreData)

			// A temporary list failure must not look like deleted workloads and
			// overwrite the saved restore data with an empty resource list.
			c.ShouldError = func(method testutil.Method, _ runtime.Object) bool {
				return method == testutil.List
			}
			before := secret.DeepCopy()
			_, err = r.Reconcile(ctx, req)
			require.Error(t, err)
			require.NoError(t, c.Get(ctx, key, secret))
			require.Equal(t, before.Data, secret.Data)

			// Recreate the reconciler to prove recovery depends only on persisted state.
			c.ShouldError = nil
			restarted := *r
			restarted.Clock = mockClock{now: tc.retryAt, t: t}
			_, err = restarted.Reconcile(ctx, req)
			require.NoError(t, err)
			require.NoError(t, c.Get(ctx, key, secret))
			require.NotContains(t, secret.Data, pendingSleepKey)
			if !tc.wantAwake {
				require.Equal(t, originalRestoreData, string(secret.Data[originalJSONPatchDataKey]))
				require.Equal(t, "2021-03-23T20:05:00Z", string(secret.Data[lastScheduleKey]))
				for _, deployment := range deployments {
					assertDeploymentReplicas(t, ctx, c, deployment, 0)
				}
				restarted.Clock = mockClock{now: "2021-03-23T20:20:00Z", t: t}
				_, err = restarted.Reconcile(ctx, req)
				require.NoError(t, err)
			}
			for _, deployment := range deployments {
				assertDeploymentReplicas(t, ctx, c, deployment, 2)
			}
			sleepInfo := &v1alpha1.SleepInfo{}
			require.NoError(t, c.Get(ctx, req.NamespacedName, sleepInfo))
			require.Equal(t, wakeUpOperation, sleepInfo.Status.OperationType)
		})
	}
}

// newRecoveryReconciler provides a scheduled SleepInfo and two awake deployments
// with a client that can inject failures at persistence and workload boundaries.
func newRecoveryReconciler(t *testing.T) (*SleepInfoReconciler, *testutil.PossiblyErroringFakeCtrlRuntimeClient, ctrl.Request, []*appsv1.Deployment) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	sleepInfo := getDefaultSleepInfo("sleep", "test")
	replicas := int32(2)
	deployments := make([]*appsv1.Deployment, 0, 2)
	objects := []client.Object{sleepInfo}
	for _, name := range []string{"first", "second"} {
		deployment := mocks.Deployment(mocks.DeploymentOptions{
			Name: name, Namespace: "test", Replicas: &replicas,
		}).Resource()
		deployments = append(deployments, deployment)
		objects = append(objects, deployment)
	}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{appsv1.SchemeGroupVersion})
	mapper.Add(appsv1.SchemeGroupVersion.WithKind("Deployment"), meta.RESTScopeNamespace)
	c := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(mapper).
			WithStatusSubresource(sleepInfo).WithObjects(objects...).Build(),
	}
	r := &SleepInfoReconciler{
		Client: c, Clock: mockClock{now: "2021-03-23T20:05:00Z", t: t},
		Log: logr.Discard(), Metrics: metrics.SetupMetricsOrDie("recovery_test"),
		SleepDelta: 60, ManagerName: "kube-green",
	}
	return r, c, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(sleepInfo)}, deployments
}

// assertDeploymentReplicas reads stored workload state so assertions do not rely
// on the original fixture objects being updated by the reconciler.
func assertDeploymentReplicas(t *testing.T, ctx context.Context, c client.Client, deployment *appsv1.Deployment, replicas int32) {
	t.Helper()
	actual := &appsv1.Deployment{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(deployment), actual))
	require.Equal(t, replicas, *actual.Spec.Replicas)
}

// TestPendingSleepWithoutWakeSchedule verifies retries and later sleep cycles
// retain restore patches until a wake schedule is configured.
func TestPendingSleepWithoutWakeSchedule(t *testing.T) {
	ctx := t.Context()
	r, c, req, deployments := newRecoveryReconciler(t)
	sleepInfo := &v1alpha1.SleepInfo{}
	require.NoError(t, c.Get(ctx, req.NamespacedName, sleepInfo))
	sleepInfo.Spec.WakeUpTime = ""
	require.NoError(t, c.Update(ctx, sleepInfo))
	applied := 0
	c.ShouldError = func(method testutil.Method, _ runtime.Object) bool {
		if method == testutil.Apply {
			applied++
			return applied == 2
		}
		return false
	}
	_, err := r.Reconcile(ctx, req)
	require.Error(t, err)

	c.ShouldError = nil
	r.Clock = mockClock{now: "2021-03-24T20:07:00Z", t: t}
	result, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Positive(t, result.RequeueAfter)
	for _, deployment := range deployments {
		assertDeploymentReplicas(t, ctx, c, deployment, 0)
	}

	// A later sleep cycle must keep restore patches for workloads already asleep.
	r.Clock = mockClock{now: "2021-03-25T20:05:00Z", t: t}
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.NoError(t, c.Get(ctx, req.NamespacedName, sleepInfo))
	sleepInfo.Spec.WakeUpTime = "*:20"
	require.NoError(t, c.Update(ctx, sleepInfo))
	r.Clock = mockClock{now: "2021-03-25T20:20:00Z", t: t}
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	for _, deployment := range deployments {
		assertDeploymentReplicas(t, ctx, c, deployment, 2)
	}
}

// TestPendingSleepRecoveryAfterListPageFailure preserves pending recovery state
// after any later-page error, then retries from the first page to wake or confirm deletion.
func TestPendingSleepRecoveryAfterListPageFailure(t *testing.T) {
	for _, tc := range []struct {
		name          string
		listError     error
		targetDeleted bool
	}{
		{name: "temporary failure", listError: errors.New("later page unavailable")},
		{
			name:          "NotFound followed by confirmed deletion",
			listError:     apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, ""),
			targetDeleted: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			r, c, req, deployments := newRecoveryReconciler(t)
			c.ShouldError = func(method testutil.Method, obj runtime.Object) bool {
				_, secret := obj.(*corev1.Secret)
				return secret && method == testutil.Update
			}
			_, err := r.Reconcile(ctx, req)
			require.Error(t, err)
			for _, deployment := range deployments {
				assertDeploymentReplicas(t, ctx, c, deployment, 0)
			}
			key := client.ObjectKey{Name: getSecretName(req.Name), Namespace: req.Namespace}
			before := &corev1.Secret{}
			require.NoError(t, c.Get(ctx, key, before))
			require.Equal(t, sleepOperation, string(before.Data[pendingSleepKey]))

			c.ShouldError = nil
			underlying := c.Client
			c.Client = interceptor.NewClient(underlying.(client.WithWatch), interceptor.Funcs{
				List: func(ctx context.Context, delegate client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					options := (&client.ListOptions{}).ApplyOptions(opts)
					if options.Continue != "" {
						return tc.listError
					}
					if err := delegate.List(ctx, list, opts...); err != nil {
						return err
					}
					result := list.(*unstructured.UnstructuredList)
					result.Items = result.Items[:1]
					result.SetContinue("next-page")
					return nil
				},
			})
			r.Clock = mockClock{now: "2021-03-23T20:23:00Z", t: t}
			_, err = r.Reconcile(ctx, req)
			require.ErrorIs(t, err, tc.listError)
			after := &corev1.Secret{}
			require.NoError(t, c.Get(ctx, key, after))
			require.Equal(t, before.Data, after.Data)
			for _, deployment := range deployments {
				assertDeploymentReplicas(t, ctx, c, deployment, 0)
			}

			c.Client = underlying
			if tc.targetDeleted {
				for _, deployment := range deployments {
					require.NoError(t, c.Delete(ctx, deployment))
				}
				c.Client = interceptor.NewClient(underlying.(client.WithWatch), interceptor.Funcs{
					List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, opts ...client.ListOption) error {
						require.Empty(t, (&client.ListOptions{}).ApplyOptions(opts).Continue)
						return tc.listError
					},
				})
			}
			_, err = r.Reconcile(ctx, req)
			require.NoError(t, err)
			if !tc.targetDeleted {
				for _, deployment := range deployments {
					assertDeploymentReplicas(t, ctx, c, deployment, 2)
				}
			}
			require.NoError(t, c.Get(ctx, key, after))
			require.NotContains(t, after.Data, pendingSleepKey)
			require.NotContains(t, after.Data, originalJSONPatchDataKey)
		})
	}
}

// TestPendingSleepRecoveryAfterWorkloadsDeleted checks that a complete empty list
// clears obsolete recovery data instead of retrying deleted workloads forever.
func TestPendingSleepRecoveryAfterWorkloadsDeleted(t *testing.T) {
	ctx := t.Context()
	r, c, req, deployments := newRecoveryReconciler(t)
	c.ShouldError = func(method testutil.Method, obj runtime.Object) bool {
		_, secret := obj.(*corev1.Secret)
		return secret && method == testutil.Update
	}
	_, err := r.Reconcile(ctx, req)
	require.Error(t, err)
	key := client.ObjectKey{Name: getSecretName(req.Name), Namespace: req.Namespace}
	secret := &corev1.Secret{}
	require.NoError(t, c.Get(ctx, key, secret))
	require.Equal(t, sleepOperation, string(secret.Data[pendingSleepKey]))
	require.Contains(t, secret.Data, originalJSONPatchDataKey)

	// A complete empty list is authoritative: removed workloads must not keep
	// the operation pending forever just because their restore data was saved.
	c.ShouldError = nil
	for _, deployment := range deployments {
		require.NoError(t, c.Delete(ctx, deployment))
	}
	r.Clock = mockClock{now: "2021-03-23T20:23:00Z", t: t}
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.NoError(t, c.Get(ctx, key, secret))
	require.NotContains(t, secret.Data, pendingSleepKey)
	require.NotContains(t, secret.Data, originalJSONPatchDataKey)
}
