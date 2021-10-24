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
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestWakeUp(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	namespace := "my-namespace"
	var replica0 int32 = 0
	var replica1 int32 = 1
	var replica5 int32 = 5

	d1 := getDeploymentMock(mockDeploymentSpec{
		namespace:       namespace,
		name:            "d1",
		replicas:        &replica0,
		resourceVersion: "2",
	})
	d2 := getDeploymentMock(mockDeploymentSpec{
		namespace:       namespace,
		name:            "d2",
		replicas:        &replica0,
		resourceVersion: "1",
	})
	dNotSleeped := getDeploymentMock(mockDeploymentSpec{
		namespace:       namespace,
		name:            "d0",
		replicas:        &replica1,
		resourceVersion: "2",
	})
	dZeroReplicas := getDeploymentMock(mockDeploymentSpec{
		namespace:       namespace,
		name:            "dZeroReplicas",
		replicas:        &replica0,
		resourceVersion: "1",
	})
	deployList := []appsv1.Deployment{d1, dNotSleeped, d2, dZeroReplicas}
	sleepInfoData := SleepInfoData{
		OriginalDeploymentsReplicas: map[string]int32{
			"d1": 1,
			"d2": 5,
		},
	}

	c := fake.NewClientBuilder().WithRuntimeObjects(&d1, &dNotSleeped, &d2, &dZeroReplicas).Build()

	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}

	t.Run("wake up deployment", func(t *testing.T) {
		fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: c,
		}

		w := wakeUp{
			Client: fakeClient,
			logger: testLogger,
		}

		err := w.wakeUpDeploymentReplicas(context.Background(), deployList, sleepInfoData)
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
				dNotSleeped,
				getDeploymentMock(mockDeploymentSpec{
					namespace:       namespace,
					name:            "d1",
					replicas:        &replica1,
					resourceVersion: "3",
				}),
				getDeploymentMock(mockDeploymentSpec{
					namespace:       namespace,
					name:            "d2",
					replicas:        &replica5,
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

		w := wakeUp{
			Client: fakeClient,
			logger: testLogger,
		}
		err := w.wakeUpDeploymentReplicas(context.Background(), deployList, sleepInfoData)
		require.EqualError(t, err, "error during patch")
	})

	t.Run("fails to patch - deployment not found", func(t *testing.T) {
		c := fake.NewClientBuilder().Build()
		fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: c,
		}

		w := wakeUp{
			Client: fakeClient,
			logger: testLogger,
		}
		err := w.wakeUpDeploymentReplicas(context.Background(), deployList, sleepInfoData)
		require.NoError(t, err)
	})
}
