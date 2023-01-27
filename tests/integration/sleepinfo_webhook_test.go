//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestValidationWebhook(t *testing.T) {
	const (
		sleepInfoName = "name"
	)

	validateWebhook := features.Table{
		{
			Name: "validate create - ok",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				sleepInfo := &kubegreenv1alpha1.SleepInfo{
					TypeMeta: metav1.TypeMeta{
						Kind:       "SleepInfo",
						APIVersion: "kube-green.com/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      sleepInfoName,
						Namespace: c.Namespace(),
					},
					Spec: kubegreenv1alpha1.SleepInfoSpec{
						Weekdays:   "1-5",
						SleepTime:  "19:00",
						WakeUpTime: "8:00",
						ExcludeRef: []kubegreenv1alpha1.ExcludeRef{
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
				k8sClient, err := testutil.NewControllerRuntimeClient(c)
				require.NoError(t, err)
				err = k8sClient.Create(ctx, sleepInfo)
				require.NoError(t, err)
				return ctx
			},
		},
		{
			Name: "validate create - ko",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				k8sClient, err := testutil.NewControllerRuntimeClient(c)
				require.NoError(t, err)
				sleepInfo := &kubegreenv1alpha1.SleepInfo{
					TypeMeta: metav1.TypeMeta{
						Kind:       "SleepInfo",
						APIVersion: "kube-green.com/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      testutil.RandString(8),
						Namespace: c.Namespace(),
					},
					Spec: kubegreenv1alpha1.SleepInfoSpec{},
				}
				err = k8sClient.Create(ctx, sleepInfo)
				require.EqualError(t, err, "admission webhook \"vsleepinfo.kb.io\" denied the request: empty weekdays from SleepInfo configuration")
				return ctx
			},
		},
		{
			Name: "validate patch - ok",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				sleepInfo := getSleepInfo(t, ctx, sleepInfoName, c)

				patch := client.MergeFrom(sleepInfo.DeepCopy())
				sleepInfo.Spec.Weekdays = "*"

				k8sClient, err := testutil.NewControllerRuntimeClient(c)
				require.NoError(t, err)
				err = k8sClient.Patch(ctx, sleepInfo, patch)
				require.NoError(t, err)
				return ctx
			},
		},
		{
			Name: "validate patch - ko",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				sleepInfo := getSleepInfo(t, ctx, sleepInfoName, c)

				patch := client.MergeFrom(sleepInfo.DeepCopy())
				sleepInfo.Spec.Weekdays = ""

				k8sClient, err := testutil.NewControllerRuntimeClient(c)
				require.NoError(t, err)
				err = k8sClient.Patch(ctx, sleepInfo, patch)
				require.EqualError(t, err, "admission webhook \"vsleepinfo.kb.io\" denied the request: empty weekdays from SleepInfo configuration")
				return ctx
			},
		},
		{
			Name: "validate update - ok",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				sleepInfo := getSleepInfo(t, ctx, sleepInfoName, c)

				sleepInfo.Spec.Weekdays = "*"
				k8sClient, err := testutil.NewControllerRuntimeClient(c)
				require.NoError(t, err)

				err = k8sClient.Update(ctx, sleepInfo)
				require.NoError(t, err)

				return ctx
			},
		},
		{
			Name: "validate update - ko",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				k8sClient, err := testutil.NewControllerRuntimeClient(c)
				require.NoError(t, err)

				sleepInfo := getSleepInfo(t, ctx, sleepInfoName, c)

				sleepInfo.Spec.Weekdays = ""
				err = k8sClient.Update(ctx, sleepInfo)
				require.EqualError(t, err, "admission webhook \"vsleepinfo.kb.io\" denied the request: empty weekdays from SleepInfo configuration")

				return ctx
			},
		},
		{
			Name: "validate delete - ok",
			Assessment: func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
				sleepInfo := getSleepInfo(t, ctx, sleepInfoName, c)

				k8sClient, err := testutil.NewControllerRuntimeClient(c)
				require.NoError(t, err)

				err = k8sClient.Delete(ctx, sleepInfo)
				require.NoError(t, err)
				return ctx
			},
		},
	}.
		Build("validate webhook").
		Feature()

	testenv.Test(t, validateWebhook)
}

func getSleepInfo(t *testing.T, ctx context.Context, name string, c *envconf.Config) *kubegreenv1alpha1.SleepInfo {
	sleepInfo := &kubegreenv1alpha1.SleepInfo{}
	k8sClient, err := testutil.NewControllerRuntimeClient(c)
	require.NoError(t, err)
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: c.Namespace(),
		Name:      name,
	}, sleepInfo)
	require.NoError(t, err)
	return sleepInfo
}
