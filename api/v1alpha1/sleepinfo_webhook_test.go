/*
Copyright 2021.
*/

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			name:          "fails - missing Name and MatchLabels in ExcludeRef item",
			expectedError: `one of "Name" or "MatchLabels" values must be set`,
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
			expectedError: `one of "Name" or "MatchLabels" values must be set`,
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:   "1-5",
				SleepTime:  "13:15",
				WakeUpTime: "13:20",
				ExcludeRef: []ExcludeRef{
					{
						ApiVersion: "apps/v1",
						Kind:       "Deployment",
					},
					{
						ApiVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "Backend",
						MatchLabels: map[string]string{
							"app": "backend",
						},
					},
				},
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
	const (
		namespace     = "default"
		sleepInfoName = "name"
	)

	Context("validate create", func() {
		ctx := context.Background()

		It("ok", func() {
			sleepInfo := &SleepInfo{
				TypeMeta: metav1.TypeMeta{
					Kind:       "SleepInfo",
					APIVersion: "kube-green.com/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      sleepInfoName,
					Namespace: namespace,
				},
				Spec: SleepInfoSpec{
					Weekdays:   "1-5",
					SleepTime:  "19:00",
					WakeUpTime: "8:00",
					ExcludeRef: []ExcludeRef{
						{
							ApiVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "Frontend",
						},
						{
							ApiVersion: "apps/v1",
							Kind:       "Deployment",
							MatchLabels: map[string]string{
								"app": "backend",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, sleepInfo)
			Expect(err).To(BeNil())
		})

		It("ko", func() {
			sleepInfo := &SleepInfo{
				TypeMeta: metav1.TypeMeta{
					Kind:       "SleepInfo",
					APIVersion: "kube-green.com/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      sleepInfoName,
					Namespace: namespace,
				},
				Spec: SleepInfoSpec{},
			}
			err := k8sClient.Create(ctx, sleepInfo)
			Expect(err.Error()).To(Equal("admission webhook \"vsleepinfo.kb.io\" denied the request: empty weekdays from SleepInfo configuration"))
		})
	})

	Context("validate patch", func() {
		ctx := context.Background()
		namespace := "default"

		It("ok", func() {
			sleepInfo := getSleepInfo(sleepInfoName, namespace)

			patch := client.MergeFrom(sleepInfo.DeepCopy())
			sleepInfo.Spec.Weekdays = "*"
			err := k8sClient.Patch(ctx, sleepInfo, patch)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ko", func() {
			sleepInfo := getSleepInfo(sleepInfoName, namespace)

			patch := client.MergeFrom(sleepInfo.DeepCopy())
			sleepInfo.Spec.Weekdays = ""
			err := k8sClient.Patch(ctx, sleepInfo, patch)
			Expect(err.Error()).To(Equal("admission webhook \"vsleepinfo.kb.io\" denied the request: empty weekdays from SleepInfo configuration"))
		})
	})

	Context("validate update", func() {
		ctx := context.Background()
		namespace := "default"

		It("ok", func() {
			sleepInfo := getSleepInfo(sleepInfoName, namespace)

			sleepInfo.Spec.Weekdays = "*"
			err := k8sClient.Update(ctx, sleepInfo)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ko", func() {
			sleepInfo := getSleepInfo(sleepInfoName, namespace)

			sleepInfo.Spec.Weekdays = ""
			err := k8sClient.Update(ctx, sleepInfo)
			Expect(err.Error()).To(Equal("admission webhook \"vsleepinfo.kb.io\" denied the request: empty weekdays from SleepInfo configuration"))
		})
	})

	Context("validate delete", func() {
		It("ok", func() {
			sleepInfo := getSleepInfo(sleepInfoName, namespace)

			err := k8sClient.Delete(ctx, sleepInfo)
			Expect(err).To(BeNil())
		})
	})
})

func getSleepInfo(name, namespace string) *SleepInfo {
	sleepInfo := &SleepInfo{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, sleepInfo)
	Expect(err).NotTo(HaveOccurred())
	return sleepInfo
}
