package testutil

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vladimirvivien/gexe"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

const (
	localBinRelativePath = "../../bin"
	envtestBin           = "setup-envtest"
)

var (
	_, b, _, _ = runtime.Caller(0)
	basepath   = filepath.Dir(b)
)

func StartEnvTest(config *envconf.Config) (*envtest.Environment, error) {
	testEnv := &envtest.Environment{}

	e := gexe.New()
	version := getK8sVersionForEnvtest()
	assetsPath, err := findOrInstallTestEnv(e, version)
	if err != nil {
		return testEnv, err
	}

	if err := os.Setenv("KUBEBUILDER_ASSETS", assetsPath); err != nil {
		return testEnv, err
	}

	cfg, err := testEnv.Start()
	if err != nil {
		return testEnv, err
	}

	client, err := klient.New(cfg)
	if err != nil {
		return testEnv, err
	}
	config.WithClient(client)

	_, err = GetClusterVersion()(context.TODO(), config)
	if err != nil {
		return nil, err
	}

	return testEnv, nil
}

func StopEnvTest(testEnv *envtest.Environment) error {
	if err := os.Unsetenv("KUBEBUILDER_ASSETS"); err != nil {
		return err
	}
	return testEnv.Stop()
}

func findOrInstallTestEnv(e *gexe.Echo, k8sVersion string) (string, error) {
	setupEnvCommand := getSetupEnvPath()
	if setupEnvCommand == "" {
		var err error
		if setupEnvCommand, err = installTestEnv(e); err != nil {
			return "", err
		}
	}

	cmd := fmt.Sprintf("%s use -p path --bin-dir %s %s", setupEnvCommand, getLocalBin(), k8sVersion)
	p := e.RunProc(cmd)
	if p.Err() != nil {
		return "", fmt.Errorf("failed to use envtest at version %s: %s. Command: %s", k8sVersion, p.Err(), cmd)
	}
	return p.Result(), nil
}

func installTestEnv(e *gexe.Echo) (string, error) {
	p := e.
		SetEnv("GOBIN", getLocalBin()).
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

func getLocalBin() string {
	localPath := filepath.Join(basepath, localBinRelativePath)
	return localPath
}

func getSetupEnvPath() string {
	localBin := getLocalBin()
	envtestBinPath := path.Join(localBin, envtestBin)
	if _, err := os.Stat(envtestBinPath); err == nil {
		return envtestBinPath
	}
	return ""
}

const lengthOfSemverVersionWithPatches = 3

func getK8sVersionForEnvtest() string {
	version, _ := os.LookupEnv(kindVersionVariableName)

	splitVersion := strings.Split(strings.TrimLeft(version, "v"), ".")
	if len(splitVersion) < lengthOfSemverVersionWithPatches {
		return version
	}

	return fmt.Sprintf("%s.%s", splitVersion[0], splitVersion[1])
}
