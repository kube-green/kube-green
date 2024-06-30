package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeepCopy(t *testing.T) {
	t.Run("sleep info", func(t *testing.T) {
		sleepInfo := &SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "kube-green.com/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "name",
			},
			Spec: SleepInfoSpec{
				Weekdays:           "*",
				SleepTime:          "*:05", // at minute 5
				WakeUpTime:         "*:20", // at minute 20
				SuspendCronjobs:    true,
				SuspendDeployments: getPtr(false),
				ExcludeRef: []FilterRef{
					{
						Name: "",
					},
					{
						MatchLabels: map[string]string{
							"label1": "value1",
						},
					},
				},
			},
			Status: SleepInfoStatus{
				OperationType:    "sleep",
				LastScheduleTime: metav1.Now(),
			},
		}

		require.Equal(t, sleepInfo, sleepInfo.DeepCopy())
		require.Equal(t, sleepInfo, sleepInfo.DeepCopyObject())

		require.Equal(t, &sleepInfo.Status, sleepInfo.Status.DeepCopy())

		require.Equal(t, &sleepInfo.Spec, sleepInfo.Spec.DeepCopy())

		require.Equal(t, &sleepInfo.Spec.ExcludeRef[0], sleepInfo.Spec.ExcludeRef[0].DeepCopy())
		require.Equal(t, &sleepInfo.Spec.ExcludeRef[1], sleepInfo.Spec.ExcludeRef[1].DeepCopy())
	})

	t.Run("sleep info list", func(t *testing.T) {
		sleepInfoList := &SleepInfoList{
			Items: []SleepInfo{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "SleepInfo",
						APIVersion: "kube-green.com/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
					},
					Spec: SleepInfoSpec{
						Weekdays:           "*",
						SleepTime:          "*:05", // at minute 5
						WakeUpTime:         "*:20", // at minute 20
						SuspendCronjobs:    true,
						SuspendDeployments: getPtr(false),
						ExcludeRef:         []FilterRef{},
					},
					Status: SleepInfoStatus{
						OperationType:    "sleep",
						LastScheduleTime: metav1.Now(),
					},
				},
			},
		}

		require.Equal(t, sleepInfoList, sleepInfoList.DeepCopy())
		require.Equal(t, sleepInfoList, sleepInfoList.DeepCopyObject())
	})

	t.Run("nil", func(t *testing.T) {
		t.Run("exclude ref", func(t *testing.T) {
			var excludeRef *ExcludeRef = nil

			require.Nil(t, excludeRef.DeepCopy())
		})

		t.Run("sleepinfo", func(t *testing.T) {
			var sleepInfo *SleepInfo = nil

			require.Nil(t, sleepInfo.DeepCopy())
			require.Nil(t, sleepInfo.DeepCopyObject())
		})

		t.Run("list", func(t *testing.T) {
			var sleepInfoList *SleepInfoList = nil

			require.Nil(t, sleepInfoList.DeepCopy())
			require.Nil(t, sleepInfoList.DeepCopyObject())
		})

		t.Run("spec", func(t *testing.T) {
			var sleepInfoSpec *SleepInfoSpec = nil

			require.Nil(t, sleepInfoSpec.DeepCopy())
		})

		t.Run("status", func(t *testing.T) {
			var sleepInfoStatus *SleepInfoStatus = nil

			require.Nil(t, sleepInfoStatus.DeepCopy())
		})
	})
}
