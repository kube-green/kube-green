package sleepinfo

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
)

func TestResources(t *testing.T) {
	namespace := "my-namespace"
	tests := []struct {
		name                     string
		deploy                   bool
		cronJob                  bool
		expectToPerformOperation bool
	}{
		{
			name:                     "empty resources",
			expectToPerformOperation: false,
		},
		{
			name:                     "empty deployments, some cronjob",
			cronJob:                  true,
			expectToPerformOperation: true,
		},
		{
			name:                     "empty cronjob, some deployments",
			deploy:                   true,
			expectToPerformOperation: true,
		},
		{
			name:                     "both cronjob and deployments",
			cronJob:                  true,
			deploy:                   true,
			expectToPerformOperation: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resources := Resources{}
			if test.cronJob {
				resources.CronJobs = []batchv1.CronJob{getCronJobMock(mockCronJobSpec{
					name:      "cronjob-1",
					namespace: namespace,
				})}
			}
			if test.deploy {
				resources.Deployments = []appsv1.Deployment{getDeploymentMock(mockDeploymentSpec{
					name:      "deploy-1",
					namespace: namespace,
				})}
			}

			require.Equal(t, test.expectToPerformOperation, resources.hasResources())
		})
	}
}
