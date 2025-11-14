package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSleepInfo(t *testing.T) {
	t.Run("sleep + wake up with timezone", func(t *testing.T) {
		sleepInfo := SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sleep-test-1",
				Namespace: "namespace",
			},
			Spec: SleepInfoSpec{
				Weekdays:   "1-5",
				SleepTime:  "20:00",
				WakeUpTime: "8:00",
				TimeZone:   "Europe/Rome",
				ExcludeRef: []FilterRef{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "deploy-1",
					},
					{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       "ss-1",
					},
				},
				IncludeRef: []FilterRef{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "deploy-2",
					},
					{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       "ss-2",
					},
				},
			},
		}
		t.Run("get sleep schedule", func(t *testing.T) {
			schedule, err := sleepInfo.GetSleepSchedule()
			require.NoError(t, err)
			require.Equal(t, "CRON_TZ=Europe/Rome 00 20 * * 1-5", schedule)
		})

		t.Run("get wake up schedule", func(t *testing.T) {
			schedule, err := sleepInfo.GetWakeUpSchedule()
			require.NoError(t, err)
			require.Equal(t, "CRON_TZ=Europe/Rome 00 8 * * 1-5", schedule)
		})

		t.Run("get exclude ref", func(t *testing.T) {
			excludeRef := sleepInfo.GetExcludeRef()
			require.Equal(t, []FilterRef{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "deploy-1",
				},
				{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "ss-1",
				},
			}, excludeRef)
		})

		t.Run("get include ref", func(t *testing.T) {
			excludeRef := sleepInfo.GetIncludeRef()
			require.Equal(t, []FilterRef{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "deploy-2",
				},
				{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "ss-2",
				},
			}, excludeRef)
		})

		t.Run("is cronjob to suspend", func(t *testing.T) {
			isCronjobsToSuspend := sleepInfo.IsCronjobsToSuspend()
			require.False(t, isCronjobsToSuspend)
		})
	})

	t.Run("sleep + wake up without timezone", func(t *testing.T) {
		sleepInfo := SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sleep-test-1",
				Namespace: "namespace",
			},
			Spec: SleepInfoSpec{
				Weekdays:   "1-5",
				SleepTime:  "20:00",
				WakeUpTime: "8:00",
			},
		}
		t.Run("get sleep schedule", func(t *testing.T) {
			schedule, err := sleepInfo.GetSleepSchedule()
			require.NoError(t, err)
			require.Equal(t, "00 20 * * 1-5", schedule)
		})

		t.Run("get wake up schedule", func(t *testing.T) {
			schedule, err := sleepInfo.GetWakeUpSchedule()
			require.NoError(t, err)
			require.Equal(t, "00 8 * * 1-5", schedule)
		})
	})

	t.Run("only sleep", func(t *testing.T) {
		sleepInfo := SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sleep-test-1",
				Namespace: "namespace",
			},
			Spec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "20:00",
				TimeZone:  "Europe/Rome",
				ExcludeRef: []FilterRef{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "deploy-1",
					},
					{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       "ss-1",
					},
				},
				IncludeRef: []FilterRef{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "deploy-2",
					},
					{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       "ss-2",
					},
				},
			},
		}
		t.Run("get sleep schedule", func(t *testing.T) {
			schedule, err := sleepInfo.GetSleepSchedule()
			require.NoError(t, err)
			require.Equal(t, "CRON_TZ=Europe/Rome 00 20 * * 1-5", schedule)
		})

		t.Run("get wake up schedule", func(t *testing.T) {
			schedule, err := sleepInfo.GetWakeUpSchedule()
			require.NoError(t, err)
			require.Equal(t, "", schedule)
		})

		t.Run("get exclude ref", func(t *testing.T) {
			excludeRef := sleepInfo.GetExcludeRef()
			require.Equal(t, []FilterRef{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "deploy-1",
				},
				{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "ss-1",
				},
			}, excludeRef)
		})

		t.Run("get include ref", func(t *testing.T) {
			excludeRef := sleepInfo.GetIncludeRef()
			require.Equal(t, []FilterRef{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "deploy-2",
				},
				{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "ss-2",
				},
			}, excludeRef)
		})

		t.Run("is cronjob to suspend", func(t *testing.T) {
			isCronjobsToSuspend := sleepInfo.IsCronjobsToSuspend()
			require.False(t, isCronjobsToSuspend)
		})
	})

	t.Run("cronjob to suspend", func(t *testing.T) {
		sleepInfo := SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sleep-test-1",
				Namespace: "namespace",
			},
			Spec: SleepInfoSpec{
				Weekdays:   "1-5",
				SleepTime:  "20:00",
				WakeUpTime: "8:00",
				TimeZone:   "Europe/Rome",
				ExcludeRef: []FilterRef{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "deploy-1",
					},
					{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       "ss-1",
					},
				},
				IncludeRef: []FilterRef{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "deploy-2",
					},
					{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       "ss-2",
					},
				},
				SuspendCronjobs: true,
			},
		}
		t.Run("get sleep schedule", func(t *testing.T) {
			schedule, err := sleepInfo.GetSleepSchedule()
			require.NoError(t, err)
			require.Equal(t, "CRON_TZ=Europe/Rome 00 20 * * 1-5", schedule)
		})

		t.Run("get wake up schedule", func(t *testing.T) {
			schedule, err := sleepInfo.GetWakeUpSchedule()
			require.NoError(t, err)
			require.Equal(t, "CRON_TZ=Europe/Rome 00 8 * * 1-5", schedule)
		})

		t.Run("get exclude ref", func(t *testing.T) {
			excludeRef := sleepInfo.GetExcludeRef()
			require.Equal(t, []FilterRef{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "deploy-1",
				},
				{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "ss-1",
				},
			}, excludeRef)
		})

		t.Run("get include ref", func(t *testing.T) {
			excludeRef := sleepInfo.GetIncludeRef()
			require.Equal(t, []FilterRef{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "deploy-2",
				},
				{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "ss-2",
				},
			}, excludeRef)
		})

		t.Run("is cronjob to suspend", func(t *testing.T) {
			isCronjobsToSuspend := sleepInfo.IsCronjobsToSuspend()
			require.True(t, isCronjobsToSuspend)
		})
	})

	t.Run("suspend deployment options", func(t *testing.T) {
		tests := []struct {
			name               string
			suspendDeployments *bool
			expected           bool
		}{
			{
				name:               "true",
				suspendDeployments: getPtr(true),
				expected:           true,
			},
			{
				name:               "false",
				suspendDeployments: getPtr(false),
				expected:           false,
			},
			{
				name:               "nil - default to true",
				suspendDeployments: nil,
				expected:           true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sleepInfo := SleepInfo{
					TypeMeta: metav1.TypeMeta{
						Kind:       "SleepInfo",
						APIVersion: "v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sleep-test-1",
						Namespace: "namespace",
					},
					Spec: SleepInfoSpec{
						SuspendDeployments: tt.suspendDeployments,
					},
				}

				isDeploymentToSuspend := sleepInfo.IsDeploymentsToSuspend()
				require.Equal(t, tt.expected, isDeploymentToSuspend)
			})
		}
	})

	t.Run("suspend statefulsets options", func(t *testing.T) {
		tests := []struct {
			name                string
			suspendStatefulSets *bool
			expected            bool
		}{
			{
				name:                "true",
				suspendStatefulSets: getPtr(true),
				expected:            true,
			},
			{
				name:                "false",
				suspendStatefulSets: getPtr(false),
				expected:            false,
			},
			{
				name:                "nil - default to true",
				suspendStatefulSets: nil,
				expected:            true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sleepInfo := SleepInfo{
					TypeMeta: metav1.TypeMeta{
						Kind:       "SleepInfo",
						APIVersion: "v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sleep-test-1",
						Namespace: "namespace",
					},
					Spec: SleepInfoSpec{
						SuspendStatefulSets: tt.suspendStatefulSets,
					},
				}

				isStatefulSetToSuspend := sleepInfo.IsStatefulSetsToSuspend()
				require.Equal(t, tt.expected, isStatefulSetToSuspend)
			})
		}
	})

	t.Run("fails if weekday is empty", func(t *testing.T) {
		sleepInfo := SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sleep-test-1",
				Namespace: "namespace",
			},
			Spec: SleepInfoSpec{},
		}

		t.Run("get sleep schedule", func(t *testing.T) {
			schedule, err := sleepInfo.GetSleepSchedule()
			require.EqualError(t, err, "empty weekdays from SleepInfo configuration")
			require.Empty(t, schedule)
		})
	})

	t.Run("fails if hours is in an invalid format", func(t *testing.T) {
		sleepInfo := SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sleep-test-1",
				Namespace: "namespace",
			},
			Spec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "20",
			},
		}

		t.Run("get sleep schedule", func(t *testing.T) {
			schedule, err := sleepInfo.GetSleepSchedule()
			require.EqualError(t, err, "time should be of format HH:mm, actual: 20")
			require.Empty(t, schedule)
		})
	})

	t.Run("custom patches", func(t *testing.T) {
		sleepInfo := SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sleep-test-1",
				Namespace: "namespace",
			},
			Spec: SleepInfoSpec{
				SuspendDeployments:  getPtr(false),
				SuspendStatefulSets: getPtr(false),
				Patches: []Patch{
					{
						Target: PatchTarget{
							Group: "apps",
							Kind:  "Deployment",
						},
						Patch: `
- op: add
  path: /spec/replicas
  value: 0
`,
					},
					{
						Target: PatchTarget{
							Group: "apps",
							Kind:  "Statefulset",
						},
						Patch: `
- op: add
  path: /spec/replicas
  value: 0
`,
					},
					{
						Target: PatchTarget{
							Group: "batch",
							Kind:  "CronJob",
						},
						Patch: `
- op: add
  path: /spec/suspend
  value: true
`,
					},
				},
			},
		}

		require.Equal(t, sleepInfo.Spec.Patches, sleepInfo.GetPatches())
	})

	t.Run("with custom and default patches", func(t *testing.T) {
		sleepInfo := SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sleep-test-1",
				Namespace: "namespace",
			},
			Spec: SleepInfoSpec{
				SuspendCronjobs: true,
				Patches: []Patch{
					{
						Target: PatchTarget{
							Group: "apps",
							Kind:  "Deployment",
						},
						Patch: `
- op: add
  path: /spec/replicas
  value: 0
`,
					},
					{
						Target: PatchTarget{
							Group: "apps",
							Kind:  "StatefulSet",
						},
						Patch: `
- op: add
  path: /spec/replicas
  value: 0
`,
					},
					{
						Target: PatchTarget{
							Group: "batch",
							Kind:  "CronJob",
						},
						Patch: `
- op: add
  path: /spec/suspend
  value: true
`,
					},
				},
			},
		}

		patches := append([]Patch{
			deploymentPatch,
			statefulSetPatch,
			cronjobPatch,
		}, sleepInfo.Spec.Patches...)
		require.Equal(t, patches, sleepInfo.GetPatches())
	})

	t.Run("PatchTarget", func(t *testing.T) {
		t.Run("String method", func(t *testing.T) {
			target := PatchTarget{
				Group: "apps",
				Kind:  "Deployment",
			}
			require.Equal(t, "Deployment.apps", target.String())
		})

		t.Run("GroupKind method", func(t *testing.T) {
			target := PatchTarget{
				Group: "apps",
				Kind:  "Deployment",
			}
			require.Equal(t, schema.GroupKind{
				Group: "apps",
				Kind:  "Deployment",
			}, target.GroupKind())
		})
	})
}

func TestValidateSleepInfo(t *testing.T) {
	sleepInfo := &SleepInfo{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SleepInfo",
			APIVersion: "kube-green.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
		Spec: SleepInfoSpec{},
	}
	var tests = []struct {
		name          string
		expectedError string
		expectedWarns []string
		sleepInfoSpec SleepInfoSpec
	}{
		{
			name:          "fails - without weekdays",
			expectedError: "empty weekdays from SleepInfo configuration",
		},
		{
			name:          "fails - without sleep",
			expectedError: "time should be of format HH:mm, actual: ",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays: "1-5",
			},
		},
		{
			name:          "fails - sleep time without minutes",
			expectedError: "expected exactly 5 fields, found 4: [15 * * 1-5]",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "15:",
			},
		},
		{
			name:          "fails - sleep time without hour",
			expectedError: "expected exactly 5 fields, found 4: [00 * * 1-5]",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: ":00",
			},
		},
		{
			name:          "fails - sleep time without `:`",
			expectedError: "time should be of format HH:mm, actual: 130",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "130",
			},
		},
		{
			name:          "fails - sleep time with double `:`",
			expectedError: "time should be of format HH:mm, actual: 1:3:0",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "1:3:0",
			},
		},
		{
			name:          "fails - sleep time with letter instead of numbers",
			expectedError: "failed to parse int from c: strconv.Atoi: parsing \"c\": invalid syntax",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "ab:c",
			},
		},
		{
			name:          "ok - no wake up time",
			expectedError: "",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "1:00",
			},
		},
		{
			name:          "fails - wake up time without minutes",
			expectedError: "expected exactly 5 fields, found 4: [15 * * 1-5]",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:   "1-5",
				SleepTime:  "1:30",
				WakeUpTime: "15:",
			},
		},
		{
			name:          "fails - wake up time without hour",
			expectedError: "expected exactly 5 fields, found 4: [00 * * 1-5]",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:   "1-5",
				SleepTime:  "1:30",
				WakeUpTime: ":00",
			},
		},
		{
			name:          "fails - wake up time without `:`",
			expectedError: "time should be of format HH:mm, actual: 11",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:   "1-5",
				SleepTime:  "13:00",
				WakeUpTime: "11",
			},
		},
		{
			name:          "fails - wake up time with double `:`",
			expectedError: "time should be of format HH:mm, actual: 1:3:0",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:   "1-5",
				SleepTime:  "13:0",
				WakeUpTime: "1:3:0",
			},
		},
		{
			name:          "fails - sleep time with letter instead of numbers",
			expectedError: "failed to parse int from c: strconv.Atoi: parsing \"c\": invalid syntax",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:   "1-5",
				SleepTime:  "13:15",
				WakeUpTime: "ab:c",
			},
		},
		{
			name: "ok - excludeRef only matchLabels",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "13:15",
				ExcludeRef: []FilterRef{
					{
						MatchLabels: map[string]string{
							"app": "backend",
						},
					},
				},
			},
		},
		{
			name: "ok - excludeRef Name,ApiVersion,Kind",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "13:15",
				ExcludeRef: []FilterRef{
					{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
						Name:       "my-deployment",
					},
				},
			},
		},
		{
			name: "ok - patches with existent resources",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "13:15",
				Patches: []Patch{
					{
						Target: PatchTarget{
							Group: "apps",
							Kind:  "StatefulSet",
						},
						Patch: `
- op: add
  path: /spec/replicas
  value: 0`,
					},
				},
			},
		},
		{
			name: "warning - patches with unsupported resources",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "13:15",
				Patches: []Patch{
					{
						Target: PatchTarget{
							Group: "apps",
							Kind:  "ReplicaSet",
						},
						Patch: `
- op: add
  path: /spec/replicas
  value: 0`,
					},
					{
						Target: PatchTarget{
							Group: "apps",
							Kind:  "UnknownResource",
						},
						Patch: `
- op: add
  path: /spec/replicas
  value: 0`,
					},
				},
			},
			expectedWarns: []string{
				"SleepInfo patch target is invalid: no matches for apps/, Resource=ReplicaSet",
				"SleepInfo patch target is invalid: no matches for apps/, Resource=UnknownResource",
			},
		},
		{
			name: "ok - invalid patch",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "13:15",
				Patches: []Patch{
					{
						Target: PatchTarget{
							Group: "apps",
							Kind:  "StatefulSet",
						},
						Patch: `- op: invalid`,
					},
				},
			},
			expectedError: "patch is invalid for target StatefulSet.apps: invalid operation {\"op\":\"invalid\"}: unsupported operation",
		},
	}

	groupVersion := []schema.GroupVersion{
		{Group: "apps", Version: "v1"},
	}
	restMapper := meta.NewDefaultRESTMapper(groupVersion)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "StatefulSet",
	}, meta.RESTScopeNamespace)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}, meta.RESTScopeNamespace)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "CronJob",
	}, meta.RESTScopeNamespace)

	for _, test := range tests {
		s := sleepInfo.DeepCopy()
		s.Spec = test.sleepInfoSpec

		t.Run(test.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithRESTMapper(restMapper).Build()
			warn, err := s.Validate(client)
			if test.expectedError != "" {
				require.EqualError(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
			}
			if len(warn) > 0 {
				require.Equal(t, test.expectedWarns, warn)
			}
		})
	}
}

func getPtr[T any](item T) *T {
	return &item
}
