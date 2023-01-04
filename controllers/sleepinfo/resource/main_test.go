package resource

import (
	"context"
	"os"
	"testing"

	"github.com/kube-green/kube-green/controllers/internal/testutil"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	testenv env.Environment
)

const (
	kindClusterName = "kube-green-resource"
)

func TestMain(m *testing.M) {
	testenv = env.New()
	runID := envconf.RandomName("kube-green-resource", 24)

	testenv.BeforeEachFeature(func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		return testutil.CreateNSForTest(ctx, c, t, runID)
	})

	testenv.AfterEachFeature(func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		return testutil.DeleteNamespace(ctx, c, t, runID)
	})

	testenv.Setup(
		testutil.CreateKindClusterWithVersion(kindClusterName),
		testutil.GetClusterVersion(),
	)

	testenv.Finish(
		testutil.DestroyKindCluster(kindClusterName),
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}
