package sleepinfo

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/sleepinfo/cronjobs"
	"github.com/kube-green/kube-green/controllers/sleepinfo/deployments"
	"github.com/kube-green/kube-green/controllers/sleepinfo/internal/mocks"
	"github.com/kube-green/kube-green/controllers/sleepinfo/jsonpatch"
	"github.com/kube-green/kube-green/controllers/sleepinfo/resource"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestNewResources(t *testing.T) {
	namespace := "my-namespace"
	replica1 := int32(1)

	cronJob := cronjobs.GetMock(cronjobs.MockSpec{
		Name:      "cronjob",
		Namespace: namespace,
	})
	deployments := deployments.GetMock(deployments.MockSpec{
		Name:      "deploy",
		Replicas:  &replica1,
		Namespace: namespace,
	})
	statefulSet := mocks.StatefulSet(mocks.StatefulSetOptions{
		Name:      "statefulset",
		Replicas:  &replica1,
		Namespace: namespace,
	}).Resource()
	statefulSetPatch := v1alpha1.Patch{
		Target: v1alpha1.PatchTarget{
			Kind:  "StatefulSet",
			Group: "apps",
		},
		Patch: `[{"op": "add", "path": "/spec/replicas", "value": 0}]`,
	}

	t.Run("errors if client is not valid", func(t *testing.T) {
		resClient := resource.ResourceClient{}
		res, err := NewResources(context.Background(), resClient, namespace, SleepInfoData{})
		require.True(t, strings.HasPrefix(err.Error(), "invalid client"))
		require.Empty(t, res)
	})

	t.Run("retrieve deployments data", func(t *testing.T) {
		resClient := resource.ResourceClient{
			Client:    mocks.FakeClient().WithRuntimeObjects(&cronJob, &deployments).Build(),
			Log:       zap.New(zap.UseDevMode(true)),
			SleepInfo: &v1alpha1.SleepInfo{},
		}
		res, err := NewResources(context.Background(), resClient, namespace, SleepInfoData{})
		require.NoError(t, err)
		require.True(t, res.deployments.HasResource())
		require.False(t, res.cronjobs.HasResource())
		require.False(t, res.genericResources.HasResource())
	})

	t.Run("retrieve deployments and cron jobs data", func(t *testing.T) {
		resClient := resource.ResourceClient{
			Client: mocks.FakeClient().WithRuntimeObjects(&cronJob, &deployments).Build(),
			Log:    zap.New(zap.UseDevMode(true)),
			SleepInfo: &v1alpha1.SleepInfo{
				Spec: v1alpha1.SleepInfoSpec{
					SuspendCronjobs: true,
				},
			},
		}
		res, err := NewResources(context.Background(), resClient, namespace, SleepInfoData{})
		require.NoError(t, err)
		require.True(t, res.deployments.HasResource())
		require.True(t, res.cronjobs.HasResource())
		require.False(t, res.genericResources.HasResource())
	})

	t.Run("retrieve generic data", func(t *testing.T) {
		resClient := resource.ResourceClient{
			Client: mocks.FakeClient().WithRuntimeObjects(statefulSet).Build(),
			Log:    zap.New(zap.UseDevMode(true)),
			SleepInfo: &v1alpha1.SleepInfo{
				Spec: v1alpha1.SleepInfoSpec{
					Patches: []v1alpha1.Patch{
						statefulSetPatch,
					},
				},
			},
		}
		res, err := NewResources(context.Background(), resClient, namespace, SleepInfoData{})
		require.NoError(t, err)
		require.False(t, res.deployments.HasResource())
		require.False(t, res.cronjobs.HasResource())
		require.True(t, res.genericResources.HasResource())

		t.Run("HasResources returns true", func(t *testing.T) {
			require.True(t, res.hasResources())
		})
	})

	t.Run("throws if fetch deployments fails", func(t *testing.T) {
		resClient := resource.ResourceClient{
			Client: testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client: mocks.FakeClient().WithRuntimeObjects(&cronJob, &deployments).Build(),
				ShouldError: func(method testutil.Method, obj runtime.Object) bool {
					_, ok := obj.(*appsv1.DeploymentList)
					return method == testutil.List && ok
				},
			},
			Log:       zap.New(zap.UseDevMode(true)),
			SleepInfo: &v1alpha1.SleepInfo{},
		}
		res, err := NewResources(context.Background(), resClient, namespace, SleepInfoData{})
		require.EqualError(t, err, "error during list")
		require.Empty(t, res)
	})

	t.Run("throws if fetch cron job fails", func(t *testing.T) {
		resClient := resource.ResourceClient{
			Client: testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client: mocks.FakeClient().WithRuntimeObjects(&cronJob, &deployments).Build(),
				ShouldError: func(method testutil.Method, obj runtime.Object) bool {
					kind := obj.GetObjectKind().GroupVersionKind().Kind
					return method == testutil.List && kind == "CronJob"
				},
			},
			Log: zap.New(zap.UseDevMode(true)),
			SleepInfo: &v1alpha1.SleepInfo{
				Spec: v1alpha1.SleepInfoSpec{
					SuspendCronjobs: true,
				},
			},
		}
		res, err := NewResources(context.Background(), resClient, namespace, SleepInfoData{})
		require.EqualError(t, err, fmt.Sprintf("%s: error during list", cronjobs.ErrFetchingCronJobs))
		require.Empty(t, res)
	})

	t.Run("throws if fetch jsonpatch fails", func(t *testing.T) {
		resClient := resource.ResourceClient{
			Client: testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client: mocks.FakeClient().Build(),
				ShouldError: func(method testutil.Method, obj runtime.Object) bool {
					return obj.GetObjectKind().GroupVersionKind().Kind == "StatefulSet"
				},
			},
			Log: zap.New(zap.UseDevMode(true)),
			SleepInfo: &v1alpha1.SleepInfo{
				Spec: v1alpha1.SleepInfoSpec{
					Patches: []v1alpha1.Patch{
						statefulSetPatch,
					},
				},
			},
		}
		res, err := NewResources(context.Background(), resClient, namespace, SleepInfoData{})
		require.EqualError(t, err, fmt.Sprintf("%s: error during list", jsonpatch.ErrListResources))
		require.Empty(t, res)
	})
}

func TestHasResources(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	namespace := "my-namespace"
	tests := []struct {
		name                     string
		deploy                   bool
		cronJob                  bool
		genericResources         bool
		expectToPerformOperation bool
	}{
		{
			name:                     "empty resources",
			expectToPerformOperation: false,
		},
		{
			name:                     "some deployments",
			deploy:                   true,
			expectToPerformOperation: true,
		},
		{
			name:                     "some cronjobs",
			cronJob:                  true,
			expectToPerformOperation: true,
		},
		{
			name:                     "cronjobs and deployments",
			cronJob:                  true,
			deploy:                   true,
			expectToPerformOperation: true,
		},
		{
			name:                     "generic resources",
			cronJob:                  false,
			deploy:                   false,
			genericResources:         true,
			expectToPerformOperation: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resources, err := NewResources(context.Background(), resource.ResourceClient{
				Log:       testLogger,
				Client:    mocks.FakeClient().Build(),
				SleepInfo: &v1alpha1.SleepInfo{},
			}, namespace, SleepInfoData{})
			require.NoError(t, err)

			resources.deployments = resource.GetResourceMock(resource.Mock{
				HasResourceResponseMock: test.deploy,
			})

			resources.cronjobs = resource.GetResourceMock(resource.Mock{
				HasResourceResponseMock: test.cronJob,
			})

			resources.genericResources = resource.GetResourceMock(resource.Mock{
				HasResourceResponseMock: test.genericResources,
			})

			require.Equal(t, test.expectToPerformOperation, resources.hasResources())
		})
	}
}

func TestResourcesSleep(t *testing.T) {
	t.Run("correctly sleep all resources", func(t *testing.T) {
		numberOfCalledDeploymentSleep := 0
		numberOfCalledCronJobSleep := 0
		numberOfCalledGenericResourceSleep := 0
		cronJobMock := resource.Mock{
			MockSleep: func(ctx context.Context) error {
				numberOfCalledCronJobSleep++
				return nil
			},
		}
		deploymentMock := resource.Mock{
			MockSleep: func(ctx context.Context) error {
				numberOfCalledDeploymentSleep++
				return nil
			},
		}
		genericResourceMock := resource.Mock{
			MockSleep: func(ctx context.Context) error {
				numberOfCalledGenericResourceSleep++
				return nil
			},
		}

		r := newResourcesMock(t, resourcesMocks{
			deployments:      deploymentMock,
			cronjobs:         cronJobMock,
			genericResources: genericResourceMock,
		})
		err := r.sleep(context.Background())
		require.NoError(t, err)

		require.Equal(t, 1, numberOfCalledDeploymentSleep, "calls deployments sleep")
		require.Equal(t, 1, numberOfCalledCronJobSleep, "calls cron job sleep")
		require.Equal(t, 1, numberOfCalledGenericResourceSleep, "calls generic resources sleep")
	})

	t.Run("throws if deployment sleep fails", func(t *testing.T) {
		deploymentMock := resource.Mock{
			MockSleep: func(ctx context.Context) error {
				return fmt.Errorf("some error")
			},
		}
		cronJobMock := resource.Mock{}

		r := newResourcesMock(t, resourcesMocks{
			deployments: deploymentMock,
			cronjobs:    cronJobMock,
		})
		err := r.sleep(context.Background())
		require.EqualError(t, err, "some error")
	})

	t.Run("throws if cron job sleep fails", func(t *testing.T) {
		deploymentMock := resource.Mock{}
		cronJobMock := resource.Mock{
			MockSleep: func(ctx context.Context) error {
				return fmt.Errorf("some error")
			},
		}

		r := newResourcesMock(t, resourcesMocks{
			deployments: deploymentMock,
			cronjobs:    cronJobMock,
		})
		err := r.sleep(context.Background())
		require.EqualError(t, err, "some error")
	})

	t.Run("throws if generic resources sleep fails", func(t *testing.T) {
		deploymentMock := resource.Mock{}
		cronJobMock := resource.Mock{}
		genericResourceMock := resource.Mock{
			MockSleep: func(ctx context.Context) error {
				return fmt.Errorf("some error")
			},
		}

		r := newResourcesMock(t, resourcesMocks{
			deployments:      deploymentMock,
			cronjobs:         cronJobMock,
			genericResources: genericResourceMock,
		})
		err := r.sleep(context.Background())
		require.EqualError(t, err, "some error")
	})
}

func TestResourcesWakeUp(t *testing.T) {
	t.Run("correctly wake up all resources", func(t *testing.T) {
		numberOfCalledDeploymentWakeUp := 0
		numberOfCalledCronJobWakeUp := 0
		numberOfCalledGenericResourceWakeUp := 0
		cronJobMock := resource.Mock{
			MockWakeUp: func(ctx context.Context) error {
				numberOfCalledCronJobWakeUp++
				return nil
			},
		}
		deploymentMock := resource.Mock{
			MockWakeUp: func(ctx context.Context) error {
				numberOfCalledDeploymentWakeUp++
				return nil
			},
		}
		genericResourceMock := resource.Mock{
			MockWakeUp: func(ctx context.Context) error {
				numberOfCalledGenericResourceWakeUp++
				return nil
			},
		}

		r := newResourcesMock(t, resourcesMocks{
			deployments:      deploymentMock,
			cronjobs:         cronJobMock,
			genericResources: genericResourceMock,
		})
		err := r.wakeUp(context.Background())
		require.NoError(t, err)

		require.Equal(t, 1, numberOfCalledDeploymentWakeUp, "calls deployments wake up")
		require.Equal(t, 1, numberOfCalledCronJobWakeUp, "calls cron job wake up")
		require.Equal(t, 1, numberOfCalledGenericResourceWakeUp, "calls generic resources wake up")
	})

	t.Run("throws if deployment wake up fails", func(t *testing.T) {
		deploymentMock := resource.Mock{
			MockWakeUp: func(ctx context.Context) error {
				return fmt.Errorf("some error")
			},
		}
		cronJobMock := resource.Mock{}

		r := newResourcesMock(t, resourcesMocks{
			deployments: deploymentMock,
			cronjobs:    cronJobMock,
		})
		err := r.wakeUp(context.Background())
		require.EqualError(t, err, "some error")
	})

	t.Run("throws if cron job wake up fails", func(t *testing.T) {
		deploymentMock := resource.Mock{}
		cronJobMock := resource.Mock{
			MockWakeUp: func(ctx context.Context) error {
				return fmt.Errorf("some error")
			},
		}

		r := newResourcesMock(t, resourcesMocks{
			deployments: deploymentMock,
			cronjobs:    cronJobMock,
		})
		err := r.wakeUp(context.Background())
		require.EqualError(t, err, "some error")
	})

	t.Run("throws if generic resources wake up fails", func(t *testing.T) {
		deploymentMock := resource.Mock{}
		cronJobMock := resource.Mock{}
		genericResourceMock := resource.Mock{
			MockWakeUp: func(ctx context.Context) error {
				return fmt.Errorf("some error")
			},
		}

		r := newResourcesMock(t, resourcesMocks{
			deployments:      deploymentMock,
			cronjobs:         cronJobMock,
			genericResources: genericResourceMock,
		})
		err := r.wakeUp(context.Background())
		require.EqualError(t, err, "some error")
	})
}

func TestGetOriginalResourceInfoToSave(t *testing.T) {
	t.Run("correctly get original resources", func(t *testing.T) {
		numberOfCalledDeploymentInfoToSave := 0
		numberOfCalledCronJobInfoToSave := 0
		numberOfCalledGenericResourceInfoToSave := 0
		deploymentMock := resource.Mock{
			MockOriginalInfoToSave: func() ([]byte, error) {
				numberOfCalledDeploymentInfoToSave++
				return nil, nil
			},
		}
		cronJobMock := resource.Mock{
			MockOriginalInfoToSave: func() ([]byte, error) {
				numberOfCalledCronJobInfoToSave++
				return []byte("[]"), nil
			},
		}
		genericResourcesMock := resource.Mock{
			MockOriginalInfoToSave: func() ([]byte, error) {
				numberOfCalledGenericResourceInfoToSave++
				return []byte("[]"), nil
			},
		}

		r := newResourcesMock(t, resourcesMocks{
			deployments:      deploymentMock,
			cronjobs:         cronJobMock,
			genericResources: genericResourcesMock,
		})
		data, err := r.getOriginalResourceInfoToSave()
		require.NoError(t, err)
		require.Equal(t, map[string][]byte{
			originalCronjobStatusKey: []byte("[]"),
			originalJSONPatchDataKey: []byte("[]"),
		}, data)

		require.Equal(t, 1, numberOfCalledDeploymentInfoToSave, "calls deployments wake up")
		require.Equal(t, 1, numberOfCalledCronJobInfoToSave, "calls cron job wake up")
		require.Equal(t, 1, numberOfCalledGenericResourceInfoToSave, "calls generic resource wake up")
	})

	t.Run("correctly get original resources only for deployments", func(t *testing.T) {
		numberOfCalledDeploymentInfoToSave := 0
		numberOfCalledCronJobInfoToSave := 0
		cronJobMock := resource.Mock{
			MockOriginalInfoToSave: func() ([]byte, error) {
				numberOfCalledCronJobInfoToSave++
				return nil, nil
			},
		}
		deploymentMock := resource.Mock{
			MockOriginalInfoToSave: func() ([]byte, error) {
				numberOfCalledDeploymentInfoToSave++
				return []byte("[]"), nil
			},
		}

		r := newResourcesMock(t, resourcesMocks{
			deployments: deploymentMock,
			cronjobs:    cronJobMock,
		})
		data, err := r.getOriginalResourceInfoToSave()
		require.NoError(t, err)
		require.Equal(t, map[string][]byte{
			replicasBeforeSleepKey: []byte("[]"),
		}, data)

		require.Equal(t, 1, numberOfCalledDeploymentInfoToSave, "calls deployments wake up")
		require.Equal(t, 1, numberOfCalledCronJobInfoToSave, "calls cron job wake up")
	})

	t.Run("throws if deployment sleep fails", func(t *testing.T) {
		deploymentMock := resource.Mock{
			MockOriginalInfoToSave: func() ([]byte, error) {
				return nil, fmt.Errorf("some error")
			},
		}
		cronJobMock := resource.Mock{}

		r := newResourcesMock(t, resourcesMocks{
			deployments: deploymentMock,
			cronjobs:    cronJobMock,
		})
		_, err := r.getOriginalResourceInfoToSave()
		require.EqualError(t, err, "some error")
	})

	t.Run("throws if cron job sleep fails", func(t *testing.T) {
		deploymentMock := resource.Mock{}
		cronJobMock := resource.Mock{
			MockOriginalInfoToSave: func() ([]byte, error) {
				return nil, fmt.Errorf("some error")
			},
		}

		r := newResourcesMock(t, resourcesMocks{
			deployments: deploymentMock,
			cronjobs:    cronJobMock,
		})
		_, err := r.getOriginalResourceInfoToSave()
		require.EqualError(t, err, "some error")
	})

	t.Run("throws if generic resources sleep fails", func(t *testing.T) {
		deploymentMock := resource.Mock{}
		cronJobMock := resource.Mock{}
		genericResourceMock := resource.Mock{
			MockOriginalInfoToSave: func() ([]byte, error) {
				return nil, fmt.Errorf("some error")
			},
		}

		r := newResourcesMock(t, resourcesMocks{
			deployments:      deploymentMock,
			cronjobs:         cronJobMock,
			genericResources: genericResourceMock,
		})
		_, err := r.getOriginalResourceInfoToSave()
		require.EqualError(t, err, "some error")
	})
}

func TestSetOriginalResourceInfoToRestoreInSleepInfo(t *testing.T) {
	t.Run("empty sleepInfoData if data is nil", func(t *testing.T) {
		sleepInfoData := SleepInfoData{}
		err := setOriginalResourceInfoToRestoreInSleepInfo(nil, &sleepInfoData)
		require.Equal(t, sleepInfoData, sleepInfoData)
		require.NoError(t, err)
	})

	t.Run("empty sleepInfoData if data is empty", func(t *testing.T) {
		sleepInfoData := SleepInfoData{}
		err := setOriginalResourceInfoToRestoreInSleepInfo(map[string][]byte{}, &sleepInfoData)
		require.Equal(t, sleepInfoData, sleepInfoData)
		require.NoError(t, err)
	})

	t.Run("deployment throws if data is not a correct json", func(t *testing.T) {
		sleepInfoData := SleepInfoData{}
		data := map[string][]byte{
			replicasBeforeSleepKey: []byte("{}"),
		}
		err := setOriginalResourceInfoToRestoreInSleepInfo(data, &sleepInfoData)
		require.EqualError(t, err, "json: cannot unmarshal object into Go value of type []deployments.OriginalReplicas")
	})

	t.Run("cronjob throws if data is not a correct json", func(t *testing.T) {
		sleepInfoData := SleepInfoData{}
		data := map[string][]byte{
			originalCronjobStatusKey: []byte("{}"),
		}
		err := setOriginalResourceInfoToRestoreInSleepInfo(data, &sleepInfoData)
		require.EqualError(t, err, "json: cannot unmarshal object into Go value of type []cronjobs.OriginalCronJobStatus")
	})

	t.Run("generic resource throws if data is not a correct json", func(t *testing.T) {
		sleepInfoData := SleepInfoData{}
		data := map[string][]byte{
			originalJSONPatchDataKey: []byte("[]"),
		}
		err := setOriginalResourceInfoToRestoreInSleepInfo(data, &sleepInfoData)
		require.EqualError(t, err, "json: cannot unmarshal array into Go value of type map[string]jsonpatch.RestorePatches")
	})

	t.Run("correctly set sleep info data for deployments and cronjobs", func(t *testing.T) {
		sleepInfoData := SleepInfoData{}
		data := map[string][]byte{
			originalCronjobStatusKey: []byte(`[{"name":"cj1","suspend":true}]`),
			replicasBeforeSleepKey:   []byte(`[{"name":"deploy1","replicas":5}]`),
			originalJSONPatchDataKey: []byte(`{"apps-Deployment": {"d2":"{\"spec\":{\"replicas\":null}}","deploy-with-replicas":"{\"spec\":{\"replicas\":3}}"}}`),
		}
		err := setOriginalResourceInfoToRestoreInSleepInfo(data, &sleepInfoData)
		require.NoError(t, err)
		require.Equal(t, SleepInfoData{
			OriginalCronJobStatus:       map[string]bool{"cj1": true},
			OriginalDeploymentsReplicas: map[string]int32{"deploy1": 5},
			OriginalGenericResourceInfo: map[string]jsonpatch.RestorePatches{
				"apps-Deployment": map[string]string{
					"d2":                   "{\"spec\":{\"replicas\":null}}",
					"deploy-with-replicas": "{\"spec\":{\"replicas\":3}}",
				},
			},
		}, sleepInfoData)
	})
}

type resourcesMocks struct {
	deployments      resource.Mock
	cronjobs         resource.Mock
	genericResources resource.Mock
}

func newResourcesMock(t *testing.T, mocks resourcesMocks) Resources {
	t.Helper()
	return Resources{
		deployments:      resource.GetResourceMock(mocks.deployments),
		cronjobs:         resource.GetResourceMock(mocks.cronjobs),
		genericResources: resource.GetResourceMock(mocks.genericResources),
	}
}
