package sleepinfo

import (
	"context"
	"testing"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/testutil"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func testBeforeEach(runID string) env.FeatureFunc {
	return func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		r, err := resources.New(c.Client().RESTConfig())
		require.NoError(t, err)
		err = kubegreenv1alpha1.AddToScheme(r.GetScheme())
		require.NoError(t, err)
		c = c.WithClient(c.Client())

		// Clear setup options from previous tests to ensure each feature starts fresh
		ctx = withSetupOptions(ctx, setupOptions{})

		return testutil.CreateNamespace(ctx, c, t, runID)
	}
}

func testAfterEach(runID string) env.FeatureFunc {
	return func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		return testutil.DeleteNamespace(ctx, c, t, runID)
	}
}

func testenvSetup(t *testing.T) env.Environment {
	config := envconf.New().WithParallelTestEnabled()
	envTest, err := testutil.StartEnvTest(config, []string{"../../../config/crd/bases"})
	require.NoError(t, err)
	t.Cleanup(func() {
		err := envTest.Stop()
		t.Log("fail to cleanup envTest:", err)
	})

	testenv := env.NewWithConfig(config)
	runID := envconf.RandomName("kube-green-test", 24)

	testenv.BeforeEachFeature(testBeforeEach(runID))
	testenv.AfterEachFeature(testAfterEach(runID))

	return testenv
}
