/*
Copyright 2021.
*/

package v1alpha1

import (
	"fmt"

	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var sleepinfolog = logf.Log.WithName("sleepinfo-resource")

func (s *SleepInfo) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(s).
		Complete()
}

//+kubebuilder:webhook:path=/validate-kube-green-com-v1alpha1-sleepinfo,mutating=false,failurePolicy=fail,sideEffects=None,groups=kube-green.com,resources=sleepinfos,verbs=create;update,versions=v1alpha1,name=vsleepinfo.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &SleepInfo{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (s *SleepInfo) ValidateCreate() (admission.Warnings, error) {
	sleepinfolog.Info("validate create", "name", s.Name, "namespace", s.Namespace)

	return nil, s.validateSleepInfo()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (s *SleepInfo) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	sleepinfolog.Info("validate update", "name", s.Name, "namespace", s.Namespace)

	return nil, s.validateSleepInfo()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (s *SleepInfo) ValidateDelete() (admission.Warnings, error) {
	sleepinfolog.Info("validate delete", "name", s.Name)
	return nil, nil
}

func (s SleepInfo) validateSleepInfo() error {
	schedule, err := s.GetSleepSchedule()
	if err != nil {
		return err
	}
	if _, err = cron.ParseStandard(schedule); err != nil {
		return err
	}

	schedule, err = s.GetWakeUpSchedule()
	if err != nil {
		return err
	}
	if schedule != "" {
		if _, err = cron.ParseStandard(schedule); err != nil {
			return err
		}
	}

	for _, excludeRef := range s.GetExcludeRef() {
		return isExcludeRefValid(excludeRef)
	}
	return nil
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
