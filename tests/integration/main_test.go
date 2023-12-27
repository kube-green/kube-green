//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/testutil"
	"github.com/stretchr/testify/require"
	"github.com/vladimirvivien/gexe"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	testenv env.Environment
)

const (
	kindClusterName    = "kube-green-e2e"
	kubegreenTestImage = "kubegreen/kube-green:e2e-test"
)

func TestMain(m *testing.M) {
	testenv = env.New()
	runID := envconf.RandomName("kube-green-test", 24)

	testenv.BeforeEachFeature(func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		r, err := resources.New(c.Client().RESTConfig())
		require.NoError(t, err)
		kubegreenv1alpha1.AddToScheme(r.GetScheme())
		c = c.WithClient(c.Client())

		return testutil.CreateNSForTest(ctx, c, t, runID)
	})

	testenv.AfterEachFeature(func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		return testutil.DeleteNamespace(ctx, c, t, runID)
	})

	testenv.Setup(
		testutil.CreateKindClusterWithVersion(kindClusterName, "testdata/kind-config.test.yaml"),
		testutil.GetClusterVersion(),
		installCertManager(),
		buildDockerImage(kubegreenTestImage),
		envfuncs.LoadDockerImageToCluster(kindClusterName, kubegreenTestImage),
		installKubeGreen(),
	)

	testenv.Finish(
		envfuncs.ExportClusterLogs(kindClusterName, fmt.Sprintf("./tests-logs/kube-green-e2e-%s", runID)),
		envfuncs.TeardownCRDs("/tmp", "kube-green-e2e-test.yaml"),
		testutil.DestroyKindCluster(kindClusterName),
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}

func buildDockerImage(image string) env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		e := gexe.New()
		p := e.RunProc(fmt.Sprintf(`docker build -t %s ../../`, image))
		if p.Err() != nil {
			return ctx, fmt.Errorf("docker: build %s: %s", p.Err(), p.Result())
		}
		fmt.Printf("kube-green docker image %s created\n", image)
		return ctx, nil
	}
}

func installCertManager() env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		e := gexe.New()
		p := e.RunProc("kubectl apply -f https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml")
		if p.Err() != nil {
			return ctx, fmt.Errorf("kubectl: apply cert-manager %s: %s", p.Err(), p.Result())
		}
		p = e.RunProc("kubectl wait --timeout=120s --for=condition=ready pod -l app=cert-manager -n cert-manager")
		if p.Err() != nil {
			return ctx, fmt.Errorf("kubectl wait cert-manager %s: %s", p.Err(), p.Result())
		}
		p = e.RunProc("kubectl wait --timeout=120s --for=condition=ready pod -l app=cainjector -n cert-manager")
		if p.Err() != nil {
			return ctx, fmt.Errorf("kubectl wait cainjector %s: %s", p.Err(), p.Result())
		}
		p = e.RunProc("kubectl wait --timeout=120s --for=condition=ready pod -l app=webhook -n cert-manager")
		if p.Err() != nil {
			return ctx, fmt.Errorf("kubectl wait cert-manager webhook %s: %s", p.Err(), p.Result())
		}
		fmt.Println("cert-manager installed")
		return ctx, nil
	}
}

func installKubeGreen() env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		ctx, err := envfuncs.SetupCRDs("/tmp", "kube-green-e2e-test.yaml")(ctx, c)
		if err != nil {
			return ctx, err
		}
		e := gexe.New()
		if p := e.RunProc("kubectl wait --for=condition=ready --timeout=120s pod -l app=kube-green -n kube-green"); p.Err() != nil {
			return ctx, fmt.Errorf("kubectl wait kube-green webhook %s: %s", p.Err(), p.Result())
		}
		// TODO: this sleep is because sometimes kube-green is flag as ready but webhook
		// is not ready. We should investigate about it.
		time.Sleep(2 * time.Second)
		fmt.Printf("kube-green running\n")
		return ctx, nil
	}
}
