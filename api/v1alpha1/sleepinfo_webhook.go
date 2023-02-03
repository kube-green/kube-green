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
)

// log is for logging in this package.
var sleepinfolog = logf.Log.WithName("sleepinfo-resource")

func (r *SleepInfo) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-kube-green-com-v1alpha1-sleepinfo,mutating=false,failurePolicy=fail,sideEffects=None,groups=kube-green.com,resources=sleepinfos,verbs=create;update,versions=v1alpha1,name=vsleepinfo.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &SleepInfo{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (s *SleepInfo) ValidateCreate() error {
	sleepinfolog.Info("validate create", "name", s.Name, "namespace", s.Namespace)

	return s.validateSleepInfo()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (s *SleepInfo) ValidateUpdate(old runtime.Object) error {
	sleepinfolog.Info("validate update", "name", s.Name, "namespace", s.Namespace)

	return s.validateSleepInfo()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *SleepInfo) ValidateDelete() error {
	sleepinfolog.Info("validate delete", "name", r.Name)
	return nil
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
	if excludeRef.Name == "" && excludeRef.ApiVersion == "" && excludeRef.Kind == "" && len(excludeRef.MatchLabels) > 0 {
		return nil
	}
	if len(excludeRef.MatchLabels) == 0 && excludeRef.Name != "" && excludeRef.ApiVersion != "" && excludeRef.Kind != "" {
		return nil
	}
	return fmt.Errorf(`excludeRef is invalid. Must have set: matchLabels or name,apiVersion and kind fields`)
}
