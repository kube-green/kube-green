package jsonpatch

import (
	"bytes"
	"context"
	"testing"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/sleepinfo/cronjobs"
	"github.com/kube-green/kube-green/controllers/sleepinfo/deployments"
	"github.com/kube-green/kube-green/controllers/sleepinfo/resource"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestUpdateResourcesJSONPatch(t *testing.T) {
	nullogger := &bytes.Buffer{}
	testLogger := zap.New(zap.WriteTo(nullogger))
	namespace := "test"

	getNewResource := func(t *testing.T, client client.Client, sleepInfo *v1alpha1.SleepInfo) managedResources {
		t.Helper()

		resource, err := NewResources(context.Background(), resource.ResourceClient{
			Client:    client,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, namespace, nil)
		require.NoError(t, err)

		generic, ok := resource.(managedResources)
		require.True(t, ok)
		return generic
	}

	t.Run("full lifecycle - one resource type", func(t *testing.T) {
		deploymentKind := "Deployment"
		cronjobKind := "CronJob"

		t.Run("not throws if no data", func(t *testing.T) {
			t.SkipNow()
			c := getNewResource(t, getFakeClient().Build(), nil)
			require.NoError(t, c.Sleep(context.Background()))
		})

		t.Run("patches resources", func(t *testing.T) {
			deployPatchData := v1alpha1.PatchJson6902{
				Target: v1alpha1.PatchTarget{
					Group: "apps",
					Kind:  "Deployment",
				},
				Patches: `[{"op": "replace", "path": "/spec/replicas", "value": 0}]`,
			}
			cronPatchData := v1alpha1.PatchJson6902{
				Target: v1alpha1.PatchTarget{
					Group: "batch",
					Kind:  "CronJob",
				},
				Patches: `[{"op": "replace", "path": "/spec/suspend", "value": true}]`,
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
						deployPatchData,
						cronPatchData,
					},
				},
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
			cronjob := cronjobs.GetMock(cronjobs.MockSpec{
				Name:      "cron-suspend-false",
				Namespace: namespace,
				Suspend:   getPtr(false),
			})
			suspendedCj := cronjobs.GetMock(cronjobs.MockSpec{
				Name:      "cron-suspend-true",
				Namespace: namespace,
				Suspend:   getPtr(true),
			})
			cjWithoutSuspendData := cronjobs.GetMock(cronjobs.MockSpec{
				Name:      "cron-no-suspend",
				Namespace: namespace,
			})

			fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client: getFakeClient().
					WithRuntimeObjects(
						&deployWithReplicas,
						&deployWithoutReplicas,
						&cronjob,
						&suspendedCj,
						&cjWithoutSuspendData,
					).
					Build(),
			}

			ctx := context.Background()
			res := getNewResource(t, fakeClient, sleepInfo)

			originalDeployments, err := res.resMapping[deploymentKind].getListByNamespace(ctx, namespace, deployPatchData)
			require.NoError(t, err)
			originalCronJob, err := res.resMapping[cronjobKind].getListByNamespace(ctx, namespace, cronPatchData)
			require.NoError(t, err)

			t.Run("sleep", func(t *testing.T) {
				require.NoError(t, res.Sleep(ctx))

				t.Run("Deployment", func(t *testing.T) {
					resList, err := res.resMapping[deploymentKind].getListByNamespace(ctx, namespace, deployPatchData)
					require.NoError(t, err)

					require.Len(t, resList, 2)
					require.Equal(t, 0, int(findResByName(resList, "d1").Object["spec"].(map[string]interface{})["replicas"].(int64)))
					require.Nil(t, findResByName(resList, "d2").Object["spec"].(map[string]interface{})["replicas"])
				})

				t.Run("CronJob", func(t *testing.T) {
					resList, err := res.resMapping[cronjobKind].getListByNamespace(ctx, namespace, cronPatchData)
					require.NoError(t, err)

					require.Len(t, resList, 3)
					suspend, ok, err := unstructured.NestedBool(findResByName(resList, "cron-suspend-false").Object, "spec", "suspend")
					require.NoError(t, err)
					require.True(t, ok)
					require.True(t, suspend)
					suspend2, ok, err := unstructured.NestedBool(findResByName(resList, "cron-suspend-true").Object, "spec", "suspend")
					require.NoError(t, err)
					require.True(t, ok)
					require.True(t, suspend2)
					_, ok, err = unstructured.NestedBool(findResByName(resList, "cron-no-suspend").Object, "spec", "suspend")
					require.NoError(t, err)
					require.False(t, ok)
				})

				t.Run("wake up", func(t *testing.T) {
					require.NoError(t, res.WakeUp(ctx))

					t.Run("Deployment", func(t *testing.T) {
						resList, err := res.resMapping[deploymentKind].getListByNamespace(ctx, namespace, deployPatchData)
						require.NoError(t, err)

						require.Len(t, resList, 2)
						requireEqualResources(t, originalDeployments, resList)
					})

					t.Run("CronJob", func(t *testing.T) {
						resList, err := res.resMapping[cronjobKind].getListByNamespace(ctx, namespace, cronPatchData)
						require.NoError(t, err)

						require.Len(t, resList, 3)
						requireEqualResources(t, originalCronJob, resList)
					})
				})
			})
		})
	})
}

func getFakeClient() *fake.ClientBuilder {
	groupVersion := []schema.GroupVersion{
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
	}
	restMapper := meta.NewDefaultRESTMapper(groupVersion)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}, meta.RESTScopeNamespace)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "batch",
		Version: "v1",
		Kind:    "CronJob",
	}, meta.RESTScopeNamespace)

	return fake.
		NewClientBuilder().
		WithRESTMapper(restMapper)
}

func getPtr[T any](v T) *T {
	return &v
}

func findResByName(list []unstructured.Unstructured, name string) *unstructured.Unstructured {
	for _, res := range list {
		if res.GetName() == name {
			return &res
		}
	}
	return nil
}

func requireEqualResources(t *testing.T, expectedList, actualList []unstructured.Unstructured) {
	t.Helper()

	require.NotEqual(t, 0, len(expectedList))

	for _, expected := range expectedList {
		unstructured.RemoveNestedField(expected.Object, "metadata", "resourceVersion")

		actual := findResByName(actualList, expected.GetName())
		require.NotNil(t, actual)
		unstructured.RemoveNestedField(actual.Object, "metadata", "resourceVersion")

		require.Equal(t, expected, *actual)
	}
}
