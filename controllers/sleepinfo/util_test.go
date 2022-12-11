package sleepinfo

import (
	"context"
	"testing"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cr "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// TODO: simplify setup with this function
func createSleepInfoCRD(ctx context.Context, t *testing.T, c *envconf.Config, sleepInfoName string, opts setupOptions) kubegreenv1alpha1.SleepInfo {
	t.Helper()

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
			Weekdays:   "*",
			SleepTime:  "*:05", // at minute 5
			WakeUpTime: "*:20", // at minute 20
		},
	}
	if opts.unsetWakeUpTime {
		sleepInfo.Spec.WakeUpTime = ""
	}
	if len(opts.excludeRef) != 0 {
		sleepInfo.Spec.ExcludeRef = opts.excludeRef
	}
	if opts.suspendCronjobs {
		sleepInfo.Spec.SuspendCronjobs = opts.suspendCronjobs
	}
	if opts.suspendDeployments != nil {
		sleepInfo.Spec.SuspendDeployments = opts.suspendDeployments
	}

	err := c.Client().Resources().Create(ctx, sleepInfo)
	require.NoError(t, err, "error creating SleepInfo")

	createdSleepInfo := &kubegreenv1alpha1.SleepInfo{}

	err = wait.For(conditions.New(c.Client().Resources()).ResourceMatch(sleepInfo, func(object k8s.Object) bool {
		createdSleepInfo = object.(*kubegreenv1alpha1.SleepInfo)
		return true
	}), wait.WithTimeout(time.Second*10), wait.WithInterval(time.Millisecond*250))
	require.NoError(t, err)

	return *createdSleepInfo
}

type AssertOperationKey struct{}

func WithAssertOperation(ctx context.Context, assert AssertOperation) context.Context {
	return context.WithValue(ctx, AssertOperationKey{}, assert)
}

func GetAssertOperation(t *testing.T, ctx context.Context) AssertOperation {
	assertOperation, ok := ctx.Value(AssertOperationKey{}).(AssertOperation)
	if !ok {
		t.Fatal("fail to get AssertOperation from context")
	}
	return assertOperation
}

// This function should be removed when e2e-framework > 0.0.8
func newControllerRuntimeClient(t *testing.T, c *envconf.Config) cr.Client {
	t.Helper()
	r, err := resources.New(c.Client().RESTConfig())
	require.NoError(t, err)

	client, err := cr.New(c.Client().RESTConfig(), cr.Options{Scheme: r.GetScheme()})
	require.NoError(t, err)

	return client
}

type mockClock struct {
	now string
	t   *testing.T
}

// TODO: when remove gomega, remove also the panic else
func (m mockClock) Now() time.Time {
	parsedTime, err := time.Parse(time.RFC3339, m.now)
	if m.t != nil {
		require.NoError(m.t, err)
	} else {
		if err != nil {
			panic(err.Error())
		}
	}
	return parsedTime
}

func getDeploymentList(t *testing.T, ctx context.Context, c *envconf.Config) []appsv1.Deployment {
	t.Helper()
	deployments := appsv1.DeploymentList{}
	require.NoError(t, c.Client().Resources(c.Namespace()).List(ctx, &deployments))
	return deployments.Items
}

func parseTime(t *testing.T, mockNowRaw string) time.Time {
	t.Helper()

	now, err := time.Parse(time.RFC3339, mockNowRaw)
	require.NoError(t, err)
	return now
}

func sleepRequeue(now string) time.Duration {
	parsedTime, _ := time.Parse(time.RFC3339, now)
	hour := parsedTime.Hour()
	if parsedTime.Minute() > 5 {
		hour += 1
	}
	return time.Duration((time.Date(2021, time.March, 23, hour, 5, 0, 0, time.UTC).UnixNano() - parsedTime.UnixNano()))
}

func wakeUpRequeue(now string) time.Duration {
	parsedTime, _ := time.Parse(time.RFC3339, now)
	hour := parsedTime.Hour()
	if parsedTime.Minute() > 20 {
		hour += 1
	}
	return time.Duration((time.Date(2021, time.March, 23, hour, 20, 0, 0, time.UTC).UnixNano() - parsedTime.UnixNano()))
}
