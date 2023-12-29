/*
Copyright 2021.
*/

package v1alpha1

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ExcludeRef struct {
	// ApiVersion of the kubernetes resources.
	// Supported api version is "apps/v1".
	APIVersion string `json:"apiVersion,omitempty"`
	// Kind of the kubernetes resources of the specific version.
	// Supported kind are "Deployment" and "CronJob".
	Kind string `json:"kind,omitempty"`
	// Name which identify the kubernetes resource.
	// +optional
	Name string `json:"name,omitempty"`
	// MatchLabels which identify the kubernetes resource by labels
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

// SleepInfoSpec defines the desired state of SleepInfo
type SleepInfoSpec struct {
	// Weekdays are in cron notation.
	//
	// For example, to configure a schedule from monday to friday, set it to "1-5"
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	Weekdays string `json:"weekdays"`
	// Hours:Minutes
	//
	// Accept cron schedule for both hour and minute.
	// For example, *:*/2 is set to configure a run every even minute.
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	SleepTime string `json:"sleepAt"`
	// Hours:Minutes
	//
	// Accept cron schedule for both hour and minute.
	// For example, *:*/2 is set to configure a run every even minute.
	// It is not required.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	WakeUpTime string `json:"wakeUpAt,omitempty"`
	// Time zone to set the schedule, in IANA time zone identifier.
	// It is not required, default to UTC.
	// For example, for the Italy time zone set Europe/Rome.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	TimeZone string `json:"timeZone,omitempty"`
	// ExcludeRef define the resource to exclude from the sleep.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	ExcludeRef []ExcludeRef `json:"excludeRef,omitempty"`
	// If SuspendCronjobs is set to true, on sleep the cronjobs of the namespace will be suspended.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	SuspendCronjobs bool `json:"suspendCronJobs,omitempty"`
	// If SuspendDeployments is set to false, on sleep the deployment of the namespace will not be suspended. By default Deployment will be suspended.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	SuspendDeployments *bool `json:"suspendDeployments,omitempty"`
	// PatchesJson6902 is a list of json6902 patches to apply to the target resources.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	Patches []Patches `json:"patches,omitempty"`
}

type Patches struct {
	// Target is the target resource to patch.
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	Target PatchTarget `json:"target"`
	// Patch is the json6902 patch to apply to the target resource.
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	Patch string `json:"patch"`
}

type PatchTarget struct {
	// Group of the Kubernetes resources.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	Group string `json:"group"`
	// Kind of the Kubernetes resources.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	Kind string `json:"kind"`
}

// SleepInfoStatus defines the observed state of SleepInfo
type SleepInfoStatus struct {
	// Information when was the last time the run was successfully scheduled.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Last Schedule Time"
	LastScheduleTime metav1.Time `json:"lastScheduleTime,omitempty"`
	// The operation type handled in last schedule. SLEEP or WAKE_UP are the
	// possibilities
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Operation Type"
	OperationType string `json:"operation,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=sleepinfos
//+operator-sdk:csv:customresourcedefinitions:displayName="SleepInfo",resources={{Secret,v1,sleepinfo}}
// +genclient - this is required for auto generated docs

// SleepInfo is the Schema for the sleepinfos API
type SleepInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SleepInfoSpec   `json:"spec,omitempty"`
	Status SleepInfoStatus `json:"status,omitempty"`
}

func (s SleepInfo) GetSleepSchedule() (string, error) {
	return s.getScheduleFromWeekdayAndTime(s.Spec.SleepTime)
}

func (s SleepInfo) GetWakeUpSchedule() (string, error) {
	if s.Spec.WakeUpTime == "" {
		return "", nil
	}
	return s.getScheduleFromWeekdayAndTime(s.Spec.WakeUpTime)
}

func (s SleepInfo) GetExcludeRef() []ExcludeRef {
	return s.Spec.ExcludeRef
}

func (s SleepInfo) getScheduleFromWeekdayAndTime(hourAndMinute string) (string, error) {
	weekday := s.Spec.Weekdays
	if weekday == "" {
		return "", fmt.Errorf("empty weekdays from SleepInfo configuration")
	}

	splittedTime := strings.Split(hourAndMinute, ":")
	//nolint:gomnd
	if len(splittedTime) != 2 {
		return "", fmt.Errorf("time should be of format HH:mm, actual: %s", hourAndMinute)
	}
	schedule := fmt.Sprintf("%s %s * * %s", splittedTime[1], splittedTime[0], weekday)
	if s.Spec.TimeZone != "" {
		schedule = fmt.Sprintf("CRON_TZ=%s %s", s.Spec.TimeZone, schedule)
	}
	return schedule, nil
}

func (s SleepInfo) IsCronjobsToSuspend() bool {
	return s.Spec.SuspendCronjobs
}

func (s SleepInfo) IsDeploymentsToSuspend() bool {
	if s.Spec.SuspendDeployments == nil {
		return true
	}
	return *s.Spec.SuspendDeployments
}

func (s SleepInfo) GetPatches() []Patches {
	return s.Spec.Patches
}

//+kubebuilder:object:root=true

// SleepInfoList contains a list of SleepInfo
type SleepInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SleepInfo `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SleepInfo{}, &SleepInfoList{})
}
