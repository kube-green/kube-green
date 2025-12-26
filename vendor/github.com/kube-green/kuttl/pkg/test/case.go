package test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/thoas/go-funk"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kube-green/kuttl/pkg/apis/testharness/v1beta1"
	"github.com/kube-green/kuttl/pkg/kubernetes"
	"github.com/kube-green/kuttl/pkg/report"
	testutils "github.com/kube-green/kuttl/pkg/test/utils"
	eventutils "github.com/kube-green/kuttl/pkg/test/utils/events"
	"github.com/kube-green/kuttl/pkg/test/utils/files"
)

// Case contains all the test steps and the Kubernetes client and other global configuration
// for a test.
type Case struct {
	Steps              []*Step
	Name               string
	Dir                string
	SkipDelete         bool
	Timeout            int
	PreferredNamespace string
	RunLabels          labels.Set

	GetClient          func(forceNew bool) (client.Client, error)
	GetDiscoveryClient func() (discovery.DiscoveryInterface, error)

	Logger testutils.Logger
	// Suppress is used to suppress logs
	Suppress []string
}

type namespace struct {
	Name        string
	AutoCreated bool
}

func (c *Case) deleteNamespace(cl client.Client, ns *namespace) error {
	if !ns.AutoCreated {
		c.Logger.Log("Skipping deletion of user-supplied namespace:", ns.Name)
		return nil
	}

	c.Logger.Log("Deleting namespace:", ns.Name)

	ctx := context.Background()
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(c.Timeout)*time.Second)
		defer cancel()
	}

	nsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns.Name,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Namespace",
		},
	}

	if err := cl.Delete(ctx, nsObj); k8serrors.IsNotFound(err) {
		c.Logger.Logf("Namespace already cleaned up.")
	} else if err != nil {
		return err
	}

	return wait.PollUntilContextCancel(ctx, 100*time.Millisecond, true, func(ctx context.Context) (done bool, err error) {
		actual := &corev1.Namespace{}
		err = cl.Get(ctx, client.ObjectKey{Name: ns.Name}, actual)
		if k8serrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (c *Case) createNamespace(test *testing.T, cl client.Client, ns *namespace) error {
	if !ns.AutoCreated {
		c.Logger.Log("Skipping creation of user-supplied namespace:", ns.Name)
		return nil
	}
	c.Logger.Log("Creating namespace:", ns.Name)

	ctx := context.Background()
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(c.Timeout)*time.Second)
		defer cancel()
	}

	if !c.SkipDelete {
		test.Cleanup(func() {
			if err := c.deleteNamespace(cl, ns); err != nil {
				test.Error(err)
			}
		})
	}

	return cl.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns.Name,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Namespace",
		},
	})
}

func (c *Case) namespaceExists(namespace string) (bool, error) {
	cl, err := c.GetClient(false)
	if err != nil {
		return false, err
	}
	ns := &corev1.Namespace{}
	err = cl.Get(context.TODO(), client.ObjectKey{Name: namespace}, ns)
	if err != nil && !k8serrors.IsNotFound(err) {
		return false, err
	}
	return ns.Name == namespace, nil
}

func (c *Case) maybeReportEvents(namespace string) {
	if funk.Contains(c.Suppress, "events") {
		c.Logger.Logf("skipping kubernetes event logging")
		return
	}
	ctx := context.TODO()
	cl, err := c.GetClient(false)
	if err != nil {
		c.Logger.Log("Failed to collect events for %s in ns %s: %v", c.Name, namespace, err)
		return
	}
	eventutils.CollectAndLog(ctx, cl, namespace, c.Name, c.Logger)
}

// Run runs a test case including all of its steps.
func (c *Case) Run(test *testing.T, rep report.TestReporter) {
	defer rep.Done()

	ns := c.setup(test, rep)

	for _, testStep := range c.Steps {
		stepReport := rep.Step("step " + testStep.String())
		testStep.Setup(c.Logger, c.GetClient, c.GetDiscoveryClient)
		stepReport.AddAssertions(len(testStep.Asserts))
		stepReport.AddAssertions(len(testStep.Errors))

		var errs []error

		// Set-up client/namespace for lazy-loaded Kubeconfig
		if testStep.KubeconfigLoading == v1beta1.KubeconfigLoadingLazy {
			cl, err := testStep.Client(false)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to lazy-load kubeconfig: %w", err))
			} else if err = c.createNamespace(test, cl, ns); k8serrors.IsAlreadyExists(err) {
				c.Logger.Logf("namespace %q already exists", ns.Name)
			} else if err != nil {
				errs = append(errs, fmt.Errorf("failed to create test namespace: %w", err))
			}
		}

		// Run test case only if no setup errors are encountered
		if len(errs) == 0 {
			errs = append(errs, testStep.Run(test, ns.Name)...)
		}

		if len(errs) > 0 {
			caseErr := fmt.Errorf("failed in step %s", testStep.String())
			stepReport.Failure(caseErr.Error(), errs...)

			test.Error(caseErr)
			for _, err := range errs {
				test.Error(err)
			}
			break
		}
	}

	c.maybeReportEvents(ns.Name)
}

func (c *Case) setup(test *testing.T, rep report.TestReporter) *namespace {
	setupReport := rep.Step("setup")
	ns, err := c.determineNamespace()
	if err != nil {
		setupReport.Failure(err.Error())
		test.Fatal(err)
	}

	cl, err := c.GetClient(false)
	if err != nil {
		setupReport.Failure(err.Error())
		test.Fatal(err)
	}

	clients := map[string]client.Client{"": cl}

	for _, testStep := range c.Steps {
		if clients[testStep.Kubeconfig] != nil || testStep.KubeconfigLoading == v1beta1.KubeconfigLoadingLazy {
			continue
		}

		cl, err = kubernetes.NewClientFunc(testStep.Kubeconfig, testStep.Context)(false)
		if err != nil {
			setupReport.Failure(err.Error())
			test.Fatal(err)
		}

		clients[testStep.Kubeconfig] = cl
	}

	for kubeConfigPath, cl := range clients {
		if err = c.createNamespace(test, cl, ns); k8serrors.IsAlreadyExists(err) {
			c.Logger.Logf("namespace %q already exists, using kubeconfig %q", ns.Name, kubeConfigPath)
		} else if err != nil {
			setupReport.Failure("failed to create test namespace", err)
			test.Fatal(err)
		}
	}
	return ns
}

func (c *Case) determineNamespace() (*namespace, error) {
	ns := &namespace{
		Name:        c.PreferredNamespace,
		AutoCreated: false,
	}
	// no preferred ns, means we auto-create with petnames
	if c.PreferredNamespace == "" {
		ns.Name = fmt.Sprintf("kuttl-test-%s", petname.Generate(2, "-"))
		ns.AutoCreated = true
	} else {
		exist, err := c.namespaceExists(c.PreferredNamespace)
		if err != nil {
			return nil, fmt.Errorf("failed to determine existence of namespace %q: %w", c.PreferredNamespace, err)
		}
		if !exist {
			ns.AutoCreated = true
		}
	}
	// if we have a preferred namespace, and it already exists, we do NOT auto-create
	return ns, nil
}

// LoadTestSteps loads all the test steps for a test case.
func (c *Case) LoadTestSteps() error {
	testStepFiles, err := files.CollectTestStepFiles(c.Dir, c.Logger)
	if err != nil {
		return err
	}

	testSteps := []*Step{}

	for index, files := range testStepFiles {
		testStep := &Step{
			Timeout:       c.Timeout,
			Index:         int(index),
			SkipDelete:    c.SkipDelete,
			Dir:           c.Dir,
			TestRunLabels: c.RunLabels,
			Asserts:       []client.Object{},
			Apply:         []client.Object{},
			Errors:        []client.Object{},
		}

		for _, file := range files {
			if err := testStep.LoadYAML(file); err != nil {
				return err
			}
		}

		testSteps = append(testSteps, testStep)
	}

	sort.Slice(testSteps, func(i, j int) bool {
		return testSteps[i].Index < testSteps[j].Index
	})

	c.Steps = testSteps
	return nil
}
