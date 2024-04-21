package sleepinfo

import (
	"context"
	"fmt"
	"testing"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/internal/mocks"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/jsonpatch"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestGetSecret(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))
	secretName := "secret-name"
	namespace := "my-namespace"

	t.Run("get secret correctly", func(t *testing.T) {
		client := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: fake.
				NewClientBuilder().
				WithRuntimeObjects(getSecret(mockSecretSpec{
					namespace:       namespace,
					name:            secretName,
					resourceVersion: "11",
					data: map[string][]byte{
						"foo": []byte("bar"),
					},
				})).
				Build(),
		}
		r := SleepInfoReconciler{
			Client:     client,
			Log:        testLogger,
			SleepDelta: 60,
		}

		secret, err := r.getSecret(context.Background(), secretName, namespace)
		require.NoError(t, err)
		require.Equal(t, &v1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            secretName,
				Namespace:       namespace,
				ResourceVersion: "11",
			},
			Data: map[string][]byte{
				"foo": []byte("bar"),
			},
		}, secret)
	})

	t.Run("secret not found", func(t *testing.T) {
		client := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: fake.
				NewClientBuilder().
				Build(),
		}
		r := SleepInfoReconciler{
			Client:     client,
			Log:        testLogger,
			SleepDelta: 60,
		}

		secret, err := r.getSecret(context.Background(), secretName, namespace)
		require.EqualError(t, err, fmt.Sprintf("secrets \"%s\" not found", secretName))
		require.Nil(t, secret)
	})
}

func TestUpsertSecrets(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	now := time.Now()
	secretName := "secret-name"
	namespace := "my-namespace"
	var replicas1 int32 = 1
	var replicas4 int32 = 4
	var replicas0 int32 = 0

	d1 := mocks.Deployment(mocks.DeploymentOptions{
		Name:      "deployment1",
		Namespace: namespace,
		Replicas:  &replicas1,
	}).Unstructured()
	d2 := mocks.Deployment(mocks.DeploymentOptions{
		Name:      "deployment2",
		Namespace: namespace,
		Replicas:  &replicas4,
	}).Unstructured()
	d3 := mocks.Deployment(mocks.DeploymentOptions{
		Name:      "deployment3",
		Namespace: namespace,
		Replicas:  &replicas0,
	}).Unstructured()
	sleepInfo := &kubegreenv1alpha1.SleepInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sleepinfo-name",
			UID:  "sleepinfo-uid",
		},
	}
	ownerRefs := []metav1.OwnerReference{
		{
			APIVersion: "kube-green.com/v1alpha1",
			Kind:       "SleepInfo",
			Name:       sleepInfo.Name,
			UID:        sleepInfo.UID,
		},
	}

	t.Run("insert and update secret - sleep and wake up", func(t *testing.T) {
		client := fakeDeploymentClient(&d1, &d2, &d3)
		r := SleepInfoReconciler{
			Client:     client,
			Log:        testLogger,
			SleepDelta: 60,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}
		resources, err := jsonpatch.NewResources(context.Background(), resource.ResourceClient{
			Client:    client,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, namespace, nil)
		require.NoError(t, err)

		err = resources.Sleep(context.Background())
		require.NoError(t, err)

		err = r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, sleepInfo, nil, sleepInfoData, resources)
		require.NoError(t, err)

		secret, err := r.getSecret(context.Background(), secretName, namespace)
		require.NoError(t, err)
		require.Equal(t, &v1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            secretName,
				Namespace:       namespace,
				ResourceVersion: "1",
				OwnerReferences: ownerRefs,
			},
			Data: map[string][]byte{
				lastOperationKey:         []byte(sleepOperation),
				lastScheduleKey:          []byte(now.Format(time.RFC3339)),
				originalJSONPatchDataKey: []byte(`{"Deployment.apps":{"deployment1":"{\"spec\":{\"replicas\":1}}","deployment2":"{\"spec\":{\"replicas\":4}}"}}`),
			},
		}, secret)

		t.Run("update existent secret - wake up", func(t *testing.T) {
			now := now.Add(10 * time.Minute)
			sleepInfoData := SleepInfoData{
				CurrentOperationType: wakeUpOperation,
				LastSchedule:         now,
			}
			err = resources.WakeUp(context.Background())
			require.NoError(t, err)

			err = r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, sleepInfo, secret, sleepInfoData, resources)
			require.NoError(t, err)

			secret, err := r.getSecret(context.Background(), secretName, namespace)
			require.NoError(t, err)
			require.Equal(t, &v1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            secretName,
					Namespace:       namespace,
					ResourceVersion: "2",
					OwnerReferences: ownerRefs,
				},
				Data: map[string][]byte{
					lastOperationKey: []byte(wakeUpOperation),
					lastScheduleKey:  []byte(now.Format(time.RFC3339)),
				},
			}, secret)
		})
	})

	t.Run("insert and update secret - only sleep", func(t *testing.T) {
		client := fakeDeploymentClient(&d1, &d2, &d3)

		r := SleepInfoReconciler{
			Client:     client,
			Log:        testLogger,
			SleepDelta: 60,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}
		resources, err := jsonpatch.NewResources(context.Background(), resource.ResourceClient{
			Client:    client,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, namespace, nil)
		require.NoError(t, err)

		err = resources.Sleep(context.Background())
		require.NoError(t, err)

		err = r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, sleepInfo, nil, sleepInfoData, resources)
		require.NoError(t, err)

		secret, err := r.getSecret(context.Background(), secretName, namespace)
		require.NoError(t, err)
		require.Equal(t, &v1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            secretName,
				Namespace:       namespace,
				ResourceVersion: "1",
				OwnerReferences: ownerRefs,
			},
			Data: map[string][]byte{
				lastOperationKey:         []byte(sleepOperation),
				lastScheduleKey:          []byte(now.Format(time.RFC3339)),
				originalJSONPatchDataKey: []byte(`{"Deployment.apps":{"deployment1":"{\"spec\":{\"replicas\":1}}","deployment2":"{\"spec\":{\"replicas\":4}}"}}`),
			},
		}, secret)

		t.Run("update existent secret - new deploy to sleep", func(t *testing.T) {
			now := now.Add(10 * time.Minute)
			sleepInfoData := SleepInfoData{
				CurrentOperationType: sleepOperation,
				LastSchedule:         now,
				OriginalGenericResourceInfo: map[string]jsonpatch.RestorePatches{
					"Deployment.apps": {
						"deployment1": "{\"spec\":{\"replicas\":1}}",
						"deployment2": "{\"spec\":{\"replicas\":4}}",
					},
				},
			}
			d4 := mocks.Deployment(mocks.DeploymentOptions{
				Namespace: namespace,
				Name:      "new-deployment",
				Replicas:  &replicas1,
			}).Unstructured()
			client := fakeDeploymentClient(&d1, &d2, &d3, &d4)

			resources, err := jsonpatch.NewResources(context.Background(), resource.ResourceClient{
				Client:    client,
				Log:       testLogger,
				SleepInfo: sleepInfo,
			}, namespace, sleepInfoData.OriginalGenericResourceInfo)
			require.NoError(t, err)

			err = resources.Sleep(context.Background())
			require.NoError(t, err)

			err = r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, sleepInfo, secret, sleepInfoData, resources)
			require.NoError(t, err)

			secret, err := r.getSecret(context.Background(), secretName, namespace)
			require.NoError(t, err)
			require.Equal(t, &v1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            secretName,
					Namespace:       namespace,
					ResourceVersion: "2",
					OwnerReferences: ownerRefs,
				},
				Data: map[string][]byte{
					lastOperationKey:         []byte(sleepOperation),
					lastScheduleKey:          []byte(now.Format(time.RFC3339)),
					originalJSONPatchDataKey: []byte(`{"Deployment.apps":{"deployment1":"{\"spec\":{\"replicas\":1}}","deployment2":"{\"spec\":{\"replicas\":4}}","new-deployment":"{\"spec\":{\"replicas\":1}}"}}`),
				},
			}, secret)
		})
	})

	t.Run("insert new secret - operation sleep 0 deployments", func(t *testing.T) {
		client := fakeDeploymentClient()
		r := SleepInfoReconciler{
			Client:     client,
			Log:        testLogger,
			SleepDelta: 60,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}
		resources, err := jsonpatch.NewResources(context.Background(), resource.ResourceClient{
			Client:    client,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, namespace, sleepInfoData.OriginalGenericResourceInfo)
		require.NoError(t, err)

		err = resources.Sleep(context.Background())
		require.NoError(t, err)

		err = r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, sleepInfo, nil, sleepInfoData, resources)
		require.NoError(t, err)

		secret, err := r.getSecret(context.Background(), secretName, namespace)
		require.NoError(t, err)
		require.Equal(t, &v1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            secretName,
				Namespace:       namespace,
				ResourceVersion: "1",
				OwnerReferences: ownerRefs,
			},
			Data: map[string][]byte{
				lastScheduleKey: []byte(now.Format(time.RFC3339)),
			},
		}, secret)
	})

	t.Run("update secret - operation sleep and 0 deployments", func(t *testing.T) {
		existentSecret := getSecret(mockSecretSpec{
			namespace:       namespace,
			name:            secretName,
			resourceVersion: "15",
			data: map[string][]byte{
				lastOperationKey: []byte(wakeUpOperation),
				lastScheduleKey:  []byte(now.Add(1 * time.Hour).Format(time.RFC3339)),
			},
		})
		client := fakeDeploymentClient(existentSecret)

		r := SleepInfoReconciler{
			Client:     client,
			Log:        testLogger,
			SleepDelta: 60,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}
		resources, err := jsonpatch.NewResources(context.Background(), resource.ResourceClient{
			Client:    client,
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, namespace, sleepInfoData.OriginalGenericResourceInfo)
		require.NoError(t, err)

		err = resources.Sleep(context.Background())
		require.NoError(t, err)

		err = r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, sleepInfo, existentSecret, sleepInfoData, resources)
		require.NoError(t, err)

		secret, err := r.getSecret(context.Background(), secretName, namespace)
		require.NoError(t, err)
		require.Equal(t, &v1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            secretName,
				Namespace:       namespace,
				ResourceVersion: "16",
				OwnerReferences: ownerRefs,
			},
			Data: map[string][]byte{
				lastScheduleKey: []byte(now.Format(time.RFC3339)),
			},
		}, secret)
	})

	t.Run("fails to create new secret", func(t *testing.T) {
		client := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: fake.
				NewClientBuilder().
				Build(),
			ShouldError: func(method testutil.Method, obj runtime.Object) bool {
				return method != testutil.List
			},
		}
		r := SleepInfoReconciler{
			Client:     client,
			Log:        testLogger,
			SleepDelta: 60,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}
		resources, err := jsonpatch.NewResources(context.Background(), resource.ResourceClient{
			Client:    fakeDeploymentClient(&d1, &d2, &d3),
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, namespace, sleepInfoData.OriginalGenericResourceInfo)
		require.NoError(t, err)

		err = resources.Sleep(context.Background())
		require.NoError(t, err)

		err = r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, sleepInfo, nil, sleepInfoData, resources)
		require.EqualError(t, err, "error during create")
	})

	t.Run("fails to update secret", func(t *testing.T) {
		existentSecret := getSecret(mockSecretSpec{
			namespace:       namespace,
			name:            secretName,
			resourceVersion: "15",
			data: map[string][]byte{
				lastOperationKey: []byte(wakeUpOperation),
				lastScheduleKey:  []byte(now.Add(1 * time.Hour).Format(time.RFC3339)),
			},
		})
		client := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: fake.
				NewClientBuilder().
				WithRuntimeObjects(existentSecret).
				Build(),
			ShouldError: func(method testutil.Method, obj runtime.Object) bool {
				return method != testutil.List
			},
		}
		r := SleepInfoReconciler{
			Client:     client,
			Log:        testLogger,
			SleepDelta: 60,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}
		resources, err := jsonpatch.NewResources(context.Background(), resource.ResourceClient{
			Client:    fakeDeploymentClient(&d1, &d2, &d3),
			Log:       testLogger,
			SleepInfo: sleepInfo,
		}, namespace, sleepInfoData.OriginalGenericResourceInfo)
		require.NoError(t, err)

		err = resources.Sleep(context.Background())
		require.NoError(t, err)

		err = r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, sleepInfo, existentSecret, sleepInfoData, resources)
		require.EqualError(t, err, "error during update")
	})
}

type mockSecretSpec struct {
	namespace       string
	name            string
	resourceVersion string
	data            map[string][]byte
}

func getSecret(opts mockSecretSpec) *v1.Secret {
	if opts.resourceVersion == "" {
		opts.resourceVersion = "1"
	}
	return &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            opts.name,
			Namespace:       opts.namespace,
			ResourceVersion: opts.resourceVersion,
		},
		Data: opts.data,
	}
}

func fakeDeploymentClient(runtimeObj ...runtime.Object) *testutil.PossiblyErroringFakeCtrlRuntimeClient {
	groupVersion := []schema.GroupVersion{
		{Group: "apps", Version: "v1"},
	}
	restMapper := meta.NewDefaultRESTMapper(groupVersion)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}, meta.RESTScopeNamespace)

	return &testutil.PossiblyErroringFakeCtrlRuntimeClient{
		Client: fake.
			NewClientBuilder().
			WithRESTMapper(restMapper).
			WithRuntimeObjects(runtimeObj...).
			Build(),
	}
}
