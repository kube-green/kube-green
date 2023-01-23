//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/kudobuilder/kuttl/pkg/apis/testharness/v1beta1"
	"github.com/kudobuilder/kuttl/pkg/test"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestSleepInfoE2E(t *testing.T) {
	e2eTests := features.New("kuttl").
		Assess("run e2e tests", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			harness := test.Harness{
				TestSuite: v1beta1.TestSuite{
					TestDirs: []string{"../e2e/"},
					Timeout:  180,
					Config: &v1beta1.RestConfig{
						RC: c.Client().RESTConfig(),
					},
					SkipDelete:        true,
					SkipClusterDelete: true,
				},
				T: t,
			}
			harness.Setup()
			harness.RunTests()
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			err := os.Remove("kubeconfig")
			require.NoError(t, err)
			return ctx
		}).Feature()

	testenv.Test(t, e2eTests)
}
