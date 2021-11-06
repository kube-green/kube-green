package sleepinfo

import (
	"context"
	"fmt"
	"testing"

	"github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/davidebianchi/kube-green/controllers/internal/testutil"
	"github.com/davidebianchi/kube-green/controllers/sleepinfo/deployments"
	"github.com/davidebianchi/kube-green/controllers/sleepinfo/resource"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestHasResources(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	namespace := "my-namespace"
	tests := []struct {
		name                     string
		deploy                   bool
		cronJob                  bool
		expectToPerformOperation bool
	}{
		{
			name:                     "empty resources",
			expectToPerformOperation: false,
		},
		{
			name:                     "some deployments",
			deploy:                   true,
			expectToPerformOperation: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resources, err := NewResources(context.Background(), resource.ResourceClient{
				Log:       testLogger,
				Client:    fake.NewClientBuilder().Build(),
				SleepInfo: &v1alpha1.SleepInfo{},
			}, namespace, SleepInfoData{})
			require.NoError(t, err)

			resources.deployments = resource.GetResourceMock(resource.ResourceMock{
				HasResourceResponseMock: test.deploy,
			})

			require.Equal(t, test.expectToPerformOperation, resources.hasResources())
		})
	}
}

func TestResourcesSleep(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	namespace := "my-namespace"
	var replica0 int32 = 0
	var replica1 int32 = 1
	var replica5 int32 = 5

	d1 := deployments.GetMock(deployments.MockSpec{
		Namespace:       namespace,
		Name:            "d1",
		Replicas:        &replica1,
		ResourceVersion: "2",
	})
	d2 := deployments.GetMock(deployments.MockSpec{
		Namespace:       namespace,
		Name:            "d2",
		Replicas:        &replica5,
		ResourceVersion: "1",
	})
	dZeroReplicas := deployments.GetMock(deployments.MockSpec{
		Namespace:       namespace,
		Name:            "dZeroReplicas",
		Replicas:        &replica0,
		ResourceVersion: "1",
	})

	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}
	ctx := context.Background()

	t.Run("update deploy to have zero replicas", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&d1, &d2, &dZeroReplicas).Build()

		resources, err := NewResources(ctx, resource.ResourceClient{
			Log:       testLogger,
			Client:    c,
			SleepInfo: &v1alpha1.SleepInfo{},
		}, namespace, SleepInfoData{})
		require.NoError(t, err)

		err = resources.sleep(ctx)
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
				deployments.GetMock(deployments.MockSpec{
					Namespace:       namespace,
					Name:            "d1",
					Replicas:        &replica0,
					ResourceVersion: "3",
				}),
				deployments.GetMock(deployments.MockSpec{
					Namespace:       namespace,
					Name:            "d2",
					Replicas:        &replica0,
					ResourceVersion: "2",
				}),
				dZeroReplicas,
			},
		}, list)
	})

	t.Run("fails to sleep", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&d1, &d2, &dZeroReplicas).Build()

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: c,
			ShouldError: func(method testutil.Method, obj runtime.Object) bool {
				fmt.Printf("fooo %v %v \n", method, testutil.Patch)
				return method == testutil.Patch
			},
		}

		resources, err := NewResources(ctx, resource.ResourceClient{
			Log:       testLogger,
			Client:    fakeClient,
			SleepInfo: &v1alpha1.SleepInfo{},
		}, namespace, SleepInfoData{})
		require.NoError(t, err)

		err = resources.sleep(ctx)
		require.EqualError(t, err, "error during patch")
	})
}
