/*
Copyright 2021.
*/

package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type TargetRef struct {
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// SleepInfoSpec defines the desired state of SleepInfo
type SleepInfoSpec struct {
	// TargetRef     TargetRef `json:"targetRef"`
	SleepSchedule string `json:"sleepSchedule"`
	// RestoreAt               string    `json:"restoreAt"`
	// StartingDeadlineSeconds int64 `json:"startingDeadlineSeconds"`
}

type DeploymentRestoreInfo struct {
	Name     string `json:"name"`
	Replicas int64  `json:"replicas"`
}

// SleepInfoStatus defines the observed state of SleepInfo
type SleepInfoStatus struct {
	// LastScheduleTime      metav1.Time             `json:"lastScheduleTime"`
	NextScheduleTime metav1.Time `json:"nextScheduledTime"`
	// DeploymentRestoreInfo []DeploymentRestoreInfo `json:"deploymentRestoreInfo"`
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

func getParsedDate(date string) (time.Time, error) {
	return time.Parse(time.RFC3339, date)
}
