package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
				ExcludeRef: []ExcludeRef{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "deploy-1",
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
			require.Equal(t, []ExcludeRef{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "deploy-1",
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
				ExcludeRef: []ExcludeRef{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "deploy-1",
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
			require.Equal(t, []ExcludeRef{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "deploy-1",
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
				ExcludeRef: []ExcludeRef{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "deploy-1",
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
			require.Equal(t, []ExcludeRef{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "deploy-1",
				},
			}, excludeRef)
		})

		t.Run("is cronjob to suspend", func(t *testing.T) {
			isCronjobsToSuspend := sleepInfo.IsCronjobsToSuspend()
			require.True(t, isCronjobsToSuspend)
		})
	})

	t.Run("suspend deployment options", func(t *testing.T) {
		t.Run("true", func(t *testing.T) {
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
					SuspendDeployments: getPtr(true),
				},
			}

			isDeploymentToSuspend := sleepInfo.IsDeploymentsToSuspend()
			require.True(t, isDeploymentToSuspend)
		})

		t.Run("false", func(t *testing.T) {
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
					SuspendDeployments: getPtr(false),
				},
			}

			isDeploymentToSuspend := sleepInfo.IsDeploymentsToSuspend()
			require.False(t, isDeploymentToSuspend)
		})

		t.Run("empty - default to true", func(t *testing.T) {
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

			isDeploymentToSuspend := sleepInfo.IsDeploymentsToSuspend()
			require.True(t, isDeploymentToSuspend)
		})

		t.Run("nil - default to true", func(t *testing.T) {
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
					SuspendDeployments: nil,
				},
			}

			isDeploymentToSuspend := sleepInfo.IsDeploymentsToSuspend()
			require.True(t, isDeploymentToSuspend)
		})
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

	t.Run("with custom patches", func(t *testing.T) {
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

		require.NotEmpty(t, sleepInfo.GetPatches())
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

func getPtr[T any](item T) *T {
	return &item
}
