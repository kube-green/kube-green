/*
Copyright 2021.
*/

package v1alpha1

import (
	"context"
	"fmt"

	"github.com/kube-green/kube-green/internal/patcher"
	"github.com/robfig/cron/v3"
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
		For(&SleepInfo{}).
		WithValidator(&customValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-kube-green-com-v1alpha1-sleepinfo,mutating=false,failurePolicy=fail,sideEffects=None,groups=kube-green.com,resources=sleepinfos,verbs=create;update,versions=v1alpha1,name=vsleepinfo.kb.io,admissionReviewVersions=v1
var _ webhook.CustomValidator = &customValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *customValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	s, ok := obj.(*SleepInfo)
	if !ok {
		return nil, fmt.Errorf("fails to decode SleepInfo")
	}
	sleepinfolog.Info("validate create", "name", s.Name, "namespace", s.Namespace)

	return s.validateSleepInfo(v.Client)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *customValidator) ValidateUpdate(ctx context.Context, _, new runtime.Object) (admission.Warnings, error) {
	s, ok := new.(*SleepInfo)
	if !ok {
		return nil, fmt.Errorf("fails to decode SleepInfo")
	}
	sleepinfolog.Info("validate update", "name", s.Name, "namespace", s.Namespace)

	return s.validateSleepInfo(v.Client)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *customValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	s, ok := obj.(*SleepInfo)
	if !ok {
		return nil, fmt.Errorf("fails to decode SleepInfo")
	}
	sleepinfolog.Info("validate delete", "name", s.Name)
	return nil, nil
}

func (s SleepInfo) validateSleepInfo(cl client.Client) ([]string, error) {
	schedule, err := s.GetSleepSchedule()
	if err != nil {
		return nil, err
	}
	if _, err = cron.ParseStandard(schedule); err != nil {
		return nil, err
	}

	schedule, err = s.GetWakeUpSchedule()
	if err != nil {
		return nil, err
	}
	if schedule != "" {
		if _, err = cron.ParseStandard(schedule); err != nil {
			return nil, err
		}
	}

	for _, excludeRef := range s.GetExcludeRef() {
		if err := isExcludeRefValid(excludeRef); err != nil {
			return nil, err
		}
	}

	return s.validatePatches(cl)
}

func isExcludeRefValid(excludeRef ExcludeRef) error {
	if excludeRef.Name == "" && excludeRef.APIVersion == "" && excludeRef.Kind == "" && len(excludeRef.MatchLabels) > 0 {
		return nil
	}
	if len(excludeRef.MatchLabels) == 0 && excludeRef.Name != "" && excludeRef.APIVersion != "" && excludeRef.Kind != "" {
		return nil
	}
	return fmt.Errorf(`excludeRef is invalid. Must have set: matchLabels or name,apiVersion and kind fields`)
}

func (s *SleepInfo) validatePatches(cl client.Client) ([]string, error) {
	warnings := []string{}
	for _, patch := range s.GetPatches() {
		if _, err := cl.RESTMapper().RESTMapping(patch.Target.GroupKind()); err != nil {
			warnings = append(warnings, fmt.Sprintf("patch target %s is not supported by the cluster", patch.Target))
		}

		if _, err := patcher.New([]byte(patch.Patch)); err != nil {
			return nil, fmt.Errorf("patch is invalid for target %s: %w", patch.Target, err)
		}
	}

	return warnings, nil
}
