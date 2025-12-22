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
	// Weekdays are in cron notation.
	//
	// For example, to configure a schedule from monday to friday, set it to "1-5"
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Weekdays string `json:"weekdays"`
	// Hours:Minutes
	//
	// Accept cron schedule for both hour and minute.
	// For example, *:*/2 is set to configure a run every even minute.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SleepTime string `json:"sleepAt"`
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
	// If SuspendDeploymentsPgbouncer is set to true, on sleep all PgBouncer CRDs in the namespace
	// will be managed by modifying spec.instances (similar to native deployments with spec.replicas).
	// NOTE: PgBouncer is a CRD that generates Deployments (not StatefulSets), hence the "Deployments" prefix.
	// Defaults to false (does not manage PgBouncer).
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SuspendDeploymentsPgbouncer *bool `json:"suspendDeploymentsPgbouncer,omitempty"`
	// If SuspendStatefulSetsPostgres is set to true, on sleep all PgCluster CRDs in the namespace
	// will be managed by applying the pgcluster.stratio.com/shutdown annotation.
	// Defaults to false (does not manage PgCluster).
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SuspendStatefulSetsPostgres *bool `json:"suspendStatefulSetsPostgres,omitempty"`
	// If SuspendStatefulSetsHdfs is set to true, on sleep all HDFSCluster CRDs in the namespace
	// will be managed by applying the hdfscluster.stratio.com/shutdown annotation.
	// Defaults to false (does not manage HDFSCluster).
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SuspendStatefulSetsHdfs *bool `json:"suspendStatefulSetsHdfs,omitempty"`
	// If SuspendStatefulSetsOpenSearch is set to true, on sleep all OsCluster CRDs in the namespace
	// will be managed by applying the oscluster.stratio.com/shutdown annotation.
	// Defaults to false (does not manage OsCluster).
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SuspendStatefulSetsOpenSearch *bool `json:"suspendStatefulSetsOpenSearch,omitempty"`
	// If SuspendStatefulSetsOsDashboards is set to true, on sleep all OsDashboards CRDs in the namespace
	// will be managed by modifying spec.replicas (similar to native deployments with spec.replicas).
	// Defaults to false (does not manage OsDashboards).
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SuspendStatefulSetsOsDashboards *bool `json:"suspendStatefulSetsOsDashboards,omitempty"`
	// If SuspendStatefulSetsKafka is set to true, on sleep all KafkaCluster CRDs in the namespace
	// will be managed by applying the kafkacluster.stratio.com/shutdown annotation.
	// Defaults to false (does not manage KafkaCluster).
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SuspendStatefulSetsKafka *bool `json:"suspendStatefulSetsKafka,omitempty"`
	// Patches is a list of json 6902 patches to apply to the target resources.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Patches []Patch `json:"patches,omitempty"`
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
	return s.getScheduleFromWeekdayAndTime(s.Spec.SleepTime)
}

func (s SleepInfo) GetWakeUpSchedule() (string, error) {
	if s.Spec.WakeUpTime == "" {
		return "", nil
	}
	return s.getScheduleFromWeekdayAndTime(s.Spec.WakeUpTime)
}

func (s SleepInfo) GetIncludeRef() []FilterRef {
	return s.Spec.IncludeRef
}

func (s SleepInfo) GetExcludeRef() []FilterRef {
	return s.Spec.ExcludeRef
}

func (s SleepInfo) getScheduleFromWeekdayAndTime(hourAndMinute string) (string, error) {
	weekday := s.Spec.Weekdays
	if weekday == "" {
		return "", fmt.Errorf("empty weekdays from SleepInfo configuration")
	}

	splittedTime := strings.Split(hourAndMinute, ":")
	//nolint:mnd
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

func (s SleepInfo) IsStatefulSetsToSuspend() bool {
	if s.Spec.SuspendStatefulSets == nil {
		return true
	}
	return *s.Spec.SuspendStatefulSets
}

func (s SleepInfo) IsPgbouncerToSuspend() bool {
	if s.Spec.SuspendDeploymentsPgbouncer == nil {
		return false
	}
	return *s.Spec.SuspendDeploymentsPgbouncer
}

func (s SleepInfo) IsPostgresToSuspend() bool {
	if s.Spec.SuspendStatefulSetsPostgres == nil {
		return false
	}
	return *s.Spec.SuspendStatefulSetsPostgres
}

func (s SleepInfo) IsHdfsToSuspend() bool {
	if s.Spec.SuspendStatefulSetsHdfs == nil {
		return false
	}
	return *s.Spec.SuspendStatefulSetsHdfs
}

func (s SleepInfo) IsOpenSearchToSuspend() bool {
	if s.Spec.SuspendStatefulSetsOpenSearch == nil {
		return false
	}
	return *s.Spec.SuspendStatefulSetsOpenSearch
}

func (s SleepInfo) IsOsDashboardsToSuspend() bool {
	if s.Spec.SuspendStatefulSetsOsDashboards == nil {
		return false
	}
	return *s.Spec.SuspendStatefulSetsOsDashboards
}

func (s SleepInfo) IsKafkaToSuspend() bool {
	if s.Spec.SuspendStatefulSetsKafka == nil {
		return false
	}
	return *s.Spec.SuspendStatefulSetsKafka
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
	// EXTENSIÓN: Patches para CRDs
	if s.IsPgbouncerToSuspend() {
		patches = append(patches, pgbouncerPatch)
	}
	if s.IsOsDashboardsToSuspend() {
		patches = append(patches, OsdashboardsPatch)
	}
	// NOTA: Patches para PgCluster y HDFSCluster se agregan dinámicamente según operación (SLEEP/WAKE)
	// en el controller, ya que dependen de la anotación (true para sleep, false para wake)
	return append(patches, s.Spec.Patches...)
}

func (s SleepInfo) Validate(cl client.Client) ([]string, error) {
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

func isExcludeRefValid(excludeRef FilterRef) error {
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
			warnings = append(warnings, fmt.Sprintf("SleepInfo patch target is invalid: %s", err))
		}

		if _, err := patcher.New([]byte(patch.Patch)); err != nil {
			return nil, fmt.Errorf("patch is invalid for target %s: %w", patch.Target, err)
		}
	}

	return warnings, nil
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
