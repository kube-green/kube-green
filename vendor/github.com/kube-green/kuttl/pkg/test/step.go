package test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/cel-go/cel"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"

	harness "github.com/kube-green/kuttl/pkg/apis/testharness/v1beta1"
	"github.com/kube-green/kuttl/pkg/env"
	"github.com/kube-green/kuttl/pkg/expressions"
	kfile "github.com/kube-green/kuttl/pkg/file"
	"github.com/kube-green/kuttl/pkg/http"
	"github.com/kube-green/kuttl/pkg/kubernetes"
	testutils "github.com/kube-green/kuttl/pkg/test/utils"
)

// fileNameRegex contains two capturing groups to determine whether a file has special
// meaning (ex. assert) or contains an appliable object, and extra name elements.
var fileNameRegex = regexp.MustCompile(`^(?:\d+-)?([^-\.]+)(-[^\.]+)?(?:\.yaml)?$`)

// A Step contains the name of the test step, its index in the test,
// and all of the test step's settings (including objects to apply and assert on).
type Step struct {
	Name       string
	Index      int
	SkipDelete bool

	Dir           string
	TestRunLabels labels.Set

	Step   *harness.TestStep
	Assert *harness.TestAssert

	Programs map[string]cel.Program

	Asserts []client.Object
	Apply   []client.Object
	Errors  []client.Object

	Timeout int

	Kubeconfig        string
	KubeconfigLoading string
	Context           string

	Client          func(forceNew bool) (client.Client, error)
	DiscoveryClient func() (discovery.DiscoveryInterface, error)

	Logger testutils.Logger
}

// Clean deletes all resources defined in the Apply list.
func (s *Step) Clean(namespace string) error {
	cl, err := s.Client(false)
	if err != nil {
		return err
	}

	dClient, err := s.DiscoveryClient()
	if err != nil {
		return err
	}

	for _, obj := range s.Apply {
		_, _, err := kubernetes.Namespaced(dClient, obj, namespace)
		if err != nil {
			return err
		}

		if err := cl.Delete(context.TODO(), obj); err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// DeleteExisting deletes any resources in the TestStep.Delete list prior to running the tests.
func (s *Step) DeleteExisting(namespace string) error {
	cl, err := s.Client(false)
	if err != nil {
		return err
	}

	dClient, err := s.DiscoveryClient()
	if err != nil {
		return err
	}

	toDelete := []client.Object{}

	if s.Step == nil {
		return nil
	}

	for _, ref := range s.Step.Delete {
		gvk := ref.GroupVersionKind()

		obj := kubernetes.NewResource(gvk.GroupVersion().String(), gvk.Kind, ref.Name, "")

		objNs := namespace
		if ref.Namespace != "" {
			objNs = ref.Namespace
		}

		_, objNs, err := kubernetes.Namespaced(dClient, obj, objNs)
		if err != nil {
			return err
		}

		if ref.Name == "" {
			u := &unstructured.UnstructuredList{}
			u.SetGroupVersionKind(gvk)

			listOptions := []client.ListOption{}

			if ref.Labels != nil {
				listOptions = append(listOptions, client.MatchingLabels(ref.Labels))
			}

			if objNs != "" {
				listOptions = append(listOptions, client.InNamespace(objNs))
			}

			err := cl.List(context.TODO(), u, listOptions...)
			if err != nil {
				return fmt.Errorf("listing matching resources: %w", err)
			}

			for index := range u.Items {
				toDelete = append(toDelete, &u.Items[index])
			}
		} else {
			// Otherwise just append the object specified.
			toDelete = append(toDelete, obj.DeepCopy())
		}
	}

	for _, obj := range toDelete {
		del := &unstructured.Unstructured{}
		del.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
		del.SetName(obj.GetName())
		del.SetNamespace(obj.GetNamespace())

		err := cl.Delete(context.TODO(), del)
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}

	// Wait for resources to be deleted.
	return wait.PollUntilContextTimeout(context.TODO(), 100*time.Millisecond, time.Duration(s.GetTimeout())*time.Second, true, func(ctx context.Context) (done bool, err error) {
		for _, obj := range toDelete {
			actual := &unstructured.Unstructured{}
			actual.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
			err = cl.Get(ctx, kubernetes.ObjectKey(obj), actual)
			if err == nil || !k8serrors.IsNotFound(err) {
				return false, err
			}
		}

		return true, nil
	})
}

// Create applies all resources defined in the Apply list.
func (s *Step) Create(test *testing.T, namespace string) []error {
	cl, err := s.Client(true)
	if err != nil {
		return []error{err}
	}

	dClient, err := s.DiscoveryClient()
	if err != nil {
		return []error{err}
	}

	errors := []error{}

	for _, obj := range s.Apply {
		_, _, err := kubernetes.Namespaced(dClient, obj, namespace)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		ctx := context.Background()
		if s.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(s.Timeout)*time.Second)
			defer cancel()
		}

		if updated, err := kubernetes.CreateOrUpdate(ctx, cl, obj, true); err != nil {
			errors = append(errors, err)
		} else {
			// if the object was created, register cleanup
			if !updated && !s.SkipDelete {
				obj := obj
				test.Cleanup(func() {
					if err := cl.Delete(context.TODO(), obj); err != nil && !k8serrors.IsNotFound(err) {
						test.Error(err)
					}
				})
			}
			action := "created"
			if updated {
				action = "updated"
			}
			s.Logger.Log(kubernetes.ResourceID(obj), action)
		}
	}

	return errors
}

// GetTimeout gets the timeout defined for the test step.
func (s *Step) GetTimeout() int {
	timeout := s.Timeout
	if s.Assert != nil && s.Assert.Timeout != 0 {
		timeout = s.Assert.Timeout
	}
	return timeout
}

func list(cl client.Client, gvk schema.GroupVersionKind, namespace string, labelsMap map[string]string) ([]unstructured.Unstructured, error) {
	list := unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)

	listOptions := []client.ListOption{}
	if namespace != "" {
		listOptions = append(listOptions, client.InNamespace(namespace))
	}

	if len(labelsMap) > 0 {
		listOptions = append(listOptions, client.MatchingLabels(labelsMap))
	}

	if err := cl.List(context.TODO(), &list, listOptions...); err != nil {
		return []unstructured.Unstructured{}, err
	}

	return list.Items, nil
}

// CheckResource checks if the expected resource's state in Kubernetes is correct.
func (s *Step) CheckResource(expected runtime.Object, namespace string) []error {
	cl, err := s.Client(false)
	if err != nil {
		return []error{err}
	}

	dClient, err := s.DiscoveryClient()
	if err != nil {
		return []error{err}
	}

	testErrors := []error{}

	name, namespace, err := kubernetes.Namespaced(dClient, expected, namespace)
	if err != nil {
		return append(testErrors, err)
	}

	gvk := expected.GetObjectKind().GroupVersionKind()

	actuals := []unstructured.Unstructured{}
	if name != "" {
		actual := unstructured.Unstructured{}
		actual.SetGroupVersionKind(gvk)

		if err := cl.Get(context.TODO(), client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}, &actual); err != nil {
			return append(testErrors, err)
		}

		actuals = append(actuals, actual)
	} else {
		m, err := meta.Accessor(expected)
		if err != nil {
			return append(testErrors, err)
		}
		matches, err := list(cl, gvk, namespace, m.GetLabels())
		if err != nil {
			return append(testErrors, err)
		}
		if len(matches) == 0 {
			testErrors = append(testErrors, fmt.Errorf("no resources matched of kind: %s", gvk.String()))
		}
		actuals = append(actuals, matches...)
	}
	expectedObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(expected)
	if err != nil {
		return append(testErrors, err)
	}

	for _, actual := range actuals {
		tmpTestErrors := []error{}

		if err := testutils.IsSubset(expectedObj, actual.UnstructuredContent()); err != nil {
			diff, diffErr := kubernetes.PrettyDiff(
				&unstructured.Unstructured{Object: expectedObj}, &actual)
			if diffErr == nil {
				tmpTestErrors = append(tmpTestErrors, errors.New(diff))
			} else {
				tmpTestErrors = append(tmpTestErrors, diffErr)
			}

			tmpTestErrors = append(tmpTestErrors, fmt.Errorf("resource %s: %s", kubernetes.ResourceID(expected), err))
		}

		if len(tmpTestErrors) == 0 {
			return tmpTestErrors
		}

		testErrors = append(testErrors, tmpTestErrors...)
	}

	return testErrors
}

// CheckResourceAbsent checks if the expected resource's state is absent in Kubernetes.
func (s *Step) CheckResourceAbsent(expected runtime.Object, namespace string) error {
	cl, err := s.Client(false)
	if err != nil {
		return err
	}

	dClient, err := s.DiscoveryClient()
	if err != nil {
		return err
	}

	name, namespace, err := kubernetes.Namespaced(dClient, expected, namespace)
	if err != nil {
		return err
	}

	gvk := expected.GetObjectKind().GroupVersionKind()

	var actuals []unstructured.Unstructured

	if name != "" {
		actual := unstructured.Unstructured{}
		actual.SetGroupVersionKind(gvk)

		if err := cl.Get(context.TODO(), client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}, &actual); err != nil {
			if k8serrors.IsNotFound(err) {
				return nil
			}

			return err
		}

		actuals = []unstructured.Unstructured{actual}
	} else {
		m, err := meta.Accessor(expected)
		if err != nil {
			return err
		}
		actuals, err = list(cl, gvk, namespace, m.GetLabels())
		if err != nil {
			return err
		}
	}

	expectedObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(expected)
	if err != nil {
		return err
	}

	var unexpectedObjects []unstructured.Unstructured
	for _, actual := range actuals {
		if err := testutils.IsSubset(expectedObj, actual.UnstructuredContent()); err == nil {
			unexpectedObjects = append(unexpectedObjects, actual)
		}
	}

	if len(unexpectedObjects) == 0 {
		return nil
	}
	if len(unexpectedObjects) == 1 {
		return fmt.Errorf("resource %s %s matched error assertion", unexpectedObjects[0].GroupVersionKind(), unexpectedObjects[0].GetName())
	}
	return fmt.Errorf("resource %s %s (and %d other resources) matched error assertion", unexpectedObjects[0].GroupVersionKind(), unexpectedObjects[0].GetName(), len(unexpectedObjects)-1)
}

// CheckAssertCommands Runs the commands provided in `commands` and check if have been run successfully.
// the errors returned can be a failure of executing the command or the failure of the command executed.
func (s *Step) CheckAssertCommands(ctx context.Context, namespace string, commands []harness.TestAssertCommand, timeout int) []error {
	testErrors := []error{}
	if _, err := testutils.RunAssertCommands(ctx, s.Logger, namespace, commands, s.Dir, timeout, s.Kubeconfig); err != nil {
		testErrors = append(testErrors, err)
	}
	return testErrors
}

func (s *Step) CheckAssertExpressions(namespace string) []error {
	client, err := s.Client(false)
	if err != nil {
		return []error{err}
	}

	variables := make(map[string]interface{})
	for _, resourceRef := range s.Assert.ResourceRefs {
		if resourceRef.Namespace == "" {
			resourceRef.Namespace = namespace
		}
		namespacedName, referencedResource := resourceRef.BuildResourceReference()
		if err := client.Get(context.TODO(), namespacedName, referencedResource); err != nil {
			return []error{fmt.Errorf("failed to get referenced resource '%v': %w", namespacedName, err)}
		}

		variables[resourceRef.Ref] = referencedResource.Object
	}

	return expressions.RunAssertExpressions(s.Programs, variables, s.Assert.AssertAny, s.Assert.AssertAll)
}

// Check checks if the resources defined in Asserts and Errors are in the correct state.
func (s *Step) Check(namespace string, timeout int) []error {
	testErrors := []error{}

	for _, expected := range s.Asserts {
		testErrors = append(testErrors, s.CheckResource(expected, namespace)...)
	}

	if s.Assert != nil {
		testErrors = append(testErrors, s.CheckAssertCommands(context.TODO(), namespace, s.Assert.Commands, timeout)...)
		testErrors = append(testErrors, s.CheckAssertExpressions(namespace)...)
	}

	for _, expected := range s.Errors {
		if testError := s.CheckResourceAbsent(expected, namespace); testError != nil {
			testErrors = append(testErrors, testError)
		}
	}

	return testErrors
}

// Run runs a KUTTL test step:
// 1. Apply all desired objects to Kubernetes.
// 2. Wait for all of the states defined in the test step's asserts to be true.'
func (s *Step) Run(test *testing.T, namespace string) []error {
	s.Logger.Log("starting test step", s.String())

	if err := s.DeleteExisting(namespace); err != nil {
		return []error{err}
	}

	testErrors := []error{}

	if s.Step != nil {
		for _, command := range s.Step.Commands {
			if command.Background {
				s.Logger.Log("background commands are not allowed for steps and will be run in foreground")
				command.Background = false
			}
		}
		if _, err := testutils.RunCommands(context.TODO(), s.Logger, namespace, s.Step.Commands, s.Dir, s.Timeout, s.Kubeconfig); err != nil {
			testErrors = append(testErrors, err)
		}
	}

	testErrors = append(testErrors, s.Create(test, namespace)...)

	if len(testErrors) != 0 {
		return testErrors
	}

	timeoutF := float64(s.GetTimeout())
	start := time.Now()

	for elapsed := 0.0; elapsed < timeoutF; elapsed = time.Since(start).Seconds() {
		testErrors = s.Check(namespace, int(timeoutF-elapsed))

		if len(testErrors) == 0 {
			break
		}
		if hasTimeoutErr(testErrors) {
			break
		}
		time.Sleep(time.Second)
	}

	// all is good
	if len(testErrors) == 0 {
		s.Logger.Log("test step completed", s.String())
		return testErrors
	}
	// test failure processing
	s.Logger.Log("test step failed", s.String())
	if s.Assert == nil {
		return testErrors
	}
	for _, collector := range s.Assert.Collectors {
		s.Logger.Logf("collecting log output for %s", collector.String())
		if collector.Command() == nil {
			s.Logger.Log("skipping invalid assertion collector")
			continue
		}
		_, err := testutils.RunCommand(context.TODO(), namespace, *collector.Command(), s.Dir, s.Logger, s.Logger, s.Logger, s.Timeout, s.Kubeconfig)
		if err != nil {
			s.Logger.Log("post assert collector failure: %s", err)
		}
	}
	s.Logger.Flush()
	return testErrors
}

// String implements the string interface, returning the name of the test step.
func (s *Step) String() string {
	return fmt.Sprintf("%d-%s", s.Index, s.Name)
}

// LoadYAML loads the resources from a YAML file for a test step:
//   - If the YAML file is called "assert", then it contains objects to
//     add to the test step's list of assertions.
//   - If the YAML file is called "errors", then it contains objects that,
//     if seen, mark a test immediately failed.
//   - All other YAML files are considered resources to create.
func (s *Step) LoadYAML(file string) error {
	skipFile, objects, err := s.loadOrSkipFile(file)
	if skipFile || err != nil {
		return err
	}

	if err = s.populateObjectsByFileName(filepath.Base(file), objects); err != nil {
		return fmt.Errorf("populating step: %v", err)
	}

	asserts := []client.Object{}

	for _, obj := range s.Asserts {
		if obj.GetObjectKind().GroupVersionKind().Kind == "TestAssert" {
			if testAssert, ok := obj.DeepCopyObject().(*harness.TestAssert); ok {
				s.Assert = testAssert
			} else {
				return fmt.Errorf("failed to load TestAssert object from %s: it contains an object of type %T", file, obj)
			}

			s.Programs, err = expressions.LoadPrograms(s.Assert)
			if err != nil {
				return fmt.Errorf("failed to prepare expression evaluation: %w", err)
			}
		} else {
			asserts = append(asserts, obj)
		}
	}

	applies := []client.Object{}

	for _, obj := range s.Apply {
		if obj.GetObjectKind().GroupVersionKind().Kind == "TestStep" {
			if testStep, ok := obj.(*harness.TestStep); ok {
				if s.Step != nil {
					return fmt.Errorf("more than 1 TestStep not allowed in step %q", s.Name)
				}
				s.Step = testStep
			} else {
				return fmt.Errorf("failed to load TestStep object from %s: it contains an object of type %T", file, obj)
			}
			s.Step.Index = s.Index
			if s.Step.Name != "" {
				s.Name = s.Step.Name
			}
			if s.Step.Kubeconfig != "" {
				exKubeconfig := env.Expand(s.Step.Kubeconfig)
				s.Kubeconfig = cleanPath(exKubeconfig, s.Dir)
			}
			s.Context = s.Step.Context

			switch s.Step.KubeconfigLoading {
			case "", harness.KubeconfigLoadingEager, harness.KubeconfigLoadingLazy:
				s.KubeconfigLoading = s.Step.KubeconfigLoading
			default:
				return fmt.Errorf("attribute 'kubeconfigLoading' has invalid value %q", s.Step.KubeconfigLoading)
			}
		} else {
			applies = append(applies, obj)
		}
	}

	// process provided steps configured TestStep kind
	if s.Step != nil {
		// process configured step applies
		for _, applyPath := range s.Step.Apply {
			exApply := env.Expand(applyPath)
			apply, err := ObjectsFromPath(exApply, s.Dir)
			if err != nil {
				return fmt.Errorf("step %q apply path %s: %w", s.Name, exApply, err)
			}
			applies = append(applies, apply...)
		}
		// process configured step asserts
		for _, assertPath := range s.Step.Assert {
			exAssert := env.Expand(assertPath)
			assert, err := ObjectsFromPath(exAssert, s.Dir)
			if err != nil {
				return fmt.Errorf("step %q assert path %s: %w", s.Name, exAssert, err)
			}
			asserts = append(asserts, assert...)
		}
		// process configured errors
		for _, errorPath := range s.Step.Error {
			exError := env.Expand(errorPath)
			errObjs, err := ObjectsFromPath(exError, s.Dir)
			if err != nil {
				return fmt.Errorf("step %q error path %s: %w", s.Name, exError, err)
			}
			s.Errors = append(s.Errors, errObjs...)
		}
	}

	s.Apply = applies
	s.Asserts = asserts
	return nil
}

func (s *Step) loadOrSkipFile(file string) (bool, []client.Object, error) {
	loadedObjects, err := kubernetes.LoadYAMLFromFile(file)
	if err != nil {
		return false, nil, fmt.Errorf("loading %s: %s", file, err)
	}

	var objects []client.Object
	shouldSkip := false
	testFileObjEncountered := false

	for i, object := range loadedObjects {
		if testFileObject, ok := object.(*harness.TestFile); ok {
			if testFileObjEncountered {
				return false, nil, fmt.Errorf("more than one TestFile object encountered in file %q", file)
			}
			testFileObjEncountered = true
			selector, err := metav1.LabelSelectorAsSelector(testFileObject.TestRunSelector)
			if err != nil {
				return false, nil, fmt.Errorf("unrecognized test run selector in object %d of %q: %w", i, file, err)
			}
			if selector.Empty() || selector.Matches(s.TestRunLabels) {
				continue
			}
			fmt.Printf("Skipping file %q, label selector does not match test run labels.\n", file)
			shouldSkip = true
		} else {
			objects = append(objects, object)
		}
	}
	return shouldSkip, objects, nil
}

// populateObjectsByFileName populates s.Asserts, s.Errors, and/or s.Apply for files containing
// "assert", "errors", or no special string, respectively.
func (s *Step) populateObjectsByFileName(fileName string, objects []client.Object) error {
	matches := fileNameRegex.FindStringSubmatch(fileName)
	if len(matches) < 2 {
		return fmt.Errorf("%s does not match file name regexp: %s", fileName, fileNameRegex.String())
	}

	switch fname := strings.ToLower(matches[1]); fname {
	case "assert":
		s.Asserts = append(s.Asserts, objects...)
	case "errors":
		s.Errors = append(s.Errors, objects...)
	default:
		if s.Name == "" {
			if len(matches) > 2 {
				// The second matching group will already have a hyphen prefix.
				s.Name = matches[1] + matches[2]
			} else {
				s.Name = matches[1]
			}
		}
		s.Apply = append(s.Apply, objects...)
	}

	return nil
}

// Setup prepares the step by configuring its logger and client provider methods.
func (s *Step) Setup(caseLogger testutils.Logger, defaultClientFunc func(forceNew bool) (client.Client, error), defaultDiscoveryClientFunc func() (discovery.DiscoveryInterface, error)) {
	s.Logger = caseLogger.WithPrefix(s.String())
	if s.Kubeconfig != "" {
		s.Client = kubernetes.NewClientFunc(s.Kubeconfig, s.Context)
		s.DiscoveryClient = kubernetes.NewDiscoveryClientFunc(s.Kubeconfig, s.Context)
	} else {
		s.Client = defaultClientFunc
		s.DiscoveryClient = defaultDiscoveryClientFunc
	}
}

// ObjectsFromPath returns an array of runtime.Objects for files / urls provided
func ObjectsFromPath(path, dir string) ([]client.Object, error) {
	if http.IsURL(path) {
		apply, err := http.ToObjects(path)
		if err != nil {
			return nil, err
		}
		return apply, nil
	}

	// it's a directory or file
	cPath := cleanPath(path, dir)
	paths, err := kfile.FromPath(cPath, "*.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to find YAML files in %s: %w", cPath, err)
	}
	apply, err := kfile.ToObjects(paths)
	if err != nil {
		return nil, err
	}
	return apply, nil
}

// cleanPath returns either the abs path or the joined path
func cleanPath(path, dir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}

func hasTimeoutErr(err []error) bool {
	for i := range err {
		if errors.Is(err[i], context.DeadlineExceeded) {
			return true
		}
	}
	return false
}
