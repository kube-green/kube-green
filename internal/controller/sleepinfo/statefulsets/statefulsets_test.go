package statefulsets

import (
	"context"
	"fmt"
	"testing"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestNewResource(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	namespace := "my-namespace"
	ss1 := GetMock(MockSpec{
		Name:      "statefulset1",
		Namespace: namespace,
	})
	ss2 := GetMock(MockSpec{
		Name:      "statefulset2",
		Namespace: namespace,
	})
	ssOtherNamespace := GetMock(MockSpec{
		Name:      "statefulsetOtherNamespace",
		Namespace: "other-namespace",
	})
	ssWithLabels := GetMock(MockSpec{
		Name:      "statefulsetWithLabels",
		Namespace: namespace,
		Labels:    map[string]string{"foo-key": "foo-value", "bar-key": "bar-value"},
	})
	emptySleepInfo := &v1alpha1.SleepInfo{}

	listStatefulsetsTests := []struct {
		name      string
		client    client.Client
		sleepInfo *v1alpha1.SleepInfo
		expected  []appsv1.StatefulSet
		throws    bool
	}{
		{
			name: "get list of statefulsets",
			client: fake.
				NewClientBuilder().
				WithRuntimeObjects([]runtime.Object{&ss1, &ss2, &ssOtherNamespace}...).
				Build(),
			expected: []appsv1.StatefulSet{ss1, ss2},
		},
		{
			name: "fails to list statefulsets",
			client: &testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client: fake.NewClientBuilder().Build(),
				ShouldError: func(method testutil.Method, obj runtime.Object) bool {
					return method == testutil.List
				},
			},
			throws: true,
		},
		{
			name: "empty list statefulset",
			client: fake.
				NewClientBuilder().
				WithRuntimeObjects([]runtime.Object{&ssOtherNamespace}...).
				Build(),
			expected: []appsv1.StatefulSet{},
		},
		{
			name: "disabled statefulset suspend",
			client: fake.
				NewClientBuilder().
				WithRuntimeObjects([]runtime.Object{&ss1, &ss2, &ssOtherNamespace}...).
				Build(),
			sleepInfo: &v1alpha1.SleepInfo{
				Spec: v1alpha1.SleepInfoSpec{
					SuspendStatefulsets: getPtr(false),
				},
			},
			expected: []appsv1.StatefulSet{},
		},
		{
			name: "with statefulset to exclude",
			client: fake.
				NewClientBuilder().
				WithRuntimeObjects([]runtime.Object{&ss1, &ss2, &ssOtherNamespace}...).
				Build(),
			sleepInfo: &v1alpha1.SleepInfo{
				Spec: v1alpha1.SleepInfoSpec{
					ExcludeRef: []v1alpha1.ExcludeRef{
						{
							APIVersion: "apps/v1",
							Kind:       "StatefulSet",
							Name:       ss2.Name,
						},
						{
							APIVersion: "apps/v1",
							Kind:       "resource",
							Name:       "foo",
						},
						{
							APIVersion: "apps/v2",
							Kind:       "StatefulSet",
							Name:       ss1.Name,
						},
					},
				},
			},
			expected: []appsv1.StatefulSet{ss1},
		},
		{
			name: "with statefulset to exclude with matchLabels",
			client: fake.
				NewClientBuilder().
				WithRuntimeObjects([]runtime.Object{&ss1, &ss2, &ssOtherNamespace, &ssWithLabels}...).
				Build(),
			sleepInfo: &v1alpha1.SleepInfo{
				Spec: v1alpha1.SleepInfoSpec{
					ExcludeRef: []v1alpha1.ExcludeRef{
						{
							APIVersion: "apps/v1",
							Kind:       "StatefulSet",
							Name:       ss2.Name,
						},
						{
							APIVersion: "apps/v1",
							Kind:       "resource",
							Name:       "foo",
						},
						{
							APIVersion:  "apps/v1",
							Kind:        "StatefulSet",
							MatchLabels: ssWithLabels.Labels,
						},
					},
				},
			},
			expected: []appsv1.StatefulSet{ss1},
		},
	}

	for _, test := range listStatefulsetsTests {
		t.Run(test.name, func(t *testing.T) {
			sleepInfo := emptySleepInfo
			if test.sleepInfo != nil {
				sleepInfo = test.sleepInfo
			}
			s, err := NewResource(context.Background(), resource.ResourceClient{
				Client:    test.client,
				Log:       testLogger,
				SleepInfo: sleepInfo,
			}, namespace, map[string]int32{})
			if test.throws {
				require.EqualError(t, err, "error during list")
			} else {
				require.NoError(t, err)
			}
			statefulsets, ok := s.(statefulsets)
			require.True(t, ok)
			require.Equal(t, test.expected, statefulsets.data)
		})
	}
}

func TestHasResource(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	namespace := "my-namespace"
	statefulset1 := GetMock(MockSpec{
		Name:      "statefulset1",
		Namespace: namespace,
	})

	t.Run("without resource", func(t *testing.T) {
		s, err := NewResource(context.Background(), resource.ResourceClient{
			Client:    fake.NewClientBuilder().Build(),
			Log:       testLogger,
			SleepInfo: &v1alpha1.SleepInfo{},
		}, namespace, map[string]int32{})
		require.NoError(t, err)

		require.False(t, s.HasResource())
	})

	t.Run("with resource", func(t *testing.T) {
		s, err := NewResource(context.Background(), resource.ResourceClient{
			Client:    fake.NewClientBuilder().WithRuntimeObjects(&statefulset1).Build(),
			Log:       testLogger,
			SleepInfo: &v1alpha1.SleepInfo{},
		}, namespace, map[string]int32{})
		require.NoError(t, err)

		require.True(t, s.HasResource())
	})
}

func TestSleep(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	var replica0 int32 = 0
	var replica1 int32 = 1
	var replica5 int32 = 5
	namespace := "my-namespace"

	ss1 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "ss1",
		Replicas:        &replica1,
		ResourceVersion: "2",
	})
	ss2 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "ss2",
		Replicas:        &replica5,
		ResourceVersion: "1",
	})
	ssZeroReplicas := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "ssZeroReplicas",
		Replicas:        &replica0,
		ResourceVersion: "1",
	})

	ctx := context.Background()
	emptySleepInfo := &v1alpha1.SleepInfo{}
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}

	t.Run("update statefulset to have zero replicas", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&ss1, &ss2, &ssZeroReplicas).Build()
		fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: c,
		}

		resource, err := NewResource(ctx, resource.ResourceClient{
			Client:    fakeClient,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{})
		require.NoError(t, err)

		require.NoError(t, resource.Sleep(ctx))

		list := appsv1.StatefulSetList{}
		err = c.List(ctx, &list, listOptions)
		require.NoError(t, err)
		require.Equal(t, appsv1.StatefulSetList{
			Items: []appsv1.StatefulSet{
				GetMock(MockSpec{
					Namespace:       namespace,
					Name:            "ss1",
					Replicas:        &replica0,
					ResourceVersion: "3",
				}),
				GetMock(MockSpec{
					Namespace:       namespace,
					Name:            "ss2",
					Replicas:        &replica0,
					ResourceVersion: "2",
				}),
				ssZeroReplicas,
			},
		}, list)
	})

	t.Run("fails to patch statefulset", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&ss1, &ss2, &ssZeroReplicas).Build()
		fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: c,
			ShouldError: func(method testutil.Method, obj runtime.Object) bool {
				return method == testutil.Patch
			},
		}

		resource, err := NewResource(ctx, resource.ResourceClient{
			Client:    fakeClient,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{})
		require.NoError(t, err)

		require.EqualError(t, resource.Sleep(ctx), "error during patch")
	})

	t.Run("not fails if statefulsets not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&ss1, &ss2, &ssZeroReplicas).Build()
		fakeClient := &testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: c,
		}

		resource, err := NewResource(ctx, resource.ResourceClient{
			Client:    fakeClient,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{})
		require.NoError(t, err)

		err = c.DeleteAllOf(ctx, &appsv1.StatefulSet{}, &client.DeleteAllOfOptions{})
		require.NoError(t, err)

		require.NoError(t, resource.Sleep(ctx))
	})
}

func TestWakeUp(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	var replica0 int32 = 0
	var replica1 int32 = 1
	var replica5 int32 = 5
	namespace := "my-namespace"

	ss1 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "ss1",
		Replicas:        &replica0,
		ResourceVersion: "2",
	})
	ss2 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "ss2",
		Replicas:        &replica0,
		ResourceVersion: "1",
	})
	ssZeroReplicas := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "ssZeroReplicas",
		Replicas:        &replica0,
		ResourceVersion: "1",
	})
	ssAfterSleep := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "aftersleep",
		Replicas:        &replica1,
		ResourceVersion: "1",
	})

	ctx := context.Background()
	emptySleepInfo := &v1alpha1.SleepInfo{}
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}

	t.Run("wake up statefulset", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&ss1, &ss2, &ssZeroReplicas, &ssAfterSleep).Build()
		r, err := NewResource(ctx, resource.ResourceClient{
			Client:    c,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{
			ss1.Name: replica1,
			ss2.Name: replica5,
		})
		require.NoError(t, err)

		err = r.WakeUp(ctx)
		require.NoError(t, err)

		list := appsv1.StatefulSetList{}
		err = c.List(ctx, &list, listOptions)
		require.NoError(t, err)
		require.Equal(t, appsv1.StatefulSetList{
			Items: []appsv1.StatefulSet{
				ssAfterSleep,
				GetMock(MockSpec{
					Namespace:       namespace,
					Name:            "ss1",
					Replicas:        &replica1,
					ResourceVersion: "3",
				}),
				GetMock(MockSpec{
					Namespace:       namespace,
					Name:            "ss2",
					Replicas:        &replica5,
					ResourceVersion: "2",
				}),
				ssZeroReplicas,
			},
		}, list)
	})

	t.Run("wake up fails", func(t *testing.T) {
		c := testutil.PossiblyErroringFakeCtrlRuntimeClient{
			Client: fake.NewClientBuilder().WithRuntimeObjects(&ss1).Build(),
			ShouldError: func(method testutil.Method, obj runtime.Object) bool {
				return method == testutil.Patch
			},
		}
		r, err := NewResource(ctx, resource.ResourceClient{
			Client:    c,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{
			ss1.Name: replica1,
			ss2.Name: replica5,
		})
		require.NoError(t, err)

		err = r.WakeUp(ctx)
		require.EqualError(t, err, "error during patch")
	})
}

func TestStatefulsetOriginalReplicas(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	ctx := context.Background()
	namespace := "my-namespace"
	emptySleepInfo := &v1alpha1.SleepInfo{}
	var replica0 int32 = 0
	var replica1 int32 = 1
	var replica5 int32 = 5

	ss1 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "ss1",
		Replicas:        &replica1,
		ResourceVersion: "2",
	})
	ss2 := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "ss2",
		Replicas:        &replica5,
		ResourceVersion: "1",
	})
	ssZeroReplicas := GetMock(MockSpec{
		Namespace:       namespace,
		Name:            "ssZeroReplica",
		Replicas:        &replica0,
		ResourceVersion: "1",
	})

	t.Run("save and restore replicas info", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&ss1, &ss2, &ssZeroReplicas).Build()
		r, err := NewResource(ctx, resource.ResourceClient{
			Client:    c,
			Log:       testLogger,
			SleepInfo: emptySleepInfo,
		}, namespace, map[string]int32{
			ss1.Name: replica1,
			ss2.Name: replica5,
		})
		require.NoError(t, err)

		res, err := r.GetOriginalInfoToSave()
		require.NoError(t, err)

		expectedInfoToSave := `[{"name":"ss1","replicas":1},{"name":"ss2","replicas":5}]`
		fmt.Println("response is: ", string(res))
		require.JSONEq(t, expectedInfoToSave, string(res))

		t.Run("restore saved info", func(t *testing.T) {
			infoToSave := []byte(expectedInfoToSave)
			restoredInfo, err := GetOriginalInfoToRestore(infoToSave)
			require.NoError(t, err)
			require.Equal(t, map[string]int32{
				ss1.Name: replica1,
				ss2.Name: replica5,
			}, restoredInfo)
		})
	})

	t.Run("restore info with data nil", func(t *testing.T) {
		info, err := GetOriginalInfoToRestore(nil)
		require.Equal(t, map[string]int32{}, info)
		require.NoError(t, err)
	})

	t.Run("fails if saved data are not valid json", func(t *testing.T) {
		info, err := GetOriginalInfoToRestore([]byte(`{}`))
		require.EqualError(t, err, "json: cannot unmarshal object into Go value of type []statefulsets.OriginalReplicas")
		require.Nil(t, info)
	})

	t.Run("do nothing if statefulsets are not to suspend", func(t *testing.T) {
		c := fake.NewClientBuilder().WithRuntimeObjects(&ss1, &ss2, &ssZeroReplicas).Build()
		r, err := NewResource(ctx, resource.ResourceClient{
			Client: c,
			Log:    testLogger,
			SleepInfo: &v1alpha1.SleepInfo{
				Spec: v1alpha1.SleepInfoSpec{
					SuspendStatefulsets: getPtr(false),
				},
			},
		}, namespace, map[string]int32{
			ss1.Name: replica1,
			ss2.Name: replica5,
		})
		require.NoError(t, err)

		res, err := r.GetOriginalInfoToSave()
		require.NoError(t, err)
		require.Nil(t, res)
	})
}

func TestLabelMatch(t *testing.T) {
	testCases := []struct {
		name        string
		labels      map[string]string
		matchLabels map[string]string
		expected    bool
	}{
		{
			name:     "Missing labels and matchLabels",
			expected: false,
		},
		{
			name: "Missing labels",
			matchLabels: map[string]string{
				"app-key": "app-value",
			},
			expected: false,
		},
		{
			name: "Match failed",
			labels: map[string]string{
				"foo-key": "foo-value",
			},
			matchLabels: map[string]string{
				"app-key": "app-value",
			},
			expected: false,
		},
		{
			name: "Match success",
			labels: map[string]string{
				"app-key": "app-value",
			},
			matchLabels: map[string]string{
				"app-key": "app-value",
			},
			expected: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			got := labelMatch(test.labels, test.matchLabels)
			require.Equal(t, test.expected, got)
		})
	}
}

func getPtr[T any](item T) *T {
	return &item
}
