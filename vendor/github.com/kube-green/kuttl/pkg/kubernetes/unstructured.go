package kubernetes

import (
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// NewResource generates a Kubernetes object using the provided apiVersion, kind, name, and namespace.
// The name and namespace are omitted if empty.
func NewResource(apiVersion, kind, name, namespace string) *unstructured.Unstructured {
	meta := map[string]interface{}{}

	if name != "" {
		meta["name"] = name
	}
	if namespace != "" {
		meta["namespace"] = namespace
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata":   meta,
		},
	}
}

// NewClusterRoleBinding Create a clusterrolebinding for the serviceAccount passed
func NewClusterRoleBinding(apiVersion, kind, name, namespace string, serviceAccount string, roleName string) runtime.Object {
	sa := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      serviceAccount,
			Namespace: namespace,
		}},
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata":   sa.ObjectMeta,
			"subjects":   sa.Subjects,
			"roleRef":    sa.RoleRef,
		},
	}
}

// NewPod creates a new pod object.
func NewPod(name, namespace string) *unstructured.Unstructured {
	return NewResource("v1", "Pod", name, namespace)
}

// WithNamespace naively applies the namespace to the object. Used mainly in tests, otherwise
// use Namespaced.
func WithNamespace(obj *unstructured.Unstructured, namespace string) *unstructured.Unstructured {
	res := obj.DeepCopy()

	m, _ := meta.Accessor(res) //nolint:errcheck // runtime.Object don't have the error issues of interface{}
	m.SetNamespace(namespace)

	return res
}

// WithSpec applies the provided spec to the Kubernetes object.
func WithSpec(t *testing.T, obj *unstructured.Unstructured, spec map[string]interface{}) *unstructured.Unstructured {
	res, err := WithKeyValue(obj, "spec", spec)
	if err != nil {
		t.Fatalf("failed to apply spec %v to object %v: %v", spec, obj, err)
	}
	return res
}

// WithStatus applies the provided status to the Kubernetes object.
func WithStatus(t *testing.T, obj *unstructured.Unstructured, status map[string]interface{}) *unstructured.Unstructured {
	res, err := WithKeyValue(obj, "status", status)
	if err != nil {
		t.Fatalf("failed to apply status %v to object %v: %v", status, obj, err)
	}
	return res
}

// WithKeyValue sets key in the provided object to value.
func WithKeyValue(obj *unstructured.Unstructured, key string, value map[string]interface{}) (*unstructured.Unstructured, error) {
	obj = obj.DeepCopy()
	// we need to convert to and from unstructured here so that the types in case_test match when comparing
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	content[key] = value

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(content, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// WithLabels sets the labels on an object.
func WithLabels(t *testing.T, obj *unstructured.Unstructured, labels map[string]string) *unstructured.Unstructured {
	obj = obj.DeepCopy()

	m, err := meta.Accessor(obj)
	if err != nil {
		t.Fatalf("failed to apply labels %v to object %v: %v", labels, obj, err)
	}
	m.SetLabels(labels)

	return obj
}

// WithAnnotations sets the annotations on an object.
func WithAnnotations(obj runtime.Object, annotations map[string]string) runtime.Object {
	obj = obj.DeepCopyObject()

	m, _ := meta.Accessor(obj) //nolint:errcheck // runtime.Object don't have the error issues of interface{}
	m.SetAnnotations(annotations)

	return obj
}

// SetAnnotation sets the given key and value in the object's annotations, returning a copy.
func SetAnnotation(obj *unstructured.Unstructured, key, value string) *unstructured.Unstructured {
	obj = obj.DeepCopy()

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)

	return obj
}
