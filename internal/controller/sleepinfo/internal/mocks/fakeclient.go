package mocks

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func FakeClient() *fake.ClientBuilder {
	groupVersion := []schema.GroupVersion{
		{Group: "batch", Version: "v1"},
		{Group: "batch", Version: "v1beta1"},
		{Group: "apps", Version: "v1"},
	}
	restMapper := meta.NewDefaultRESTMapper(groupVersion)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "batch",
		Version: "v1",
		Kind:    "CronJob",
	}, meta.RESTScopeNamespace)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "batch",
		Version: "v1beta1",
		Kind:    "CronJob",
	}, meta.RESTScopeNamespace)
	restMapper.Add(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "StatefulSet",
	}, meta.RESTScopeNamespace)

	return fake.
		NewClientBuilder().
		WithRESTMapper(restMapper)
}
