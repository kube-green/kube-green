package testutil

import (
	"context"
	"errors"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Method int

const (
	List = iota
	Create
	Update
	Patch
)

type PossiblyErroringFakeCtrlRuntimeClient struct {
	client.Client
	ShouldError func(method Method, obj runtime.Object) bool
}

func (p PossiblyErroringFakeCtrlRuntimeClient) List(ctx context.Context, dpl client.ObjectList, opts ...client.ListOption) error {
	if p.ShouldError != nil && p.ShouldError(List, dpl) {
		return errors.New("error during list")
	}
	listOpts := client.ListOptions{}
	listOpts.ApplyOptions(opts)

	fieldSelector := listOpts.FieldSelector
	// TODO: we use != operator, which is not supported by fake client because it
	// does not use indexes: https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/client/fake/client.go#L521
	listOpts.FieldSelector = nil

	err := p.Client.List(ctx, dpl, &listOpts)
	if err != nil {
		return err
	}

	if fieldSelector != nil {
		objList, err := meta.ExtractList(dpl)
		if err != nil {
			return err
		}
		filteredObjs, err := filterObjectsByName(objList, fieldSelector)
		if err != nil {
			return err
		}
		err = meta.SetList(dpl, filteredObjs)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p PossiblyErroringFakeCtrlRuntimeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if p.ShouldError != nil && p.ShouldError(Create, obj) {
		return errors.New("error during create")
	}
	if secret, ok := obj.(*v1.Secret); ok {
		convertSecretStringData(secret)
	}
	return p.Client.Create(ctx, obj, opts...)
}

func (p PossiblyErroringFakeCtrlRuntimeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if p.ShouldError != nil && p.ShouldError(Update, obj) {
		return errors.New("error during update")
	}
	if secret, ok := obj.(*v1.Secret); ok {
		convertSecretStringData(secret)
	}
	return p.Client.Update(ctx, obj, opts...)
}

func (p PossiblyErroringFakeCtrlRuntimeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if p.ShouldError != nil && p.ShouldError(Patch, obj) {
		return errors.New("error during patch")
	}
	if secret, ok := obj.(*v1.Secret); ok {
		convertSecretStringData(secret)
	}

	return p.Client.Patch(ctx, obj, patch, opts...)
}

func convertSecretStringData(secret *v1.Secret) {
	// From v1.Secret types:
	// StringData is provided as a write-only input field for convenience.
	// All keys and values are merged into the data field on write, overwriting any existing values.
	// The stringData field is never output when reading from the API.
	if secret.StringData != nil {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		for k, v := range secret.StringData {
			secret.Data[k] = []byte(v)
		}
	}
	secret.StringData = nil
}

func filterObjectsByName(objList []runtime.Object, fieldSelector fields.Selector) ([]runtime.Object, error) {
	items := make([]runtime.Object, 0, len(objList))
	for _, obj := range objList {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		name := accessor.GetName()
		fieldSel := fields.Set{
			"metadata.name": name,
		}
		if fieldSelector.Matches(fieldSel) {
			items = append(items, obj.DeepCopyObject())
		}
	}
	return items, nil
}
