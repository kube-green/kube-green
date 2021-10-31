package sleepinfo

import (
	"testing"
	"time"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func TestGetSleepInfoData(t *testing.T) {
	sleepInfo := kubegreenv1alpha1.SleepInfo{
		Spec: kubegreenv1alpha1.SleepInfoSpec{
			Weekdays:  "1-5",
			SleepTime: "20:00",
		},
	}
	sleepInfoWithWakeUp := sleepInfo
	sleepInfoWithWakeUp.Spec.WakeUpTime = "8:00"

	sleepInfoWithWakeUpAndSuspendCronJobs := sleepInfoWithWakeUp
	sleepInfoWithWakeUpAndSuspendCronJobs.Spec.SuspendCronjobs = true

	savedSecretReplicas := []byte(`[{"name":"deployment1","replicas":1},{"name":"deployment2","replicas":4},{"name":"replicas-not-saved"},{}]`)
	savedSecretCronJobs := []byte(`[{"name":"cj1","suspend":false},{}]`)
	schedule := time.Now()

	tests := []struct {
		name                  string
		secret                *v1.Secret
		sleepInfo             *kubegreenv1alpha1.SleepInfo
		expectedError         string
		expectedSleepInfoData SleepInfoData
		throws                bool
	}{
		{
			name:      "empty secret - only sleep",
			sleepInfo: &sleepInfo,
			expectedSleepInfoData: SleepInfoData{
				CurrentOperationType:     sleepOperation,
				CurrentOperationSchedule: "00 20 * * 1-5",
				NextOperationSchedule:    "00 20 * * 1-5",
			},
		},
		{
			name:      "empty secret - with wake up",
			sleepInfo: &sleepInfoWithWakeUp,
			expectedSleepInfoData: SleepInfoData{
				CurrentOperationType:     sleepOperation,
				CurrentOperationSchedule: "00 20 * * 1-5",
				NextOperationSchedule:    "00 8 * * 1-5",
			},
		},
		{
			name: "invalid sleep schedule",
			sleepInfo: &kubegreenv1alpha1.SleepInfo{
				Spec: kubegreenv1alpha1.SleepInfoSpec{
					Weekdays:  "1-5",
					SleepTime: "20:00:00",
				},
			},
			expectedError: "time should be of format HH:mm, actual: 20:00:00",
		},
		{
			name: "invalid wakeup schedule",
			sleepInfo: &kubegreenv1alpha1.SleepInfo{
				Spec: kubegreenv1alpha1.SleepInfoSpec{
					Weekdays:   "1-5",
					SleepTime:  "20:00",
					WakeUpTime: "8:00:00",
				},
			},
			expectedError: "time should be of format HH:mm, actual: 8:00:00",
		},
		{
			name: "empty secret data",
			secret: getSecret(mockSecretSpec{
				name: "secret-name",
				data: nil,
			}),
			sleepInfo: &sleepInfoWithWakeUp,
			expectedSleepInfoData: SleepInfoData{
				CurrentOperationType:     sleepOperation,
				CurrentOperationSchedule: "00 20 * * 1-5",
				NextOperationSchedule:    "00 8 * * 1-5",
			},
		},
		{
			name:      "empty secret - with suspend cronjob",
			sleepInfo: &sleepInfoWithWakeUpAndSuspendCronJobs,
			expectedSleepInfoData: SleepInfoData{
				CurrentOperationType:     sleepOperation,
				CurrentOperationSchedule: "00 20 * * 1-5",
				NextOperationSchedule:    "00 8 * * 1-5",
				SuspendCronjobs:          true,
			},
		},
		{
			name:      "with complete secret",
			sleepInfo: &sleepInfoWithWakeUpAndSuspendCronJobs,
			secret: getSecret(mockSecretSpec{
				namespace: "namespace",
				name:      "secretName",
				data: map[string][]byte{
					replicasBeforeSleepKey:         savedSecretReplicas,
					suspendedCronJobBeforeSleepKey: savedSecretCronJobs,
					lastScheduleKey:                []byte(schedule.Format(time.RFC3339)),
					lastOperationKey:               []byte(sleepOperation),
				},
			}),
			expectedSleepInfoData: SleepInfoData{
				CurrentOperationType:     wakeUpOperation,
				CurrentOperationSchedule: "00 8 * * 1-5",
				NextOperationSchedule:    "00 20 * * 1-5",
				SuspendCronjobs:          true,
				OriginalDeploymentsReplicas: map[string]int32{
					"deployment1":        1,
					"deployment2":        4,
					"replicas-not-saved": 0,
				},
				OriginalCronJobSuspendState: map[string]bool{
					"cj1": false,
				},
				LastSchedule: schedule.Truncate(time.Second),
			},
		},
		{
			name:      "invalid schedule",
			sleepInfo: &sleepInfoWithWakeUpAndSuspendCronJobs,
			secret: getSecret(mockSecretSpec{
				namespace: "namespace",
				name:      "secretName",
				data: map[string][]byte{
					replicasBeforeSleepKey:         savedSecretReplicas,
					suspendedCronJobBeforeSleepKey: savedSecretCronJobs,
					lastScheduleKey:                []byte("123"),
					lastOperationKey:               []byte(sleepOperation),
				},
			}),
			expectedError: `fails to parse scheduled-at: parsing time "123" as "2006-01-02T15:04:05Z07:00": cannot parse "123" as "2006"`,
		},
		{
			name:      "invalid saved replicas",
			sleepInfo: &sleepInfoWithWakeUpAndSuspendCronJobs,
			secret: getSecret(mockSecretSpec{
				namespace: "namespace",
				name:      "secretName",
				data: map[string][]byte{
					replicasBeforeSleepKey:         []byte(`"["`),
					suspendedCronJobBeforeSleepKey: savedSecretCronJobs,
					lastScheduleKey:                []byte(schedule.Format(time.RFC3339)),
					lastOperationKey:               []byte(sleepOperation),
				},
			}),
			throws: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sleepInfoData, err := getSleepInfoData(sleepInfoSecret{test.secret}, test.sleepInfo)

			if test.expectedError != "" {
				require.EqualError(t, err, test.expectedError)
			} else if test.throws {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, test.expectedSleepInfoData, sleepInfoData)
		})
	}
}
