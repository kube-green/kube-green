/*
Copyright 2021.
*/

package v1alpha1

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SleepInfoSpec defines the desired state of SleepInfo
type SleepInfoSpec struct {
	// Weekdays are in cron notation.
	//
	// For example, to configure a schedule from monday to friday, set it to "1-5"
	Weekdays string `json:"weekdays"`
	// Hours:Minutes
	//
	// Accept cron schedule for both hour and minute.
	// For example, *:*/2 is set to configure a run every even minute.
	SleepTime string `json:"sleepAt"`
	// Hours:Minutes
	//
	// Accept cron schedule for both hour and minute.
	// For example, *:*/2 is set to configure a run every even minute.
	// It is not required.
	// +optional
	WakeUpTime string `json:"wakeUpAt,omitempty"`
	// Time zone to set the schedule, in IANA time zone identifier.
	// It is not required, default to UTC.
	// For example, for the Italy time zone set Europe/Rome.
	// +optional
	TimeZone string `json:"timeZone,omitempty"`
}

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

func (s SleepInfo) GetSleepSchedule() (string, error) {
	return s.getScheduleFromWeekdayAndTime(s.Spec.SleepTime)
}

func (s SleepInfo) GetWakeUpSchedule() (string, error) {
	if s.Spec.WakeUpTime == "" {
		return "", nil
	}
	return s.getScheduleFromWeekdayAndTime(s.Spec.WakeUpTime)
}

func (s SleepInfo) getScheduleFromWeekdayAndTime(hourAndMinute string) (string, error) {
	weekday := s.Spec.Weekdays
	if weekday == "" {
		return "", fmt.Errorf("empty weekdays from sleep info configuration")
	}

	splittedTime := strings.Split(hourAndMinute, ":")
	if len(splittedTime) != 2 {
		return "", fmt.Errorf("time should be of format HH:mm, actual: %s", hourAndMinute)
	}
	schedule := fmt.Sprintf("%s %s * * %s", splittedTime[1], splittedTime[0], weekday)
	if s.Spec.TimeZone != "" {
		schedule = fmt.Sprintf("CRON_TZ=%s %s", s.Spec.TimeZone, schedule)
	}
	return schedule, nil
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
