package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const KubeconfigLoadingEager = "Eager"
const KubeconfigLoadingLazy = "Lazy"

// Create embedded struct to implement custom DeepCopyInto method
type RestConfig struct {
	RC *rest.Config
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TestFile contains attributes of a single test file.
type TestFile struct {
	// The type meta object, should always be a GVK of kuttl.dev/v1beta1/TestFile.
	metav1.TypeMeta `json:",inline"`
	// Set labels or the test suite name.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Which test runs should this file be used in. Empty selector matches all test runs.
	TestRunSelector *metav1.LabelSelector `json:"testRunSelector,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TestSuite configures which tests should be loaded.
type TestSuite struct {
	// The type meta object, should always be a GVK of kuttl.dev/v1beta1/TestSuite or kuttl.dev/v1beta1/TestSuite.
	metav1.TypeMeta `json:",inline"`
	// Set labels or the test suite name.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Path to CRDs to install before running tests.
	CRDDir string `json:"crdDir"`
	// Paths to directories containing manifests to install before running tests.
	ManifestDirs []string `json:"manifestDirs"`
	// Directories containing test cases to run.
	TestDirs []string `json:"testDirs"`
	// Whether or not to start a local etcd and kubernetes API server for the tests.
	StartControlPlane bool `json:"startControlPlane"`
	// ControlPlaneArgs defaults to APIServerDefaultArgs from controller-runtime pkg/internal/testing/integration/internal/apiserver.go
	// this allows for control over the args, however these are not serialized from a TestSuite.yaml
	// deprecated and is no longer used!
	// TODO: remove after v0.16.0 (provide warning message until then)
	ControlPlaneArgs []string `json:"controlPlaneArgs"`
	// AttachControlPlaneOutput if true, attaches control plane logs (api-server, etcd) into stdout. This is useful for debugging.
	// defaults to false
	AttachControlPlaneOutput bool `json:"attachControlPlaneOutput"`
	// Whether to start a local kind cluster for the tests
	// TODO: fix json tag and remove nolint when bumping API version.
	StartKIND bool `json:"startKIND"` //nolint:tagliatelle
	// Path to the KIND configuration file to use.
	KINDConfig string `json:"kindConfig"`
	// KIND context to use.
	KINDContext string `json:"kindContext"`
	// If set, each node defined in the kind configuration will have a docker named volume mounted into it to persist
	// pulled container images across test runs.
	KINDNodeCache bool `json:"kindNodeCache"`
	// Containers to load to each KIND node prior to running the tests.
	KINDContainers []string `json:"kindContainers"`
	// If set, do not delete the resources after running the tests (implies SkipClusterDelete).
	SkipDelete bool `json:"skipDelete"`
	// If set, do not delete the mocked control plane or kind cluster.
	SkipClusterDelete bool `json:"skipClusterDelete"`
	// Override the default timeout of 30 seconds (in seconds).
	// +kubebuilder:validation:Format:=int64
	Timeout int `json:"timeout"`
	// The maximum number of tests to run at once (default: 8).
	// +kubebuilder:validation:Format:=int64
	Parallel int `json:"parallel"`
	// The directory to output artifacts to (current working directory if not specified).
	ArtifactsDir string `json:"artifactsDir"`
	// Commands to run prior to running the tests.
	Commands []Command `json:"commands"`

	// ReportFormat determines test report format (JSON|XML|nil) nil == no report
	// maps to report.Type, however we don't want generated.deepcopy to have reference to it.
	ReportFormat string `json:"reportFormat"`

	// ReportName defines the name of report to create.  It defaults to "kuttl-report" and is not used unless ReportFormat is defined.
	ReportName string `json:"reportName"`

	// ReportGranularity defines the granularity at which failures are reported. It defaults to "step".
	ReportGranularity string `json:"reportGranularity"`

	// Namespace defines the namespace to use for tests
	// The value "" means to auto-generate tests namespaces, these namespaces will be created and removed for each test
	// Any other value is the name of the namespace to use.  This namespace will be created if it does not exist and will
	// be removed it was created (unless --skipDelete is used).
	Namespace string `json:"namespace"`
	// Suppress is used to suppress logs
	Suppress []string `json:"suppress"`

	Config *RestConfig `json:"config,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TestStep settings to apply to a test step.go
type TestStep struct {
	// The type meta object, should always be a GVK of kuttl.dev/v1beta1/TestStep or kuttl.dev/v1beta1/TestStep.
	metav1.TypeMeta `json:",inline"`
	// Override the default metadata. Set labels or override the test step name.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Format:=int64
	Index int `json:"index,omitempty"`

	// Apply, Assert and Error lists of files or directories to use in the test step.
	// Useful to reuse a number of applies across tests / test steps.
	// all relative paths are relative to the folder the TestStep is defined in.
	Apply  []string `json:"apply,omitempty"`
	Assert []string `json:"assert,omitempty"`
	Error  []string `json:"error,omitempty"`

	// Objects to delete at the beginning of the test step.
	Delete []ObjectReference `json:"delete,omitempty"`

	// Indicates that this is a unit test - safe to run without a real Kubernetes cluster.
	UnitTest bool `json:"unitTest"`

	// Commands to run prior at the beginning of the test step.
	Commands []Command `json:"commands"`

	// Allowed environment labels
	// Disallowed environment labels

	// Kubeconfig to use when applying and asserting for this step.
	Kubeconfig string `json:"kubeconfig,omitempty"`

	// Specifies the mode for loading Kubeconfig: Eager/Lazy. Defaults to Eager.
	// +kubebuilder:default=Eager
	// +kubebuilder:validation:Enum=Eager;Lazy
	KubeconfigLoading string `json:"kubeconfigLoading,omitempty"`

	// Specifies the context to use from the Kubeconfig.
	Context string `json:"context,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TestAssert represents the settings needed to verify the result of a test step.
type TestAssert struct {
	// The type meta object, should always be a GVK of  kuttl.dev/v1beta1/TestAssert or kuttl.dev/v1beta1/TestAssert.
	metav1.TypeMeta `json:",inline"`
	// Override the default metadata. Set labels or override the test step name.
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Override the default timeout of 30 seconds (in seconds).
	Timeout int `json:"timeout"`
	// Collectors is a set of pod log collectors fired on an assert failure
	Collectors []*TestCollector `json:"collectors,omitempty"`
	// Commands is a set of commands to be run as assertions for the current step
	Commands []TestAssertCommand `json:"commands,omitempty"`

	ResourceRefs []TestResourceRef `json:"resourceRefs,omitempty"`

	AssertAny []*Assertion `json:"assertAny,omitempty"`
	AssertAll []*Assertion `json:"assertAll,omitempty"`
}

// TestAssertCommand an assertion based on the result of the execution of a command
type TestAssertCommand struct {
	// The command and argument to run as a string.
	Command string `json:"command"`
	// If set, the `--namespace` flag will be appended to the command with the namespace to use.
	Namespaced bool `json:"namespaced"`
	// Ability to run a shell script from TestStep (without a script file)
	// namespaced and command should not be used with script.  namespaced is ignored and command is an error.
	// env expansion is depended upon the shell but ENV is passed to the runtime env.
	Script string `json:"script"`
	// If set, the output from the command is NOT logged.  Useful for sensitive logs or to reduce noise.
	SkipLogOutput bool `json:"skipLogOutput"`
}

// ObjectReference is a Kubernetes object reference with added labels to allow referencing
// objects by label.
type ObjectReference struct {
	corev1.ObjectReference `json:",inline"`
	// Labels to match on.
	Labels map[string]string `json:"labels"`
}

// Command describes a command to run as a part of a test step or suite.
type Command struct {
	// The command and argument to run as a string.
	Command string `json:"command"`
	// If set, the `--namespace` flag will be appended to the command with the namespace to use.
	Namespaced bool `json:"namespaced"`
	// Ability to run a shell script from TestStep (without a script file)
	// namespaced and command should not be used with script.  namespaced is ignored and command is an error.
	// env expansion is depended upon the shell but ENV is passed to the runtime env.
	Script string `json:"script"`
	// If set, exit failures (`exec.ExitError`) will be ignored. `exec.Error` are NOT ignored.
	IgnoreFailure bool `json:"ignoreFailure"`
	// If set, the command is run in the background.
	Background bool `json:"background"`
	// Override the TestSuite timeout for this command (in seconds).
	Timeout int `json:"timeout"`
	// If set, the output from the command is NOT logged.  Useful for sensitive logs or to reduce noise.
	SkipLogOutput bool `json:"skipLogOutput"`
}

// TestCollector are post assert / error commands that allow for the collection of information sent to the test log.
// Type can be pod, command or event.  For backward compatibility, pod is default and doesn't need to be specified
// For pod, At least one of `pod` or `selector` is required.
// For command, Command must be specified and Type can be == "command" but no other fields are valid
// For event, Type must be == "events" and Namespace and Name can be specified, if no ns or name, the default events are provided.  If no name, than all events for that ns are provided.
type TestCollector struct {
	// Type is a collector type which is pod, command or events
	// command is default type if command field is not empty
	// misconfiguration will lead to warning message in the logs
	Type string `json:"type,omitempty"`
	// The pod name to access logs.
	Pod string `json:"pod,omitempty"`
	// namespace to use. The current test namespace will be used by default.
	Namespace string `json:"namespace,omitempty"`
	// Container in pod to get logs from else --all-containers is used.
	Container string `json:"container,omitempty"`
	// Selector is a label query to select pod.
	Selector string `json:"selector,omitempty"`
	// Tail is the number of last lines to collect from pods. If omitted or zero,
	// then the default is 10 if you use a selector, or -1 (all) if you use a pod name.
	// This matches default behavior of `kubectl logs`.
	Tail int `json:"tail,omitempty"`
	// Cmd is a command to run for collection.  It requires an empty Type or Type=command
	Cmd string `json:"command,omitempty"`
}

type TestResourceRef struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

type Assertion struct {
	CELExpression string `json:"celExpr,omitempty"`
}

// DefaultKINDContext defines the default kind context to use.
const DefaultKINDContext = "kind"

func (in *RestConfig) DeepCopyInto(out *RestConfig) {
	out.RC = rest.CopyConfig(in.RC)
}
