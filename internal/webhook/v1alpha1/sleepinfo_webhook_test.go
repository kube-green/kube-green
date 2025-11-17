/*
Copyright 2021.
*/

package v1alpha1

import (
	"context"
	"testing"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSleepInfoValidation(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	sleepInfoOk := &v1alpha1.SleepInfo{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SleepInfo",
			APIVersion: "kube-green.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
		Spec: v1alpha1.SleepInfoSpec{
			SleepTime: "20:00",
			Weekdays:  "1-5",
		},
	}
	sleepInfoWeekdaySleepOk := &v1alpha1.SleepInfo{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SleepInfo",
			APIVersion: "kube-green.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
		Spec: v1alpha1.SleepInfoSpec{
			SleepTime:    "20:00",
			WeekDaySleep: "1-5",
		},
	}
	sleepInfoKo := &v1alpha1.SleepInfo{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SleepInfo",
			APIVersion: "kube-green.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
		Spec: v1alpha1.SleepInfoSpec{},
	}
	customValidator := &customValidator{
		Client: client,
	}

	t.Run("create - ok", func(t *testing.T) {
		_, err := customValidator.ValidateCreate(context.Background(), sleepInfoOk)
		require.NoError(t, err)
	})

	t.Run("create - ok", func(t *testing.T) {
		_, err := customValidator.ValidateCreate(context.Background(), sleepInfoWeekdaySleepOk)
		require.NoError(t, err)
	})

	t.Run("create weekday sleep - ko", func(t *testing.T) {
		_, err := customValidator.ValidateCreate(context.Background(), sleepInfoKo)
		require.EqualError(t, err, "empty weekdays and weekdaySleep or weekdayWakeUp from SleepInfo configuration")
	})

	t.Run("update - ok", func(t *testing.T) {
		oldSleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "kube-green.com/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
			},
			Spec: v1alpha1.SleepInfoSpec{
				SleepTime: "22:00",
				Weekdays:  "1-5",
			},
		}
		_, err := customValidator.ValidateUpdate(context.Background(), oldSleepInfo, sleepInfoOk)
		require.NoError(t, err)
	})

	t.Run("update - ok", func(t *testing.T) {
		oldSleepInfo := &v1alpha1.SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "kube-green.com/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
			},
			Spec: v1alpha1.SleepInfoSpec{
				SleepTime: "22:00",
				Weekdays:  "1-5",
			},
		}
		_, err := customValidator.ValidateUpdate(context.Background(), oldSleepInfo, sleepInfoKo)
		require.EqualError(t, err, "empty weekdays and weekdaySleep or weekdayWakeUp from SleepInfo configuration")
	})

	t.Run("delete - ok", func(t *testing.T) {
		_, err := customValidator.ValidateDelete(context.Background(), sleepInfoOk)
		require.NoError(t, err)
	})
}
