package resource

import (
	"context"
	"fmt"
	"testing"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/davidebianchi/kube-green/controllers/internal/testutil"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResource(t *testing.T) {
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-name",
			Namespace: "test-namespace",
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

	t.Run("IsClientValid", func(t *testing.T) {
		t.Run("without any field", func(t *testing.T) {
			r := ResourceClient{}

			require.EqualError(t, r.IsClientValid(), fmt.Sprintf("%s: %s and %s and %s", ErrInvalidClient, errClientEmpty, errLogEmpty, errSleepInfoEmpty))
		})

		t.Run("without Log", func(t *testing.T) {
			r := ResourceClient{
				Client:    fake.NewClientBuilder().Build(),
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
			}

			require.EqualError(t, r.IsClientValid(), fmt.Sprintf("%s: %s", ErrInvalidClient, errLogEmpty))
		})

		t.Run("without SleepInfo", func(t *testing.T) {
			r := ResourceClient{
				Client: fake.NewClientBuilder().Build(),
				Log:    logr.DiscardLogger{},
			}

			require.EqualError(t, r.IsClientValid(), fmt.Sprintf("%s: %s", ErrInvalidClient, errSleepInfoEmpty))
		})

		t.Run("without Client", func(t *testing.T) {
			r := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.DiscardLogger{},
			}

			require.EqualError(t, r.IsClientValid(), fmt.Sprintf("%s: %s", ErrInvalidClient, errClientEmpty))
		})

		t.Run("valid client", func(t *testing.T) {
			r := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.DiscardLogger{},
				Client:    fake.NewClientBuilder().Build(),
			}

			require.NoError(t, r.IsClientValid())
		})
	})

	t.Run("Patch", func(t *testing.T) {
		t.Run("fails if client is not valid", func(t *testing.T) {
			c := ResourceClient{}

			require.ErrorIs(t, c.Patch(context.Background(), nil, nil), ErrInvalidClient)
		})

		t.Run("correctly patch resource", func(t *testing.T) {
			k8sClient := fake.NewClientBuilder().WithRuntimeObjects(&deployment).Build()
			c := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.DiscardLogger{},
				Client:    k8sClient,
			}

			newD1 := deployment.DeepCopy()
			newD1.Spec.Template.Spec.Containers[0].Image = "new-image"

			require.NoError(t, c.Patch(context.Background(), &deployment, newD1))

			actualDeployment := &appsv1.Deployment{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      deployment.Name,
				Namespace: deployment.Namespace,
			}, actualDeployment)
			require.NoError(t, err)
			require.Equal(t, newD1, actualDeployment)
		})

		t.Run("correctly patch the same resource", func(t *testing.T) {
			k8sClient := fake.NewClientBuilder().WithRuntimeObjects(&deployment).Build()
			c := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.DiscardLogger{},
				Client:    k8sClient,
			}

			require.NoError(t, c.Patch(context.Background(), &deployment, &deployment))

			actualDeployment := &appsv1.Deployment{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      deployment.Name,
				Namespace: deployment.Namespace,
			}, actualDeployment)
			require.NoError(t, err)
			require.Equal(t, &deployment, actualDeployment)
		})

		t.Run("does not throw if resource not found", func(t *testing.T) {
			k8sClient := fake.NewClientBuilder().Build()
			c := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.DiscardLogger{},
				Client:    k8sClient,
			}

			newD1 := deployment.DeepCopy()
			newD1.Spec.Template.Spec.Containers[0].Image = "new-image"

			require.NoError(t, c.Patch(context.Background(), &deployment, newD1))

			actualDeployment := &appsv1.Deployment{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      deployment.Name,
				Namespace: deployment.Namespace,
			}, actualDeployment)
			require.True(t, apierrors.IsNotFound(err))
		})

		t.Run("throws if patch fails", func(t *testing.T) {
			k8sClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client: fake.NewClientBuilder().Build(),
				ShouldError: func(method testutil.Method, obj runtime.Object) bool {
					return true
				},
			}
			c := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.DiscardLogger{},
				Client:    k8sClient,
			}

			newD1 := deployment.DeepCopy()
			newD1.Spec.Template.Spec.Containers[0].Image = "new-image"

			require.EqualError(t, c.Patch(context.Background(), &deployment, newD1), "error during patch")
		})
	})

	t.Run("Server Side Apply patch", func(t *testing.T) {
		t.Run("fails if client is not valid", func(t *testing.T) {
			c := ResourceClient{}

			require.ErrorIs(t, c.SSAPatch(context.Background(), nil), ErrInvalidClient)
		})

		t.Run("throws if patch fails", func(t *testing.T) {
			k8sClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client: fake.NewClientBuilder().Build(),
				ShouldError: func(method testutil.Method, obj runtime.Object) bool {
					return true
				},
			}
			c := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.DiscardLogger{},
				Client:    k8sClient,
			}

			newD1 := deployment.DeepCopy()
			newD1.Spec.Template.Spec.Containers[0].Image = "new-image"

			require.EqualError(t, c.SSAPatch(context.Background(), &deployment), "error during patch")
		})
	})
}
