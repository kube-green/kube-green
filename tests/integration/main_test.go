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
	"sigs.k8s.io/e2e-framework/support/kind"
	"sigs.k8s.io/e2e-framework/third_party/helm"
)

var (
	testenv env.Environment
)

const (
	kindClusterName                  = "kube-green-e2e"
	kubegreenTestImageRepository     = "localhost/kubegreen/kube-green"
	kubegreenTestImageTag            = "e2e-test"
	kindVersionVariableName          = "KIND_K8S_VERSION"
	kindNodeImage                    = "kindest/node"
	disableDeleteClusterVariableName = "DISABLE_DELETE_CLUSTER"
	installationModeVariableName     = "INSTALLATION_MODE"
	kubeGreenInstallationMode        = "helm"
	containerToolVariableName        = "CONTAINER_TOOL"
	containerToolDocker              = "docker"
	containerToolPodman              = "podman"
	defaultContainerTool             = containerToolDocker
)

func TestMain(m *testing.M) {
	testenv = env.New()
	runID := envconf.RandomName("kube-green-test", 24)

	testenv.BeforeEachFeature(func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		r, err := resources.New(c.Client().RESTConfig())
		require.NoError(t, err)
		kubegreenv1alpha1.AddToScheme(r.GetScheme())
		c = c.WithClient(c.Client())

		return testutil.CreateNamespace(ctx, c, t, runID)
	})

	testenv.AfterEachFeature(func(ctx context.Context, c *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		return testutil.DeleteNamespace(ctx, c, t, runID)
	})

	testenv.Setup(
		createKindClusterWithVersion(kindClusterName, "testdata/kind-config.test.yaml"),
		setContextOrPanic(kindClusterName),
		testutil.GetClusterVersion(),
		buildContainerImage(),
		loadImageToCluster(kindClusterName),
		installKubeGreen(),
	)

	testenv.Finish(
		envfuncs.ExportClusterLogs(kindClusterName, fmt.Sprintf("./tests-logs/kube-green-e2e-%s", runID)),
		envfuncs.TeardownCRDs("/tmp", "kube-green-e2e-test.yaml"),
		destroyKindCluster(kindClusterName),
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}

func getContainerTool() string {
	if tool, ok := os.LookupEnv(containerToolVariableName); ok {
		return tool
	}
	return defaultContainerTool
}

// getTestImage returns the test image name with the local registry prefix.
// Uses localhost registry to clearly indicate this is a local test image.
func getTestImage() string {
	return kubegreenTestImageRepository + ":" + kubegreenTestImageTag
}

func buildContainerImage() env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		containerTool := getContainerTool()
		image := getTestImage()
		e := gexe.New()
		p := e.NewProc("make build").SetWorkDir("../..").Run()
		if p.Err() != nil {
			return ctx, fmt.Errorf("make build: %s", p.Result())
		}
		p = e.RunProc(fmt.Sprintf(`%s build -t %s ../../`, containerTool, image))
		if p.Err() != nil {
			return ctx, fmt.Errorf("%s build %s: %s", containerTool, p.Err(), p.Result())
		}
		fmt.Printf("kube-green container image %s created with %s\n", image, containerTool)
		return ctx, nil
	}
}

func loadImageToCluster(clusterName string) env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		containerTool := getContainerTool()
		image := getTestImage()

		if containerTool == containerToolPodman {
			// Save image to tar archive
			archivePath := "/tmp/kube-green-e2e.tar"
			e := gexe.New()
			p := e.RunProc(fmt.Sprintf("%s save -o %s %s", containerToolPodman, archivePath, image))
			if p.Err() != nil {
				return ctx, fmt.Errorf("%s save: %s", containerToolPodman, p.Result())
			}

			// Load archive into kind using e2e-framework
			ctx, err := envfuncs.LoadImageArchiveToCluster(clusterName, archivePath)(ctx, c)

			// Clean up tar file
			os.Remove(archivePath)

			return ctx, err
		}

		// Docker: use existing e2e-framework function
		return envfuncs.LoadDockerImageToCluster(clusterName, image)(ctx, c)
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
		mode, ok := os.LookupEnv(installationModeVariableName)
		if !ok {
			mode = kubeGreenInstallationMode
		}
		switch mode {
		case "kustomize":
			fmt.Println("installing kube-green with kustomize")
			return installKubeGreenWithKustomize()(ctx, c)
		case "helm":
			fmt.Println("installing kube-green with helm")
			return installWithHelmChart()(ctx, c)
		default:
			return ctx, fmt.Errorf("installation mode %s not supported", mode)
		}
	}
}

func installKubeGreenWithKustomize() env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		ctx, err := installCertManager()(ctx, c)
		if err != nil {
			return ctx, err
		}

		ctx, err = envfuncs.SetupCRDs("/tmp", "kube-green-e2e-test.yaml")(ctx, c)
		if err != nil {
			return ctx, err
		}
		e := gexe.New()
		if p := e.RunProc("kubectl wait --for=condition=ready --timeout=160s pod -l app=kube-green -n kube-green"); p.Err() != nil {
			return ctx, fmt.Errorf("kubectl wait kube-green webhook %s: %s", p.Err(), p.Result())
		}
		// TODO: this sleep is because sometimes kube-green is flagged as ready but webhook
		// is not ready. We should investigate about it.
		time.Sleep(2 * time.Second)
		fmt.Println("kube-green running")
		return ctx, nil
	}
}

func installWithHelmChart() env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		manager := helm.New(c.KubeconfigFile())
		err := manager.RunInstall(
			helm.WithChart("../../charts/kube-green"),
			helm.WithNamespace("kube-green-system"),
			helm.WithArgs(
				"--set", fmt.Sprintf("manager.image.repository=%s", kubegreenTestImageRepository),
				"--set", fmt.Sprintf("manager.image.tag=%s", kubegreenTestImageTag),
				"--set", "certManager.enabled=false",
				"--set", "jobsCert.enabled=true",

				"--generate-name",
				"--create-namespace",
				"--wait",
			),
		)
		if err != nil {
			return nil, err
		}

		return ctx, nil
	}
}

// createKindClusterWithVersion create KinD cluster with a specific version
func createKindClusterWithVersion(clusterName, configPath string) env.Func {
	version, ok := os.LookupEnv(kindVersionVariableName)
	if !ok {
		return envfuncs.CreateCluster(kind.NewProvider(), clusterName)
	}
	fmt.Printf("kind use version %s\n", version)

	image := fmt.Sprintf("%s:%s", kindNodeImage, version)
	return envfuncs.CreateClusterWithConfig(kind.NewProvider(), clusterName, configPath, kind.WithImage(image))
}

// destroyKindCluster destroy KinD cluster with cluster name.
// If skipDeleteClusterFlag is set, it avoid the delete of the cluster
// (useful when running tests locally various times).
func destroyKindCluster(clusterName string) env.Func {
	if _, ok := os.LookupEnv(disableDeleteClusterVariableName); ok {
		return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
			return ctx, nil
		}
	}
	return envfuncs.DestroyCluster(clusterName)
}

func setContextOrPanic(kindClusterName string) env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		e := gexe.New()
		if p := e.RunProc(fmt.Sprintf("kubectl config use-context kind-%s", kindClusterName)); p.Err() != nil {
			// If the context is not found, we should exit as kind cluster setup is not correct.
			os.Exit(1)
			return ctx, fmt.Errorf("invalid context kind-%s set %s: %s", kindClusterName, p.Err(), p.Result())
		}

		return ctx, nil
	}
}
