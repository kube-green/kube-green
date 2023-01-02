package sleepinfo

import (
	"context"
	"fmt"
	"os"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	testenv env.Environment
)

const (
	kindVersionVariableName = "KIND_K8S_VERSION"
	kindClusterName         = "kube-green-e2e"
	kindNodeImage           = "kindest/node"
)

func TestMain(m *testing.M) {
	testenv = env.New()
	runID := envconf.RandomName("kube-green-test", 24)

	testenv.BeforeEachFeature(func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		return createNSForTest(ctx, c, t, runID)
	})

	testenv.AfterEachFeature(func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		return deleteNSForTest(ctx, c, t, runID)
	})

	// Use pre-defined environment funcs to create a kind cluster prior to test run
	testenv.Setup(
		createKindClusterWithVersion(),
		getClusterVersion(),
		envfuncs.SetupCRDs("../../config/crd/bases", "*"),
	)

	testenv.Finish(
		envfuncs.TeardownCRDs("../../config/crd/bases", "*"),
		envfuncs.DestroyKindCluster(kindClusterName),
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}

// createNSForTest creates a random namespace with the runID as a prefix. It is stored in the context
// so that the deleteNSForTest routine can look it up and delete it.
func createNSForTest(ctx context.Context, cfg *envconf.Config, t *testing.T, runID string) (context.Context, error) {
	ns := envconf.RandomName(runID, 32)
	ctx = context.WithValue(ctx, nsKey(t), ns)

	cfg.WithNamespace(ns)

	t.Logf("Creating NS %v for test %v", ns, t.Name())
	nsObj := v1.Namespace{}
	nsObj.Name = ns
	nsObj.SetLabels(map[string]string{
		"app.kubernetes.io/managed-by": "kube-green-test",
	})
	return ctx, cfg.Client().Resources().Create(ctx, &nsObj)
}

// deleteNSForTest looks up the namespace corresponding to the given test and deletes it.
func deleteNSForTest(ctx context.Context, cfg *envconf.Config, t *testing.T, runID string) (context.Context, error) {
	ns := fmt.Sprint(ctx.Value(nsKey(t)))

	t.Logf("Deleting NS %v for test %v", ns, t.Name())
	nsObj := v1.Namespace{}
	nsObj.Name = ns
	return ctx, cfg.Client().Resources().Delete(ctx, &nsObj)
}

func nsKey(t *testing.T) string {
	return "NS-for-%v" + t.Name()
}

func createKindClusterWithVersion() env.Func {
	version, ok := os.LookupEnv(kindVersionVariableName)
	if !ok {
		return envfuncs.CreateKindCluster(kindClusterName)
	}
	fmt.Printf("kind use version %s", version)

	image := fmt.Sprintf("%s:%s", kindNodeImage, version)
	return envfuncs.CreateKindClusterWithConfig(kindClusterName, image, "../../kind-config.test.yaml")
}

func getClusterVersion() env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		discoveryClient, err := discovery.NewDiscoveryClientForConfig(c.Client().RESTConfig())
		if err != nil {
			return ctx, err
		}

		info, err := discoveryClient.ServerVersion()
		if err != nil {
			return ctx, err
		}

		fmt.Printf("cluster version: %s\n", info.String())

		return ctx, nil
	}
}
