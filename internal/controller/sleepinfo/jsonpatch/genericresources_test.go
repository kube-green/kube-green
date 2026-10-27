package jsonpatch

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/internal/mocks"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
	d4 := mocks.Deployment(mocks.DeploymentOptions{
		Name:      "d4",
		Namespace: namespace,
		Replicas:  getPtr[int32](1),
		Labels: map[string]string{
			"kube-green.dev/include": "true",
		},
	})
	d5 := mocks.Deployment(mocks.DeploymentOptions{
		Name:      "d5",
		Namespace: namespace,
		Replicas:  getPtr[int32](1),
		Labels: map[string]string{
			"kube-green.dev/include": "true",
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
			Client: getFakeClient().WithObjects(d1.Resource(), d2.Resource(), d3.Resource(), d4.Resource()).Build(),
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
				ExcludeRef: []v1alpha1.FilterRef{
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
			Client: getFakeClient().WithObjects(d1.Resource(), d2.Resource(), d3.Resource(), d4.Resource()).Build(),
		}

		generic := newGenericResource(resource.ResourceClient{
			Client:    fakeClient,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, deployPatchData, RestorePatches{})
		list, err := generic.getListByNamespace(context.Background(), namespace, deployPatchData.Target)
		require.NoError(t, err)
		require.Len(t, list, 2)

		cleanResourceVersion(list)
		require.Equal(t, []unstructured.Unstructured{d3.Unstructured(), d4.Unstructured()}, list)
	})

	t.Run("exclude configured resources by name", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
				},
				ExcludeRef: []v1alpha1.FilterRef{
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
			Client: getFakeClient().WithObjects(d1.Resource(), d2.Resource(), d3.Resource(), d4.Resource()).Build(),
		}

		generic := newGenericResource(resource.ResourceClient{
			Client:    fakeClient,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, deployPatchData, RestorePatches{})
		list, err := generic.getListByNamespace(context.Background(), namespace, deployPatchData.Target)
		require.NoError(t, err)
		require.Len(t, list, 2)

		cleanResourceVersion(list)
		require.Equal(t, []unstructured.Unstructured{d1.Unstructured(), d4.Unstructured()}, list)
	})

	t.Run("include configured resources by name", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
				},
				IncludeRef: []v1alpha1.FilterRef{
					{
						Kind:       "Deployment",
						Name:       "d1",
						APIVersion: "apps/v1",
					},
				},
			},
		}

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().WithObjects(d1.Resource(), d2.Resource(), d3.Resource(), d4.Resource()).Build(),
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

	t.Run("include configured resources by labels", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
				},
				IncludeRef: []v1alpha1.FilterRef{
					{
						MatchLabels: map[string]string{
							"kube-green.dev/include": "true",
						},
					},
				},
			},
		}

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().WithObjects(d1.Resource(), d2.Resource(), d3.Resource(), d4.Resource()).Build(),
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
		require.Equal(t, []unstructured.Unstructured{d4.Unstructured()}, list)
	})

	t.Run("include configured resources by labels and exclude by name", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
				},
				IncludeRef: []v1alpha1.FilterRef{
					{
						MatchLabels: map[string]string{
							"kube-green.dev/include": "true",
						},
					},
				},
				ExcludeRef: []v1alpha1.FilterRef{
					{
						Kind:       "Deployment",
						Name:       "d5",
						APIVersion: "apps/v1",
					},
				},
			},
		}

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().WithObjects(d1.Resource(), d2.Resource(), d3.Resource(), d4.Resource(), d5.Resource()).Build(),
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
		require.Equal(t, []unstructured.Unstructured{d4.Unstructured()}, list)
	})
}

func cleanResourceVersion(list []unstructured.Unstructured) {
	for _, item := range list {
		unstructured.RemoveNestedField(item.Object, "metadata", "resourceVersion")
	}
}

// TestListResourcesPagination checks complete snapshots, empty continuation pages,
// and the distinction between an absent target and a failed paginated request.
func TestListResourcesPagination(t *testing.T) {
	first := mocks.Deployment(mocks.DeploymentOptions{Name: "first", Namespace: "test"}).Unstructured()
	second := mocks.Deployment(mocks.DeploymentOptions{Name: "second", Namespace: "test"}).Unstructured()
	listError := errors.New("later page unavailable")
	notFound := apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "")
	type page struct {
		items []unstructured.Unstructured
		next  string
		err   error
	}
	for _, tc := range []struct {
		name  string
		pages []page
		want  []unstructured.Unstructured
		err   error
	}{
		{
			name: "multiple pages",
			pages: []page{
				{items: []unstructured.Unstructured{first}, next: "next-page"},
				{items: []unstructured.Unstructured{second}},
			},
			want: []unstructured.Unstructured{first, second},
		},
		{
			name: "empty page with continuation",
			pages: []page{
				{next: "next-page"},
				{items: []unstructured.Unstructured{second}},
			},
			want: []unstructured.Unstructured{second},
		},
		{
			name:  "authoritative empty list",
			pages: []page{{}},
		},
		{
			name: "later page failure discards partial results",
			pages: []page{
				{items: []unstructured.Unstructured{first}, next: "next-page"},
				{err: listError},
			},
			err: listError,
		},
		{
			name: "later page NotFound discards partial results and fails",
			pages: []page{
				{items: []unstructured.Unstructured{first}, next: "next-page"},
				{err: notFound},
			},
			err: notFound,
		},
		{
			name:  "first page NotFound confirms absent target",
			pages: []page{{err: notFound}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			fakeClient := getFakeClient().WithInterceptorFuncs(interceptor.Funcs{
				List: func(_ context.Context, _ client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					options := (&client.ListOptions{}).ApplyOptions(opts)
					require.Equal(t, "test", options.Namespace)
					require.Equal(t, int64(500), options.Limit)
					require.Equal(t, "metadata.name!=excluded", options.FieldSelector.String())
					require.Equal(t, "tier==standard", options.LabelSelector.String())
					expectedContinue := ""
					if calls > 0 {
						expectedContinue = tc.pages[calls-1].next
					}
					require.Equal(t, expectedContinue, options.Continue)
					require.Less(t, calls, len(tc.pages))
					page := tc.pages[calls]
					calls++
					if page.err != nil {
						return page.err
					}
					result := list.(*unstructured.UnstructuredList)
					result.Items = page.items
					result.SetContinue(page.next)
					return nil
				},
			}).Build()
			sleepInfo := &v1alpha1.SleepInfo{Spec: v1alpha1.SleepInfoSpec{
				IncludeRef: []v1alpha1.FilterRef{{MatchLabels: map[string]string{"tier": "standard"}}},
				ExcludeRef: []v1alpha1.FilterRef{{APIVersion: "apps/v1", Kind: "Deployment", Name: "excluded"}},
			}}
			generic := newGenericResource(resource.ResourceClient{
				Client: fakeClient, Log: zap.New(zap.WriteTo(&bytes.Buffer{})), SleepInfo: sleepInfo,
			}, deployPatchData, RestorePatches{})
			got, err := generic.getListByNamespace(t.Context(), "test", deployPatchData.Target)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.want, got)
			require.Equal(t, len(tc.pages), calls)
		})
	}
}
