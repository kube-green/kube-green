package sleepinfo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/davidebianchi/kube-green/controllers/internal/testutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			Client: client,
			Log:    testLogger,
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
			Client: client,
			Log:    testLogger,
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

	deployList := []appsv1.Deployment{
		getDeploymentMock(mockDeploymentSpec{
			name:      "deployment1",
			namespace: namespace,
			replicas:  &replicas1,
		}),
		getDeploymentMock(mockDeploymentSpec{
			name:      "deployment2",
			namespace: namespace,
			replicas:  &replicas4,
		}),
		getDeploymentMock(mockDeploymentSpec{
			name:      "deployment3",
			namespace: namespace,
			replicas:  &replicas0,
		}),
	}

	t.Run("insert and update secret - sleep and wake up", func(t *testing.T) {
		client := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: fake.
				NewClientBuilder().
				Build(),
		}

		r := SleepInfoReconciler{
			Client: client,
			Log:    testLogger,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}

		err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, nil, sleepInfoData, deployList)
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
			},
			Data: map[string][]byte{
				lastOperationKey:       []byte(sleepOperation),
				lastScheduleKey:        []byte(now.Format(time.RFC3339)),
				replicasBeforeSleepKey: []byte(`[{"name":"deployment1","replicas":1},{"name":"deployment2","replicas":4}]`),
			},
		}, secret)

		t.Run("update existent secret - wake up", func(t *testing.T) {
			now := now.Add(10 * time.Minute)
			sleepInfoData := SleepInfoData{
				CurrentOperationType: wakeUpOperation,
				LastSchedule:         now,
			}
			err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, secret, sleepInfoData, deployList)
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
				},
				Data: map[string][]byte{
					lastOperationKey: []byte(wakeUpOperation),
					lastScheduleKey:  []byte(now.Format(time.RFC3339)),
				},
			}, secret)
		})
	})

	t.Run("insert and update secret - only wake up", func(t *testing.T) {
		client := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: fake.
				NewClientBuilder().
				Build(),
		}

		r := SleepInfoReconciler{
			Client: client,
			Log:    testLogger,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}

		err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, nil, sleepInfoData, deployList)
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
			},
			Data: map[string][]byte{
				lastOperationKey:       []byte(sleepOperation),
				lastScheduleKey:        []byte(now.Format(time.RFC3339)),
				replicasBeforeSleepKey: []byte(`[{"name":"deployment1","replicas":1},{"name":"deployment2","replicas":4}]`),
			},
		}, secret)

		t.Run("update existent secret - new deploy to sleep", func(t *testing.T) {
			now := now.Add(10 * time.Minute)
			sleepInfoData := SleepInfoData{
				CurrentOperationType: sleepOperation,
				LastSchedule:         now,
				OriginalDeploymentsReplicas: map[string]int32{
					"deployment1": 1,
					"deployment2": 4,
				},
			}
			updatedDeployList := append(deployList, getDeploymentMock(mockDeploymentSpec{
				namespace: namespace,
				name:      "new-deployment",
				replicas:  &replicas1,
			}))
			err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, secret, sleepInfoData, updatedDeployList)
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
				},
				Data: map[string][]byte{
					lastOperationKey:       []byte(sleepOperation),
					lastScheduleKey:        []byte(now.Format(time.RFC3339)),
					replicasBeforeSleepKey: []byte(`[{"name":"deployment1","replicas":1},{"name":"deployment2","replicas":4},{"name":"new-deployment","replicas":1}]`),
				},
			}, secret)
		})
	})

	t.Run("insert new secret - operation sleep 0 deployments", func(t *testing.T) {
		client := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: fake.
				NewClientBuilder().
				Build(),
		}
		r := SleepInfoReconciler{
			Client: client,
			Log:    testLogger,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}

		err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, nil, sleepInfoData, []appsv1.Deployment{})
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
		client := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: fake.
				NewClientBuilder().
				WithRuntimeObjects(existentSecret).
				Build(),
		}

		r := SleepInfoReconciler{
			Client: client,
			Log:    testLogger,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}

		err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, existentSecret, sleepInfoData, []appsv1.Deployment{})
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
			ShouldError: true,
		}
		r := SleepInfoReconciler{
			Client: client,
			Log:    testLogger,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}

		err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, nil, sleepInfoData, deployList)
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
			ShouldError: true,
		}
		r := SleepInfoReconciler{
			Client: client,
			Log:    testLogger,
		}
		sleepInfoData := SleepInfoData{
			CurrentOperationType: sleepOperation,
		}

		err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, existentSecret, sleepInfoData, deployList)
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
