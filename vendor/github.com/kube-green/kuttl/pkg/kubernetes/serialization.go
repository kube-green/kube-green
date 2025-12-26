package kubernetes

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kube-green/kuttl/pkg/apis/testharness/v1beta1"
)

func pruneLargeAdditions(expected *unstructured.Unstructured, actual *unstructured.Unstructured) runtime.Object {
	pruned := actual.DeepCopy()
	prune(expected.Object, pruned.Object)
	return pruned
}

// prune replaces some fields in the actual tree to make it smaller for display.
//
// The goal is to make diffs on large objects much less verbose but not any less useful,
// by omitting these fields in the object which are not specified in the assertion and are at least
// moderately long when serialized.
//
// This way, for example when asserting on status.availableReplicas of a Deployment
// (which is missing if zero replicas are available) will still show the status.unavailableReplicas
// for example, but will omit spec completely unless the assertion also mentions it.
//
// This saves hundreds to thousands of lines of logs to scroll when debugging failures of some operator tests.
func prune(expected map[string]interface{}, actual map[string]interface{}) {
	// This value was chosen so that it is low enough to hide huge fields like `metadata.managedFields`,
	// but large enough such that for example a typical `metadata.labels` still shows,
	// since it might be useful for identifying reported objects like pods.
	// This could potentially be turned into a knob in the future.
	const maxLines = 10
	var toRemove []string
	for k, v := range actual {
		if _, inExpected := expected[k]; inExpected {
			expectedMap, isExpectedMap := expected[k].(map[string]interface{})
			actualMap, isActualMap := actual[k].(map[string]interface{})
			if isActualMap && isExpectedMap {
				prune(expectedMap, actualMap)
			}
			continue
		}
		numLines, err := countLines(k, v)
		if err != nil || numLines < maxLines {
			continue
		}
		toRemove = append(toRemove, k)
	}
	for _, s := range toRemove {
		actual[s] = fmt.Sprintf("[... elided field over %d lines long ...]", maxLines)
	}
}

func countLines(k string, v interface{}) (int, error) {
	buf := strings.Builder{}
	dummyObj := &unstructured.Unstructured{
		Object: map[string]interface{}{k: v}}
	err := marshalObject(dummyObj, &buf)
	if err != nil {
		return 0, fmt.Errorf("cannot marshal field %s to compute its length in lines: %w", k, err)
	}
	return strings.Count(buf.String(), "\n"), nil
}

// PrettyDiff creates a unified diff highlighting the differences between two Kubernetes resources
func PrettyDiff(expected *unstructured.Unstructured, actual *unstructured.Unstructured) (string, error) {
	actualPruned := pruneLargeAdditions(expected, actual)

	expectedBuf := &bytes.Buffer{}
	actualBuf := &bytes.Buffer{}

	if err := marshalObject(expected, expectedBuf); err != nil {
		return "", err
	}

	if err := marshalObject(actualPruned, actualBuf); err != nil {
		return "", err
	}

	diffed := difflib.UnifiedDiff{
		A:        difflib.SplitLines(expectedBuf.String()),
		B:        difflib.SplitLines(actualBuf.String()),
		FromFile: ResourceID(expected),
		ToFile:   ResourceID(actual),
		Context:  3,
	}

	return difflib.GetUnifiedDiffString(diffed)
}

// convertUnstructured converts an unstructured object to the known struct. If the type is not known, then
// the unstructured object is returned unmodified.
func convertUnstructured(in client.Object) (client.Object, error) {
	unstruct, err := runtime.DefaultUnstructuredConverter.ToUnstructured(in)
	if err != nil {
		return nil, fmt.Errorf("error converting %s to unstructured error: %w", ResourceID(in), err)
	}

	var converted client.Object

	kind := in.GetObjectKind().GroupVersionKind().Kind
	group := in.GetObjectKind().GroupVersionKind().Group

	kuttlGroup := "kuttl.dev"
	if group != kuttlGroup {
		return in, nil
	}
	switch {
	case kind == "TestFile":
		converted = &v1beta1.TestFile{}
	case kind == "TestStep":
		converted = &v1beta1.TestStep{}
	case kind == "TestAssert":
		converted = &v1beta1.TestAssert{}
	case kind == "TestSuite":
		converted = &v1beta1.TestSuite{}
	default:
		return in, nil
	}

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct, converted)
	if err != nil {
		return nil, fmt.Errorf("error converting %s from unstructured error: %w", ResourceID(in), err)
	}

	return converted, nil
}

// cleanObjectForMarshalling removes unnecessary object metadata that should not be included in serialization and diffs.
func cleanObjectForMarshalling(o runtime.Object) (runtime.Object, error) {
	copied := o.DeepCopyObject()

	meta, err := meta.Accessor(copied)
	if err != nil {
		return nil, err
	}

	meta.SetResourceVersion("")
	meta.SetCreationTimestamp(metav1.Time{})
	meta.SetSelfLink("")
	meta.SetUID(types.UID(""))
	meta.SetGeneration(0)

	annotations := meta.GetAnnotations()
	delete(annotations, "deployment.kubernetes.io/revision")

	if len(annotations) > 0 {
		meta.SetAnnotations(annotations)
	} else {
		meta.SetAnnotations(nil)
	}

	return copied, nil
}

// marshalObject marshals a Kubernetes object to a YAML string.
func marshalObject(o runtime.Object, w io.Writer) error {
	copied, err := cleanObjectForMarshalling(o)
	if err != nil {
		return err
	}

	return json.NewYAMLSerializer(json.DefaultMetaFactory, nil, nil).Encode(copied, w)
}

// MarshalObjectJSON marshals a Kubernetes object to a JSON string.
func MarshalObjectJSON(o runtime.Object, w io.Writer) error {
	copied, err := cleanObjectForMarshalling(o)
	if err != nil {
		return err
	}

	return json.NewSerializer(json.DefaultMetaFactory, nil, nil, false).Encode(copied, w)
}

// LoadYAMLFromFile loads all objects from a YAML file.
func LoadYAMLFromFile(path string) ([]client.Object, error) {
	opened, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer opened.Close()

	return LoadYAML(path, opened)
}

// LoadYAML loads all objects from a reader
func LoadYAML(path string, r io.Reader) ([]client.Object, error) {
	yamlReader := yaml.NewYAMLReader(bufio.NewReader(r))

	objects := []client.Object{}

	for {
		data, err := yamlReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("error reading yaml %s: %w", path, err)
		}

		unstructuredObj := &unstructured.Unstructured{}
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewBuffer(data), len(data))

		if err = decoder.Decode(unstructuredObj); err != nil {
			return nil, fmt.Errorf("error decoding yaml %s: %w", path, err)
		}

		obj, err := convertUnstructured(unstructuredObj)
		if err != nil {
			return nil, fmt.Errorf("error converting unstructured object %s (%s): %w", ResourceID(unstructuredObj), path, err)
		}
		// discovered reader will return empty objects if a number of lines are preceding a yaml separator (---)
		// this detects that, logs and continues
		if obj.GetObjectKind().GroupVersionKind().Kind == "" {
			log.Println("object detected with no GVK Kind for path", path)
		} else {
			objects = append(objects, obj)
		}
	}

	return objects, nil
}
