/*
Copyright 2021.
*/

package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/kube-green/kube-green/internal/patcher"

	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Define a resource to filter, used to include or exclude resources from the sleep.
type FilterRef struct {
	// ApiVersion of the kubernetes resources.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
	// Kind of the kubernetes resources of the specific version.
	// +optional
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
	// WeekdaySleep are in cron notation.
	//
	// For example, to configure a schedule from monday to friday, set it to "1-5"
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	WeekDaySleep string `json:"weekdaySleep,omitempty"`
	// WeekDayWakeUp are in cron notation.
	//
	// For example, to configure a schedule from monday to friday, set it to "1-5"
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	WeekDayWakeUp string `json:"weekdayWakeUp,omitempty"`
	// Weekdays are in cron notation. Deprecated: use weekdaySleep and weekdayWakeUp instead.
	//
	// For example, to configure a schedule from monday to friday, set it to "1-5"
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Weekdays string `json:"weekdays,omitempty"`
	// Hours:Minutes
	//
	// Accept cron schedule for both hour and minute.
	// For example, *:*/2 is set to configure a run every even minute.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SleepTime string `json:"sleepAt,omitempty"`
	// Hours:Minutes
	//
	// Accept cron schedule for both hour and minute.
	// For example, *:*/2 is set to configure a run every even minute.
	// It is not required.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	WakeUpTime string `json:"wakeUpAt,omitempty"`
	// Time zone to set the schedule, in IANA time zone identifier.
	// It is not required, default to UTC.
	// For example, for the Italy time zone set Europe/Rome.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	TimeZone string `json:"timeZone,omitempty"`
	// ExcludeRef define the resource to exclude from the sleep.
	// Exclusion rules are evaluated in AND condition.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ExcludeRef []FilterRef `json:"excludeRef,omitempty"`
	// IncludeRef define the resource to include from the sleep.
	// Inclusion rules are evaluated in AND condition.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	IncludeRef []FilterRef `json:"includeRef,omitempty"`
	// If SuspendCronjobs is set to true, on sleep the cronjobs of the namespace will be suspended.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SuspendCronjobs bool `json:"suspendCronJobs,omitempty"`
	// If SuspendDeployments is set to false, on sleep the deployment of the namespace will not be suspended. By default Deployment will be suspended.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SuspendDeployments *bool `json:"suspendDeployments,omitempty"`
	// If SuspendStatefulSets is set to false, on sleep the statefulset of the namespace will not be suspended. By default StatefulSet will be suspended.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SuspendStatefulSets *bool `json:"suspendStatefulSets,omitempty"`
	// Patches is a list of json 6902 patches to apply to the target resources.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Patches []Patch `json:"patches,omitempty"`
	// ScheduleException define the exceptions to the schedule.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ScheduleException []ScheduleException `json:"scheduleException,omitempty"`
}

type ScheduleException struct {
	// Type of the schedule exception. Valid values are "override" and "disable".
	// "override" allows custom sleep/wake times for specific dates.
	// "disable" completely disables kube-green operations for specific dates.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Type ScheduleExceptionType `json:"type,omitempty"`
	// List of Day-Month dates.
	//
	// Accept cron schedule for both day and month.
	// For example, *-*/2 is set to configure a run every even month.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Dates []string `json:"dates"`
	// Description provides a human-readable explanation for the exception.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Description string `json:"description,omitempty"`
	// Hours:Minutes
	//
	// Accept cron schedule for both hour and minute.
	// For example, *:*/2 is set to configure a run every even minute.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SleepTime string `json:"sleepAt"`
	// Hours:Minutes for wake up time on override exceptions.
	//
	// Accept cron schedule for both hour and minute.
	// For example, *:*/2 is set to configure a run every even minute.
	// Only used when Type is "override".
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	WakeUpTime string `json:"wakeUpAt,omitempty"`
}

type Patch struct {
	// Target is the target resource to patch.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Target PatchTarget `json:"target"`
	// Patch is the json6902 patch to apply to the target resource.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Patch string `json:"patch"`
}

type PatchTarget struct {
	// Group of the Kubernetes resources.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Group string `json:"group"`
	// Kind of the Kubernetes resources.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Kind string `json:"kind"`
}

func (p PatchTarget) String() string {
	return fmt.Sprintf("%s.%s", p.Kind, p.Group)
}

func (p PatchTarget) GroupKind() schema.GroupKind {
	return schema.GroupKind{
		Group: p.Group,
		Kind:  p.Kind,
	}
}

// SleepInfoStatus defines the observed state of SleepInfo
type SleepInfoStatus struct {
	// Information when was the last time the run was successfully scheduled.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Last Schedule Time"
	LastScheduleTime metav1.Time `json:"lastScheduleTime,omitempty"`
	// The operation type handled in last schedule. SLEEP or WAKE_UP are the
	// possibilities
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Operation Type"
	OperationType string `json:"operation,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=sleepinfos
// +operator-sdk:csv:customresourcedefinitions:displayName="SleepInfo",resources={{Secret,v1,sleepinfo}}
// +genclient - this is required for auto generated docs

// SleepInfo is the Schema for the sleepinfos API
type SleepInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SleepInfoSpec   `json:"spec,omitempty"`
	Status SleepInfoStatus `json:"status,omitempty"`
}

func (s SleepInfo) GetSleepSchedule() (string, error) {
	return s.getScheduleFromWeekdayAndTime(s.Spec.WeekDaySleep, s.Spec.SleepTime)
}

func (s SleepInfo) GetWakeUpSchedule() (string, error) {
	if s.Spec.WakeUpTime == "" {
		return "", nil
	}
	return s.getScheduleFromWeekdayAndTime(s.Spec.WeekDayWakeUp, s.Spec.WakeUpTime)
}

func (s SleepInfo) GetIncludeRef() []FilterRef {
	return s.Spec.IncludeRef
}

func (s SleepInfo) GetExcludeRef() []FilterRef {
	return s.Spec.ExcludeRef
}

type ScheduleExceptionType string

const (
	Override ScheduleExceptionType = "override"
	Disable  ScheduleExceptionType = "disable"
)

type ParsedExceptionSchedule struct {
	WakeUpAt    string
	SleepAt     string
	DisableDate string
}

func (p ParsedExceptionSchedule) IsValid() bool {
	return (p.SleepAt != "" && p.WakeUpAt != "" && p.DisableDate == "") ||
		(p.SleepAt == "" && p.WakeUpAt == "" && p.DisableDate != "")
}

func (s SleepInfo) GetScheduleException() ([]ParsedExceptionSchedule, error) {
	scheduleExceptions := []ParsedExceptionSchedule{}
	for _, exception := range s.Spec.ScheduleException {
		if len(exception.Dates) == 0 {
			return nil, fmt.Errorf("field Dates must not be empty")
		}
		for _, date := range exception.Dates {
			switch exception.Type {
			case Disable:
				scheduleExceptions = append(scheduleExceptions, ParsedExceptionSchedule{
					DisableDate: date,
				})
			case Override:
				wakeUp, err := s.getSchedule(exception.WakeUpTime, "", date)
				if err != nil {
					return nil, err
				}
				sleepAt, err := s.getSchedule(exception.SleepTime, "", date)
				if err != nil {
					return nil, err
				}
				scheduleExceptions = append(scheduleExceptions, ParsedExceptionSchedule{
					WakeUpAt: wakeUp,
					SleepAt:  sleepAt,
				})
			default:
				return nil, fmt.Errorf("invalid exception type: '%s'", exception.Type)
			}
		}
	}
	return scheduleExceptions, nil
}

func (s SleepInfo) getScheduleFromWeekdayAndTime(weekday string, hourAndMinute string) (string, error) {
	if weekday == "" {
		weekday = s.Spec.Weekdays
	}
	if weekday == "" {
		return "", fmt.Errorf("empty weekdays and weekdaySleep or weekdayWakeUp from SleepInfo configuration")
	}

	return s.getSchedule(hourAndMinute, weekday, "")
}

func (s SleepInfo) getSchedule(hourAndMinute, weekday, date string) (string, error) {
	day := "*"
	month := "*"
	if weekday == "" {
		weekday = "*"
	}

	hour, minute, err := splitTime(hourAndMinute)
	if err != nil {
		return "", err
	}
	if date != "" {
		if day, month, err = splitDate(date); err != nil {
			return "", err
		}
	}
	schedule := fmt.Sprintf("%s %s %s %s %s", minute, hour, day, month, weekday)
	if s.Spec.TimeZone != "" {
		schedule = fmt.Sprintf("CRON_TZ=%s %s", s.Spec.TimeZone, schedule)
	}

	return schedule, nil
}

func splitTime(hourAndMinute string) (string, string, error) {
	splittedTime := strings.Split(hourAndMinute, ":")
	//nolint:mnd
	if len(splittedTime) != 2 {
		return "", "", fmt.Errorf("time should be of format HH:mm, actual: '%s'", hourAndMinute)
	}
	return splittedTime[0], splittedTime[1], nil
}

func splitDate(date string) (string, string, error) {
	splittedDate := strings.Split(date, "-")
	//nolint:mnd
	if len(splittedDate) != 2 {
		return "", "", fmt.Errorf("date should be of format dd-MM, actual: '%s'", date)
	}
	return splittedDate[0], splittedDate[1], nil
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

func (s SleepInfo) IsStatefulSetsToSuspend() bool {
	if s.Spec.SuspendStatefulSets == nil {
		return true
	}
	return *s.Spec.SuspendStatefulSets
}

func (s SleepInfo) GetPatches() []Patch {
	patches := []Patch{}
	if s.IsDeploymentsToSuspend() {
		patches = append(patches, deploymentPatch)
	}
	if s.IsStatefulSetsToSuspend() {
		patches = append(patches, statefulSetPatch)
	}
	if s.IsCronjobsToSuspend() {
		patches = append(patches, cronjobPatch)
	}
	return append(patches, s.Spec.Patches...)
}

func (s SleepInfo) Validate(cl client.Client) ([]string, error) {
	schedule, err := s.GetSleepSchedule()
	if err != nil {
		return nil, err
	}
	if err := validateSchedule(schedule); err != nil {
		return nil, err
	}

	schedule, err = s.GetWakeUpSchedule()
	if err != nil {
		return nil, err
	}
	if err := validateSchedule(schedule); err != nil {
		return nil, err
	}

	if err := s.validateScheduleException(); err != nil {
		return nil, err
	}

	for _, excludeRef := range s.GetExcludeRef() {
		if err := isExcludeRefValid(excludeRef); err != nil {
			return nil, err
		}
	}

	return s.validatePatches(cl)
}

func isExcludeRefValid(excludeRef FilterRef) error {
	if excludeRef.Name == "" && excludeRef.APIVersion == "" && excludeRef.Kind == "" && len(excludeRef.MatchLabels) > 0 {
		return nil
	}
	if len(excludeRef.MatchLabels) == 0 && excludeRef.Name != "" && excludeRef.APIVersion != "" && excludeRef.Kind != "" {
		return nil
	}
	return fmt.Errorf(`excludeRef is invalid. Must have set: matchLabels or name,apiVersion and kind fields`)
}

func (s SleepInfo) validateScheduleException() error {
	exceptions, err := s.GetScheduleException()
	if err != nil {
		return err
	}
	for _, exception := range exceptions {
		if !exception.IsValid() {
			return fmt.Errorf("invalid schedule exception: %v", exception)
		}
		if err := validateSchedule(exception.SleepAt); err != nil {
			return fmt.Errorf("invalid sleepAt schedule: %s", err)
		}
		if err := validateSchedule(exception.WakeUpAt); err != nil {
			return fmt.Errorf("invalid wakeUpAt schedule: %s", err)
		}
		if exception.DisableDate != "" {
			day, month, err := splitDate(exception.DisableDate)
			if err != nil {
				return err
			}

			if err := validateSchedule(fmt.Sprintf("* * %s %s *", day, month)); err != nil {
				return fmt.Errorf("invalid date %s: %w", exception.DisableDate, err)
			}
		}
	}
	return nil
}

func (s *SleepInfo) validatePatches(cl client.Client) ([]string, error) {
	warnings := []string{}
	for _, patch := range s.GetPatches() {
		if _, err := cl.RESTMapper().RESTMapping(patch.Target.GroupKind()); err != nil {
			warnings = append(warnings, fmt.Sprintf("SleepInfo patch target is invalid: %s", err))
		}

		if _, err := patcher.New([]byte(patch.Patch)); err != nil {
			return nil, fmt.Errorf("patch is invalid for target %s: %w", patch.Target, err)
		}
	}

	return warnings, nil
}

func validateSchedule(schedule string) error {
	if schedule == "" {
		return nil
	}
	if _, err := cron.ParseStandard(schedule); err != nil {
		return fmt.Errorf("invalid cron schedule: %s", err)
	}
	return nil
}

// +kubebuilder:object:root=true

// SleepInfoList contains a list of SleepInfo
type SleepInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SleepInfo `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SleepInfo{}, &SleepInfoList{})
}
