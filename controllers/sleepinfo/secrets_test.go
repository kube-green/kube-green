package sleepinfo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/davidebianchi/kube-green/controllers/internal/testutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
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
	var boolFalse bool = false

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
	cronjobList := []batchv1.CronJob{
		getCronJobMock(mockCronJobSpec{
			name:            "cj1",
			namespace:       namespace,
			resourceVersion: "12",
			schedule:        "* * * * *",
		}),
		getCronJobMock(mockCronJobSpec{
			name:            "cj-suspended",
			namespace:       namespace,
			resourceVersion: "1",
			schedule:        "* * * * *",
			suspend:         &boolFalse,
		}),
	}

	t.Run("only sleep twice - deployment released from first to second run", func(t *testing.T) {
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
		resources := Resources{
			Deployments: deployList,
		}

		err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, nil, sleepInfoData, resources)
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

		t.Run("update existent secret - new sleep", func(t *testing.T) {
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
			resources := Resources{
				Deployments: updatedDeployList,
			}

			err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, secret, sleepInfoData, resources)
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

	t.Run("insert new secret - operation sleep 0 deployments and cronjobs", func(t *testing.T) {
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
			SuspendCronjobs:      true,
		}

		err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, nil, sleepInfoData, Resources{})
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

	t.Run("update secret - operation sleep and 0 deployments and cronjobs", func(t *testing.T) {
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
			SuspendCronjobs:      true,
		}

		err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, existentSecret, sleepInfoData, Resources{})
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
		resources := Resources{
			Deployments: deployList,
		}

		err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, nil, sleepInfoData, resources)
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
		resources := Resources{
			Deployments: deployList,
		}

		err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, existentSecret, sleepInfoData, resources)
		require.EqualError(t, err, "error during update")
	})

	sleepAndWakeupTests := []struct {
		name                        string
		resources                   Resources
		suspendCronjobs             bool
		expectedDeploymentsReplicas string
		expectedOriginalCronjobs    string
	}{
		{
			name: "save deployment info to secret - suspend cronjob disabled",
			resources: Resources{
				Deployments: deployList,
				CronJobs:    cronjobList,
			},
			expectedDeploymentsReplicas: `[{"name":"deployment1","replicas":1},{"name":"deployment2","replicas":4}]`,
		},
		{
			name: "save cronjobs and deployments info to secret",
			resources: Resources{
				Deployments: deployList,
				CronJobs:    cronjobList,
			},
			suspendCronjobs:             true,
			expectedDeploymentsReplicas: `[{"name":"deployment1","replicas":1},{"name":"deployment2","replicas":4}]`,
			expectedOriginalCronjobs:    `[{"name":"cj1","suspend":false}]`,
		},
		{
			name: "save only cronjobs info to secret",
			resources: Resources{
				CronJobs: cronjobList,
			},
			suspendCronjobs:             true,
			expectedDeploymentsReplicas: `[]`,
			expectedOriginalCronjobs:    `[{"name":"cj1","suspend":false}]`,
		},
		{
			name: "save deployments info to secret - suspend cronjob enabled",
			resources: Resources{
				Deployments: deployList,
			},
			suspendCronjobs:             true,
			expectedDeploymentsReplicas: `[{"name":"deployment1","replicas":1},{"name":"deployment2","replicas":4}]`,
			expectedOriginalCronjobs:    `[]`,
		},
	}
	for _, test := range sleepAndWakeupTests {
		t.Run(fmt.Sprintf("insert and update secret sleep + wakeup - %s", test.name), func(t *testing.T) {
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
				SuspendCronjobs:      test.suspendCronjobs,
			}

			err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, nil, sleepInfoData, test.resources)
			require.NoError(t, err)

			secret, err := r.getSecret(context.Background(), secretName, namespace)
			require.NoError(t, err)

			expectedData := map[string][]byte{
				lastOperationKey:       []byte(sleepOperation),
				lastScheduleKey:        []byte(now.Format(time.RFC3339)),
				replicasBeforeSleepKey: []byte(test.expectedDeploymentsReplicas),
			}
			if test.expectedOriginalCronjobs != "" {
				expectedData[suspendedCronJobBeforeSleepKey] = []byte(test.expectedOriginalCronjobs)
			}
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
				Data: expectedData,
			}, secret)

			t.Run("update existent secret - wake up", func(t *testing.T) {
				now := now.Add(10 * time.Minute)
				sleepInfoData := SleepInfoData{
					CurrentOperationType: wakeUpOperation,
					LastSchedule:         now,
				}
				err := r.upsertSecret(context.Background(), testLogger, now, secretName, namespace, secret, sleepInfoData, test.resources)
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
	}
}

func TestSleepInfoSecret(t *testing.T) {
	secretName := "secret-name"
	namespace := "my-namespace"
	savedSecretReplicas := []byte(`[{"name":"deployment1","replicas":1},{"name":"deployment2","replicas":4},{"name":"replicas-not-saved"},{}]`)
	savedSecretCronJobs := []byte(`[{"name":"cj1","suspend":false},{}]`)
	schedule := time.Now().Format(time.RFC3339)
	operation := sleepOperation
	s := sleepInfoSecret{
		Secret: getSecret(mockSecretSpec{
			namespace: namespace,
			name:      secretName,
			data: map[string][]byte{
				replicasBeforeSleepKey:         savedSecretReplicas,
				suspendedCronJobBeforeSleepKey: savedSecretCronJobs,
				lastScheduleKey:                []byte(schedule),
				lastOperationKey:               []byte(sleepOperation),
			},
		}),
	}

	t.Run("getOriginalDeploymentReplicas", func(t *testing.T) {
		originalDeploymentsReplicas, err := s.getOriginalDeploymentReplicas()
		require.NoError(t, err)
		require.Equal(t, map[string]int32{
			"deployment1":        1,
			"deployment2":        4,
			"replicas-not-saved": 0,
		}, originalDeploymentsReplicas)
	})

	t.Run("getOriginalCronJobSuspendedState", func(t *testing.T) {
		cronJobSuspendedState, err := s.getOriginalCronJobSuspendedState()
		require.NoError(t, err)
		require.Equal(t, map[string]bool{
			"cj1": false,
		}, cronJobSuspendedState)
	})

	t.Run("getLastSchedule", func(t *testing.T) {
		lastSchedule := s.getLastSchedule()

		require.Equal(t, schedule, lastSchedule)
	})

	t.Run("getLastOperation", func(t *testing.T) {
		lastOperation := s.getLastOperation()

		require.Equal(t, operation, lastOperation)
	})

	t.Run("fails to get original deployment replicas - invalid json", func(t *testing.T) {
		s := sleepInfoSecret{
			Secret: getSecret(mockSecretSpec{
				namespace: namespace,
				name:      secretName,
				data: map[string][]byte{
					replicasBeforeSleepKey: []byte(`[`),
				},
			}),
		}

		originalDeploymentsReplicas, err := s.getOriginalDeploymentReplicas()
		require.EqualError(t, err, "unexpected end of JSON input")
		require.Nil(t, originalDeploymentsReplicas)
	})

	t.Run("fails to get original cronjob suspended state - invalid json", func(t *testing.T) {
		s := sleepInfoSecret{
			Secret: getSecret(mockSecretSpec{
				namespace: namespace,
				name:      secretName,
				data: map[string][]byte{
					suspendedCronJobBeforeSleepKey: []byte(`[`),
				},
			}),
		}

		originalDeploymentsReplicas, err := s.getOriginalCronJobSuspendedState()
		require.EqualError(t, err, "unexpected end of JSON input")
		require.Nil(t, originalDeploymentsReplicas)
	})

	t.Run("empty secret", func(t *testing.T) {
		s := sleepInfoSecret{
			Secret: nil,
		}

		require.Empty(t, s.getLastOperation())
		require.Empty(t, s.getLastSchedule())
		originalCronjob, err := s.getOriginalCronJobSuspendedState()
		require.NoError(t, err)
		require.Empty(t, originalCronjob)
		originalDeployment, err := s.getOriginalDeploymentReplicas()
		require.NoError(t, err)
		require.Empty(t, originalDeployment)
	})

	t.Run("empty keys in secret", func(t *testing.T) {
		s := sleepInfoSecret{
			Secret: getSecret(mockSecretSpec{
				name:      secretName,
				namespace: namespace,
				data:      nil,
			}),
		}

		require.Empty(t, s.getLastOperation())
		require.Empty(t, s.getLastSchedule())
		originalCronjob, err := s.getOriginalCronJobSuspendedState()
		require.NoError(t, err)
		require.Empty(t, originalCronjob)
		originalDeployment, err := s.getOriginalDeploymentReplicas()
		require.NoError(t, err)
		require.Empty(t, originalDeployment)
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
