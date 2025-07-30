package sleepinfo

import (
	"testing"
	"time"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/jsonpatch"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSleepInfoData(t *testing.T) {
	defaultSleepInfo := &v1alpha1.SleepInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sleepinfo-test",
		},
		Spec: v1alpha1.SleepInfoSpec{
			Weekdays:   "0-5",
			SleepTime:  "19:00",
			WakeUpTime: "09:00",
		},
	}
	lastScheduleValue := "2021-01-01T00:00:00Z"
	lastSchedule, err := time.Parse(time.RFC3339, lastScheduleValue)
	require.NoError(t, err)

	t.Run("getSleepInfoData", func(t *testing.T) {
		type TestCase struct {
			name        string
			secret      *v1.Secret
			sleepInfo   *v1alpha1.SleepInfo
			expected    SleepInfoData
			expectedErr string
		}
		testCases := []TestCase{
			{
				name: "if sleep schedule not correct",
				sleepInfo: &v1alpha1.SleepInfo{
					Spec: v1alpha1.SleepInfoSpec{
						Weekdays:  "0-5",
						SleepTime: "19",
					},
				},
				expectedErr: "time should be of format HH:mm, actual: '19'",
			},
			{
				name: "if wake up schedule not correct",
				sleepInfo: &v1alpha1.SleepInfo{
					Spec: v1alpha1.SleepInfoSpec{
						Weekdays:   "0-5",
						SleepTime:  "19:00",
						WakeUpTime: "09",
					},
				},
				expectedErr: "time should be of format HH:mm, actual: '09'",
			},
			{
				name: "if wake up not set, next operation schedule should be next sleep operation",
				sleepInfo: &v1alpha1.SleepInfo{
					Spec: v1alpha1.SleepInfoSpec{
						Weekdays:  "0-5",
						SleepTime: "19:00",
					},
				},
				expected: SleepInfoData{
					CurrentOperationType:     sleepOperation,
					CurrentOperationSchedule: "00 19 * * 0-5",
					NextOperationSchedule:    "00 19 * * 0-5",
				},
			},
			{
				name:      "when secret is nil returns first run data",
				sleepInfo: defaultSleepInfo,
				expected: SleepInfoData{
					CurrentOperationType:     sleepOperation,
					CurrentOperationSchedule: "00 19 * * 0-5",
					NextOperationSchedule:    "00 09 * * 0-5",
				},
			},
			{
				name: "if lastSchedule not set correctly",
				sleepInfo: &v1alpha1.SleepInfo{
					Spec: v1alpha1.SleepInfoSpec{
						Weekdays:   "0-5",
						SleepTime:  "19:00",
						WakeUpTime: "09:00",
					},
				},
				secret: &v1.Secret{
					Data: map[string][]byte{
						lastOperationKey: []byte(sleepOperation),
					},
				},
				expectedErr: "fails to parse scheduled-at: parsing time \"\" as",
			},
			{
				name: "when secret contains originalJSONPatchDataKey field set OriginalGenericResourceInfo",
				secret: &v1.Secret{
					Data: map[string][]byte{
						originalJSONPatchDataKey: []byte(`{"ReplicaSet.apps":{"echo-service-replicaset":"{\"spec\":{\"replicas\":2}}"}}`),
						lastScheduleKey:          []byte("2021-01-01T00:00:00Z"),
						lastOperationKey:         []byte(sleepOperation),
					},
				},
				sleepInfo: defaultSleepInfo,
				expected: SleepInfoData{
					LastSchedule:             lastSchedule,
					CurrentOperationType:     wakeUpOperation,
					CurrentOperationSchedule: "00 09 * * 0-5",
					NextOperationSchedule:    "00 19 * * 0-5",
					OriginalGenericResourceInfo: map[string]jsonpatch.RestorePatches{
						"ReplicaSet.apps": {
							"echo-service-replicaset": "{\"spec\":{\"replicas\":2}}",
						},
					},
				},
			},
			{
				name: "when secret originalJSONPatchDataKey content is not correct, returns error",
				secret: &v1.Secret{
					Data: map[string][]byte{
						originalJSONPatchDataKey: []byte(`"ReplicaSet.apps":{"echo-service-replicaset":"{\"spec\":{\"replicas\":2}}"}`),
						lastScheduleKey:          []byte("2021-01-01T00:00:00Z"),
						lastOperationKey:         []byte(sleepOperation),
					},
				},
				sleepInfo:   defaultSleepInfo,
				expectedErr: "fails to set original resource info to restore in SleepInfo sleepinfo-test: invalid character ':' after top-level value",
			},
			{
				name: "when secret contains replicasBeforeSleepKey and originalCronjobStatusKey format data fields",
				secret: &v1.Secret{
					Data: map[string][]byte{
						originalCronjobStatusKey: []byte(`[{"name":"cj-1","suspend":false}]`),
						replicasBeforeSleepKey:   []byte(`[{"name":"echo-service-replica-4","replicas":4}]`),
						lastScheduleKey:          []byte("2021-01-01T00:00:00Z"),
						lastOperationKey:         []byte(sleepOperation),
					},
				},
				sleepInfo: defaultSleepInfo,
				expected: SleepInfoData{
					LastSchedule:             lastSchedule,
					CurrentOperationType:     wakeUpOperation,
					CurrentOperationSchedule: "00 09 * * 0-5",
					NextOperationSchedule:    "00 19 * * 0-5",
					OriginalGenericResourceInfo: map[string]jsonpatch.RestorePatches{
						"CronJob.batch": {
							"cj-1": "{\"spec\":{\"suspend\":false}}",
						},
						"Deployment.apps": {
							"echo-service-replica-4": "{\"spec\":{\"replicas\":4}}",
						},
					},
				},
			},
			{
				name: "when secret contains all replicasBeforeSleepKey, originalCronjobStatusKey and originalJSONPatchDataKey format data fields",
				secret: &v1.Secret{
					Data: map[string][]byte{
						originalCronjobStatusKey: []byte(`[{"name":"cj-1","suspend":false}]`),
						replicasBeforeSleepKey:   []byte(`[{"name":"echo-service-replica-4","replicas":4}]`),
						originalJSONPatchDataKey: []byte(`{"ReplicaSet.apps":{"echo-service-replicaset":"{\"spec\":{\"replicas\":2}}"}}`),
						lastScheduleKey:          []byte("2021-01-01T00:00:00Z"),
						lastOperationKey:         []byte(sleepOperation),
					},
				},
				sleepInfo: defaultSleepInfo,
				expected: SleepInfoData{
					LastSchedule:             lastSchedule,
					CurrentOperationType:     wakeUpOperation,
					CurrentOperationSchedule: "00 09 * * 0-5",
					NextOperationSchedule:    "00 19 * * 0-5",
					OriginalGenericResourceInfo: map[string]jsonpatch.RestorePatches{
						"CronJob.batch": {
							"cj-1": "{\"spec\":{\"suspend\":false}}",
						},
						"Deployment.apps": {
							"echo-service-replica-4": "{\"spec\":{\"replicas\":4}}",
						},
						"ReplicaSet.apps": {
							"echo-service-replicaset": "{\"spec\":{\"replicas\":2}}",
						},
					},
				},
			},
			{
				name: "replicasBeforeSleepKey content invalid",
				secret: &v1.Secret{
					Data: map[string][]byte{
						replicasBeforeSleepKey: []byte(`{}`),
						lastScheduleKey:        []byte("2021-01-01T00:00:00Z"),
						lastOperationKey:       []byte(sleepOperation),
					},
				},
				sleepInfo:   defaultSleepInfo,
				expectedErr: "fails to set original deployment replicas info to restore: json:",
			},
			{
				name: "originalCronjobStatusKey content invalid",
				secret: &v1.Secret{
					Data: map[string][]byte{
						originalCronjobStatusKey: []byte(`{}`),
						lastScheduleKey:          []byte("2021-01-01T00:00:00Z"),
						lastOperationKey:         []byte(sleepOperation),
					},
				},
				sleepInfo:   defaultSleepInfo,
				expectedErr: "fails to set original cronjob status info to restore: json:",
			},
			{
				name: "when wake up schedule not set, next operation schedule should be next sleep operation",
				secret: &v1.Secret{
					Data: map[string][]byte{
						originalJSONPatchDataKey: []byte(`{"ReplicaSet.apps":{"echo-service-replicaset":"{\"spec\":{\"replicas\":2}}"}}`),
						lastScheduleKey:          []byte("2021-01-01T00:00:00Z"),
						lastOperationKey:         []byte(sleepOperation),
					},
				},
				sleepInfo: &v1alpha1.SleepInfo{
					Spec: v1alpha1.SleepInfoSpec{
						Weekdays:  "0-5",
						SleepTime: "19:00",
					},
				},
				expected: SleepInfoData{
					LastSchedule:             lastSchedule,
					CurrentOperationType:     sleepOperation,
					CurrentOperationSchedule: "00 19 * * 0-5",
					NextOperationSchedule:    "00 19 * * 0-5",
					OriginalGenericResourceInfo: map[string]jsonpatch.RestorePatches{
						"ReplicaSet.apps": {
							"echo-service-replicaset": "{\"spec\":{\"replicas\":2}}",
						},
					},
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				data, err := getSleepInfoData(tc.secret, tc.sleepInfo)
				if tc.expectedErr != "" {
					require.ErrorContains(t, err, tc.expectedErr)
				} else {
					require.NoError(t, err)
				}
				if len(tc.expectedErr) == 0 {
					require.Equal(t, tc.expected, data)
				}
			})
		}
	})

	t.Run("isWakeUpOperation and isSleepOperation", func(t *testing.T) {
		t.Run("wake up", func(t *testing.T) {
			sleepData := SleepInfoData{
				CurrentOperationType: wakeUpOperation,
			}

			require.True(t, sleepData.IsWakeUpOperation())
			require.False(t, sleepData.IsSleepOperation())
		})

		t.Run("sleep", func(t *testing.T) {
			sleepData := SleepInfoData{
				CurrentOperationType: sleepOperation,
			}

			require.False(t, sleepData.IsWakeUpOperation())
			require.True(t, sleepData.IsSleepOperation())
		})
	})
}
