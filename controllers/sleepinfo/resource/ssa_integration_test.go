package resource

import (
	"context"
	"testing"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestServerSideApply(t *testing.T) {
	const (
		kindClusterName = "kube-green-resource"
	)
	testenv := env.New()
	runID := envconf.RandomName("kube-green-resource", 24)

	ssaPatch := features.Table{
		{
			Name: "correctly patch resource",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()

				resource := upsertResource(t, ctx, c)

				client := ResourceClient{
					SleepInfo:        &kubegreenv1alpha1.SleepInfo{},
					Log:              logr.Discard(),
					Client:           k8sClient,
					FieldManagerName: "kube-green-test",
				}

				newResource := resource.DeepCopy()
				newResource.SetLabels(map[string]string{
					"new-label": "new-value",
				})

				err := client.SSAPatch(ctx, newResource)
				require.NoError(t, err)

				unstructuredRes := &unstructured.Unstructured{
					Object: resource.NewEmptyInstance().UnstructuredContent(),
				}
				err = testutil.GetResource(ctx, k8sClient, resource.GetName(), resource.GetNamespace(), unstructuredRes)
				require.NoError(t, err)
				require.Equal(t, newResource.GetLabels(), unstructuredRes.GetLabels())
				require.Equal(t, newResource.GetAnnotations(), unstructuredRes.GetAnnotations())
				require.Equal(t, newResource.Object["spec"], unstructuredRes.Object["spec"])

				return ctx
			},
		},
		{
			Name: "correctly patch the same resource",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()

				resource := upsertResource(t, ctx, c)

				client := ResourceClient{
					SleepInfo:        &kubegreenv1alpha1.SleepInfo{},
					Log:              logr.Discard(),
					Client:           k8sClient,
					FieldManagerName: "kube-green-test",
				}

				err := client.SSAPatch(context.Background(), &resource)
				require.NoError(t, err)

				unstructuredRes := unstructured.Unstructured{
					Object: resource.NewEmptyInstance().UnstructuredContent(),
				}
				err = testutil.GetResource(context.Background(), k8sClient, resource.GetName(), resource.GetNamespace(), &unstructuredRes)
				require.NoError(t, err)
				require.Equal(t, resource.GetLabels(), unstructuredRes.GetLabels())
				require.Equal(t, resource.GetAnnotations(), unstructuredRes.GetAnnotations())
				require.Equal(t, resource.Object["spec"], unstructuredRes.Object["spec"])

				return ctx
			},
		},
		{
			Name: "does not throw if resource not found",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()

				client := ResourceClient{
					SleepInfo:        &kubegreenv1alpha1.SleepInfo{},
					Log:              logr.Discard(),
					Client:           k8sClient,
					FieldManagerName: "kube-green-test",
				}

				resNotFound, err := runtime.DefaultUnstructuredConverter.ToUnstructured(getDeployment("not-exists", c))
				require.NoError(t, err)

				res := unstructured.Unstructured{
					Object: resNotFound,
				}
				res.SetGroupVersionKind(schema.FromAPIVersionAndKind("apps/v1", "Deployment"))
				err = testutil.GetResource(context.Background(), k8sClient, res.GetName(), res.GetNamespace(), &res)
				require.True(t, apierrors.IsNotFound(err))

				err = client.SSAPatch(context.Background(), &res)
				require.NoError(t, err)

				unstructuredRes := unstructured.Unstructured{
					Object: res.NewEmptyInstance().UnstructuredContent(),
				}
				err = testutil.GetResource(context.Background(), k8sClient, res.GetName(), res.GetNamespace(), &unstructuredRes)
				require.NoError(t, err)
				require.Equal(t, unstructuredRes.GetName(), "not-exists")

				return ctx
			},
		},
	}.
		Build("Server Side Apply").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			ctx, err := testutil.SetupEnvTest()(ctx, c)
			require.NoError(t, err)

			ctx, err = testutil.GetClusterVersion()(ctx, c)
			require.NoError(t, err)

			ctx, err = testutil.CreateNSForTest(ctx, c, t, runID)
			require.NoError(t, err)

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			cleanupNamespaceDeployments(t, c)

			ctx, err := testutil.StopEnvTest()(ctx, c)
			require.NoError(t, err)

			return ctx
		}).
		Feature()

	testenv.Test(t, ssaPatch)
}

func cleanupNamespaceDeployments(t *testing.T, c *envconf.Config) {
	res := unstructured.Unstructured{}
	res.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "Deployment",
		Version: "v1",
	})

	k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()
	err := k8sClient.DeleteAllOf(context.Background(), &res, client.InNamespace(c.Namespace()))
	require.NoError(t, client.IgnoreNotFound(err))
}

func upsertResource(t *testing.T, ctx context.Context, c *envconf.Config) unstructured.Unstructured {
	name := testutil.RandString(8)
	deployment := getDeployment(name, c)

	resource, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&deployment)
	require.NoError(t, err)

	unstructuredResource := unstructured.Unstructured{
		Object: resource,
	}
	unstructuredResource.SetGroupVersionKind(schema.FromAPIVersionAndKind("apps/v1", "Deployment"))

	k8sClient := c.Client().Resources(c.Namespace()).GetControllerRuntimeClient()
	if err := k8sClient.Create(ctx, deployment); err != nil {
		require.NoError(t, err)
	}

	err = wait.For(conditions.New(c.Client().Resources(c.Namespace())).
		ResourceMatch(unstructuredResource.DeepCopy(), func(object k8s.Object) bool {
			return true
		}), wait.WithInterval(100*time.Millisecond), wait.WithTimeout(5*time.Second),
	)
	require.NoError(t, err)

	return unstructuredResource
}

func getDeployment(name string, c *envconf.Config) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.Namespace(),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": name,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "my-container",
							Image: "my-image",
						},
					},
				},
			},
		},
	}
}
