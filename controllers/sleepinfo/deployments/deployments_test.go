package deployments

import (
	"context"
	"testing"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/internal/testutil"
	"github.com/kube-green/kube-green/controllers/sleepinfo/resource"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestNewResource(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	namespace := "my-namespace"
	deployment1 := GetMock(MockSpec{
		Name:      "deployment1",
		Namespace: namespace,
	})
	deployment2 := GetMock(MockSpec{
		Name:      "deployment2",
		Namespace: namespace,
	})
	deploymentOtherNamespace := GetMock(MockSpec{
		Name:      "deploymentOtherNamespace",
		Namespace: "other-namespace",
	})
	emptySleepInfo := &v1alpha1.SleepInfo{}

	listDeploymentsTests := []struct {
		name      string
		client    client.Client
		sleepInfo *v1alpha1.SleepInfo
		expected  []appsv1.Deployment
		throws    bool
	}{
		{
			name: "get list of deployments",
			client: fake.
				NewClientBuilder().
				WithRuntimeObjects([]runtime.Object{&deployment1, &deployment2, &deploymentOtherNamespace}...).
				Build(),
			expected: []appsv1.Deployment{deployment1, deployment2},
		},
		{
			name: "fails to list deployments",
			client: &testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client: fake.NewClientBuilder().Build(),
				ShouldError: func(method testutil.Method, obj runtime.Object) bool {
					return method == testutil.List
				},
			},
			throws: true,
		},
		{
			name: "empty list deployments",
			client: fake.
				NewClientBuilder().
				WithRuntimeObjects([]runtime.Object{&deploymentOtherNamespace}...).
				Build(),
			expected: []appsv1.Deployment{},
		},
		{
			name: "disabled cronjob suspend",
			client: fake.
				NewClientBuilder().
				WithRuntimeObjects([]runtime.Object{&deployment1, &deployment2, &deploymentOtherNamespace}...).
				Build(),
			sleepInfo: &v1alpha1.SleepInfo{
				Spec: v1alpha1.SleepInfoSpec{
					SuspendDeployments: getPtr(true),
				},
			},
			expected: []appsv1.Deployment{},
		},
		{
			name: "with deployment to exclude",
			client: fake.
				NewClientBuilder().
				WithRuntimeObjects([]runtime.Object{&deployment1, &deployment2, &deploymentOtherNamespace}...).
				Build(),
			sleepInfo: &v1alpha1.SleepInfo{
				Spec: v1alpha1.SleepInfoSpec{
					ExcludeRef: []v1alpha1.ExcludeRef{
						{
							ApiVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       deployment2.Name,
						},
						{
							ApiVersion: "apps/v1",
							Kind:       "resource",
							Name:       "foo",
						},
						{
							ApiVersion: "apps/v2",
							Kind:       "Deployment",
							Name:       deployment1.Name,
						},
					},
				},
			},
			expected: []appsv1.Deployment{deployment1},
		},
	}

	for _, test := range listDeploymentsTests {
		t.Run(test.name, func(t *testing.T) {
			sleepInfo := emptySleepInfo
			if test.sleepInfo != nil {
				sleepInfo = test.sleepInfo
			}
			d, err := NewResource(context.Background(), resource.ResourceClient{
				Client:    test.client,
				Log:       testLogger,
				SleepInfo: sleepInfo,
			}, namespace, map[string]int32{})
			if test.throws {
				require.EqualError(t, err, "error during list")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.expected, d.data)
		})
	}
}

func TestHasResource(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	namespace := "my-namespace"
	deployment1 := GetMock(MockSpec{
		Name:      "deployment1",
		Namespace: namespace,
	})

	t.Run("without resource", func(t *testing.T) {
		d, err := NewResource(context.Background(), resource.ResourceClient{
			Client:    fake.NewClientBuilder().Build(),
			Log:       testLogger,
			SleepInfo: &v1alpha1.SleepInfo{},
		}, namespace, map[string]int32{})
		require.NoError(t, err)

		require.False(t, d.HasResource())
	})

	t.Run("with resource", func(t *testing.T) {
		d, err := NewResource(context.Background(), resource.ResourceClient{
			Client:    fake.NewClientBuilder().WithRuntimeObjects(&deployment1).Build(),
			Log:       testLogger,
			SleepInfo: &v1alpha1.SleepInfo{},
		}, namespace, map[string]int32{})
		require.NoError(t, err)

		require.True(t, d.HasResource())
	})
}

func TestSleep(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	var replica0 int32 = 0
	var replica1 int32 = 1
	var replica5 int32 = 5
	namespace := "my-namespace"

	d1 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "d1",
		Replicas:        &replica1,
		ResourceVersion: "2",
	})
	d2 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "d2",
		Replicas:        &replica5,
		ResourceVersion: "1",
	})
	dZeroReplicas := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "dZeroReplicas",
		Replicas:        &replica0,
		ResourceVersion: "1",
	})

	ctx := context.Background()
	emptySleepInfo := &v1alpha1.SleepInfo{}
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}

	t.Run("update deploy to have zero replicas", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&d1, &d2, &dZeroReplicas).Build()
		fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: c,
		}

		resource, err := NewResource(ctx, resource.ResourceClient{
			Client:    fakeClient,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{})
		require.NoError(t, err)

		require.NoError(t, resource.Sleep(ctx))

		list := appsv1.DeploymentList{}
		err = c.List(context.Background(), &list, listOptions)
		require.NoError(t, err)
		require.Equal(t, appsv1.DeploymentList{
			TypeMeta: metav1.TypeMeta{
				Kind:       "DeploymentList",
				APIVersion: "apps/v1",
			},
			Items: []appsv1.Deployment{
				GetMock(MockSpec{
					Namespace:       namespace,
					Name:            "d1",
					Replicas:        &replica0,
					ResourceVersion: "3",
				}),
				GetMock(MockSpec{
					Namespace:       namespace,
					Name:            "d2",
					Replicas:        &replica0,
					ResourceVersion: "2",
				}),
				dZeroReplicas,
			},
		}, list)
	})

	t.Run("fails to patch deployment", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&d1, &d2, &dZeroReplicas).Build()
		fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: c,
			ShouldError: func(method testutil.Method, obj runtime.Object) bool {
				return method == testutil.Patch
			},
		}

		resource, err := NewResource(ctx, resource.ResourceClient{
			Client:    fakeClient,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{})
		require.NoError(t, err)

		require.EqualError(t, resource.Sleep(ctx), "error during patch")
	})

	t.Run("not fails if deployments not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&d1, &d2, &dZeroReplicas).Build()
		fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: c,
		}

		resource, err := NewResource(ctx, resource.ResourceClient{
			Client:    fakeClient,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{})
		require.NoError(t, err)

		err = c.DeleteAllOf(ctx, &appsv1.Deployment{}, &client.DeleteAllOfOptions{})
		require.NoError(t, err)

		require.NoError(t, resource.Sleep(ctx))
	})
}

func TestWakeUp(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	var replica0 int32 = 0
	var replica1 int32 = 1
	var replica5 int32 = 5
	namespace := "my-namespace"

	d1 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "d1",
		Replicas:        &replica0,
		ResourceVersion: "2",
	})
	d2 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "d2",
		Replicas:        &replica0,
		ResourceVersion: "1",
	})
	dZeroReplicas := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "dZeroReplicas",
		Replicas:        &replica0,
		ResourceVersion: "1",
	})
	dAfterSleep := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "aftersleep",
		Replicas:        &replica1,
		ResourceVersion: "1",
	})

	ctx := context.Background()
	emptySleepInfo := &v1alpha1.SleepInfo{}
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}

	t.Run("wake up deploy", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&d1, &d2, &dZeroReplicas, &dAfterSleep).Build()
		r, err := NewResource(ctx, resource.ResourceClient{
			Client:    c,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{
			d1.Name: replica1,
			d2.Name: replica5,
		})
		require.NoError(t, err)

		err = r.WakeUp(ctx)
		require.NoError(t, err)

		list := appsv1.DeploymentList{}
		err = c.List(context.Background(), &list, listOptions)
		require.NoError(t, err)
		require.Equal(t, appsv1.DeploymentList{
			TypeMeta: metav1.TypeMeta{
				Kind:       "DeploymentList",
				APIVersion: "apps/v1",
			},
			Items: []appsv1.Deployment{
				dAfterSleep,
				GetMock(MockSpec{
					Namespace:       namespace,
					Name:            "d1",
					Replicas:        &replica1,
					ResourceVersion: "3",
				}),
				GetMock(MockSpec{
					Namespace:       namespace,
					Name:            "d2",
					Replicas:        &replica5,
					ResourceVersion: "2",
				}),
				dZeroReplicas,
			},
		}, list)
	})

	t.Run("wake up fails", func(t *testing.T) {
		c := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: fake.NewClientBuilder().WithRuntimeObjects(&d1).Build(),
			ShouldError: func(method testutil.Method, obj runtime.Object) bool {
				return method == testutil.Patch
			},
		}
		r, err := NewResource(ctx, resource.ResourceClient{
			Client:    c,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{
			d1.Name: replica1,
			d2.Name: replica5,
		})
		require.NoError(t, err)

		err = r.WakeUp(ctx)
		require.EqualError(t, err, "error during patch")
	})
}

func TestDeploymentOriginalReplicas(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	ctx := context.Background()
	namespace := "my-namespace"
	emptySleepInfo := &v1alpha1.SleepInfo{}
	var replica0 int32 = 0
	var replica1 int32 = 1
	var replica5 int32 = 5

	d1 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "d1",
		Replicas:        &replica1,
		ResourceVersion: "2",
	})
	d2 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "d2",
		Replicas:        &replica5,
		ResourceVersion: "1",
	})
	dZeroReplicas := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "dZeroReplica",
		Replicas:        &replica0,
		ResourceVersion: "1",
	})

	t.Run("save and restore replicas info", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&d1, &d2, &dZeroReplicas).Build()
		r, err := NewResource(ctx, resource.ResourceClient{
			Client:    c,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{
			d1.Name: replica1,
			d2.Name: replica5,
		})
		require.NoError(t, err)

		res, err := r.GetOriginalInfoToSave()
		require.NoError(t, err)

		expectedInfoToSave := `[{"name":"d1","replicas":1},{"name":"d2","replicas":5}]`
		require.JSONEq(t, expectedInfoToSave, string(res))

		t.Run("restore saved info", func(t *testing.T) {
			infoToSave := []byte(expectedInfoToSave)
			restoredInfo, err := GetOriginalInfoToRestore(infoToSave)
			require.NoError(t, err)
			require.Equal(t, map[string]int32{
				d1.Name: replica1,
				d2.Name: replica5,
			}, restoredInfo)
		})
	})

	t.Run("restore info with data nil", func(t *testing.T) {
		info, err := GetOriginalInfoToRestore(nil)
		require.Equal(t, map[string]int32{}, info)
		require.NoError(t, err)
	})

	t.Run("fails if saved data are not valid json", func(t *testing.T) {
		info, err := GetOriginalInfoToRestore([]byte(`{}`))
		require.EqualError(t, err, "json: cannot unmarshal object into Go value of type []deployments.OriginalReplicas")
		require.Nil(t, info)
	})
}

func getPtr[T any](item T) *T {
	return &item
}
