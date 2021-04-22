/*
Copyright 2021.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SleepInfoSpec defines the desired state of SleepInfo
type SleepInfoSpec struct {
	// Weekdays are in cron notation
	Weekdays string `json:"weekdays"`
	// Hours:Minutes
	// Accept cron schedule for both hour and minute. A possible configuration, for example, is *:*/2 for every even minute.
	SleepTime string `json:"sleepAt"`
	// Hours:Minutes
	// Accept cron schedule for both hour and minute. A possible configuration, for example, is *:*/2 for every even minute.
	// +optional
	WakeUpTime string `json:"wakeUpAt"`
}

// TODO: save changed replica deployment info in sleepinfo status?
// type DeploymentWakeUpInfo struct {
// 	Name     string `json:"name"`
// 	Replicas int64  `json:"replicas"`
// }

// SleepInfoStatus defines the observed state of SleepInfo
type SleepInfoStatus struct {
	// Information when was the last time the run was successfully scheduled.
	// +optional
	LastScheduleTime metav1.Time `json:"lastScheduleTime,omitempty"`
	// The operation type handled in last schedule. SLEEP or WAKE_UP are the
	// possibilities
	// +optional
	OperationType string `json:"operation,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=sleepinfos

// SleepInfo is the Schema for the sleepinfos API
type SleepInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SleepInfoSpec   `json:"spec,omitempty"`
	Status SleepInfoStatus `json:"status,omitempty"`
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
