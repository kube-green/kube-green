package sleepinfo

import (
	"context"
	"os"
	"testing"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/testutil"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	testenv env.Environment
)

func TestMain(m *testing.M) {
	testenv = env.New()
	runID := envconf.RandomName("kube-green-test", 24)

	testenv.BeforeEachFeature(func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		r, err := resources.New(c.Client().RESTConfig())
		require.NoError(t, err)
		err = kubegreenv1alpha1.AddToScheme(r.GetScheme())
		require.NoError(t, err)
		c = c.WithClient(c.Client())

		return testutil.CreateNamespace(ctx, c, t, runID)
	})

	testenv.AfterEachFeature(func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		return testutil.DeleteNamespace(ctx, c, t, runID)
	})

	testenv.Setup(
		testutil.SetupEnvTest(),
		testutil.GetClusterVersion(),
		testutil.SetupCRDs("../../../config/crd/bases", "*"),
	)

	testenv.Finish(
		testutil.StopEnvTest(),
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}
