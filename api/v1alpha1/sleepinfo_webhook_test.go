/*
Copyright 2021.
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("validate sleep info", func() {
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
			expectedError: "empty weekday from sleep info configuration",
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
	}

	for _, test := range tests {
		test := test //necessary to ensure the correct value is passed to the closure
		s := sleepInfo.DeepCopy()
		s.Spec = test.sleepInfoSpec

		It(test.name, func() {
			err := s.validateSleepInfo()
			if test.expectedError != "" {
				Expect(err.Error()).To(Equal(test.expectedError))
			} else {
				Expect(err).To(BeNil())
			}
		})
	}

	Context("validate create", func() {
		It("ok", func() {
			sleepInfo := &SleepInfo{
				TypeMeta: metav1.TypeMeta{
					Kind:       "SleepInfo",
					APIVersion: "kube-green.com/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "namespace",
				},
				Spec: SleepInfoSpec{
					Weekdays:   "1-5",
					SleepTime:  "19:00",
					WakeUpTime: "8:00",
				},
			}
			err := sleepInfo.ValidateCreate()
			Expect(err).To(BeNil())
		})

		It("ko", func() {
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
			err := sleepInfo.ValidateCreate()
			Expect(err.Error()).To(Equal("empty weekday from sleep info configuration"))
		})
	})

	Context("validate update", func() {
		It("ok", func() {
			sleepInfo := &SleepInfo{
				TypeMeta: metav1.TypeMeta{
					Kind:       "SleepInfo",
					APIVersion: "kube-green.com/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "namespace",
				},
				Spec: SleepInfoSpec{
					Weekdays:   "1-5",
					SleepTime:  "19:00",
					WakeUpTime: "8:00",
				},
			}

			err := sleepInfo.ValidateUpdate(sleepInfo)
			Expect(err).To(BeNil())
		})

		It("ko", func() {
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

			err := sleepInfo.ValidateUpdate(sleepInfo)
			Expect(err.Error()).To(Equal("empty weekday from sleep info configuration"))
		})
	})
})
