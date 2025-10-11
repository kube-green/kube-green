package resource

import (
	"context"
	"fmt"
	"testing"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/internal/mocks"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResource(t *testing.T) {
	const newImageName = "new-image"

	deployment := mocks.Deployment(mocks.DeploymentOptions{
		Name:      "my-name",
		Namespace: "test-namespace",
	})

	t.Run("IsClientValid", func(t *testing.T) {
		t.Run("without any field", func(t *testing.T) {
			r := ResourceClient{}

			require.EqualError(t, r.IsClientValid(), fmt.Sprintf("%s: %s and %s", ErrInvalidClient, errClientEmpty, errSleepInfoEmpty))
		})

		t.Run("without SleepInfo", func(t *testing.T) {
			r := ResourceClient{
				Client: fake.NewClientBuilder().Build(),
				Log:    logr.Discard(),
			}

			require.EqualError(t, r.IsClientValid(), fmt.Sprintf("%s: %s", ErrInvalidClient, errSleepInfoEmpty))
		})

		t.Run("without Client", func(t *testing.T) {
			r := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.Discard(),
			}

			require.EqualError(t, r.IsClientValid(), fmt.Sprintf("%s: %s", ErrInvalidClient, errClientEmpty))
		})

		t.Run("valid client", func(t *testing.T) {
			r := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.Discard(),
				Client:    fake.NewClientBuilder().Build(),
			}

			require.NoError(t, r.IsClientValid())
		})
	})

	t.Run("Patch", func(t *testing.T) {
		deployRes := deployment.Resource()

		t.Run("fails if client is not valid", func(t *testing.T) {
			c := ResourceClient{}

			require.ErrorIs(t, c.Patch(context.Background(), nil, nil), ErrInvalidClient)
		})

		t.Run("correctly patch resource", func(t *testing.T) {
			k8sClient := fake.NewClientBuilder().WithRuntimeObjects(deployRes).Build()
			c := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.Discard(),
				Client:    k8sClient,
			}

			newD1 := deployRes.DeepCopy()
			newD1.Spec.Template.Spec.Containers[0].Image = newImageName

			require.NoError(t, c.Patch(context.Background(), deployRes, newD1))

			actualDeployment := &appsv1.Deployment{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      deployRes.Name,
				Namespace: deployRes.Namespace,
			}, actualDeployment)
			require.NoError(t, err)
			require.Equal(t, newD1, actualDeployment)
		})

		t.Run("correctly patch the same resource", func(t *testing.T) {
			k8sClient := fake.NewClientBuilder().WithRuntimeObjects(deployRes).Build()
			c := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.Discard(),
				Client:    k8sClient,
			}

			require.NoError(t, c.Patch(context.Background(), deployRes, deployRes))

			actualDeployment := &appsv1.Deployment{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      deployRes.Name,
				Namespace: deployRes.Namespace,
			}, actualDeployment)
			require.NoError(t, err)
			require.Equal(t, deployRes, actualDeployment)
		})

		t.Run("does not throw if resource not found", func(t *testing.T) {
			k8sClient := fake.NewClientBuilder().Build()
			c := ResourceClient{
				SleepInfo: &kubegreenv1alpha1.SleepInfo{},
				Log:       logr.Discard(),
				Client:    k8sClient,
			}

			newD1 := deployRes.DeepCopy()
			newD1.Spec.Template.Spec.Containers[0].Image = newImageName

			require.NoError(t, c.Patch(context.Background(), deployRes, newD1))

			actualDeployment := &appsv1.Deployment{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      deployRes.Name,
				Namespace: deployRes.Namespace,
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
				Log:       logr.Discard(),
				Client:    k8sClient,
			}

			newD1 := deployRes.DeepCopy()
			newD1.Spec.Template.Spec.Containers[0].Image = newImageName

			require.EqualError(t, c.Patch(context.Background(), deployRes, newD1), "error during patch")
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
				SleepInfo:        &kubegreenv1alpha1.SleepInfo{},
				Log:              logr.Discard(),
				Client:           k8sClient,
				FieldManagerName: "mock-field-manager-name",
			}

			deploy := deployment.Unstructured()

			require.EqualError(t, c.SSAPatch(context.Background(), &deploy), "error during apply")
		})
	})
}
