package sleepinfo

import (
	"context"
	"errors"

	"github.com/davidebianchi/kube-green/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("Test Schedule", func() {
	testLogger := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))

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
			client: &possiblyErroringFakeCtrlRuntimeClient{
				Client:      fake.NewClientBuilder().Build(),
				shouldError: true,
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

	for _, dt := range listDeploymentsTests {
		test := dt //necessary to ensure the correct value is passed to the closure
		It(test.name, func() {
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
				Expect(err).To(MatchError("error during list"))
			} else {
				Expect(err).To(BeNil())
			}
			Expect(listOfDeployments).To(Equal(test.expected))
		})
	}
})

type mockDeploymentSpec struct {
	namespace string
	name      string
}

func getDeploymentMock(opts mockDeploymentSpec) appsv1.Deployment {
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.name,
			Namespace: opts.namespace,
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
		},
	}
}

type possiblyErroringFakeCtrlRuntimeClient struct {
	client.Client
	shouldError bool
}

func (p *possiblyErroringFakeCtrlRuntimeClient) List(ctx context.Context, dpl client.ObjectList, opts ...client.ListOption) error {
	if p.shouldError {
		return errors.New("error during list")
	}
	return p.Client.List(ctx, dpl, opts...)
}
