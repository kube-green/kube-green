package sleepinfo

import (
	"context"
	"os"
	"testing"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/testutil"
	"k8s.io/client-go/rest"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	testenv env.Environment
	envTest *envtest.Environment
)

// This will cleanup envTest also if some test panics.
// It is a workaround for this issue: https://github.com/kubernetes-sigs/e2e-framework/issues/219
func TestStartCleanupEnvTest(t *testing.T) {
	t.Cleanup(func() {
		envTest.Stop()
	})
}

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

	var cfg *rest.Config
	var err error
	envTest, cfg, err = testutil.StartEnvTest()
	if err != nil {
		panic(err)
	}

	testenv.Setup(
		testutil.SetupEnvTest(envTest, cfg),
		testutil.GetClusterVersion(),
		testutil.SetupCRDs("../../../config/crd/bases", "*"),
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}
