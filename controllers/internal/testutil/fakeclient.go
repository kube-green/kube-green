package testutil

import (
	"context"
	"errors"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PossiblyErroringFakeCtrlRuntimeClient struct {
	client.Client
	ShouldError bool
}

func (p *PossiblyErroringFakeCtrlRuntimeClient) List(ctx context.Context, dpl client.ObjectList, opts ...client.ListOption) error {
	if p.ShouldError {
		return errors.New("error during list")
	}
	return p.Client.List(ctx, dpl, opts...)
}

func (p *PossiblyErroringFakeCtrlRuntimeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if p.ShouldError {
		return errors.New("error during create")
	}
	if secret, ok := obj.(*v1.Secret); ok {
		convertSecretStringData(secret)
	}
	return p.Client.Create(ctx, obj, opts...)
}

func (p *PossiblyErroringFakeCtrlRuntimeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if p.ShouldError {
		return errors.New("error during update")
	}
	if secret, ok := obj.(*v1.Secret); ok {
		convertSecretStringData(secret)
	}
	return p.Client.Update(ctx, obj, opts...)
}

func (p *PossiblyErroringFakeCtrlRuntimeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if p.ShouldError {
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
