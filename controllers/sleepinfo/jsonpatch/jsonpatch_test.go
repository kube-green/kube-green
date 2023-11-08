package jsonpatch

import (
	"context"
	"testing"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/sleepinfo/deployments"
	"github.com/kube-green/kube-green/controllers/sleepinfo/resource"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestUpdateResourcesJSONPatch(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))
	namespace := "test"

	getNewResource := func(t *testing.T, client client.Client, sleepInfo *v1alpha1.SleepInfo) genericResource {
		t.Helper()

		resource, err := NewResource(context.Background(), resource.ResourceClient{
			Client:    client,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, namespace, nil)
		require.NoError(t, err)

		generic, ok := resource.(genericResource)
		require.True(t, ok)
		return generic
	}

	t.Run("full lifecycle - one resource type", func(t *testing.T) {
		t.Run("not throws if no data", func(t *testing.T) {
			t.SkipNow()
			c := getNewResource(t, getFakeClient().Build(), nil)
			require.NoError(t, c.Sleep(context.Background()))
		})

		t.Run("patches resources", func(t *testing.T) {
			patchData := &v1alpha1.PatchJson6902{
				Target: v1alpha1.PatchTarget{
					Group: "apps",
					Kind:  "Deployment",
				},
				Patches: `[{"op": "replace", "path": "/spec/replicas", "value": 0}]`,
			}
			deployWithReplicas := deployments.GetMock(deployments.MockSpec{
				Name:      "d1",
				Namespace: namespace,
				Replicas:  getPtr(int32(3)),
			})
			deployWithoutReplicas := deployments.GetMock(deployments.MockSpec{
				Name:      "d2",
				Namespace: namespace,
			})

			fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client: getFakeClient().
					WithRuntimeObjects(&deployWithReplicas, &deployWithoutReplicas).
					Build(),
			}
			sleepInfo := &v1alpha1.SleepInfo{
				TypeMeta: v1.TypeMeta{
					Kind: "SleepInfo",
				},
				ObjectMeta: v1.ObjectMeta{
					Namespace: namespace,
					Name:      "test-sleepinfo",
				},
				Spec: v1alpha1.SleepInfoSpec{
					PatchesJson6902: []v1alpha1.PatchJson6902{
						*patchData,
					},
				},
			}

			ctx := context.Background()
			res := getNewResource(t, fakeClient, sleepInfo)

			t.Run("sleep", func(t *testing.T) {
				require.NoError(t, res.Sleep(ctx))

				resList, err := res.getListByNamespace(ctx, namespace, patchData)
				require.NoError(t, err)

				require.Equal(t, 0, int(resList[0].Object["spec"].(map[string]interface{})["replicas"].(int64)))
				require.Equal(t, 0, int(resList[0].Object["spec"].(map[string]interface{})["replicas"].(int64)))

				t.Run("wake up", func(t *testing.T) {
					require.NoError(t, res.WakeUp(ctx))

					resList, err := res.getListByNamespace(ctx, namespace, patchData)
					require.NoError(t, err)

					require.Equal(t, 3, int(resList[0].Object["spec"].(map[string]interface{})["replicas"].(int64)))
				})
			})
		})
	})
}

func getFakeClient() *fake.ClientBuilder {
	groupVersion := []schema.GroupVersion{
		{Group: "apps", Version: "v1"},
	}
	restMapper := meta.NewDefaultRESTMapper(groupVersion)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}, meta.RESTScopeNamespace)

	return fake.
		NewClientBuilder().
		WithRESTMapper(restMapper)
}

func getPtr[T any](v T) *T {
	return &v
}
