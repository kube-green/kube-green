package jsonpatch

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/internal/mocks"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	deployPatchData = v1alpha1.Patch{
		Target: v1alpha1.PatchTarget{
			Group: "apps",
			Kind:  "Deployment",
		},
		Patch: `
- op: add
  path: /spec/replicas
  value: 0
`,
	}
	cronPatchData = v1alpha1.Patch{
		Target: v1alpha1.PatchTarget{
			Group: "batch",
			Kind:  "CronJob",
		},
		Patch: `
- op: add
  path: /spec/suspend
  value: true
`,
	}
	replicaSetPatchData = v1alpha1.Patch{
		Target: v1alpha1.PatchTarget{
			Group: "apps",
			Kind:  "ReplicaSet",
		},
		Patch: `
- op: add
  path: /spec/replicas
  value: 0
`,
	}
	unsupportedResourcePatchData = v1alpha1.Patch{
		Target: v1alpha1.PatchTarget{
			Group: "unsupported",
			Kind:  "ResourceKind",
		},
		Patch: `
- op: add
  path: /suspend
  value: true`,
	}
)

func TestNewResources(t *testing.T) {
	nullogger := &bytes.Buffer{}
	testLogger := zap.New(zap.WriteTo(nullogger))
	namespace := "test"

	t.Run("throws if SleepInfo not provided", func(t *testing.T) {
		_, err := NewResources(context.Background(), resource.ResourceClient{
			Client: getFakeClient().Build(),
			Log:    testLogger,
		}, namespace, nil)
		require.EqualError(t, err, fmt.Sprintf("%s: sleepInfo is not provided", ErrJSONPatch))
	})
}

func TestUpdateResourcesJSONPatch(t *testing.T) {
	namespace := "test"

	deployWithReplicas := mocks.Deployment(mocks.DeploymentOptions{
		Name:      "deploy-with-replicas",
		Namespace: namespace,
		Replicas:  getPtr(int32(3)),
	}).Resource()
	deployWithoutReplicas := mocks.Deployment(mocks.DeploymentOptions{
		Name:      "d2",
		Namespace: namespace,
	}).Resource()
	cronjob := mocks.CronJob(mocks.CronJobOptions{
		Name:      "cron-suspend-false",
		Namespace: namespace,
		Suspend:   getPtr(false),
	})
	suspendedCj := mocks.CronJob(mocks.CronJobOptions{
		Name:      "cron-suspend-true",
		Namespace: namespace,
		Suspend:   getPtr(true),
	})
	cjWithoutSuspendData := mocks.CronJob(mocks.CronJobOptions{
		Name:      "cron-no-suspend",
		Namespace: namespace,
	})

	t.Run("full lifecycle - deployment and cronjob", func(t *testing.T) {
		labelsKeyToExclude := "kube-green.dev/exclude"
		labelsValueToExclude := "true"
		deployToExclude := mocks.Deployment(mocks.DeploymentOptions{
			Name:      "deploy-to-exclude",
			Namespace: namespace,
			Replicas:  getPtr(int32(1)),
			Labels: map[string]string{
				labelsKeyToExclude: labelsValueToExclude,
			},
		}).Resource()

		sleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: v1.TypeMeta{
				Kind: "SleepInfo",
			},
			ObjectMeta: v1.ObjectMeta{
				Namespace: namespace,
				Name:      "test-sleepinfo",
			},
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
					cronPatchData,
				},
				ExcludeRef: []v1alpha1.ExcludeRef{
					{
						MatchLabels: map[string]string{
							labelsKeyToExclude: labelsValueToExclude,
						},
					},
				},
			},
		}

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().
				WithRuntimeObjects(
					deployWithReplicas.DeepCopy(),
					deployWithoutReplicas.DeepCopy(),
					deployToExclude.DeepCopy(),
					cronjob.DeepCopy(),
					suspendedCj.DeepCopy(),
					cjWithoutSuspendData.DeepCopy(),
				).
				Build(),
		}

		ctx := context.Background()
		res := getNewResource(t, fakeClient, sleepInfo, namespace)

		t.Run("check that there are resources", func(t *testing.T) {
			require.True(t, res.HasResource())
		})

		deployRes := res.resMapping[deployPatchData.Target]
		cronRes := res.resMapping[cronPatchData.Target]

		originalDeployments, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
		require.NoError(t, err)
		originalCronJob, err := cronRes.getListByNamespace(ctx, namespace, cronPatchData.Target)
		require.NoError(t, err)

		t.Run("sleep", func(t *testing.T) {
			require.NoError(t, res.Sleep(ctx))

			t.Run("Deployment", func(t *testing.T) {
				resList, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
				require.NoError(t, err)

				require.Len(t, resList, 2)
				require.Equal(t, int64(0), findResByName(resList, "deploy-with-replicas").Object["spec"].(map[string]interface{})["replicas"].(int64))
				require.Equal(t, int64(0), findResByName(resList, "d2").Object["spec"].(map[string]interface{})["replicas"].(int64))
			})

			t.Run("CronJob", func(t *testing.T) {
				resList, err := res.resMapping[cronPatchData.Target].getListByNamespace(ctx, namespace, cronPatchData.Target)
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
				suspend3, ok, err := unstructured.NestedBool(findResByName(resList, "cron-no-suspend").Object, "spec", "suspend")
				require.NoError(t, err)
				require.True(t, ok)
				require.True(t, suspend3)
			})

			t.Run("GetOriginalInfoToSave", func(t *testing.T) {
				originalInfo, err := res.GetOriginalInfoToSave()
				require.NoError(t, err)
				require.JSONEq(t, `{
					"Deployment.apps": {"d2":"{\"spec\":{\"replicas\":null}}","deploy-with-replicas":"{\"spec\":{\"replicas\":3}}"},
					"CronJob.batch": {"cron-no-suspend":"{\"spec\":{\"suspend\":null}}", "cron-suspend-false":"{\"spec\":{\"suspend\":false}}"}
				}`, string(originalInfo))

				t.Run("GetOriginalInfoToRestore", func(t *testing.T) {
					patches, err := GetOriginalInfoToRestore(originalInfo)
					require.NoError(t, err)
					require.Equal(t, map[string]RestorePatches{
						"Deployment.apps": map[string]string{
							"d2":                   "{\"spec\":{\"replicas\":null}}",
							"deploy-with-replicas": "{\"spec\":{\"replicas\":3}}",
						},
						"CronJob.batch": map[string]string{
							"cron-no-suspend":    "{\"spec\":{\"suspend\":null}}",
							"cron-suspend-false": "{\"spec\":{\"suspend\":false}}",
						},
					}, patches)
				})
			})

			t.Run("wake up", func(t *testing.T) {
				require.NoError(t, res.WakeUp(ctx))

				t.Run("Deployment", func(t *testing.T) {
					resList, err := res.resMapping[deployPatchData.Target].getListByNamespace(ctx, namespace, deployPatchData.Target)
					require.NoError(t, err)

					require.Len(t, resList, 2)
					requireEqualResources(t, originalDeployments, resList)
				})

				t.Run("CronJob", func(t *testing.T) {
					resList, err := res.resMapping[cronPatchData.Target].getListByNamespace(ctx, namespace, cronPatchData.Target)
					require.NoError(t, err)

					require.Len(t, resList, 3)
					requireEqualResources(t, originalCronJob, resList)
				})

				t.Run("sleep", func(t *testing.T) {
					require.NoError(t, res.Sleep(ctx))

					t.Run("Deployment", func(t *testing.T) {
						resList, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
						require.NoError(t, err)

						require.Len(t, resList, 2)
						require.Equal(t, int64(0), findResByName(resList, "deploy-with-replicas").Object["spec"].(map[string]interface{})["replicas"].(int64))
						require.Equal(t, int64(0), findResByName(resList, "d2").Object["spec"].(map[string]interface{})["replicas"].(int64))
					})

					t.Run("CronJob", func(t *testing.T) {
						resList, err := res.resMapping[cronPatchData.Target].getListByNamespace(ctx, namespace, cronPatchData.Target)
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
						suspend3, ok, err := unstructured.NestedBool(findResByName(resList, "cron-no-suspend").Object, "spec", "suspend")
						require.NoError(t, err)
						require.True(t, ok)
						require.True(t, suspend3)
					})
				})
			})
		})
	})

	t.Run("full lifecycle - deployment and replicaset controlled by deployment", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: v1.TypeMeta{
				Kind: "SleepInfo",
			},
			ObjectMeta: v1.ObjectMeta{
				Namespace: namespace,
				Name:      "test-sleepinfo",
			},
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					replicaSetPatchData,
					deployPatchData,
				},
			},
		}

		ownerDeployment := mocks.Deployment(mocks.DeploymentOptions{
			Name:      "deployment-1",
			Namespace: namespace,
			Replicas:  getPtr(int32(1)),
		}).Resource()
		replicaSetWithOwner := mocks.ReplicaSet(mocks.ReplicaSetSetOptions{
			Name:      "controlled-replica-set",
			Namespace: namespace,
			Replicas:  getPtr(int32(1)),
			OwnerReferences: []v1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "deployment-1",
					Controller: getPtr(true),
				},
			},
		}).Resource()

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().
				WithRuntimeObjects(
					replicaSetWithOwner,
					ownerDeployment,
				).
				Build(),
		}

		ctx := context.Background()
		res := getNewResource(t, fakeClient, sleepInfo, namespace)

		t.Run("check that there are resources", func(t *testing.T) {
			require.True(t, res.HasResource())
		})

		replicaSetRes := res.resMapping[replicaSetPatchData.Target]
		deployRes := res.resMapping[deployPatchData.Target]

		t.Run("sleep", func(t *testing.T) {
			require.NoError(t, res.Sleep(ctx))

			t.Run("Deployment", func(t *testing.T) {
				resList, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
				require.NoError(t, err)

				require.Len(t, resList, 1)
				require.Equal(t, int64(0), resList[0].Object["spec"].(map[string]interface{})["replicas"].(int64))
			})

			t.Run("ReplicaSet", func(t *testing.T) {
				resList, err := replicaSetRes.getListByNamespace(ctx, namespace, replicaSetPatchData.Target)
				require.NoError(t, err)

				require.Len(t, resList, 1)
				require.Equal(t, int64(1), resList[0].Object["spec"].(map[string]interface{})["replicas"].(int64))
			})

			t.Run("GetOriginalInfoToSave", func(t *testing.T) {
				originalInfo, err := res.GetOriginalInfoToSave()
				require.NoError(t, err)
				require.JSONEq(t, `{
					"Deployment.apps": {"deployment-1":"{\"spec\":{\"replicas\":1}}"}
				}`, string(originalInfo))

				t.Run("GetOriginalInfoToRestore", func(t *testing.T) {
					patches, err := GetOriginalInfoToRestore(originalInfo)
					require.NoError(t, err)
					require.Equal(t, map[string]RestorePatches{
						"Deployment.apps": map[string]string{
							"deployment-1": "{\"spec\":{\"replicas\":1}}",
						},
					}, patches)
				})
			})
		})
	})

	t.Run("full lifecycle - keda ScaledObject", func(t *testing.T) {
		scaledObjectPatchData := v1alpha1.Patch{
			Target: v1alpha1.PatchTarget{
				Group: "keda.sh",
				Kind:  "ScaledObject",
			},
			Patch: `
- op: add
  path: /metadata/annotations/autoscaling.keda.sh~1paused-replicas
  value: "0"
`,
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
				Patches: []v1alpha1.Patch{
					scaledObjectPatchData,
				},
			},
		}

		scaledObject := &unstructured.Unstructured{}
		scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "keda.sh",
			Version: "v1alpha1",
			Kind:    "ScaledObject",
		})
		scaledObject.SetName("test-scaledobject-1")
		scaledObject.SetNamespace(namespace)

		scaledObject2 := &unstructured.Unstructured{}
		scaledObject2.SetAnnotations(map[string]string{
			"some": "field",
		})
		scaledObject2.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "keda.sh",
			Version: "v1alpha1",
			Kind:    "ScaledObject",
		})
		scaledObject2.SetName("test-scaledobject-2")
		scaledObject2.SetNamespace(namespace)

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().
				WithRuntimeObjects(
					scaledObject,
					scaledObject2,
				).
				Build(),
		}

		ctx := context.Background()
		res := getNewResource(t, fakeClient, sleepInfo, namespace)

		scaledObjectResource := res.resMapping[scaledObjectPatchData.Target]

		originalScaledObject, err := scaledObjectResource.getListByNamespace(ctx, namespace, scaledObjectPatchData.Target)
		require.NoError(t, err)

		t.Run("sleep", func(t *testing.T) {
			require.NoError(t, res.Sleep(ctx))

			t.Run("ScaledObject", func(t *testing.T) {
				resList, err := scaledObjectResource.getListByNamespace(ctx, namespace, scaledObjectPatchData.Target)
				require.NoError(t, err)

				require.Len(t, resList, 2)

				v, ok, err := unstructured.NestedString(findResByName(resList, "test-scaledobject-1").Object, "metadata", "annotations", "autoscaling.keda.sh/paused-replicas")
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, "0", v)
				v, ok, err = unstructured.NestedString(findResByName(resList, "test-scaledobject-2").Object, "metadata", "annotations", "autoscaling.keda.sh/paused-replicas")
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, "0", v)
			})

			t.Run("wake up", func(t *testing.T) {
				require.NoError(t, res.WakeUp(ctx))

				t.Run("ScaledObject", func(t *testing.T) {
					resList, err := scaledObjectResource.getListByNamespace(ctx, namespace, scaledObjectPatchData.Target)
					require.NoError(t, err)

					require.Len(t, resList, 2)
					requireEqualResources(t, originalScaledObject, resList)
				})
			})
		})
	})

	t.Run("full lifecycle - patch to restore", func(t *testing.T) {
		labelsKeyToExclude := "kube-green.dev/exclude"
		labelsValueToExclude := "true"

		sleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: v1.TypeMeta{
				Kind: "SleepInfo",
			},
			ObjectMeta: v1.ObjectMeta{
				Namespace: namespace,
				Name:      "test-sleepinfo",
			},
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
					cronPatchData,
				},
				ExcludeRef: []v1alpha1.ExcludeRef{
					{
						MatchLabels: map[string]string{
							labelsKeyToExclude: labelsValueToExclude,
						},
					},
				},
			},
		}

		deployWithReplicas := mocks.Deployment(mocks.DeploymentOptions{
			Name:      "deploy-with-replicas",
			Namespace: namespace,
			Replicas:  getPtr(int32(0)),
		}).Resource()

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().
				WithRuntimeObjects(
					deployWithReplicas,
				).
				Build(),
		}

		restorePatches := map[string]RestorePatches{
			"Deployment.apps": {"deploy-with-replicas": "{\"spec\":{\"replicas\":3}}"},
		}

		ctx := context.Background()
		res := getNewResourceWithPatchToRestore(t, fakeClient, sleepInfo, namespace, restorePatches)

		t.Run("check that there are resources", func(t *testing.T) {
			require.True(t, res.HasResource())
		})

		t.Run("wake up", func(t *testing.T) {
			require.NoError(t, res.WakeUp(ctx))

			t.Run("Deployment", func(t *testing.T) {
				resList, err := res.resMapping[deployPatchData.Target].getListByNamespace(ctx, namespace, deployPatchData.Target)
				require.NoError(t, err)

				require.Len(t, resList, 1)
				expectedDeploy := deployWithReplicas.DeepCopy()
				expectedDeploy.Spec.Replicas = getPtr(int32(3))
				unstructuredDeploy, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedDeploy)
				require.NoError(t, err)

				requireEqualResources(t, []unstructured.Unstructured{
					{Object: unstructuredDeploy},
				}, resList)
			})
		})
	})

	t.Run("full lifecycle - with patch target to resource not supported by the cluster", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: v1.TypeMeta{
				Kind: "SleepInfo",
			},
			ObjectMeta: v1.ObjectMeta{
				Namespace: namespace,
				Name:      "test-sleepinfo",
			},
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
					unsupportedResourcePatchData,
				},
			},
		}

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().
				WithRuntimeObjects(
					deployWithReplicas.DeepCopy(),
					deployWithoutReplicas.DeepCopy(),
				).
				Build(),
		}

		ctx := context.Background()
		res := getNewResource(t, fakeClient, sleepInfo, namespace)

		t.Run("check that there are resources", func(t *testing.T) {
			require.True(t, res.HasResource())
		})

		deployRes := res.resMapping[deployPatchData.Target]

		originalDeployments, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
		require.NoError(t, err)

		t.Run("sleep", func(t *testing.T) {
			require.NoError(t, res.Sleep(ctx))

			t.Run("Deployment", func(t *testing.T) {
				resList, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
				require.NoError(t, err)

				require.Len(t, resList, 2)
				require.Equal(t, int64(0), findResByName(resList, "deploy-with-replicas").Object["spec"].(map[string]interface{})["replicas"].(int64))
				require.Equal(t, int64(0), findResByName(resList, "d2").Object["spec"].(map[string]interface{})["replicas"].(int64))
			})

			t.Run("GetOriginalInfoToSave", func(t *testing.T) {
				originalInfo, err := res.GetOriginalInfoToSave()
				require.NoError(t, err)
				require.JSONEq(t, `{
					"Deployment.apps": {"d2":"{\"spec\":{\"replicas\":null}}","deploy-with-replicas":"{\"spec\":{\"replicas\":3}}"}
				}`, string(originalInfo))

				t.Run("GetOriginalInfoToRestore", func(t *testing.T) {
					patches, err := GetOriginalInfoToRestore(originalInfo)
					require.NoError(t, err)
					require.Equal(t, map[string]RestorePatches{
						"Deployment.apps": map[string]string{
							"d2":                   "{\"spec\":{\"replicas\":null}}",
							"deploy-with-replicas": "{\"spec\":{\"replicas\":3}}",
						},
					}, patches)
				})
			})

			t.Run("wake up", func(t *testing.T) {
				require.NoError(t, res.WakeUp(ctx))

				t.Run("Deployment", func(t *testing.T) {
					resList, err := res.resMapping[deployPatchData.Target].getListByNamespace(ctx, namespace, deployPatchData.Target)
					require.NoError(t, err)

					require.Len(t, resList, 2)
					requireEqualResources(t, originalDeployments, resList)
				})

				t.Run("sleep", func(t *testing.T) {
					require.NoError(t, res.Sleep(ctx))

					t.Run("Deployment", func(t *testing.T) {
						resList, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
						require.NoError(t, err)

						require.Len(t, resList, 2)
						require.Equal(t, int64(0), findResByName(resList, "deploy-with-replicas").Object["spec"].(map[string]interface{})["replicas"].(int64))
						require.Equal(t, int64(0), findResByName(resList, "d2").Object["spec"].(map[string]interface{})["replicas"].(int64))
					})
				})
			})
		})
	})

	t.Run("resources changed between sleep and wake up", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: v1.TypeMeta{
				Kind: "SleepInfo",
			},
			ObjectMeta: v1.ObjectMeta{
				Namespace: namespace,
				Name:      "test-sleepinfo",
			},
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
				},
			},
		}

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().
				WithRuntimeObjects(
					deployWithReplicas.DeepCopy(),
					deployWithoutReplicas.DeepCopy(),
				).
				Build(),
		}

		ctx := context.Background()
		res := getNewResource(t, fakeClient, sleepInfo, namespace)

		deployRes := res.resMapping[deployPatchData.Target]

		originalDeployments, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
		require.NoError(t, err)

		t.Run("sleep", func(t *testing.T) {
			require.NoError(t, res.Sleep(ctx))
			resList, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
			require.NoError(t, err)

			t.Run("Deployment", func(t *testing.T) {
				require.Len(t, resList, 2)
				require.Equal(t, int64(0), findResByName(resList, "deploy-with-replicas").Object["spec"].(map[string]interface{})["replicas"].(int64))
				require.Equal(t, int64(0), findResByName(resList, "d2").Object["spec"].(map[string]interface{})["replicas"].(int64))
			})

			deployClient := deployRes.Client

			// change replicas to 1
			resList, err = deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
			require.NoError(t, err)
			sleepDeployWithReplicas := findResByName(resList, "deploy-with-replicas")
			newDeployWithReplicas := sleepDeployWithReplicas.DeepCopy()
			require.NoError(t, unstructured.SetNestedField(newDeployWithReplicas.Object, int64(1), "spec", "replicas"))

			err = deployClient.Patch(ctx, newDeployWithReplicas, client.MergeFrom(sleepDeployWithReplicas))
			require.NoError(t, err)

			t.Run("wake up", func(t *testing.T) {
				require.NoError(t, res.WakeUp(ctx))

				t.Run("Deployment", func(t *testing.T) {
					resList, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
					require.NoError(t, err)

					require.Len(t, resList, 2)

					unstructuredCronJob, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&newDeployWithReplicas)
					require.NoError(t, err)
					expectedDeployments := []unstructured.Unstructured{
						{Object: unstructuredCronJob},
						*findResByName(originalDeployments, "d2"),
					}

					requireEqualResources(t, expectedDeployments, resList)
				})
			})
		})
	})

	t.Run("add a new resource between sleep and wake up", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: v1.TypeMeta{
				Kind: "SleepInfo",
			},
			ObjectMeta: v1.ObjectMeta{
				Namespace: namespace,
				Name:      "test-sleepinfo",
			},
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
				},
			},
		}

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().
				WithRuntimeObjects(
					deployWithReplicas.DeepCopy(),
					deployWithoutReplicas.DeepCopy(),
				).
				Build(),
		}

		ctx := context.Background()
		res := getNewResource(t, fakeClient, sleepInfo, namespace)

		deployRes := res.resMapping[deployPatchData.Target]

		originalDeployments, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
		require.NoError(t, err)

		t.Run("sleep", func(t *testing.T) {
			require.NoError(t, res.Sleep(ctx))
			resList, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
			require.NoError(t, err)

			t.Run("Deployment", func(t *testing.T) {
				require.Len(t, resList, 2)
				require.Equal(t, int64(0), findResByName(resList, "deploy-with-replicas").Object["spec"].(map[string]interface{})["replicas"].(int64))
				require.Equal(t, int64(0), findResByName(resList, "d2").Object["spec"].(map[string]interface{})["replicas"].(int64))
			})

			// add a new deployment
			deployClient := deployRes.Client

			deployToAdd := mocks.Deployment(mocks.DeploymentOptions{
				Name:      "new-deploy",
				Namespace: namespace,
				Replicas:  getPtr(int32(2)),
			}).Resource()
			require.NoError(t, deployClient.Create(ctx, deployToAdd))

			t.Run("wake up", func(t *testing.T) {
				require.NoError(t, res.WakeUp(ctx))

				t.Run("Deployment", func(t *testing.T) {
					resList, err := deployRes.getListByNamespace(ctx, namespace, deployPatchData.Target)
					require.NoError(t, err)

					require.Len(t, resList, 3)

					unstructuredCronJob, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&deployToAdd)
					require.NoError(t, err)
					expectedDeployments := []unstructured.Unstructured{
						originalDeployments[0],
						originalDeployments[1],
						{Object: unstructuredCronJob},
					}

					requireEqualResources(t, expectedDeployments, resList)
				})
			})
		})
	})

	t.Run("full lifecycle - no resources set to patch", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: v1.TypeMeta{
				Kind: "SleepInfo",
			},
			ObjectMeta: v1.ObjectMeta{
				Namespace: namespace,
				Name:      "test-sleepinfo",
			},
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{},
			},
		}

		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().
				Build(),
		}

		ctx := context.Background()
		res := getNewResource(t, fakeClient, sleepInfo, namespace)

		t.Run("check that there are resources", func(t *testing.T) {
			require.False(t, res.HasResource())
		})

		t.Run("sleep", func(t *testing.T) {
			require.NoError(t, res.Sleep(ctx))

			t.Run("GetOriginalInfoToSave", func(t *testing.T) {
				originalInfo, err := res.GetOriginalInfoToSave()
				require.NoError(t, err)
				fmt.Printf("originalInfo: %s\n", originalInfo)
				require.Nil(t, originalInfo)

				t.Run("GetOriginalInfoToRestore", func(t *testing.T) {
					patches, err := GetOriginalInfoToRestore(originalInfo)
					require.NoError(t, err)
					require.Nil(t, patches)
				})
			})
		})
	})

	t.Run("throws if patch is invalid", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: v1.TypeMeta{
				Kind: "SleepInfo",
			},
			ObjectMeta: v1.ObjectMeta{
				Namespace: namespace,
				Name:      "test-sleepinfo",
			},
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					{
						Target: v1alpha1.PatchTarget{
							Group: "apps",
							Kind:  "Deployment",
						},
						Patch: `
- op: wrong
  path: /spec/replicas
  value: 0
`,
					},
				},
			},
		}

		fakeClient := getFakeClient().WithRuntimeObjects(deployWithReplicas.DeepCopy()).Build()
		m := getNewResource(t, fakeClient, sleepInfo, namespace)
		require.EqualError(t, m.Sleep(context.Background()), `jsonpatch error: invalid operation {"op":"wrong","path":"/spec/replicas","value":0}: unsupported operation`)
	})

	t.Run("not throws if resource group not in cluster", func(t *testing.T) {
		nullogger := &bytes.Buffer{}
		testLogger := zap.New(zap.WriteTo(nullogger))
		sleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: v1.TypeMeta{
				Kind: "SleepInfo",
			},
			ObjectMeta: v1.ObjectMeta{
				Namespace: namespace,
				Name:      "test-sleepinfo",
			},
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					{
						Target: v1alpha1.PatchTarget{
							Group: "not-existing-group",
							Kind:  "something",
						},
						Patch: `[]`,
					},
				},
			},
		}
		res, err := NewResources(context.Background(), resource.ResourceClient{
			Client:    getFakeClient().Build(),
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, namespace, nil)
		require.NoError(t, err)
		require.False(t, res.HasResource())
	})

	t.Run("throws if patch not exists", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: v1.TypeMeta{
				Kind: "SleepInfo",
			},
			ObjectMeta: v1.ObjectMeta{
				Namespace: namespace,
				Name:      "test-sleepinfo",
			},
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					{
						Target: v1alpha1.PatchTarget{
							Group: "apps",
							Kind:  "Deployment",
						},
					},
				},
			},
		}
		fakeClient := getFakeClient().WithRuntimeObjects(deployWithReplicas.DeepCopy()).Build()
		res := getNewResource(t, fakeClient, sleepInfo, namespace)
		err := res.Sleep(context.Background())
		require.EqualError(t, err, fmt.Sprintf(`%s: invalid empty patch`, ErrJSONPatch))
	})

	t.Run("no resources in cluster", func(t *testing.T) {
		sleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: v1.TypeMeta{
				Kind: "SleepInfo",
			},
			ObjectMeta: v1.ObjectMeta{
				Namespace: namespace,
				Name:      "test-sleepinfo",
			},
			Spec: v1alpha1.SleepInfoSpec{
				Patches: []v1alpha1.Patch{
					deployPatchData,
				},
			},
		}
		fakeClient := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: getFakeClient().Build(),
		}
		res := getNewResource(t, fakeClient, sleepInfo, namespace)
		require.False(t, res.HasResource())
	})
}

func getFakeClient() *fake.ClientBuilder {
	groupVersion := []schema.GroupVersion{
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
		{Group: "keda.sh", Version: "v1alpha1"},
	}
	restMapper := meta.NewDefaultRESTMapper(groupVersion)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}, meta.RESTScopeNamespace)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "ReplicaSet",
	}, meta.RESTScopeNamespace)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "batch",
		Version: "v1",
		Kind:    "CronJob",
	}, meta.RESTScopeNamespace)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
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

func getNewResource(t *testing.T, client client.Client, sleepInfo *v1alpha1.SleepInfo, namespace string) managedResources {
	return getNewResourceWithPatchToRestore(t, client, sleepInfo, namespace, nil)
}

func getNewResourceWithPatchToRestore(t *testing.T, client client.Client, sleepInfo *v1alpha1.SleepInfo, namespace string, patchToRestore map[string]RestorePatches) managedResources {
	nullogger := &bytes.Buffer{}
	testLogger := zap.New(zap.WriteTo(nullogger))
	t.Helper()

	resource, err := NewResources(context.Background(), resource.ResourceClient{
		Client:    client,
		Log:       testLogger,
		SleepInfo: sleepInfo,
	}, namespace, patchToRestore)
	require.NoError(t, err)

	generic, ok := resource.(managedResources)
	require.True(t, ok)
	return generic
}
