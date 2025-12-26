package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	volumetypes "github.com/docker/docker/api/types/volume"
	docker "github.com/docker/docker/client"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	kindConfig "sigs.k8s.io/kind/pkg/apis/config/v1alpha4"

	harness "github.com/kube-green/kuttl/pkg/apis/testharness/v1beta1"
	"github.com/kube-green/kuttl/pkg/file"
	"github.com/kube-green/kuttl/pkg/http"
	"github.com/kube-green/kuttl/pkg/kubernetes"
	"github.com/kube-green/kuttl/pkg/report"
	testutils "github.com/kube-green/kuttl/pkg/test/utils"
)

// Harness loads and runs tests based on the configuration provided.
type Harness struct {
	TestSuite harness.TestSuite
	T         *testing.T

	logger        testutils.Logger
	managerStopCh chan struct{}
	config        *rest.Config
	docker        testutils.DockerClient
	client        client.Client
	dclient       discovery.DiscoveryInterface
	env           *envtest.Environment
	kind          *kind
	tempPath      string
	clientLock    sync.Mutex
	configLock    sync.Mutex
	stopping      bool
	bgProcesses   []*exec.Cmd
	report        *report.Testsuites
	RunLabels     labels.Set
}

// LoadTests loads all of the tests in a given directory.
func (h *Harness) LoadTests(dir string) ([]*Case, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var tests []*Case

	timeout := h.GetTimeout()
	h.T.Logf("going to run test suite with timeout of %d seconds for each step", timeout)

	for _, file := range files {
		if !file.IsDir() {
			continue
		}

		tests = append(tests, &Case{
			Timeout:            timeout,
			Steps:              []*Step{},
			Name:               file.Name(),
			PreferredNamespace: h.TestSuite.Namespace,
			Dir:                filepath.Join(dir, file.Name()),
			SkipDelete:         h.TestSuite.SkipDelete,
			Suppress:           h.TestSuite.Suppress,
			RunLabels:          h.RunLabels,
		})
	}

	return tests, nil
}

// GetLogger returns an initialized test logger.
func (h *Harness) GetLogger() testutils.Logger {
	if h.logger == nil {
		h.logger = testutils.NewTestLogger(h.T, "")
	}

	return h.logger
}

// GetTimeout returns the configured timeout for the test suite.
func (h *Harness) GetTimeout() int {
	timeout := 30
	if h.TestSuite.Timeout != 0 {
		timeout = h.TestSuite.Timeout
	}
	return timeout
}

// RunKIND starts a KIND cluster.
func (h *Harness) RunKIND() (*rest.Config, error) {
	if h.kind == nil {
		var err error

		err = h.initTempPath()
		if err != nil {
			return nil, err
		}

		kind := newKind(h.TestSuite.KINDContext, h.kubeconfigPath(), h.GetLogger())
		h.kind = &kind

		if h.kind.IsRunning() {
			// we don't take over an existing kind cluster for --start-kind
			// which means we do not stop that cluster.  User will either need to switch to existing cluster or stop it.
			h.kind = nil
			msg := "KIND is already running, unable to start"
			h.T.Log(msg)
			return nil, errors.New(msg)
		}

		kindCfg := &kindConfig.Cluster{}

		if h.TestSuite.KINDConfig != "" {
			h.T.Logf("Loading KIND config from %s", h.TestSuite.KINDConfig)
			var err error
			kindCfg, err = h.loadKindConfig(h.TestSuite.KINDConfig)
			if err != nil {
				return nil, err
			}
		}

		dockerClient, err := h.DockerClient()
		if err != nil {
			return nil, err
		}

		// Determine the correct API version to use with the user's Docker client.
		dockerClient.NegotiateAPIVersion(context.TODO())

		h.addNodeCaches(dockerClient, kindCfg)

		h.T.Log("Starting KIND cluster")
		if err := h.kind.Run(kindCfg); err != nil {
			return nil, err
		}

		if err := h.kind.AddContainers(dockerClient, h.TestSuite.KINDContainers, h.T); err != nil {
			return nil, err
		}
	}

	return clientcmd.BuildConfigFromFlags("", h.kubeconfigPath())
}

// initTempPath creates the temp folder if needed.
// various parts of system may need it, starting with kind, or working with tar test suites
func (h *Harness) initTempPath() (err error) {
	if h.tempPath == "" {
		h.tempPath, err = os.MkdirTemp("", "kuttl")
		h.T.Log("temp folder created", h.tempPath)
	}
	return err
}

func (h *Harness) addNodeCaches(dockerClient testutils.DockerClient, kindCfg *kindConfig.Cluster) {
	if !h.TestSuite.KINDNodeCache {
		return
	}

	// add a default node if there are none specified.
	if len(kindCfg.Nodes) == 0 {
		kindCfg.Nodes = append(kindCfg.Nodes, kindConfig.Node{})
	}

	if h.TestSuite.KINDContext == "" {
		h.TestSuite.KINDContext = harness.DefaultKINDContext
	}

	for index := range kindCfg.Nodes {
		volume, err := dockerClient.VolumeCreate(context.TODO(), volumetypes.CreateOptions{
			Driver: "local",
			Name:   fmt.Sprintf("%s-%d", h.TestSuite.KINDContext, index),
		})
		if err != nil {
			h.T.Log("error creating volume for node", err)
			continue
		}

		h.T.Log("node mount point", volume.Mountpoint)
		kindCfg.Nodes[index].ExtraMounts = append(kindCfg.Nodes[index].ExtraMounts, kindConfig.Mount{
			ContainerPath: "/var/lib/containerd",
			HostPath:      volume.Mountpoint,
		})
	}
}

// RunTestEnv starts a Kubernetes API server and etcd server for use in the
// tests and returns the Kubernetes configuration.
func (h *Harness) RunTestEnv() (*rest.Config, error) {
	started := time.Now()

	testenv, err := kubernetes.StartTestEnvironment(h.TestSuite.AttachControlPlaneOutput)
	if err != nil {
		return nil, err
	}

	h.T.Logf("started test environment (kube-apiserver and etcd) in %v, with following options:\n%s",
		time.Since(started),
		strings.Join(testenv.Environment.ControlPlane.GetAPIServer().Configure().AsStrings(nil), "\n"))
	h.env = testenv.Environment

	return testenv.Config, nil
}

// Config returns the current Kubernetes configuration - either from the environment
// or from the created temporary control plane.
// As a side effect, on first successful call this method also writes a kubernetes client config file in YAML format
// to a file called "kubeconfig" in the current directory.
func (h *Harness) Config() (*rest.Config, error) {
	h.configLock.Lock()
	defer h.configLock.Unlock()

	if h.config != nil {
		return h.config, nil
	}

	var err error
	switch {
	case h.TestSuite.Config != nil:
		h.T.Log("running tests with passed rest config.")
		h.config = h.TestSuite.Config.RC
	case h.TestSuite.StartControlPlane:
		h.T.Log("running tests with a mocked control plane (kube-apiserver and etcd).")
		h.config, err = h.RunTestEnv()
	case h.TestSuite.StartKIND:
		h.T.Log("running tests with KIND.")
		h.config, err = h.RunKIND()
	default:
		h.T.Log("running tests using configured kubeconfig.")
		h.config, err = kubernetes.GetConfig()
		if err != nil {
			return nil, err
		}
		h.config.WarningHandler = rest.NewWarningWriter(os.Stderr, rest.WarningWriterOptions{Deduplicate: true})
	}
	if err != nil {
		return nil, err
	}

	// Newly started clusters aren't ready until default service account is ready.
	// We need to wait until one is present. Otherwise, we sometimes hit an error such as:
	//   error looking up service account <namespace>/default: serviceaccount "default" not found
	//
	// We avoid doing this for the mocked control plane case (because in that case the default service
	// account is not provided anyway?)
	// We still do this when running inside a cluster, because the cluster kuttl is pointed *at* might
	// be different from the cluster it is running *in*, and it does not hurt when it is the same cluster.
	if !h.TestSuite.StartControlPlane {
		if err := h.waitForFunctionalCluster(); err != nil {
			return nil, err
		}
		h.T.Logf("Successful connection to cluster at: %s", h.config.Host)
	}

	// The creation of the "kubeconfig" is necessary for out of cluster execution of kubectl,
	// as well as in-cluster when the supplied KUBECONFIG is some *other* cluster.
	f, err := os.Create("kubeconfig")
	if err != nil {
		return nil, err
	}

	defer f.Close()

	return h.config, kubernetes.Kubeconfig(h.config, f)
}

func (h *Harness) waitForFunctionalCluster() error {
	err := kubernetes.WaitForSA(h.config, "default", "default")
	if err == nil {
		return nil
	}
	// if there is a namespace provided but no "default"/"default" SA found, also check a SA in the provided NS
	if h.TestSuite.Namespace != "" {
		tempErr := kubernetes.WaitForSA(h.config, "default", h.TestSuite.Namespace)
		if tempErr == nil {
			return nil
		}
	}
	// either way, return the first "default"/"default" error
	return err
}

// Client returns the current Kubernetes client for the test harness.
func (h *Harness) Client(forceNew bool) (client.Client, error) {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	if h.client != nil && !forceNew {
		return h.client, nil
	}

	cfg, err := h.Config()
	if err != nil {
		return nil, err
	}

	h.client, err = kubernetes.NewRetryClient(cfg, client.Options{
		Scheme: kubernetes.Scheme(),
	})
	return h.client, err
}

// DiscoveryClient returns the current Kubernetes discovery client for the test harness.
func (h *Harness) DiscoveryClient() (discovery.DiscoveryInterface, error) {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	if h.dclient != nil {
		return h.dclient, nil
	}

	cfg, err := h.Config()
	if err != nil {
		return nil, err
	}

	h.dclient, err = discovery.NewDiscoveryClientForConfig(cfg)
	return h.dclient, err
}

// DockerClient returns the Docker client to use for the test harness.
func (h *Harness) DockerClient() (testutils.DockerClient, error) {
	if h.docker != nil {
		return h.docker, nil
	}

	var err error
	h.docker, err = docker.NewClientWithOpts(docker.FromEnv)
	return h.docker, err
}

// RunTests should be called from within a Go test (t) and launches all of the KUTTL integration
// tests at dir.
func (h *Harness) RunTests() {
	// cleanup after running tests
	h.T.Cleanup(h.Stop)
	h.T.Log("running tests")

	testDirs := h.testPreProcessing()

	//todo: testsuite + testsuites (extend case to have what we need (need testdir here)
	// TestSuite is a TestSuiteCollection and should be renamed for v1beta2
	realTestSuite := make(map[string][]*Case)
	for _, testDir := range testDirs {
		tempTests, err := h.LoadTests(testDir)
		if err != nil {
			h.T.Fatal(err)
		}
		h.T.Logf("testsuite: %s has %d tests", testDir, len(tempTests))
		// array of test cases tied to testsuite (by testdir)
		realTestSuite[testDir] = tempTests
	}

	h.T.Run("harness", func(t *testing.T) {
		for testDir, tests := range realTestSuite {
			suiteReport := h.NewSuiteReport(testDir)
			for _, test := range tests {
				test.GetClient = h.Client
				test.GetDiscoveryClient = h.DiscoveryClient

				t.Run(test.Name, func(t *testing.T) {
					// testing.T.Parallel may block, so run it before we read time for our
					// elapsed time calculations.
					t.Parallel()

					test.Logger = testutils.NewTestLogger(t, test.Name)

					if err := test.LoadTestSteps(); err != nil {
						t.Fatal(err)
					}

					test.Run(t, suiteReport.NewTestReporter(test.Name))
				})
			}
		}
	})

	h.T.Log("run tests finished")
}

// testPreProcessing provides preprocessing bring all tests suites local if there are any refers to URLs
func (h *Harness) testPreProcessing() []string {
	testDirs := []string{}
	// preprocessing step
	for _, dir := range h.TestSuite.TestDirs {
		if http.IsURL(dir) {
			err := h.initTempPath()
			if err != nil {
				h.T.Fatal(err)
			}
			client := http.NewClient()
			h.T.Logf("downloading %s", dir)
			// fresh temp dir created for each download to prevent overwriting
			folder, err := os.MkdirTemp(h.tempPath, filepath.Base(dir))
			if err != nil {
				h.T.Fatal(err)
			}
			filePath, err := client.DownloadFile(dir, folder)
			if err != nil {
				h.T.Fatal(err)
			}
			err = file.UntarInPlace(filePath)
			if err != nil {
				h.T.Fatal(err)
			}
			testDirs = append(testDirs, file.TrimExt(filePath))
		} else {
			testDirs = append(testDirs, dir)
		}
	}
	return testDirs
}

// Run the test harness - start the control plane and then run the tests.
func (h *Harness) Run() {
	// capture ctrl+c and provide clean up
	go func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, os.Interrupt)
		sig := <-sigchan
		h.Stop()
		h.T.Log("failed with", sig)
		os.Exit(-1)
	}()

	h.Setup()
	h.RunTests()
}

// Setup spins up the test env based on configuration
// It can be used to start env which can than be modified prior to running tests, otherwise use Run().
func (h *Harness) Setup() {
	h.report = report.NewSuiteCollection(h.TestSuite.Name)
	h.T.Log("starting setup")

	cl, err := h.Client(false)
	if err != nil {
		h.fatal(fmt.Errorf("fatal error getting client: %v", err))
	}

	dClient, err := h.DiscoveryClient()
	if err != nil {
		h.fatal(fmt.Errorf("fatal error getting discovery client: %v", err))
	}

	// Install CRDs
	crdKinds := []runtime.Object{
		kubernetes.NewResource("apiextensions.k8s.io/v1", "CustomResourceDefinition", "", ""),
		kubernetes.NewResource("apiextensions.k8s.io/v1beta1", "CustomResourceDefinition", "", ""),
	}
	crds, err := kubernetes.InstallManifests(context.TODO(), cl, dClient, h.TestSuite.CRDDir, crdKinds...)
	if err != nil {
		h.fatal(fmt.Errorf("fatal error installing crds: %v", err))
	}

	if err := envtest.WaitForCRDs(h.config, crds, envtest.CRDInstallOptions{
		PollInterval: 100 * time.Millisecond,
		MaxTime:      10 * time.Second,
	}); err != nil {
		h.fatal(fmt.Errorf("fatal error waiting for crds: %v", err))
	}

	// Create a new client to bust the client's CRD cache.
	cl, err = h.Client(true)
	if err != nil {
		h.fatal(fmt.Errorf("fatal error getting client after crd update: %v", err))
	}

	// Install required manifests.
	for _, manifestDir := range h.TestSuite.ManifestDirs {
		if _, err := kubernetes.InstallManifests(context.TODO(), cl, dClient, manifestDir); err != nil {
			h.fatal(fmt.Errorf("fatal error installing manifests: %v", err))
		}
	}
	bgs, err := testutils.RunCommands(context.TODO(), h.GetLogger(), "default", h.TestSuite.Commands, "", h.TestSuite.Timeout, "")
	// assign any background processes first for cleanup in case of any errors
	h.bgProcesses = append(h.bgProcesses, bgs...)
	if err != nil {
		h.fatal(fmt.Errorf("fatal error running commands: %v", err))
	}
}

// Stop the test environment and clean up the harness.
func (h *Harness) Stop() {
	h.T.Log("cleaning up")
	if h.managerStopCh != nil {
		close(h.managerStopCh)
		h.managerStopCh = nil
	}

	if h.kind != nil {
		logDir := filepath.Join(h.TestSuite.ArtifactsDir, fmt.Sprintf("kind-logs-%d", time.Now().Unix()))

		h.T.Log("collecting cluster logs to", logDir)

		if err := h.kind.CollectLogs(logDir); err != nil {
			h.T.Log("error collecting kind cluster logs", err)
		}
	}

	if h.bgProcesses != nil {
		for _, p := range h.bgProcesses {
			h.T.Logf("killing process %q", p)
			err := p.Process.Kill()
			if err != nil {
				h.T.Logf("bg process: %q kill error %v", p, err)
			}
			ps, err := p.Process.Wait()
			if err != nil {
				h.T.Logf("bg process: %q kill wait error %v", p, err)
			}
			if ps != nil {
				h.T.Logf("bg process: %q exit code %v", p, ps.ExitCode())
			}
		}
	}

	h.Report()

	if h.TestSuite.SkipClusterDelete {
		cwd, err := os.Getwd()
		if err != nil {
			h.T.Logf("issue getting work directory %v", err)
		}
		kubeconfig := filepath.Join(cwd, "kubeconfig")

		h.T.Log("skipping cluster tear down")
		h.T.Logf("to connect to the cluster, run: export KUBECONFIG=\"%s\"", kubeconfig)

		return
	}

	if h.env != nil {
		h.T.Log("tearing down mock control plane")
		if err := h.env.Stop(); err != nil {
			h.T.Log("error tearing down mock control plane", err)
		}

		h.env = nil
	}

	h.T.Logf("removing temp folder: %q", h.tempPath)
	if err := os.RemoveAll(h.tempPath); err != nil {
		h.T.Log("error removing temporary directory", err)
	}

	if h.kind != nil {
		h.T.Log("tearing down kind cluster")
		if err := h.kind.Stop(); err != nil {
			h.T.Log("error tearing down kind cluster", err)
		}

		h.kind = nil
	}
}

// wraps Test.Fatal in order to clean up harness
// fatal should NOT be used with a go routine, it is not thread safe
func (h *Harness) fatal(err error) {
	// clean up on fatal in setup
	if !h.stopping {
		h.report.SetFailure(err.Error())
		// stopping prevents reentry into h.Stop
		h.stopping = true
		h.Stop()
	}
	h.T.Fatal(err)
}

func (h *Harness) kubeconfigPath() string {
	return filepath.Join(h.tempPath, "kubeconfig")
}

// Report defines the report phase of the kuttl tests.  If report format is nil it is skipped.
// otherwise it will provide a json or xml format report of tests in a junit format.
func (h *Harness) Report() {
	if len(h.TestSuite.ReportFormat) == 0 {
		return
	}
	if err := h.report.Report(h.TestSuite.ArtifactsDir, h.reportName(), report.Type(h.TestSuite.ReportFormat)); err != nil {
		h.fatal(fmt.Errorf("fatal error writing report: %v", err))
	}
}

// NewSuiteReport creates and assigns a TestSuite to the TestSuites (then returns the suite),
func (h *Harness) NewSuiteReport(name string) *report.Testsuite {
	suite := report.NewSuite(name, h.TestSuite.ReportGranularity)
	h.report.AddTestSuite(suite)
	return suite
}

// reportName returns the configured ReportName.
func (h *Harness) reportName() string {
	if h.TestSuite.ReportName != "" {
		return h.TestSuite.ReportName
	}
	return "kuttl-report"
}

func (h *Harness) loadKindConfig(path string) (*kindConfig.Cluster, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cluster := &kindConfig.Cluster{}

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.SetStrict(true)

	if err := decoder.Decode(cluster); err != nil {
		return nil, err
	}
	if !IsMinVersion(cluster.APIVersion) {
		h.T.Logf("Warning: %q in %s is not a supported version.\n", cluster.APIVersion, path)
	}
	return cluster, nil
}
