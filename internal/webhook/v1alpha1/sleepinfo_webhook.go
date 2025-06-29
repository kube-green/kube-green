/*
Copyright 2021.
*/

package v1alpha1

import (
	"context"
	"fmt"

	"github.com/kube-green/kube-green/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var sleepinfolog = logf.Log.WithName("sleepinfo-resource")

type customValidator struct {
	Client client.Client
}

func SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.SleepInfo{}).
		WithValidator(&customValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-kube-green-com-v1alpha1-sleepinfo,mutating=false,failurePolicy=fail,sideEffects=None,groups=kube-green.com,resources=sleepinfos,verbs=create;update,versions=v1alpha1,name=vsleepinfo.kb.io,admissionReviewVersions=v1
var _ webhook.CustomValidator = &customValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *customValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	s, ok := obj.(*v1alpha1.SleepInfo)
	if !ok {
		return nil, fmt.Errorf("fails to decode SleepInfo")
	}
	sleepinfolog.Info("validate create", "name", s.Name, "namespace", s.Namespace)

	return s.Validate(v.Client)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *customValidator) ValidateUpdate(ctx context.Context, _, new runtime.Object) (admission.Warnings, error) {
	s, ok := new.(*v1alpha1.SleepInfo)
	if !ok {
		return nil, fmt.Errorf("fails to decode SleepInfo")
	}
	sleepinfolog.Info("validate update", "name", s.Name, "namespace", s.Namespace)

	return s.Validate(v.Client)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *customValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	s, ok := obj.(*v1alpha1.SleepInfo)
	if !ok {
		return nil, fmt.Errorf("fails to decode SleepInfo")
	}
	sleepinfolog.Info("validate delete", "name", s.Name)
	return nil, nil
}
