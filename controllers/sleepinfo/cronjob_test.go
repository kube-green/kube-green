package sleepinfo

import (
	"context"
	"testing"

	"github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/davidebianchi/kube-green/controllers/internal/testutil"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestGetCronJobList(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))

	namespace := "my-namespace"
	cronjob1 := getCronJobMock(mockCronJobSpec{
		name:      "cj1",
		namespace: namespace,
	})
	cronjob2 := getCronJobMock(mockCronJobSpec{
		name:      "cj2",
		namespace: namespace,
	})
	cronjobOtherNamespace := getCronJobMock(mockCronJobSpec{
		name:      "cjOtherNamespace",
		namespace: "other-namespace",
	})
	emptySleepInfo := &v1alpha1.SleepInfo{}

	listDeploymentsTests := []struct {
		name     string
		client   client.Client
		expected []batchv1.CronJob
		throws   bool
	}{
		{
			name: "get list of cron jobs",
			client: fake.
				NewClientBuilder().
				WithRuntimeObjects([]runtime.Object{&cronjob1, &cronjob2, &cronjobOtherNamespace}...).
				Build(),
			expected: []batchv1.CronJob{cronjob1, cronjob2},
		},
		{
			name: "fails to list cron job",
			client: &testutil.PossiblyErroringFakeCtrlRuntimeClient{
				Client:      fake.NewClientBuilder().Build(),
				ShouldError: true,
			},
			throws: true,
		},
		{
			name: "empty list cron job",
			client: fake.
				NewClientBuilder().
				WithRuntimeObjects([]runtime.Object{&cronjobOtherNamespace}...).
				Build(),
			expected: []batchv1.CronJob{},
		},
	}

	for _, test := range listDeploymentsTests {
		t.Run(test.name, func(t *testing.T) {
			r := SleepInfoReconciler{
				Client: test.client,
				Log:    testLogger,
			}

			listOfDeployments, err := r.getCronJobList(context.Background(), namespace, emptySleepInfo)
			if test.throws {
				require.EqualError(t, err, "error during list")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.expected, listOfDeployments)
		})
	}
}

type mockCronJobSpec struct {
	namespace       string
	name            string
	resourceVersion string
	schedule        string
	suspend         *bool
}

func getCronJobMock(opts mockCronJobSpec) batchv1.CronJob {
	return batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:            opts.name,
			Namespace:       opts.namespace,
			ResourceVersion: opts.resourceVersion,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: opts.schedule,
			Suspend:  opts.suspend,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      opts.name,
					Namespace: opts.namespace,
				},
				Spec: batchv1.JobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "my-image",
								},
							},
						},
					},
				},
			},
		},
	}
}
