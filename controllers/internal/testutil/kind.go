package testutil

import (
	"context"
	"fmt"
	"os"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
)

const (
	kindVersionVariableName          = "KIND_K8S_VERSION"
	kindNodeImage                    = "kindest/node"
	disableDeleteClusterVariableName = "DISABLE_DELETE_CLUSTER"
)

// CreateKindClusterWithVersion create KinD cluster with a specific version
func CreateKindClusterWithVersion(clusterName string) env.Func {
	version, ok := os.LookupEnv(kindVersionVariableName)
	if !ok {
		return envfuncs.CreateKindCluster(clusterName)
	}
	fmt.Printf("kind use version %s\n", version)

	image := fmt.Sprintf("%s:%s", kindNodeImage, version)
	return envfuncs.CreateKindClusterWithConfig(clusterName, image, "./testdata/kind-config.test.yaml")
}

// CreateKindClusterWithVersion destroy KinD cluster with cluster name.
// If skipDeleteClusterFlag is set, it avoid the delete of the cluster
// (useful when running tests locally various times).
func DestroyKindCluster(clusterName string) env.Func {
	if _, ok := os.LookupEnv(disableDeleteClusterVariableName); ok {
		return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
			return ctx, nil
		}
	}
	return envfuncs.DestroyKindCluster(clusterName)
}
