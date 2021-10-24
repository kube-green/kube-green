package sleepinfo

import (
	"context"
	"testing"

	"github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/davidebianchi/kube-green/controllers/internal/testutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestSchedule(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	namespace := "my-namespace"
	deployment1 := getDeploymentMock(mockDeploymentSpec{
		name:      "deployment1",
		namespace: namespace,
	})
	deployment2 := getDeploymentMock(mockDeploymentSpec{
		name:      "deployment2",
		namespace: namespace,
	})
	deploymentOtherNamespace := getDeploymentMock(mockDeploymentSpec{
		name:      "deploymentOtherNamespace",
		namespace: "other-namespace",
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
			r := SleepInfoReconciler{
				Client: test.client,
				Log:    testLogger,
			}
			sleepInfo := emptySleepInfo
			if test.sleepInfo != nil {
				sleepInfo = test.sleepInfo
			}

			listOfDeployments, err := r.getDeploymentsList(context.Background(), namespace, sleepInfo)
			if test.throws {
				require.EqualError(t, err, "error during list")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.expected, listOfDeployments)
		})
	}
}

type mockDeploymentSpec struct {
	namespace       string
	name            string
	replicas        *int32
	resourceVersion string
}

func getDeploymentMock(opts mockDeploymentSpec) appsv1.Deployment {
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            opts.name,
			Namespace:       opts.namespace,
			ResourceVersion: opts.resourceVersion,
		},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "my-image",
						},
					},
				},
			},
			Replicas: opts.replicas,
		},
	}
}
