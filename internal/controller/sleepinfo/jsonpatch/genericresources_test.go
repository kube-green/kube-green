package jsonpatch

import (
	"bytes"
	"context"
	"testing"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/internal/mocks"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestListResources(t *testing.T) {
	nullogger := &bytes.Buffer{}
	testLogger := zap.New(zap.WriteTo(nullogger))
	namespace := "test"

	d1 := mocks.Deployment(mocks.DeploymentOptions{
		Name:      "d1",
		Namespace: namespace,
		Replicas:  getPtr[int32](3),
	})
	d2 := mocks.Deployment(mocks.DeploymentOptions{
		Name:      "d2",
		Namespace: namespace,
		Replicas:  getPtr[int32](1),
		Labels: map[string]string{
			"kube-green.dev/exclude": "true",
		},
	})
	d3 := mocks.Deployment(mocks.DeploymentOptions{
		Name:      "d3",
		Namespace: namespace,
		Replicas:  getPtr[int32](1),
		Labels: map[string]string{
			"kube-green.dev/exclude": "true",
		},
	})

	t.Run("returns empty list if target not supported by cluster", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					unsupportedResourcePatchData,
				},
			},
		}

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().WithObjects(d1.Resource(), d2.Resource(), d3.Resource()).Build(),
		}

		generic := newGenericResource(resource.ResourceClient{
			Client:    fakeClient,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, deployPatchData, RestorePatches{})
		list, err := generic.getListByNamespace(context.Background(), namespace, unsupportedResourcePatchData.Target)
		require.NoError(t, err)
		require.Len(t, list, 0)
	})

	t.Run("exclude configured resources by name", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
				},
				ExcludeRef: []v1alpha1.ExcludeRef{
					{
						Kind:       "Deployment",
						Name:       "d1",
						APIVersion: "apps/v1",
					},
					{
						Kind:       "Deployment",
						Name:       "d2",
						APIVersion: "apps/v1",
					},
					{
						Kind:       "StatefulSet",
						Name:       "d3",
						APIVersion: "apps/v1",
					},
				},
			},
		}

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().WithObjects(d1.Resource(), d2.Resource(), d3.Resource()).Build(),
		}

		generic := newGenericResource(resource.ResourceClient{
			Client:    fakeClient,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, deployPatchData, RestorePatches{})
		list, err := generic.getListByNamespace(context.Background(), namespace, deployPatchData.Target)
		require.NoError(t, err)
		require.Len(t, list, 1)

		cleanResourceVersion(list)
		require.Equal(t, []unstructured.Unstructured{d3.Unstructured()}, list)
	})

	t.Run("exclude configured resources by name", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
				},
				ExcludeRef: []v1alpha1.ExcludeRef{
					{
						MatchLabels: map[string]string{
							"kube-green.dev/exclude": "true",
						},
					},
					{
						Kind:       "StatefulSet",
						Name:       "d1",
						APIVersion: "apps/v1",
					},
				},
			},
		}

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().WithObjects(d1.Resource(), d2.Resource(), d3.Resource()).Build(),
		}

		generic := newGenericResource(resource.ResourceClient{
			Client:    fakeClient,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, deployPatchData, RestorePatches{})
		list, err := generic.getListByNamespace(context.Background(), namespace, deployPatchData.Target)
		require.NoError(t, err)
		require.Len(t, list, 1)

		cleanResourceVersion(list)
		require.Equal(t, []unstructured.Unstructured{d1.Unstructured()}, list)
	})
}

func cleanResourceVersion(list []unstructured.Unstructured) {
	for _, item := range list {
		unstructured.RemoveNestedField(item.Object, "metadata", "resourceVersion")
	}
}
