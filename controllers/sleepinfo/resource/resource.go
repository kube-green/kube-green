package resource

import (
	"context"
	"fmt"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Resource interface {
	HasResource() bool
	Sleep(ctx context.Context) error
	WakeUp(ctx context.Context) error
	GetOriginalInfoToSave() ([]byte, error)
}

type ResourceClient struct {
	Client    client.Client
	SleepInfo *kubegreenv1alpha1.SleepInfo
	Log       logr.Logger
}

func (r ResourceClient) Patch(ctx context.Context, oldObj, newObj client.Object) error {
	if r.Client == nil {
		return fmt.Errorf("invalid resource setup")
	}
	if err := r.Client.Patch(ctx, newObj, client.MergeFrom(oldObj)); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil
		}
		return err
	}
	return nil
}

func (r ResourceClient) IsClientValid() error {
	if r.Client == nil || r.SleepInfo == nil || r.Log == nil {
		return fmt.Errorf("invalid resource setup")
	}
	return nil
}
