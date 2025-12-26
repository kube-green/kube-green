package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	json2 "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// isJSONSyntaxError returns true if the error is a JSON syntax error.
func isJSONSyntaxError(err error) bool {
	_, ok := err.(*json.SyntaxError)
	return ok
}

// validateErrors accepts an error as its first argument and passes it to each function in the errValidationFuncs slice,
// if any of the methods returns true, the method returns nil, otherwise it returns the original error.
func validateErrors(err error, errValidationFuncs ...func(error) bool) error {
	for _, errFunc := range errValidationFuncs {
		if errFunc(err) {
			return nil
		}
	}

	return err
}

// retry retries a method until the context expires or the method returns an unvalidated error.
func retry(ctx context.Context, fn func(context.Context) error, errValidationFuncs ...func(error) bool) error {
	var lastErr error
	errCh := make(chan error)
	doneCh := make(chan struct{})

	if fn == nil {
		log.Println("retry func is nil and will be ignored")
		return nil
	}

	// do { } while (err != nil): https://stackoverflow.com/a/32844744/10892393
	for ok := true; ok; ok = lastErr != nil {
		// run the function in a goroutine and close it once it is finished so that
		// we can use select to wait for both the function return and the context deadline.

		go func() {
			// if the func we are calling panics, clean up and call it done
			// the common case is when a shared reference, like a client, is nil and is called in the function
			defer func() {
				if r := recover(); r != nil {
					errCh <- errors.New("func passed to retry panicked.  expected if testsuite is shutting down")
				}
			}()

			if err := fn(ctx); err != nil {
				errCh <- err
			} else {
				doneCh <- struct{}{}
			}
		}()

		select {
		// the callback finished
		case <-doneCh:
			lastErr = nil
		case err := <-errCh:
			// check if we tolerate the error, return it if not.
			if e := validateErrors(err, errValidationFuncs...); e != nil {
				return e
			}
			lastErr = err
		// timeout exceeded
		case <-ctx.Done():
			if lastErr == nil {
				// there's no previous error, so just return the timeout error
				return ctx.Err()
			}

			// return the most recent error
			return lastErr
		}
	}

	return lastErr
}

// RetryClient implements the Client interface, with retries built in.
type RetryClient struct {
	Client    client.Client
	dynamic   dynamic.Interface
	discovery discovery.DiscoveryInterface
}

// RetryStatusWriter implements the StatusWriter interface, with retries built in.
type RetryStatusWriter struct {
	StatusWriter client.StatusWriter
}

// NewRetryClient initializes a new Kubernetes client that automatically retries on network-related errors.
func NewRetryClient(cfg *rest.Config, opts client.Options) (*RetryClient, error) {
	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	discovery, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}

	if opts.Mapper == nil {
		httpClient, err := rest.HTTPClientFor(cfg)
		if err != nil {
			return nil, err
		}
		opts.Mapper, err = apiutil.NewDynamicRESTMapper(cfg, httpClient)
		if err != nil {
			return nil, err
		}
	}

	client, err := client.New(cfg, opts)
	return &RetryClient{Client: client, dynamic: dynamicClient, discovery: discovery}, err
}

// Scheme returns the scheme this client is using.
func (r *RetryClient) Scheme() *runtime.Scheme {
	return r.Client.Scheme()
}

// RESTMapper returns the rest mapper this client is using.
func (r *RetryClient) RESTMapper() meta.RESTMapper {
	return r.Client.RESTMapper()
}

// SubResource returns a subresource client for the named subResource.
func (r *RetryClient) SubResource(subResource string) client.SubResourceClient {
	return r.Client.SubResource(subResource)
}

// GroupVersionKindFor returns the GroupVersionKind for the provided object.
func (r *RetryClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return r.Client.GroupVersionKindFor(obj)
}

// IsObjectNamespaced returns true if the object is namespaced.
func (r *RetryClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return r.Client.IsObjectNamespaced(obj)
}

// Create saves the object obj in the Kubernetes cluster.
func (r *RetryClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return retry(ctx, func(ctx context.Context) error {
		return r.Client.Create(ctx, obj, opts...)
	}, isJSONSyntaxError)
}

// Delete deletes the given obj from Kubernetes cluster.
func (r *RetryClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return retry(ctx, func(ctx context.Context) error {
		return r.Client.Delete(ctx, obj, opts...)
	}, isJSONSyntaxError)
}

// DeleteAllOf deletes the given obj from Kubernetes cluster.
func (r *RetryClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return retry(ctx, func(ctx context.Context) error {
		return r.Client.DeleteAllOf(ctx, obj, opts...)
	}, isJSONSyntaxError)
}

// Update updates the given obj in the Kubernetes cluster. obj must be a
// struct pointer so that obj can be updated with the content returned by the Server.
func (r *RetryClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return retry(ctx, func(ctx context.Context) error {
		return r.Client.Update(ctx, obj, opts...)
	}, isJSONSyntaxError)
}

// Patch patches the given obj in the Kubernetes cluster. obj must be a
// struct pointer so that obj can be updated with the content returned by the Server.
func (r *RetryClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return retry(ctx, func(ctx context.Context) error {
		return r.Client.Patch(ctx, obj, patch, opts...)
	}, isJSONSyntaxError)
}

func (r *RetryClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	return retry(ctx, func(ctx context.Context) error {
		return r.Client.Apply(ctx, obj, opts...)
	}, isJSONSyntaxError)
}

// Get retrieves an obj for the given object key from the Kubernetes Cluster.
// obj must be a struct pointer so that obj can be updated with the response
// returned by the Server.
func (r *RetryClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return retry(ctx, func(ctx context.Context) error {
		return r.Client.Get(ctx, key, obj, opts...)
	}, isJSONSyntaxError)
}

// List retrieves list of objects for a given namespace and list options. On a
// successful call, Items field in the list will be populated with the
// result returned from the server.
func (r *RetryClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return retry(ctx, func(ctx context.Context) error {
		return r.Client.List(ctx, list, opts...)
	}, isJSONSyntaxError)
}

// Watch watches a specific object and returns all events for it.
func (r *RetryClient) Watch(ctx context.Context, obj runtime.Object) (watch.Interface, error) {
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	gvk := obj.GetObjectKind().GroupVersionKind()

	groupResources, err := restmapper.GetAPIGroupResources(r.discovery)
	if err != nil {
		return nil, err
	}

	mapping, err := restmapper.NewDiscoveryRESTMapper(groupResources).RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}

	return r.dynamic.Resource(mapping.Resource).Watch(ctx, v1.SingleObject(v1.ObjectMeta{
		Name:      objMeta.GetName(),
		Namespace: objMeta.GetNamespace(),
	}))
}

// Status returns a client which can update status subresource for kubernetes objects.
func (r *RetryClient) Status() client.StatusWriter {
	return &RetryStatusWriter{
		StatusWriter: r.Client.Status(),
	}
}

// Create saves the subResource object in the Kubernetes cluster. obj must be a
// struct pointer so that obj can be updated with the content returned by the Server.
func (r *RetryStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return retry(ctx, func(ctx context.Context) error {
		return r.StatusWriter.Create(ctx, obj, subResource, opts...)
	}, isJSONSyntaxError)
}

// Update updates the given obj in the Kubernetes cluster. obj must be a
// struct pointer so that obj can be updated with the content returned by the Server.
func (r *RetryStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return retry(ctx, func(ctx context.Context) error {
		return r.StatusWriter.Update(ctx, obj, opts...)
	}, isJSONSyntaxError)
}

// Patch patches the given obj in the Kubernetes cluster. obj must be a
// struct pointer so that obj can be updated with the content returned by the Server.
func (r *RetryStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return retry(ctx, func(ctx context.Context) error {
		return r.StatusWriter.Patch(ctx, obj, patch, opts...)
	}, isJSONSyntaxError)
}

// patchObject updates expected with the Resource Version from actual.
// In the future, patchObject may perform a strategic merge of actual into expected.
func patchObject(actual, expected runtime.Object) error {
	actualMeta, err := meta.Accessor(actual)
	if err != nil {
		return err
	}

	expectedMeta, err := meta.Accessor(expected)
	if err != nil {
		return err
	}

	expectedMeta.SetResourceVersion(actualMeta.GetResourceVersion())
	return nil
}

// CreateOrUpdate will create obj if it does not exist and update it if it does.
// retryonerror indicates whether we retry in case of conflict
// Returns true if the object was updated and false if it was created.
func CreateOrUpdate(ctx context.Context, cl client.Client, obj client.Object, retryOnError bool) (updated bool, err error) {
	orig := obj.DeepCopyObject()

	validators := []func(err error) bool{apierrors.IsAlreadyExists}

	if retryOnError {
		validators = append(validators, apierrors.IsConflict)
	}
	err = retry(ctx, func(ctx context.Context) error {
		expected := orig.DeepCopyObject()
		actual := &unstructured.Unstructured{}
		actual.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())

		err := cl.Get(ctx, ObjectKey(expected), actual)
		if err == nil {
			if err = patchObject(actual, expected); err != nil {
				return err
			}

			var expectedBytes []byte
			expectedBytes, err = json2.Marshal(expected)
			if err != nil {
				return err
			}

			err = cl.Patch(ctx, actual, client.RawPatch(types.MergePatchType, expectedBytes))
			updated = true
		} else if apierrors.IsNotFound(err) {
			err = cl.Create(ctx, obj)
			updated = false
		}
		return err
	}, validators...)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = errors.New("create/update timeout exceeded")
	}
	return updated, err
}
