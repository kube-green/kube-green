/*
Copyright 2021.
*/

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
			name:          "fails - missing Name in ExcludeRef item",
			expectedError: `excludeRef is invalid. Must have set: matchLabels or name,apiVersion and kind fields`,
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:   "1-5",
				SleepTime:  "13:15",
				WakeUpTime: "13:20",
				ExcludeRef: []ExcludeRef{
					{
						ApiVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
			},
		},
		{
			name:          "fails - Name and MatchLabels both sets in ExcludeRef item",
			expectedError: `excludeRef is invalid. Must have set: matchLabels or name,apiVersion and kind fields`,
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "13:15",
				ExcludeRef: []ExcludeRef{
					{
						Name: "Backend",
						MatchLabels: map[string]string{
							"app": "backend",
						},
					},
				},
			},
		},
		{
			name: "ok - only matchLabels",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "13:15",
				ExcludeRef: []ExcludeRef{
					{
						MatchLabels: map[string]string{
							"app": "backend",
						},
					},
				},
			},
		},
		{
			name: "ok - Name,ApiVersion,Kind",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "13:15",
				ExcludeRef: []ExcludeRef{
					{
						Kind:       "Deployment",
						ApiVersion: "apps/v1",
						Name:       "my-deployment",
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test //necessary to ensure the correct value is passed to the closure
		s := sleepInfo.DeepCopy()
		s.Spec = test.sleepInfoSpec

		t.Run(test.name, func(t *testing.T) {
			err := s.validateSleepInfo()
			if test.expectedError != "" {
				require.EqualError(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSleepInfoValidation(t *testing.T) {
	sleepInfoOk := &SleepInfo{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SleepInfo",
			APIVersion: "kube-green.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
		Spec: SleepInfoSpec{
			SleepTime: "20:00",
			Weekdays:  "1-5",
		},
	}
	sleepInfoKo := &SleepInfo{
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

	t.Run("create - ok", func(t *testing.T) {
		require.NoError(t, sleepInfoOk.ValidateCreate())
	})

	t.Run("create - ko", func(t *testing.T) {
		require.EqualError(t, sleepInfoKo.ValidateCreate(), "empty weekdays from SleepInfo configuration")
	})

	t.Run("update - ok", func(t *testing.T) {
		oldSleepInfo := &SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "kube-green.com/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
			},
			Spec: SleepInfoSpec{
				SleepTime: "22:00",
				Weekdays:  "1-5",
			},
		}
		require.NoError(t, sleepInfoOk.ValidateUpdate(oldSleepInfo))
	})

	t.Run("update - ok", func(t *testing.T) {
		oldSleepInfo := &SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "kube-green.com/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
			},
			Spec: SleepInfoSpec{
				SleepTime: "22:00",
				Weekdays:  "1-5",
			},
		}
		require.EqualError(t, sleepInfoKo.ValidateUpdate(oldSleepInfo), "empty weekdays from SleepInfo configuration")
	})

	t.Run("delete - ok", func(t *testing.T) {
		require.NoError(t, (&SleepInfo{}).ValidateDelete())
	})
}
