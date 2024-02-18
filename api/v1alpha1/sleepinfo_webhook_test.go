/*
Copyright 2021.
*/

package v1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			name:          "fails - missing Name in ExcludeRef item",
			expectedError: `excludeRef is invalid. Must have set: matchLabels or name,apiVersion and kind fields`,
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:   "1-5",
				SleepTime:  "13:15",
				WakeUpTime: "13:20",
				ExcludeRef: []ExcludeRef{
					{
						APIVersion: "apps/v1",
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
			name: "ok - excludeRef only matchLabels",
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
			name: "ok - excludeRef Name,ApiVersion,Kind",
			sleepInfoSpec: SleepInfoSpec{
				Weekdays:  "1-5",
				SleepTime: "13:15",
				ExcludeRef: []ExcludeRef{
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
							Kind:  "Deployment",
						},
						Patch: `
- op: add
  path: /spec/replicas
  value: 0`,
					},
				},
			},
			expectedWarns: []string{
				"patch target ReplicaSet.apps is not supported by the cluster",
				"patch target Deployment.apps is not supported by the cluster",
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

	for _, test := range tests {
		test := test // necessary to ensure the correct value is passed to the closure
		s := sleepInfo.DeepCopy()
		s.Spec = test.sleepInfoSpec

		t.Run(test.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithRESTMapper(restMapper).Build()
			warn, err := s.validateSleepInfo(client)
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

func TestSleepInfoValidation(t *testing.T) {
	client := fake.NewClientBuilder().Build()
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
	customValidator := &customValidator{
		Client: client,
	}

	t.Run("create - ok", func(t *testing.T) {
		_, err := customValidator.ValidateCreate(context.Background(), sleepInfoOk)
		require.NoError(t, err)
	})

	t.Run("create - ko", func(t *testing.T) {
		_, err := customValidator.ValidateCreate(context.Background(), sleepInfoKo)
		require.EqualError(t, err, "empty weekdays from SleepInfo configuration")
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
		_, err := customValidator.ValidateUpdate(context.Background(), oldSleepInfo, sleepInfoOk)
		require.NoError(t, err)
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
		_, err := customValidator.ValidateUpdate(context.Background(), oldSleepInfo, sleepInfoKo)
		require.EqualError(t, err, "empty weekdays from SleepInfo configuration")
	})

	t.Run("delete - ok", func(t *testing.T) {
		_, err := customValidator.ValidateDelete(context.Background(), sleepInfoOk)
		require.NoError(t, err)
	})
}
