package deployments

import (
	"context"
	"testing"

	"github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/davidebianchi/kube-green/controllers/internal/testutil"
	"github.com/davidebianchi/kube-green/controllers/sleepinfo/resource"
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
				Client:      fake.NewClientBuilder().Build(),
				ShouldError: true,
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

	c := fake.NewClientBuilder().WithRuntimeObjects(&d1, &d2, &dZeroReplicas).Build()
	ctx := context.Background()
	emptySleepInfo := &v1alpha1.SleepInfo{}
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}

	t.Run("update deploy to have zero replicas", func(t *testing.T) {
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

	// t.Run("fails to patch deployment", func(t *testing.T) {
	// 	fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
	// 		Client:      c,
	// 		ShouldError: true,
	// 	}

	// 	resource, err := NewResource(ctx, resource.ResourceClient{
	// 		Client:    fakeClient,
	// 		Log:       testLogger,
	// 		SleepInfo: emptySleepInfo,
	// 	}, namespace, map[string]int32{})
	// 	require.NoError(t, err)

	// 	require.EqualError(t, resource.Sleep(ctx), "error during patch")
	// })

	t.Run("not fails if deployments not found", func(t *testing.T) {
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
