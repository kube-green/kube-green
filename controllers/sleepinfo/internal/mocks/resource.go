package mocks

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type ResourceMock[T any] struct {
	resource T
}

func (r ResourceMock[T]) Resource() T {
	return r.resource
}

func (r ResourceMock[T]) Unstructured(t *testing.T) unstructured.Unstructured {
	unstructuredResource, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&r.resource)
	if err != nil {
		panic(err)
	}
	return unstructured.Unstructured{
		Object: unstructuredResource,
	}
}
