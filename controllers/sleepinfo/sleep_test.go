package sleepinfo

import (
	"context"
	"testing"

	"github.com/davidebianchi/kube-green/controllers/internal/testutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSleep(t *testing.T) {
	namespace := "my-namespace"
	var replica0 int32 = 0
	var replica1 int32 = 1
	var replica5 int32 = 5

	d1 := getDeploymentMock(mockDeploymentSpec{
		namespace:       namespace,
		name:            "d1",
		replicas:        &replica1,
		resourceVersion: "2",
	})
	d2 := getDeploymentMock(mockDeploymentSpec{
		namespace:       namespace,
		name:            "d2",
		replicas:        &replica5,
		resourceVersion: "1",
	})
	dZeroReplicas := getDeploymentMock(mockDeploymentSpec{
		namespace:       namespace,
		name:            "dZeroReplicas",
		replicas:        &replica0,
		resourceVersion: "1",
	})
	deployList := []appsv1.Deployment{d1, d2, dZeroReplicas}

	c := fake.NewClientBuilder().WithRuntimeObjects(&d1, &d2, &dZeroReplicas).Build()

	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}

	t.Run("update deploy to have zero replicas", func(t *testing.T) {
		fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: c,
		}

		s := sleep{
			Client: fakeClient,
		}
		err := s.updateDeploymentsWithZeroReplicas(context.Background(), deployList)
		require.NoError(t, err)

		list := appsv1.DeploymentList{}
		err = c.List(context.Background(), &list, listOptions)
		require.NoError(t, err)
		require.Equal(t, appsv1.DeploymentList{
			TypeMeta: v1.TypeMeta{
				Kind:       "DeploymentList",
				APIVersion: "apps/v1",
			},
			Items: []appsv1.Deployment{
				getDeploymentMock(mockDeploymentSpec{
					namespace:       namespace,
					name:            "d1",
					replicas:        &replica0,
					resourceVersion: "3",
				}),
				getDeploymentMock(mockDeploymentSpec{
					namespace:       namespace,
					name:            "d2",
					replicas:        &replica0,
					resourceVersion: "2",
				}),
				dZeroReplicas,
			},
		}, list)
	})

	t.Run("fails to patch deployment", func(t *testing.T) {
		fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client:      c,
			ShouldError: true,
		}

		s := sleep{
			Client: fakeClient,
		}
		err := s.updateDeploymentsWithZeroReplicas(context.Background(), deployList)
		require.EqualError(t, err, "error during patch")
	})

	t.Run("fails to patch - deployment not found", func(t *testing.T) {
		c := fake.NewClientBuilder().Build()
		fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: c,
		}

		s := sleep{
			Client: fakeClient,
		}
		err := s.updateDeploymentsWithZeroReplicas(context.Background(), deployList)
		require.NoError(t, err)
	})
}
