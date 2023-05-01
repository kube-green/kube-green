package testutil

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/vladimirvivien/gexe"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

const (
	localBinRelativePath = "../../bin"
	envtestBin           = "setup-envtest"
)

type testEnvKey struct{}

func SetupEnvTest() env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		testEnv := &envtest.Environment{}

		e := gexe.New()
		version := getK8sVersionForEnvtest()
		assetsPath, err := findOrInstallTestEnv(e, version)
		if err != nil {
			return ctx, err
		}

		if err := os.Setenv("KUBEBUILDER_ASSETS", assetsPath); err != nil {
			return ctx, err
		}

		cfg, err := testEnv.Start()
		if err != nil {
			return ctx, err
		}
		client, err := klient.New(cfg)
		if err != nil {
			return ctx, err
		}
		c.WithClient(client)

		ctx = context.WithValue(ctx, testEnvKey{}, testEnv)

		return ctx, nil
	}
}

func StopEnvTest() env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		if err := os.Unsetenv("KUBEBUILDER_ASSETS"); err != nil {
			return ctx, err
		}
		testEnv, ok := ctx.Value(testEnvKey{}).(*envtest.Environment)
		if !ok {
			return ctx, fmt.Errorf("invalid environment in context")
		}
		return ctx, testEnv.Stop()
	}
}

func findOrInstallTestEnv(e *gexe.Echo, k8sVersion string) (string, error) {
	setupEnvCommand := getSetupEnvPath()
	if setupEnvCommand == "" {
		var err error
		if setupEnvCommand, err = installTestEnv(e); err != nil {
			return "", err
		}
	}

	p := e.RunProc(fmt.Sprintf("%s use -p path %s", setupEnvCommand, k8sVersion))
	if p.Err() != nil {
		return "", fmt.Errorf("failed to use envtest at version %s: %s", k8sVersion, p.Err())
	}
	return p.Result(), nil
}

func installTestEnv(e *gexe.Echo) (string, error) {
	localBin, err := getLocalBin()
	if err != nil {
		return "", err
	}

	p := e.
		SetEnv("GOBIN", localBin).
		RunProc("go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest")
	if p.Err() != nil {
		return "", fmt.Errorf("failed to install setup-envtest: %s", p.Err())
	}

	setupEnvPath := getSetupEnvPath()
	if setupEnvPath != "" {
		return setupEnvPath, nil
	}

	return "", fmt.Errorf("setup-envtest not available even after installation")
}

func getLocalBin() (string, error) {
	localPath, err := filepath.Abs(localBinRelativePath)
	if err != nil {
		return "", err
	}
	return localPath, nil
}

func getSetupEnvPath() string {
	localBin, err := getLocalBin()
	if err != nil {
		return ""
	}
	envtestBinPath := path.Join(localBin, envtestBin)
	if _, err := os.Stat(envtestBinPath); err == nil {
		return envtestBinPath
	}
	return ""
}

func getK8sVersionForEnvtest() string {
	version, _ := os.LookupEnv(kindVersionVariableName)
	return strings.TrimPrefix(version, "v")
}
