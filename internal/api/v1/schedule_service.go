/*
Copyright 2025.
*/

package v1

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ValidNamespaceSuffixes are the supported namespace suffixes
	ValidNamespaceSuffixes = "datastores,apps,rocket,intelligence,airflowsso"
)

var (
	validSuffixes = []string{"datastores", "apps", "rocket", "intelligence", "airflowsso"}
)

// ScheduleService handles schedule operations
type ScheduleService struct {
	client client.Client
	logger logger
}

type logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(err error, msg string, keysAndValues ...interface{})
}

// NewScheduleService creates a new schedule service
func NewScheduleService(c client.Client, l logger) *ScheduleService {
	return &ScheduleService{
		client: c,
		logger: l,
	}
}

// CreateSchedule creates SleepInfo objects for the tenant
func (s *ScheduleService) CreateSchedule(ctx context.Context, req CreateScheduleRequest) error {
	s.logger.Info("CreateSchedule CALLED", "tenant", req.Tenant, "off", req.Off, "on", req.On, "weekdays", req.Weekdays, "sleepDays", req.SleepDays, "wakeDays", req.WakeDays, "namespaces", fmt.Sprintf("%v", req.Namespaces))

	// 1. Normalize weekdays
	wdDefault := "0-6"
	wdSleep := wdDefault
	wdWake := wdDefault

	// Use SleepDays and WakeDays if provided, otherwise use Weekdays
	if req.SleepDays != "" {
		var err error
		wdSleep, err = HumanWeekdaysToKube(req.SleepDays)
		if err != nil {
			return fmt.Errorf("invalid sleepDays: %w", err)
		}
	} else if req.Weekdays != "" {
		var err error
		wdSleep, err = HumanWeekdaysToKube(req.Weekdays)
		if err != nil {
			return fmt.Errorf("invalid weekdays: %w", err)
		}
	}

	if req.WakeDays != "" {
		var err error
		wdWake, err = HumanWeekdaysToKube(req.WakeDays)
		if err != nil {
			return fmt.Errorf("invalid wakeDays: %w", err)
		}
	} else {
		// If wakeDays is not provided, use sleepDays or weekdays
		wdWake = wdSleep
	}

	// 2. Convert times from local timezone (America/Bogota) to UTC
	userTZ := TZLocal // Default to America/Bogota
	clusterTZ := TZUTC // Default to UTC

	offConv, err := ToUTCHHMM(req.Off, userTZ)
	if err != nil {
		s.logger.Error(err, "failed to convert off time", "off", req.Off, "userTZ", userTZ)
		return fmt.Errorf("invalid off time: %w", err)
	}
	s.logger.Info("Time conversion: off", "userTime", req.Off, "clusterTime", offConv.TimeUTC, "dayShift", offConv.DayShift, "userTZ", userTZ, "clusterTZ", clusterTZ)

	onConv, err := ToUTCHHMM(req.On, userTZ)
	if err != nil {
		s.logger.Error(err, "failed to convert on time", "on", req.On, "userTZ", userTZ)
		return fmt.Errorf("invalid on time: %w", err)
	}
	s.logger.Info("Time conversion: on", "userTime", req.On, "clusterTime", onConv.TimeUTC, "dayShift", onConv.DayShift, "userTZ", userTZ, "clusterTZ", clusterTZ)

	// 3. Adjust weekdays for timezone shift
	wdSleepUTC, err := ShiftWeekdaysStr(wdSleep, offConv.DayShift)
	if err != nil {
		return fmt.Errorf("failed to shift sleep weekdays: %w", err)
	}

	wdWakeUTC, err := ShiftWeekdaysStr(wdWake, onConv.DayShift)
	if err != nil {
		return fmt.Errorf("failed to shift wake weekdays: %w", err)
	}

	// 4. Calculate staggered wake times based on delays
	// IMPORTANTE: Los delays por defecto SOLO se aplican para datastores (que tiene CRDs)
	// Para namespaces simples (apps, rocket, intelligence, airflowsso), usar el tiempo convertido directamente
	onPgHDFS := onConv.TimeUTC
	onPgBouncer := onConv.TimeUTC
	onDeployments := onConv.TimeUTC

	// Solo aplicar delays si se especifican explícitamente en req.Delays
	// Los delays por defecto (5m, 7m) SOLO se aplicarán en createDatastoresSleepInfos cuando sea necesario
	if req.Delays != nil {
		// Parse delays and apply them
		if req.Delays.PgHdfsDelay != "" {
			delayMinutes, _ := parseDelayToMinutes(req.Delays.PgHdfsDelay)
			onPgHDFS, _ = AddMinutes(onConv.TimeUTC, delayMinutes)
		}
		if req.Delays.PgbouncerDelay != "" {
			delayMinutes, _ := parseDelayToMinutes(req.Delays.PgbouncerDelay)
			onPgBouncer, _ = AddMinutes(onConv.TimeUTC, delayMinutes)
		}
		if req.Delays.DeploymentsDelay != "" {
			delayMinutes, _ := parseDelayToMinutes(req.Delays.DeploymentsDelay)
			onDeployments, _ = AddMinutes(onConv.TimeUTC, delayMinutes)
		}
	}
	// NO aplicar delays por defecto aquí - se aplicarán solo en createDatastoresSleepInfos si es necesario

	// 5. Determine which namespaces to process
	selectedNamespaces := normalizeNamespaces(req.Namespaces)

	// 6. Build excludeRef from exclusions (no exclusions in CreateScheduleRequest, use defaults)

	// 7. Validate scheduleName uniqueness if provided
	if req.ScheduleName != "" {
		for suffix := range selectedNamespaces {
			namespace := fmt.Sprintf("%s-%s", req.Tenant, suffix)
			if err := s.validateScheduleNameUniqueness(ctx, namespace, req.ScheduleName); err != nil {
				return err
			}
		}
	}

	// 8. Create SleepInfo objects for each namespace
	// NO iterar sobre validSuffixes hardcodeados - usar los namespaces seleccionados dinámicamente
	s.logger.Info("CreateSchedule: processing namespaces", "count", len(selectedNamespaces), "namespaces", fmt.Sprintf("%v", selectedNamespaces))
	for suffix := range selectedNamespaces {
		namespace := fmt.Sprintf("%s-%s", req.Tenant, suffix)
		s.logger.Info("CreateSchedule: processing namespace", "suffix", suffix, "namespace", namespace)

		// Build excludeRef from exclusions (no custom exclusions in CreateScheduleRequest)
		excludeRefs := getExcludeRefsForOperators()

		// DYNAMIC LOGIC: Detect resources in namespace to determine what type of SleepInfos to create
		// This replaces hardcoded switch statements and works with ANY namespace
		resources, err := s.GetNamespaceResources(ctx, req.Tenant, suffix)
		if err != nil {
			s.logger.Error(err, "failed to detect resources in namespace", "namespace", namespace)
			// Continue with default behavior if resource detection fails
			resources = &NamespaceResourceInfo{
				Namespace:      namespace,
				HasPgCluster:   false,
				HasHdfsCluster: false,
				HasPgBouncer:   false,
				HasVirtualizer: false,
				AutoExclusions: []ExclusionFilter{},
			}
		}

		// Note: Virtualizer detection is available in resources.HasVirtualizer
		// but exclusion must be configured manually from frontend if needed
		// No automatic exclusion is applied

		// Add auto-exclusions from resource detection
		for _, autoExcl := range resources.AutoExclusions {
			excludeRefs = append(excludeRefs, kubegreenv1alpha1.FilterRef{
				MatchLabels: autoExcl.MatchLabels,
			})
		}

		// Calculate wake times - apply delays if provided, otherwise use defaults for CRDs
		onPgHDFSFinal := onPgHDFS
		onPgBouncerFinal := onPgBouncer
		onDeploymentsFinal := onDeployments

		// If no custom delays provided, apply default staggered wake for namespaces with CRDs
		if req.Delays == nil {
			hasCRDs := resources.HasPgCluster || resources.HasHdfsCluster || resources.HasPgBouncer
			if hasCRDs {
				// Default staggered wake: PgHDFS at t0, PgBouncer at t0+5m, Deployments at t0+7m
				onPgHDFSFinal = onConv.TimeUTC
				onPgBouncerFinal, _ = AddMinutes(onConv.TimeUTC, 5)
				onDeploymentsFinal, _ = AddMinutes(onConv.TimeUTC, 7)
			} else {
				// Simple namespaces: all wake at the same time
				onPgHDFSFinal = onConv.TimeUTC
				onPgBouncerFinal = onConv.TimeUTC
				onDeploymentsFinal = onConv.TimeUTC
			}
		} else {
			// Use custom delays from request
			onPgHDFSFinal = onPgHDFS
			onPgBouncerFinal = onPgBouncer
			onDeploymentsFinal = onDeployments
		}

		// Generate SleepInfos based on detected resources (DYNAMIC LOGIC - no hardcoded names)
		hasCRDs := resources.HasPgCluster || resources.HasHdfsCluster || resources.HasPgBouncer

		if hasCRDs {
			// Namespace has CRDs: use staggered wake logic
			s.logger.Info("CreateSchedule: creating staggered SleepInfos (CRDs detected)", "namespace", namespace, "hasPgCluster", resources.HasPgCluster, "hasHdfsCluster", resources.HasHdfsCluster, "hasPgBouncer", resources.HasPgBouncer)
			if err := s.createDatastoresSleepInfosWithExclusions(ctx, req.Tenant, namespace, offConv.TimeUTC, onDeploymentsFinal, onPgHDFSFinal, onPgBouncerFinal, wdSleepUTC, wdWakeUTC, excludeRefs, req.ScheduleName, req.Description, userTZ); err != nil {
				s.logger.Error(err, "failed to create staggered sleepinfos", "namespace", namespace)
				return fmt.Errorf("failed to create staggered sleepinfos for %s: %w", namespace, err)
			}
			s.logger.Info("CreateSchedule: staggered SleepInfos created successfully", "namespace", namespace)
		} else {
			// Simple namespace without CRDs
			suspendStatefulSets := false

			// Special case: if namespace has PgCluster but wasn't detected as CRD, still suspend StatefulSets
			if resources.HasPgCluster {
				suspendStatefulSets = true
			}

			s.logger.Info("CreateSchedule: creating simple namespace SleepInfos", "namespace", namespace, "suspendStatefulSets", suspendStatefulSets)
			if err := s.createNamespaceSleepInfoWithExclusions(ctx, req.Tenant, namespace, suffix, offConv.TimeUTC, onDeploymentsFinal, wdSleepUTC, wdWakeUTC, suspendStatefulSets, excludeRefs, req.ScheduleName, req.Description, userTZ); err != nil {
				s.logger.Error(err, "failed to create namespace sleepinfo", "namespace", namespace)
				return fmt.Errorf("failed to create sleepinfo for %s: %w", namespace, err)
			}
			s.logger.Info("CreateSchedule: namespace SleepInfos created successfully", "namespace", namespace)
		}
	}

	s.logger.Info("CreateSchedule COMPLETED", "tenant", req.Tenant, "namespaces_processed", len(selectedNamespaces))
	return nil
}

// parseDelayToMinutes parses a delay string (e.g., "5m", "10m", "30s") to minutes
func parseDelayToMinutes(delayStr string) (int, error) {
	if delayStr == "" {
		return 0, nil
	}

	// Remove trailing 'm' or 's' or 'h'
	delayStr = strings.TrimSpace(delayStr)
	if len(delayStr) < 2 {
		return 0, fmt.Errorf("invalid delay format: %s", delayStr)
	}

	unit := delayStr[len(delayStr)-1:]
	valueStr := delayStr[:len(delayStr)-1]

	var value int
	if _, err := fmt.Sscanf(valueStr, "%d", &value); err != nil {
		return 0, fmt.Errorf("invalid delay value: %s", delayStr)
	}

	switch unit {
	case "s":
		return value / 60, nil
	case "m":
		return value, nil
	case "h":
		return value * 60, nil
	default:
		return 0, fmt.Errorf("invalid delay unit: %s (expected s, m, or h)", unit)
	}
}

// normalizeNamespaces normalizes namespace input
// NO filtra por validSuffixes - acepta cualquier namespace dinámicamente
func normalizeNamespaces(nsInput []string) map[string]bool {
	result := make(map[string]bool)

	// Si no hay input, retornar mapa vacío (no todos los namespaces por defecto)
	// El llamado debe especificar explícitamente los namespaces deseados
	if len(nsInput) == 0 {
		return result
	}

	// Agregar todos los namespaces proporcionados (sin filtrar por validSuffixes)
	for _, ns := range nsInput {
		ns = strings.ToLower(strings.TrimSpace(ns))
		if ns != "" {
			result[ns] = true
		}
	}

	return result
}

func isNamespaceSelected(selected map[string]bool, suffix string) bool {
	return selected[suffix]
}

// createNamespaceSleepInfoWithExclusions creates a simple SleepInfo for a namespace with custom exclusions
func (s *ScheduleService) createNamespaceSleepInfoWithExclusions(ctx context.Context, tenant, namespace, suffix, offUTC, onUTC, wdSleep, wdWake string, suspendStatefulSets bool, excludeRefs []kubegreenv1alpha1.FilterRef, scheduleName, description, userTimezone string) error {
	// Check if weekdays are the same
	sleepDays, _ := ExpandWeekdaysStr(wdSleep)
	wakeDays, _ := ExpandWeekdaysStr(wdWake)

	daysEqual := len(sleepDays) == len(wakeDays)
	if daysEqual {
		for i, d := range sleepDays {
			if i >= len(wakeDays) || d != wakeDays[i] {
				daysEqual = false
				break
			}
		}
	}

	var sleepInfo *kubegreenv1alpha1.SleepInfo

	if daysEqual {
		// Single SleepInfo with sleepAt and wakeUpAt
		suspendDeployments := true
		suspendCronJobs := true

		// Generate name based on scheduleName or default pattern
		name := fmt.Sprintf("%s-%s", tenant, suffix)
		if scheduleName != "" {
			name = scheduleName
		}

		// Initialize annotations with schedule name and description
		// Build annotations including userTimezone
		annotations := make(map[string]string)
		if scheduleName != "" {
			annotations["kube-green.stratio.com/schedule-name"] = scheduleName
		}
		if description != "" {
			annotations["kube-green.stratio.com/schedule-description"] = description
		}
		// Store userTimezone in annotations for easy access
		if userTimezone != "" {
			annotations["kube-green.stratio.com/user-timezone"] = userTimezone
			s.logger.Info("createNamespaceSleepInfoWithExclusions: ADDED user-timezone to single SleepInfo annotations", "name", name, "namespace", namespace, "userTimezone", userTimezone)
		} else {
			s.logger.Error(nil, "createNamespaceSleepInfoWithExclusions: userTimezone is EMPTY for single SleepInfo", "name", name, "namespace", namespace)
		}

		s.logger.Info("createNamespaceSleepInfoWithExclusions: creating single SleepInfo", "name", name, "namespace", namespace, "userTimezone", userTimezone, "hasUserTimezoneAnnotation", annotations["kube-green.stratio.com/user-timezone"] != "")
		sleepInfo = &kubegreenv1alpha1.SleepInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: annotations,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays:           wdSleep,
				SleepTime:          offUTC,
				WakeUpTime:         onUTC,
				TimeZone:           "UTC",
				SuspendDeployments: &suspendDeployments,
				SuspendStatefulSets: func() *bool {
					b := suspendStatefulSets
					return &b
				}(),
				SuspendCronjobs: suspendCronJobs,
			},
		}

		// Add Virtualizer exclusion for apps
		if suffix == "apps" {
			virtualizerExclusion := kubegreenv1alpha1.FilterRef{
				MatchLabels: map[string]string{
					"cct.stratio.com/application_id": fmt.Sprintf("virtualizer.%s", namespace),
				},
			}
			excludeRefs = append(excludeRefs, virtualizerExclusion)
		}

		if len(excludeRefs) > 0 {
			sleepInfo.Spec.ExcludeRef = excludeRefs
		}
	} else {
		// Separate SleepInfos for sleep and wake
		sharedID := fmt.Sprintf("%s-%s", tenant, suffix)
		if scheduleName != "" {
			sharedID = scheduleName
		}

		suspendDeployments := true
		suspendCronJobs := true

		// Generate names based on scheduleName or default pattern
		sleepName := fmt.Sprintf("sleep-%s-%s", tenant, suffix)
		wakeName := fmt.Sprintf("wake-%s-%s", tenant, suffix)
		if scheduleName != "" {
			sleepName = fmt.Sprintf("sleep-%s", scheduleName)
			wakeName = fmt.Sprintf("wake-%s", scheduleName)
		}

		// Initialize annotations with schedule name, description, and userTimezone
		sleepAnnotations := map[string]string{
			"kube-green.stratio.com/pair-id":   sharedID,
			"kube-green.stratio.com/pair-role": "sleep",
		}
		wakeAnnotations := map[string]string{
			"kube-green.stratio.com/pair-id":   sharedID,
			"kube-green.stratio.com/pair-role": "wake",
		}
		if scheduleName != "" {
			sleepAnnotations["kube-green.stratio.com/schedule-name"] = scheduleName
			wakeAnnotations["kube-green.stratio.com/schedule-name"] = scheduleName
		}
		if description != "" {
			sleepAnnotations["kube-green.stratio.com/schedule-description"] = description
			wakeAnnotations["kube-green.stratio.com/schedule-description"] = description
		}
		// Store userTimezone in annotations for easy access
		if userTimezone != "" {
			sleepAnnotations["kube-green.stratio.com/user-timezone"] = userTimezone
			wakeAnnotations["kube-green.stratio.com/user-timezone"] = userTimezone
			s.logger.Info("createNamespaceSleepInfoWithExclusions: ADDED user-timezone to annotations", "name", sleepName, "namespace", namespace, "userTimezone", userTimezone, "sleepAnnotations", fmt.Sprintf("%+v", sleepAnnotations))
		} else {
			s.logger.Error(nil, "createNamespaceSleepInfoWithExclusions: userTimezone is EMPTY, not adding to annotations", "name", sleepName, "namespace", namespace)
		}

		// Sleep SleepInfo
		s.logger.Info("createNamespaceSleepInfoWithExclusions: creating sleep SleepInfo", "name", sleepName, "namespace", namespace, "userTimezone", userTimezone, "hasUserTimezoneAnnotation", sleepAnnotations["kube-green.stratio.com/user-timezone"] != "")
		sleepSleepInfo := &kubegreenv1alpha1.SleepInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:        sleepName,
				Namespace:   namespace,
				Annotations: sleepAnnotations,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays:           wdSleep,
				SleepTime:          offUTC,
				TimeZone:           "UTC",
				SuspendDeployments: &suspendDeployments,
				SuspendStatefulSets: func() *bool {
					b := suspendStatefulSets
					return &b
				}(),
				SuspendCronjobs: suspendCronJobs,
			},
		}

		// Wake SleepInfo
		wakeSleepInfo := &kubegreenv1alpha1.SleepInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:        wakeName,
				Namespace:   namespace,
				Annotations: wakeAnnotations,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays:           wdWake,
				SleepTime:          onUTC,
				TimeZone:           "UTC",
				SuspendDeployments: &suspendDeployments,
				SuspendStatefulSets: func() *bool {
					b := suspendStatefulSets
					return &b
				}(),
				SuspendCronjobs: suspendCronJobs,
			},
		}

		// Add Virtualizer exclusion for apps
		if suffix == "apps" {
			virtualizerExclusion := kubegreenv1alpha1.FilterRef{
				MatchLabels: map[string]string{
					"cct.stratio.com/application_id": fmt.Sprintf("virtualizer.%s", namespace),
				},
			}
			excludeRefs = append(excludeRefs, virtualizerExclusion)
		}

		if len(excludeRefs) > 0 {
			sleepSleepInfo.Spec.ExcludeRef = excludeRefs
			wakeSleepInfo.Spec.ExcludeRef = excludeRefs
		}

		// Create or update both SleepInfos
		s.logger.Info("createNamespaceSleepInfoWithExclusions: creating/updating sleep SleepInfo", "name", sleepSleepInfo.Name, "namespace", sleepSleepInfo.Namespace, "sleepTime", sleepSleepInfo.Spec.SleepTime, "weekdays", sleepSleepInfo.Spec.Weekdays)
		if err := s.createOrUpdateSleepInfo(ctx, sleepSleepInfo, userTimezone); err != nil {
			s.logger.Error(err, "failed to create/update sleep SleepInfo", "name", sleepSleepInfo.Name, "namespace", sleepSleepInfo.Namespace)
			return err
		}
		s.logger.Info("createNamespaceSleepInfoWithExclusions: sleep SleepInfo created/updated successfully", "name", sleepSleepInfo.Name, "namespace", sleepSleepInfo.Namespace)

		s.logger.Info("createNamespaceSleepInfoWithExclusions: creating/updating wake SleepInfo", "name", wakeSleepInfo.Name, "namespace", wakeSleepInfo.Namespace, "sleepTime", wakeSleepInfo.Spec.SleepTime, "weekdays", wakeSleepInfo.Spec.Weekdays)
		if err := s.createOrUpdateSleepInfo(ctx, wakeSleepInfo, userTimezone); err != nil {
			s.logger.Error(err, "failed to create/update wake SleepInfo", "name", wakeSleepInfo.Name, "namespace", wakeSleepInfo.Namespace)
			return err
		}
		s.logger.Info("createNamespaceSleepInfoWithExclusions: wake SleepInfo created/updated successfully", "name", wakeSleepInfo.Name, "namespace", wakeSleepInfo.Namespace)

		return nil
	}

	// Create or update the SleepInfo (single SleepInfo case)
	s.logger.Info("createNamespaceSleepInfoWithExclusions: creating/updating single SleepInfo", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace, "sleepTime", sleepInfo.Spec.SleepTime, "wakeTime", sleepInfo.Spec.WakeUpTime, "weekdays", sleepInfo.Spec.Weekdays)
	if err := s.createOrUpdateSleepInfo(ctx, sleepInfo, userTimezone); err != nil {
		s.logger.Error(err, "failed to create/update SleepInfo", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
		return err
	}
	s.logger.Info("createNamespaceSleepInfoWithExclusions: SleepInfo created/updated successfully", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)

	return nil
}

// createDatastoresSleepInfosWithExclusions creates the complex SleepInfos for datastores namespace with custom exclusions
func (s *ScheduleService) createDatastoresSleepInfosWithExclusions(ctx context.Context, tenant, namespace, offUTC, onDeployments, onPgHDFS, onPgBouncer, wdSleep, wdWake string, excludeRefs []kubegreenv1alpha1.FilterRef, scheduleName, description, userTimezone string) error {
	suspendDeployments := true
	suspendStatefulSets := true
	suspendCronJobs := true
	suspendPgbouncer := true
	suspendPostgres := true
	suspendHdfs := true

	// Check if weekdays are the same
	sleepDays, _ := ExpandWeekdaysStr(wdSleep)
	wakeDays, _ := ExpandWeekdaysStr(wdWake)

	daysEqual := len(sleepDays) == len(wakeDays)
	if daysEqual {
		for i, d := range sleepDays {
			if i >= len(wakeDays) || d != wakeDays[i] {
				daysEqual = false
				break
			}
		}
	}

	sharedID := fmt.Sprintf("%s-datastores", tenant)
	if scheduleName != "" {
		sharedID = scheduleName
	}

	if daysEqual {
		// Single sleep SleepInfo with all resources
		// Generate name based on scheduleName or default pattern
		sleepName := fmt.Sprintf("sleep-ds-deploys-%s", tenant)
		if scheduleName != "" {
			sleepName = fmt.Sprintf("sleep-%s", scheduleName)
		}

		// Initialize annotations with schedule name, description, and userTimezone
		sleepAnnotations := map[string]string{
			"kube-green.stratio.com/pair-id":   sharedID,
			"kube-green.stratio.com/pair-role": "sleep",
		}
		if scheduleName != "" {
			sleepAnnotations["kube-green.stratio.com/schedule-name"] = scheduleName
		}
		if description != "" {
			sleepAnnotations["kube-green.stratio.com/schedule-description"] = description
		}
		// Store userTimezone in annotations for easy access
		if userTimezone != "" {
			sleepAnnotations["kube-green.stratio.com/user-timezone"] = userTimezone
		}

		sleepInfo := &kubegreenv1alpha1.SleepInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:        sleepName,
				Namespace:   namespace,
				Annotations: sleepAnnotations,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays:                    wdSleep,
				SleepTime:                   offUTC,
				TimeZone:                    "UTC",
				SuspendDeployments:          &suspendDeployments,
				SuspendStatefulSets:         &suspendStatefulSets,
				SuspendCronjobs:             suspendCronJobs,
				SuspendDeploymentsPgbouncer: &suspendPgbouncer,
				SuspendStatefulSetsPostgres: &suspendPostgres,
				SuspendStatefulSetsHdfs:     &suspendHdfs,
				ExcludeRef:                  excludeRefs,
			},
		}

		// Create wake SleepInfos (staged)
		// 1. Postgres and HDFS first
		wakePgHdfsName := fmt.Sprintf("wake-ds-deploys-%s-pg-hdfs", tenant)
		if scheduleName != "" {
			wakePgHdfsName = fmt.Sprintf("wake-%s-pg-hdfs", scheduleName)
		}

		wakePgHdfsAnnotations := map[string]string{
			"kube-green.stratio.com/pair-id":   sharedID,
			"kube-green.stratio.com/pair-role": "wake",
		}
		if scheduleName != "" {
			wakePgHdfsAnnotations["kube-green.stratio.com/schedule-name"] = scheduleName
		}
		if description != "" {
			wakePgHdfsAnnotations["kube-green.stratio.com/schedule-description"] = description
		}

		// Wake PgHDFS: debe tener suspendDeployments=False, suspendStatefulSets=False, suspendCronJobs=False, suspendDeploymentsPgbouncer=False
		// pero suspendStatefulSetsPostgres=True y suspendStatefulSetsHdfs=True (para restaurar solo Postgres y HDFS)
		suspendDeploymentsFalse := false
		suspendStatefulSetsFalse := false
		suspendCronJobsFalse := false
		suspendPgbouncerFalse := false

		wakePgHdfs := &kubegreenv1alpha1.SleepInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:        wakePgHdfsName,
				Namespace:   namespace,
				Annotations: wakePgHdfsAnnotations,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays:                    wdWake,
				SleepTime:                   onPgHDFS,
				TimeZone:                    "UTC",
				SuspendDeployments:          &suspendDeploymentsFalse,
				SuspendStatefulSets:         &suspendStatefulSetsFalse,
				SuspendCronjobs:             suspendCronJobsFalse,
				SuspendDeploymentsPgbouncer: &suspendPgbouncerFalse,
				SuspendStatefulSetsPostgres: &suspendPostgres,
				SuspendStatefulSetsHdfs:     &suspendHdfs,
				ExcludeRef:                  excludeRefs,
			},
		}

		// 2. PgBouncer second
		wakePgbouncerName := fmt.Sprintf("wake-ds-deploys-%s-pgbouncer", tenant)
		if scheduleName != "" {
			wakePgbouncerName = fmt.Sprintf("wake-%s-pgbouncer", scheduleName)
		}

		wakePgbouncerAnnotations := map[string]string{
			"kube-green.stratio.com/pair-id":   sharedID,
			"kube-green.stratio.com/pair-role": "wake",
		}
		if scheduleName != "" {
			wakePgbouncerAnnotations["kube-green.stratio.com/schedule-name"] = scheduleName
		}
		if description != "" {
			wakePgbouncerAnnotations["kube-green.stratio.com/schedule-description"] = description
		}

		// Wake PgBouncer: debe tener suspendDeployments=False, suspendStatefulSets=False, suspendCronJobs=False
		// pero suspendDeploymentsPgbouncer=True (para restaurar solo PgBouncer)
		wakePgbouncer := &kubegreenv1alpha1.SleepInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:        wakePgbouncerName,
				Namespace:   namespace,
				Annotations: wakePgbouncerAnnotations,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays:                    wdWake,
				SleepTime:                   onPgBouncer,
				TimeZone:                    "UTC",
				SuspendDeployments:          &suspendDeploymentsFalse,
				SuspendStatefulSets:         &suspendStatefulSetsFalse,
				SuspendCronjobs:             suspendCronJobsFalse,
				SuspendDeploymentsPgbouncer: &suspendPgbouncer,
				SuspendStatefulSetsPostgres: &suspendStatefulSetsFalse,
				SuspendStatefulSetsHdfs:     &suspendStatefulSetsFalse,
				ExcludeRef:                  excludeRefs,
			},
		}

		// 3. Native deployments last
		wakeDeploymentsName := fmt.Sprintf("wake-ds-deploys-%s", tenant)
		if scheduleName != "" {
			wakeDeploymentsName = fmt.Sprintf("wake-%s", scheduleName)
		}

		wakeDeploymentsAnnotations := map[string]string{
			"kube-green.stratio.com/pair-id":   sharedID,
			"kube-green.stratio.com/pair-role": "wake",
		}
		if scheduleName != "" {
			wakeDeploymentsAnnotations["kube-green.stratio.com/schedule-name"] = scheduleName
		}
		if description != "" {
			wakeDeploymentsAnnotations["kube-green.stratio.com/schedule-description"] = description
		}

		wakeDeployments := &kubegreenv1alpha1.SleepInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:        wakeDeploymentsName,
				Namespace:   namespace,
				Annotations: wakeDeploymentsAnnotations,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays:                    wdWake,
				SleepTime:                   onDeployments,
				TimeZone:                    "UTC",
				SuspendDeployments:          &suspendDeployments,
				SuspendStatefulSets:         &suspendStatefulSets,
				SuspendCronjobs:             suspendCronJobs,
				SuspendDeploymentsPgbouncer: &suspendPgbouncer,
				ExcludeRef:                  excludeRefs,
			},
		}

		sleepInfos := []*kubegreenv1alpha1.SleepInfo{sleepInfo, wakePgHdfs, wakePgbouncer, wakeDeployments}
		for _, si := range sleepInfos {
			if err := s.createOrUpdateSleepInfo(ctx, si, userTimezone); err != nil {
				return err
			}
		}
	} else {
		// Different weekdays: create separate sleep/wake SleepInfos with staggered sleepAt
		// Sleep: suspend ALL resources with wdSleep
		sleepName := fmt.Sprintf("sleep-ds-deploys-%s", tenant)
		if scheduleName != "" {
			sleepName = fmt.Sprintf("sleep-%s", scheduleName)
		}

		sleepAnnotations := map[string]string{
			"kube-green.stratio.com/pair-id":   sharedID,
			"kube-green.stratio.com/pair-role": "sleep",
		}
		if scheduleName != "" {
			sleepAnnotations["kube-green.stratio.com/schedule-name"] = scheduleName
		}
		if description != "" {
			sleepAnnotations["kube-green.stratio.com/schedule-description"] = description
		}

		sleepInfo := &kubegreenv1alpha1.SleepInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:        sleepName,
				Namespace:   namespace,
				Annotations: sleepAnnotations,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays:                    wdSleep,
				SleepTime:                   offUTC,
				TimeZone:                    "UTC",
				SuspendDeployments:          &suspendDeployments,
				SuspendStatefulSets:         &suspendStatefulSets,
				SuspendCronjobs:             suspendCronJobs,
				SuspendDeploymentsPgbouncer: &suspendPgbouncer,
				SuspendStatefulSetsPostgres: &suspendPostgres,
				SuspendStatefulSetsHdfs:     &suspendHdfs,
				ExcludeRef:                  excludeRefs,
			},
		}

		// Wake staggered: create separate SleepInfos by type with wdWake
		// 1. Wake PgCluster + HDFSCluster first (onPgHDFS)
		wakePgHdfsName := fmt.Sprintf("wake-ds-deploys-%s-pg-hdfs", tenant)
		if scheduleName != "" {
			wakePgHdfsName = fmt.Sprintf("wake-%s-pg-hdfs", scheduleName)
		}

		wakePgHdfsAnnotations := map[string]string{
			"kube-green.stratio.com/pair-id":   sharedID,
			"kube-green.stratio.com/pair-role": "wake",
		}
		if scheduleName != "" {
			wakePgHdfsAnnotations["kube-green.stratio.com/schedule-name"] = scheduleName
		}
		if description != "" {
			wakePgHdfsAnnotations["kube-green.stratio.com/schedule-description"] = description
		}

		// Wake PgHDFS: debe tener suspendDeployments=False, suspendStatefulSets=False, suspendCronJobs=False, suspendDeploymentsPgbouncer=False
		// pero suspendStatefulSetsPostgres=True y suspendStatefulSetsHdfs=True (para restaurar solo Postgres y HDFS)
		suspendDeploymentsFalse := false
		suspendStatefulSetsFalse := false
		suspendCronJobsFalse := false
		suspendPgbouncerFalse := false

		wakePgHdfs := &kubegreenv1alpha1.SleepInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:        wakePgHdfsName,
				Namespace:   namespace,
				Annotations: wakePgHdfsAnnotations,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays:                    wdWake,
				SleepTime:                   onPgHDFS,
				TimeZone:                    "UTC",
				SuspendDeployments:          &suspendDeploymentsFalse,
				SuspendStatefulSets:         &suspendStatefulSetsFalse,
				SuspendCronjobs:             suspendCronJobsFalse,
				SuspendDeploymentsPgbouncer: &suspendPgbouncerFalse,
				SuspendStatefulSetsPostgres: &suspendPostgres,
				SuspendStatefulSetsHdfs:     &suspendHdfs,
				ExcludeRef:                  excludeRefs,
			},
		}

		// 2. Wake PgBouncer second (onPgBouncer)
		wakePgbouncerName := fmt.Sprintf("wake-ds-deploys-%s-pgbouncer", tenant)
		if scheduleName != "" {
			wakePgbouncerName = fmt.Sprintf("wake-%s-pgbouncer", scheduleName)
		}

		wakePgbouncerAnnotations := map[string]string{
			"kube-green.stratio.com/pair-id":   sharedID,
			"kube-green.stratio.com/pair-role": "wake",
		}
		if scheduleName != "" {
			wakePgbouncerAnnotations["kube-green.stratio.com/schedule-name"] = scheduleName
		}
		if description != "" {
			wakePgbouncerAnnotations["kube-green.stratio.com/schedule-description"] = description
		}

		// Wake PgBouncer: debe tener suspendDeployments=False, suspendStatefulSets=False, suspendCronJobs=False
		// pero suspendDeploymentsPgbouncer=True (para restaurar solo PgBouncer)
		wakePgbouncer := &kubegreenv1alpha1.SleepInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:        wakePgbouncerName,
				Namespace:   namespace,
				Annotations: wakePgbouncerAnnotations,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays:                    wdWake,
				SleepTime:                   onPgBouncer,
				TimeZone:                    "UTC",
				SuspendDeployments:          &suspendDeploymentsFalse,
				SuspendStatefulSets:         &suspendStatefulSetsFalse,
				SuspendCronjobs:             suspendCronJobsFalse,
				SuspendDeploymentsPgbouncer: &suspendPgbouncer,
				SuspendStatefulSetsPostgres: &suspendStatefulSetsFalse,
				SuspendStatefulSetsHdfs:     &suspendStatefulSetsFalse,
				ExcludeRef:                  excludeRefs,
			},
		}

		// 3. Wake native deployments last (onDeployments)
		wakeDeploymentsName := fmt.Sprintf("wake-ds-deploys-%s", tenant)
		if scheduleName != "" {
			wakeDeploymentsName = fmt.Sprintf("wake-%s", scheduleName)
		}

		wakeDeploymentsAnnotations := map[string]string{
			"kube-green.stratio.com/pair-id":   sharedID,
			"kube-green.stratio.com/pair-role": "wake",
		}
		if scheduleName != "" {
			wakeDeploymentsAnnotations["kube-green.stratio.com/schedule-name"] = scheduleName
		}
		if description != "" {
			wakeDeploymentsAnnotations["kube-green.stratio.com/schedule-description"] = description
		}

		wakeDeployments := &kubegreenv1alpha1.SleepInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:        wakeDeploymentsName,
				Namespace:   namespace,
				Annotations: wakeDeploymentsAnnotations,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays:                    wdWake,
				SleepTime:                   onDeployments,
				TimeZone:                    "UTC",
				SuspendDeployments:          &suspendDeployments,
				SuspendStatefulSets:         &suspendStatefulSets,
				SuspendCronjobs:             suspendCronJobs,
				SuspendDeploymentsPgbouncer: &suspendPgbouncer, // TRUE to restore PgBouncer during WAKE
				ExcludeRef:                  excludeRefs,
			},
		}

		sleepInfos := []*kubegreenv1alpha1.SleepInfo{sleepInfo, wakePgHdfs, wakePgbouncer, wakeDeployments}
		for _, si := range sleepInfos {
			if err := s.createOrUpdateSleepInfo(ctx, si, userTimezone); err != nil {
				return err
			}
		}
	}

	return nil
}

// createDatastoresSleepInfos creates the complex SleepInfos for datastores namespace (wrapper for backward compatibility)
// IMPORTANTE: Si los tiempos no tienen delays aplicados (onDeployments == onPgHDFS == onPgBouncer),
// aplicar delays por defecto (5m para PgBouncer, 7m para Deployments) como en tenant_power.py
func (s *ScheduleService) createDatastoresSleepInfos(ctx context.Context, tenant, namespace, offUTC, onDeployments, onPgHDFS, onPgBouncer, wdSleep, wdWake string, scheduleName, description, userTimezone string) error {
	excludeRefs := getExcludeRefsForOperators()

	// Si todos los tiempos son iguales, significa que no se aplicaron delays
	// Aplicar delays por defecto como en tenant_power.py
	if onDeployments == onPgHDFS && onPgHDFS == onPgBouncer {
		// Aplicar delays por defecto: PgHDFS a t0, PgBouncer a t0+5m, Deployments a t0+7m
		onPgBouncer, _ = AddMinutes(onPgHDFS, 5)
		onDeployments, _ = AddMinutes(onPgHDFS, 7)
		s.logger.Info("createDatastoresSleepInfos: applying default delays", "onPgHDFS", onPgHDFS, "onPgBouncer", onPgBouncer, "onDeployments", onDeployments)
	}

	return s.createDatastoresSleepInfosWithExclusions(ctx, tenant, namespace, offUTC, onDeployments, onPgHDFS, onPgBouncer, wdSleep, wdWake, excludeRefs, scheduleName, description, userTimezone)
}

// createNamespaceSleepInfo creates a simple SleepInfo for a namespace (wrapper for backward compatibility)
func (s *ScheduleService) createNamespaceSleepInfo(ctx context.Context, tenant, namespace, suffix, offUTC, onUTC, wdSleep, wdWake string, suspendStatefulSets bool, scheduleName, description, userTimezone string) error {
	excludeRefs := getExcludeRefsForOperators()
	return s.createNamespaceSleepInfoWithExclusions(ctx, tenant, namespace, suffix, offUTC, onUTC, wdSleep, wdWake, suspendStatefulSets, excludeRefs, scheduleName, description, userTimezone)
}

// getExcludeRefsForOperators returns exclude refs for operator-managed resources
func getExcludeRefsForOperators() []kubegreenv1alpha1.FilterRef {
	return []kubegreenv1alpha1.FilterRef{
		{MatchLabels: map[string]string{"app.kubernetes.io/managed-by": "postgres-operator"}},
		{MatchLabels: map[string]string{"postgres.stratio.com/cluster": "true"}},
		{MatchLabels: map[string]string{"app.kubernetes.io/part-of": "postgres"}},
		{MatchLabels: map[string]string{"app.kubernetes.io/managed-by": "hdfs-operator"}},
		{MatchLabels: map[string]string{"hdfs.stratio.com/cluster": "true"}},
		{MatchLabels: map[string]string{"app.kubernetes.io/part-of": "hdfs"}},
	}
}

// validateScheduleNameUniqueness checks if a schedule name is unique within a namespace
func (s *ScheduleService) validateScheduleNameUniqueness(ctx context.Context, namespace, scheduleName string) error {
	if scheduleName == "" {
		return nil
	}

	// List all SleepInfo objects in the namespace
	var sleepInfoList kubegreenv1alpha1.SleepInfoList
	if err := s.client.List(ctx, &sleepInfoList, client.InNamespace(namespace)); err != nil {
		// If namespace doesn't exist or error, skip validation (will fail later during creation)
		return nil
	}

	// Check if any SleepInfo has the same schedule name in annotations
	for _, si := range sleepInfoList.Items {
		if existingName, ok := si.Annotations["kube-green.stratio.com/schedule-name"]; ok && existingName == scheduleName {
			return fmt.Errorf("schedule name '%s' already exists in namespace '%s'", scheduleName, namespace)
		}
	}

	return nil
}

// createOrUpdateSleepInfo creates or updates a SleepInfo and its associated secret
func (s *ScheduleService) createOrUpdateSleepInfo(ctx context.Context, sleepInfo *kubegreenv1alpha1.SleepInfo, userTimezone string) error {
	var existing kubegreenv1alpha1.SleepInfo
	err := s.client.Get(ctx, client.ObjectKeyFromObject(sleepInfo), &existing)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Not found, create
			// IMPORTANTE: Asegurar que user-timezone esté en las anotaciones si se proporciona
			if userTimezone != "" {
				if sleepInfo.Annotations == nil {
					sleepInfo.Annotations = make(map[string]string)
				}
				// Solo agregar si no está presente (para no sobrescribir si ya está)
				if _, exists := sleepInfo.Annotations["kube-green.stratio.com/user-timezone"]; !exists {
					sleepInfo.Annotations["kube-green.stratio.com/user-timezone"] = userTimezone
					s.logger.Info("createOrUpdateSleepInfo: ADDED user-timezone to annotations before creating", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace, "userTimezone", userTimezone)
				}
			}
			userTZInAnnotations := ""
			if sleepInfo.Annotations != nil {
				userTZInAnnotations = sleepInfo.Annotations["kube-green.stratio.com/user-timezone"]
			}
			s.logger.Info("createOrUpdateSleepInfo: creating new SleepInfo", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace, "sleepTime", sleepInfo.Spec.SleepTime, "wakeTime", sleepInfo.Spec.WakeUpTime, "weekdays", sleepInfo.Spec.Weekdays, "userTimezoneParam", userTimezone, "userTimezoneInAnnotations", userTZInAnnotations, "annotationsCount", len(sleepInfo.Annotations))
			if err := s.client.Create(ctx, sleepInfo); err != nil {
				s.logger.Error(err, "failed to create SleepInfo", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
				return err
			}
			s.logger.Info("createOrUpdateSleepInfo: SleepInfo created successfully", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
			// After Create(), the sleepInfo object should have the UID populated by the Kubernetes API
			// However, if it's not available, try to get it with a retry
			if sleepInfo.UID == "" {
				// Retry getting the created SleepInfo to obtain its UID
				var created kubegreenv1alpha1.SleepInfo
				maxRetries := 3
				for i := 0; i < maxRetries; i++ {
					if err := s.client.Get(ctx, client.ObjectKeyFromObject(sleepInfo), &created); err == nil {
						sleepInfo.UID = created.UID
						break
					}
					if i < maxRetries-1 {
						// Wait a bit before retrying (exponential backoff: 10ms, 20ms, 40ms)
						time.Sleep(time.Duration(10*(1<<uint(i))) * time.Millisecond)
					}
				}
				if sleepInfo.UID == "" {
					s.logger.Error(fmt.Errorf("failed to get UID after %d retries", maxRetries), "failed to get created SleepInfo for UID", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
					// Continue without creating secret - controller will create it
					return nil
				}
			}
			// Create associated secret after SleepInfo is created (now we have the UID)
			// CRITICAL: Always create the secret, retry if needed
			secretCreated := false
			maxSecretRetries := 5
			for i := 0; i < maxSecretRetries; i++ {
				if err := s.createOrUpdateSecretForSleepInfo(ctx, sleepInfo, userTimezone); err != nil {
					s.logger.Error(err, "failed to create secret for SleepInfo (retry)", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace, "attempt", i+1, "maxRetries", maxSecretRetries)
					if i < maxSecretRetries-1 {
						// Wait before retrying (exponential backoff: 50ms, 100ms, 200ms, 400ms, 800ms)
						time.Sleep(time.Duration(50*(1<<uint(i))) * time.Millisecond)
						// Try to get the SleepInfo again to ensure we have the latest UID
						var updated kubegreenv1alpha1.SleepInfo
						if err := s.client.Get(ctx, client.ObjectKeyFromObject(sleepInfo), &updated); err == nil {
							sleepInfo.UID = updated.UID
						}
					}
				} else {
					secretCreated = true
					s.logger.Info("Secret created successfully for SleepInfo", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace, "attempt", i+1)
					break
				}
			}
			if !secretCreated {
				s.logger.Error(fmt.Errorf("failed to create secret after %d retries", maxSecretRetries), "CRITICAL: Secret not created for SleepInfo", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
				// Don't fail the entire operation - controller will create it, but log as error
			}
			return nil
		}
		return err
	}

	// Exists, update
	s.logger.Info("createOrUpdateSleepInfo: updating existing SleepInfo", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace, "sleepTime", sleepInfo.Spec.SleepTime, "wakeTime", sleepInfo.Spec.WakeUpTime, "weekdays", sleepInfo.Spec.Weekdays)

	// IMPORTANTE: Merge inteligente de anotaciones
	// 1. Inicializar annotations si es nil
	if sleepInfo.Annotations == nil {
		sleepInfo.Annotations = make(map[string]string)
	}

	// 2. Guardar las anotaciones nuevas que vienen en sleepInfo (schedule-name, schedule-description, etc.)
	newAnnotations := make(map[string]string)
	for k, v := range sleepInfo.Annotations {
		newAnnotations[k] = v
	}

	// 3. Copiar TODAS las anotaciones existentes primero para preservarlas
	for k, v := range existing.Annotations {
		sleepInfo.Annotations[k] = v
	}

	// 4. SOBRESCRIBIR con las anotaciones nuevas (schedule-name, schedule-description, pair-id, pair-role, etc.)
	// Esto asegura que las anotaciones del request tengan prioridad
	for k, v := range newAnnotations {
		sleepInfo.Annotations[k] = v
	}

	// 5. Actualizar userTimezone en las anotaciones si se proporciona
	// Si no se proporciona, preservar el existente de las anotaciones
	timezoneToUse := userTimezone
	if timezoneToUse == "" {
		// Leer userTimezone de las anotaciones existentes
		if existingTZ, ok := sleepInfo.Annotations["kube-green.stratio.com/user-timezone"]; ok {
			timezoneToUse = existingTZ
			s.logger.Info("createOrUpdateSleepInfo: using timezone from existing annotations", "timezone", timezoneToUse, "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
		}
	} else {
		// Agregar/actualizar user-timezone
		sleepInfo.Annotations["kube-green.stratio.com/user-timezone"] = timezoneToUse
		s.logger.Info("createOrUpdateSleepInfo: updating timezone in annotations", "timezone", timezoneToUse, "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
	}

	// Log para debug
	s.logger.Info("createOrUpdateSleepInfo: merged annotations", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace,
		"scheduleName", sleepInfo.Annotations["kube-green.stratio.com/schedule-name"],
		"description", sleepInfo.Annotations["kube-green.stratio.com/schedule-description"],
		"userTimezone", sleepInfo.Annotations["kube-green.stratio.com/user-timezone"],
		"totalAnnotations", len(sleepInfo.Annotations))

	sleepInfo.ResourceVersion = existing.ResourceVersion
	if err := s.client.Update(ctx, sleepInfo); err != nil {
		s.logger.Error(err, "failed to update SleepInfo", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
		return err
	}
	s.logger.Info("createOrUpdateSleepInfo: SleepInfo updated successfully", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)

	// Update associated secret - CRITICAL: Always update/create the secret
	// Use the timezone from request if available, otherwise use the one from existing annotations
	if err := s.createOrUpdateSecretForSleepInfo(ctx, sleepInfo, timezoneToUse); err != nil {
		s.logger.Error(err, "CRITICAL: failed to update secret for SleepInfo", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
		// Retry once
		time.Sleep(100 * time.Millisecond)
		if err := s.createOrUpdateSecretForSleepInfo(ctx, sleepInfo, timezoneToUse); err != nil {
			s.logger.Error(err, "CRITICAL: failed to update secret after retry", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
			// Don't fail the entire operation - controller will update it, but log as error
		} else {
			s.logger.Info("Secret updated successfully after retry", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
		}
	} else {
		s.logger.Info("Secret updated successfully for SleepInfo", "name", sleepInfo.Name, "namespace", sleepInfo.Namespace)
	}
	return nil
}

// createOrUpdateSecretForSleepInfo creates or updates the secret associated with a SleepInfo
func (s *ScheduleService) createOrUpdateSecretForSleepInfo(ctx context.Context, sleepInfo *kubegreenv1alpha1.SleepInfo, userTimezone string) error {
	s.logger.Info("createOrUpdateSecretForSleepInfo CALLED", "sleepInfo", sleepInfo.Name, "namespace", sleepInfo.Namespace, "userTimezone", userTimezone, "userTimezoneEmpty", userTimezone == "")
	secretName := fmt.Sprintf("sleepinfo-%s", sleepInfo.Name)

	// Determine operation type based on SleepInfo annotations or spec
	operationType := "sleep"
	if role, ok := sleepInfo.Annotations["kube-green.stratio.com/pair-role"]; ok && role == "wake" {
		operationType = "wake"
	} else if sleepInfo.Spec.WakeUpTime != "" && sleepInfo.Spec.SleepTime == "" {
		// If only WakeUpTime is set, it's a wake operation
		operationType = "wake"
	}

	// Get current time for scheduled-at (use time.Now() for RFC3339 format)
	now := time.Now()

	// Check if secret already exists
	var existingSecret v1.Secret
	secretKey := client.ObjectKey{
		Name:      secretName,
		Namespace: sleepInfo.Namespace,
	}
	err := s.client.Get(ctx, secretKey, &existingSecret)
	secretExists := err == nil

	// Build secret
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: sleepInfo.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kube-green",
			},
		},
		StringData: map[string]string{
			"scheduled-at":   now.Format(time.RFC3339), // RFC3339 format
			"operation-type": operationType,
		},
		Data: make(map[string][]byte),
	}

	// Add userTimezone to secret if provided
	if userTimezone != "" {
		secret.StringData["user-timezone"] = userTimezone
		s.logger.Info("createOrUpdateSecretForSleepInfo: adding user-timezone to secret", "userTimezone", userTimezone, "secret", secretName, "namespace", sleepInfo.Namespace)
	} else {
		s.logger.Error(nil, "createOrUpdateSecretForSleepInfo: userTimezone is empty, not adding to secret", "secret", secretName, "namespace", sleepInfo.Namespace)
	}

	// Add OwnerReference only if UID is available
	if sleepInfo.UID != "" {
		secret.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: kubegreenv1alpha1.GroupVersion.String(),
				Kind:       "SleepInfo",
				Name:       sleepInfo.Name,
				UID:        sleepInfo.UID,
			},
		}
	}

	// If secret exists, preserve existing original-resource-info if present
	if secretExists {
		secret.ResourceVersion = existingSecret.ResourceVersion
		// CRITICAL: Preserve existing original-resource-info - this contains deployment/statefulset state
		// that the controller saved during sleep operation. We should NEVER overwrite this.
		if originalData, ok := existingSecret.Data["original-resource-info"]; ok {
			secret.Data["original-resource-info"] = originalData
		}
		// Also preserve any other existing data fields (except user-timezone which we update)
		for key, value := range existingSecret.Data {
			if key != "original-resource-info" && key != "scheduled-at" && key != "operation-type" && key != "user-timezone" {
				secret.Data[key] = value
			}
		}
		// Update user-timezone if provided
		if userTimezone != "" {
			secret.StringData["user-timezone"] = userTimezone
		}
		// Update the secret
		if err := s.client.Update(ctx, secret); err != nil {
			s.logger.Error(err, "CRITICAL: failed to update secret", "secret", secretName, "namespace", sleepInfo.Namespace, "operationType", operationType)
			return fmt.Errorf("failed to update secret %s in namespace %s: %w", secretName, sleepInfo.Namespace, err)
		}
		s.logger.Info("Secret updated successfully", "secret", secretName, "namespace", sleepInfo.Namespace, "operationType", operationType, "scheduledAt", now.Format(time.RFC3339))
		return nil
	}

	// Create new secret
	// NOTE: original-resource-info will be added by the controller when it executes sleep operation
	stringDataKeys := make([]string, 0, len(secret.StringData))
	for k := range secret.StringData {
		stringDataKeys = append(stringDataKeys, k)
	}
	s.logger.Info("createOrUpdateSecretForSleepInfo: creating new secret", "secret", secretName, "namespace", sleepInfo.Namespace, "userTimezone", userTimezone, "stringDataKeys", strings.Join(stringDataKeys, ","))
	if err := s.client.Create(ctx, secret); err != nil {
		s.logger.Error(err, "CRITICAL: failed to create secret", "secret", secretName, "namespace", sleepInfo.Namespace, "operationType", operationType)
		return fmt.Errorf("failed to create secret %s in namespace %s: %w", secretName, sleepInfo.Namespace, err)
	}
	s.logger.Info("Secret created successfully", "secret", secretName, "namespace", sleepInfo.Namespace, "operationType", operationType, "scheduledAt", now.Format(time.RFC3339), "userTimezone", userTimezone)
	return nil
}

// ScheduleResponse represents a schedule for a tenant
type ScheduleResponse struct {
	Tenant     string                   `json:"tenant"`
	Namespaces map[string]NamespaceInfo `json:"namespaces"`
}

// NamespaceInfo represents schedule information for a namespace
type NamespaceInfo struct {
	Namespace string             `json:"namespace"`
	Weekdays  string             `json:"weekdays"`
	Timezone  string             `json:"timezone"`
	Schedule  []SleepInfoSummary `json:"schedule"` // Chronologically ordered schedule
	Summary   ScheduleSummary    `json:"summary"`  // Human-readable summary
}

// ScheduleSummary provides a human-readable summary of the schedule
type ScheduleSummary struct {
	SleepTime   string   `json:"sleepTime,omitempty"` // When resources go to sleep
	WakeTime    string   `json:"wakeTime,omitempty"`  // When resources wake up
	Operations  []string `json:"operations"`          // List of operations in order
	Description string   `json:"description"`         // Human-readable description
}

// FilterRef represents a filter for excluding resources
type FilterRef struct {
	MatchLabels map[string]string `json:"matchLabels"`
}

// SleepInfoSummary represents a summary of a SleepInfo
type SleepInfoSummary struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Role        string            `json:"role"`      // "sleep" or "wake"
	Operation   string            `json:"operation"` // Human-readable description
	Time        string            `json:"time"`      // Sleep or wake time
	Weekdays    string            `json:"weekdays"`
	TimeZone    string            `json:"timeZone"`
	Resources   []string          `json:"resources"` // List of resources managed (Postgres, HDFS, PgBouncer, Deployments, etc.)
	WakeTime    string            `json:"wakeTime,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	ExcludeRef  []FilterRef       `json:"excludeRef,omitempty"` // Exclusion filters
}

// ListSchedules lists all schedules grouped by tenant
func (s *ScheduleService) ListSchedules(ctx context.Context) ([]ScheduleResponse, error) {
	// List all SleepInfos across all namespaces
	sleepInfoList := &kubegreenv1alpha1.SleepInfoList{}
	if err := s.client.List(ctx, sleepInfoList); err != nil {
		return nil, fmt.Errorf("failed to list SleepInfos: %w", err)
	}

	// Group by tenant (extract from namespace: tenant-suffix)
	tenantMap := make(map[string]map[string][]kubegreenv1alpha1.SleepInfo)

	for _, si := range sleepInfoList.Items {
		// Extract tenant from namespace (e.g., "bdadevdat-datastores" -> "bdadevdat")
		nsParts := strings.Split(si.Namespace, "-")
		if len(nsParts) < 2 {
			continue // Skip namespaces that don't match tenant-suffix pattern
		}

		// Reconstruct tenant (handle cases like "bdadevdat-datastores")
		// Take all parts except the last one as tenant
		tenant := strings.Join(nsParts[:len(nsParts)-1], "-")
		suffix := nsParts[len(nsParts)-1]

		if tenantMap[tenant] == nil {
			tenantMap[tenant] = make(map[string][]kubegreenv1alpha1.SleepInfo)
		}

		tenantMap[tenant][suffix] = append(tenantMap[tenant][suffix], si)
	}

	// Convert to response format
	result := make([]ScheduleResponse, 0, len(tenantMap))
	for tenant, namespaceGroups := range tenantMap {
		namespaces := make(map[string]NamespaceInfo)
		for suffix, sleepInfos := range namespaceGroups {
			namespaces[suffix] = s.buildNamespaceInfo(ctx, sleepInfos)
		}
		result = append(result, ScheduleResponse{
			Tenant:     tenant,
			Namespaces: namespaces,
		})
	}

	return result, nil
}

// GetSchedule gets all SleepInfos for a specific tenant
func (s *ScheduleService) GetSchedule(ctx context.Context, tenant string, namespaceSuffix ...string) (*ScheduleResponse, error) {
	// List all SleepInfos
	sleepInfoList := &kubegreenv1alpha1.SleepInfoList{}
	if err := s.client.List(ctx, sleepInfoList); err != nil {
		return nil, fmt.Errorf("failed to list SleepInfos: %w", err)
	}

	namespaces := make(map[string]NamespaceInfo)
	var filterNamespace string
	if len(namespaceSuffix) > 0 && namespaceSuffix[0] != "" {
		filterNamespace = namespaceSuffix[0]
	}

	// Filter by tenant and group by namespace suffix
	namespaceGroups := make(map[string][]kubegreenv1alpha1.SleepInfo)
	for _, si := range sleepInfoList.Items {
		// Extract tenant from namespace
		nsParts := strings.Split(si.Namespace, "-")
		if len(nsParts) < 2 {
			continue
		}

		tenantFromNS := strings.Join(nsParts[:len(nsParts)-1], "-")
		if tenantFromNS != tenant {
			continue
		}

		suffix := nsParts[len(nsParts)-1]

		// Filter by namespace suffix if provided
		if filterNamespace != "" && suffix != filterNamespace {
			continue
		}

		namespaceGroups[suffix] = append(namespaceGroups[suffix], si)
	}

	if len(namespaceGroups) == 0 {
		if filterNamespace != "" {
			return nil, fmt.Errorf("no schedules found for tenant: %s in namespace: %s", tenant, filterNamespace)
		}
		return nil, fmt.Errorf("no schedules found for tenant: %s", tenant)
	}

	// Process each namespace group
	for suffix, sleepInfos := range namespaceGroups {
		namespaceInfo := s.buildNamespaceInfo(ctx, sleepInfos)
		namespaces[suffix] = namespaceInfo
	}

	return &ScheduleResponse{
		Tenant:     tenant,
		Namespaces: namespaces,
	}, nil
}

// buildNamespaceInfo creates a NamespaceInfo from a list of SleepInfos
func (s *ScheduleService) buildNamespaceInfo(ctx context.Context, sleepInfos []kubegreenv1alpha1.SleepInfo) NamespaceInfo {
	if len(sleepInfos) == 0 {
		return NamespaceInfo{}
	}

	// Get common info from first SleepInfo
	first := sleepInfos[0]
	nsInfo := NamespaceInfo{
		Namespace: first.Namespace,
		Weekdays:  first.Spec.Weekdays,
		Timezone:  first.Spec.TimeZone,
	}

	// Convert weekdays to human-readable (keep numeric for now, can enhance later)
	// For now, just use the numeric format

	// Build summaries for each SleepInfo
	summaries := make([]SleepInfoSummary, 0, len(sleepInfos))
	var sleepTime, wakeTime string
	var operations []string

	for _, si := range sleepInfos {
		summary := s.buildSleepInfoSummary(ctx, si)
		summaries = append(summaries, summary)

		// Track times for summary
		if summary.Role == "sleep" && sleepTime == "" {
			sleepTime = summary.Time
		}
		if summary.Role == "wake" {
			if wakeTime == "" || summary.Time > wakeTime {
				wakeTime = summary.Time
			}
			operations = append(operations, fmt.Sprintf("%s a las %s (%s)", summary.Operation, summary.Time, strings.Join(summary.Resources, ", ")))
		} else if summary.Role == "sleep" {
			operations = append(operations, fmt.Sprintf("%s a las %s", summary.Operation, summary.Time))
		}
	}

	// Sort summaries by time
	sortSummariesByTime(summaries)
	nsInfo.Schedule = summaries

	// Build human-readable summary
	nsInfo.Summary = ScheduleSummary{
		SleepTime:   sleepTime,
		WakeTime:    wakeTime,
		Operations:  operations,
		Description: buildScheduleDescription(summaries),
	}

	return nsInfo
}

// buildSleepInfoSummary creates a SleepInfoSummary from a SleepInfo
// It reads the associated Secret to extract userTimezone and adds it to annotations in the response
func (s *ScheduleService) buildSleepInfoSummary(ctx context.Context, si kubegreenv1alpha1.SleepInfo) SleepInfoSummary {
	// Determine role from annotations or name
	role := "wake"
	operation := "Encender servicios"
	if pairRole, ok := si.Annotations["kube-green.stratio.com/pair-role"]; ok {
		role = pairRole
	} else if strings.HasPrefix(si.Name, "sleep-") {
		role = "sleep"
		operation = "Apagar servicios"
	}

	// Determine time
	// IMPORTANTE: El CRD usa sleepAt y wakeUpAt en JSON, pero el struct Go usa SleepTime y WakeUpTime
	// Para sleep: usar SleepTime (que es sleepAt en JSON)
	// Para wake: si es un SleepInfo separado, SleepTime contiene la hora de wake (sleepAt en JSON)
	//            si es un SleepInfo único, WakeUpTime contiene la hora de wake (wakeUpAt en JSON)
	time := si.Spec.SleepTime
	if time == "" {
		time = si.Spec.WakeUpTime
	}

	// Si el role es "wake" y no hay WakeUpTime, entonces SleepTime es la hora de wake
	// (esto pasa cuando weekdays son diferentes y se crean SleepInfos separados)
	if role == "wake" && time == "" && si.Spec.SleepTime != "" {
		time = si.Spec.SleepTime
	}

	// Determine resources managed
	resources := determineManagedResources(si, role)

	// Build operation description
	operation = buildOperationDescription(role, resources)

	// Convert ExcludeRef to FilterRef format for API response
	excludeRefs := make([]FilterRef, 0)
	// IMPORTANTE: Verificar si ExcludeRef está presente y copiar correctamente
	if si.Spec.ExcludeRef != nil && len(si.Spec.ExcludeRef) > 0 {
		for _, excl := range si.Spec.ExcludeRef {
			// Asegurarse de que MatchLabels no es nil
			if excl.MatchLabels != nil && len(excl.MatchLabels) > 0 {
				excludeRefs = append(excludeRefs, FilterRef{
					MatchLabels: excl.MatchLabels,
				})
			}
		}
	}

	// Copy annotations (userTimezone is already stored in annotations)
	annotations := make(map[string]string)
	for k, v := range si.Annotations {
		annotations[k] = v
	}
	// userTimezone is already in annotations if it was set during creation

	summary := SleepInfoSummary{
		Name:        si.Name,
		Namespace:   si.Namespace,
		Role:        role,
		Operation:   operation,
		Time:        time,
		Weekdays:    si.Spec.Weekdays,
		TimeZone:    si.Spec.TimeZone,
		WakeTime:    si.Spec.WakeUpTime,
		Resources:   resources,
		Annotations: annotations,
		ExcludeRef:  excludeRefs,
	}

	return summary
}

// determineManagedResources determines which resources are managed by a SleepInfo
func determineManagedResources(si kubegreenv1alpha1.SleepInfo, role string) []string {
	var resources []string

	// Check CRD-specific flags
	if si.Spec.SuspendStatefulSetsPostgres != nil && *si.Spec.SuspendStatefulSetsPostgres {
		resources = append(resources, "Postgres")
	}
	if si.Spec.SuspendStatefulSetsHdfs != nil && *si.Spec.SuspendStatefulSetsHdfs {
		resources = append(resources, "HDFS")
	}
	if si.Spec.SuspendDeploymentsPgbouncer != nil && *si.Spec.SuspendDeploymentsPgbouncer {
		resources = append(resources, "PgBouncer")
	}

	// Check native resource flags
	if si.Spec.SuspendDeployments != nil && *si.Spec.SuspendDeployments {
		resources = append(resources, "Deployments")
	}
	if si.Spec.SuspendStatefulSets != nil && *si.Spec.SuspendStatefulSets {
		resources = append(resources, "StatefulSets")
	}
	if si.Spec.SuspendCronjobs {
		resources = append(resources, "CronJobs")
	}

	// If no specific resources, check role
	if len(resources) == 0 {
		if role == "sleep" {
			resources = []string{"Todos los servicios"}
		} else {
			resources = []string{"Todos los servicios"}
		}
	}

	return resources
}

// buildOperationDescription creates a human-readable operation description
func buildOperationDescription(role string, resources []string) string {
	action := "Encender"
	if role == "sleep" {
		action = "Apagar"
	}

	if len(resources) == 0 {
		return fmt.Sprintf("%s servicios", action)
	}

	if len(resources) == 1 {
		return fmt.Sprintf("%s %s", action, resources[0])
	}

	// Join all except last with comma, last with "y"
	if len(resources) == 2 {
		return fmt.Sprintf("%s %s y %s", action, resources[0], resources[1])
	}

	allButLast := strings.Join(resources[:len(resources)-1], ", ")
	return fmt.Sprintf("%s %s y %s", action, allButLast, resources[len(resources)-1])
}

// buildScheduleDescription creates a human-readable description of the schedule
func buildScheduleDescription(summaries []SleepInfoSummary) string {
	if len(summaries) == 0 {
		return "Sin programación configurada"
	}

	var parts []string
	for _, s := range summaries {
		parts = append(parts, fmt.Sprintf("%s a las %s", s.Operation, s.Time))
	}
	return strings.Join(parts, " → ")
}

// sortSummariesByTime sorts summaries chronologically (sleep first, then wake by time)
func sortSummariesByTime(summaries []SleepInfoSummary) {
	// Simple bubble sort for small lists
	for i := 0; i < len(summaries); i++ {
		for j := i + 1; j < len(summaries); j++ {
			// Sleep always comes before wake at same time
			if summaries[i].Role == "wake" && summaries[j].Role == "sleep" {
				if summaries[i].Time == summaries[j].Time {
					summaries[i], summaries[j] = summaries[j], summaries[i]
					continue
				}
			}
			// Sort by time
			if summaries[i].Time > summaries[j].Time {
				summaries[i], summaries[j] = summaries[j], summaries[i]
			}
		}
	}
}

// UpdateSchedule updates schedules for a tenant
// If fields are empty, they will be extracted from existing schedule
func (s *ScheduleService) UpdateSchedule(ctx context.Context, tenant string, req CreateScheduleRequest, namespaceSuffix ...string) error {
	// LOG CRÍTICO: Confirmar que la función se está ejecutando
	s.logger.Info("UpdateSchedule CALLED", "tenant", tenant, "req.Off", req.Off, "req.On", req.On, "req.Namespaces", fmt.Sprintf("%v", req.Namespaces))

	var filterNamespace string
	if len(namespaceSuffix) > 0 && namespaceSuffix[0] != "" {
		filterNamespace = namespaceSuffix[0]
	}

	// IMPORTANTE: El frontend SIEMPRE debe enviar los tiempos cuando se actualiza
	// Solo extraer valores del schedule existente si realmente están vacíos (no sobrescribir valores del frontend)
	// Los tiempos del schedule existente están en UTC, necesitamos convertirlos a la timezone del usuario
	// IMPORTANTE: NO extraer si el frontend ya envió los valores
	s.logger.Info("UpdateSchedule", "tenant", tenant, "namespace", filterNamespace, "req.Off", req.Off, "req.On", req.On, "req.Weekdays", req.Weekdays, "req.SleepDays", req.SleepDays, "req.WakeDays", req.WakeDays)

	if req.Off == "" || req.On == "" {
		s.logger.Info("UpdateSchedule: extracting times from existing schedule", "off_empty", req.Off == "", "on_empty", req.On == "")
		existing, err := s.GetSchedule(ctx, tenant, filterNamespace)
		if err == nil && existing != nil {
			// Get timezones for conversion (default to America/Bogota -> UTC)
			userTZ := TZLocal
			clusterTZ := TZUTC

			// Extract values from existing schedule
			// IMPORTANTE: Buscar específicamente schedules sleep y wake, no usar el primero
			for _, nsInfo := range existing.Namespaces {
				if len(nsInfo.Schedule) > 0 {
					first := nsInfo.Schedule[0]
					// Get cluster timezone from existing schedule
					if first.TimeZone != "" {
						clusterTZ = first.TimeZone
					}

					// Buscar schedule sleep específicamente para obtener el tiempo de sleep
					if req.Off == "" {
						var sleepSchedule *SleepInfoSummary
						for i := range nsInfo.Schedule {
							if nsInfo.Schedule[i].Role == "sleep" {
								sleepSchedule = &nsInfo.Schedule[i]
								break
							}
						}

						// Si no se encuentra schedule sleep separado, buscar en el schedule único
						if sleepSchedule == nil {
							// Buscar schedule con sleepTime o que tenga ambos sleepTime y wakeUpAt
							for i := range nsInfo.Schedule {
								sched := &nsInfo.Schedule[i]
								if sched.Time != "" && (sched.Role == "sleep" || sched.Role == "") {
									// Si tiene WakeTime, es un schedule único con ambos tiempos
									if sched.WakeTime != "" {
										// Es un schedule único, usar el Time como sleepTime
										sleepSchedule = sched
										break
									} else if sched.Role == "sleep" {
										// Es un schedule sleep separado
										sleepSchedule = sched
										break
									}
								}
							}
						}

						// Si aún no se encuentra, usar el primero como fallback
						if sleepSchedule == nil {
							sleepSchedule = &nsInfo.Schedule[0]
						}

						// Extraer tiempo de sleep
						sleepTimeUTC := ""
						if sleepSchedule.Role == "sleep" {
							// Schedule sleep separado: usar Time
							sleepTimeUTC = sleepSchedule.Time
						} else if sleepSchedule.WakeTime != "" {
							// Schedule único con ambos tiempos: Time es el sleepTime
							sleepTimeUTC = sleepSchedule.Time
						} else {
							// Fallback: usar Time
							sleepTimeUTC = sleepSchedule.Time
						}

						if sleepTimeUTC != "" {
							offConv, err := FromClusterToUserTimezone(sleepTimeUTC, clusterTZ, userTZ)
							if err == nil {
								req.Off = offConv.TimeUTC
								s.logger.Info("UpdateSchedule: converted Off time from sleep schedule", "from_utc", sleepTimeUTC, "to_user", req.Off, "clusterTZ", clusterTZ, "userTZ", userTZ, "role", sleepSchedule.Role)
							} else {
								// Fallback: usar el tiempo directamente si la conversión falla
								req.Off = sleepTimeUTC
								s.logger.Info("UpdateSchedule: using Off time directly (conversion failed)", "time", req.Off, "error", err)
							}
						}
					}

					// Buscar schedule wake específicamente para obtener el tiempo de wake
					if req.On == "" {
						var wakeSchedule *SleepInfoSummary
						for i := range nsInfo.Schedule {
							if nsInfo.Schedule[i].Role == "wake" {
								wakeSchedule = &nsInfo.Schedule[i]
								break
							}
						}

						// Si no se encuentra schedule wake separado, buscar en el schedule único
						if wakeSchedule == nil {
							// Buscar schedule con wakeTime
							for i := range nsInfo.Schedule {
								sched := &nsInfo.Schedule[i]
								if sched.WakeTime != "" {
									// Es un schedule único con ambos tiempos
									wakeSchedule = sched
									break
								}
							}
						}

						// Extraer tiempo de wake
						wakeTimeUTC := ""
						if wakeSchedule != nil {
							if wakeSchedule.Role == "wake" {
								// Schedule wake separado: usar Time
								wakeTimeUTC = wakeSchedule.Time
							} else if wakeSchedule.WakeTime != "" {
								// Schedule único con ambos tiempos: WakeTime es el wakeTime
								wakeTimeUTC = wakeSchedule.WakeTime
							}
						}

						if wakeTimeUTC != "" {
							onConv, err := FromClusterToUserTimezone(wakeTimeUTC, clusterTZ, userTZ)
							if err == nil {
								req.On = onConv.TimeUTC
								s.logger.Info("UpdateSchedule: converted On time from wake schedule", "from_utc", wakeTimeUTC, "to_user", req.On, "clusterTZ", clusterTZ, "userTZ", userTZ, "role", wakeSchedule.Role)
							} else {
								// Fallback: usar el tiempo directamente si la conversión falla
								req.On = wakeTimeUTC
								s.logger.Info("UpdateSchedule: using On time directly (conversion failed)", "time", req.On, "error", err)
							}
						}
					}
					break
				}
			}
		}
	} else {
		s.logger.Info("UpdateSchedule: using times from request", "off", req.Off, "on", req.On)
	}

	// IMPORTANTE: Si req.Namespaces está vacío, obtener todos los namespaces del schedule existente
	// Esto asegura que se actualicen todos los namespaces que tienen schedules, no solo los que el frontend envía
	var existingSchedule *ScheduleResponse
	if len(req.Namespaces) == 0 {
		s.logger.Info("UpdateSchedule: req.Namespaces is empty, extracting from existing schedule", "tenant", tenant, "namespace", filterNamespace)
		existing, err := s.GetSchedule(ctx, tenant, filterNamespace)
		if err == nil && existing != nil {
			existingSchedule = existing
			// Extraer todos los namespaces que tienen schedules
			namespacesList := make([]string, 0, len(existing.Namespaces))
			for nsSuffix := range existing.Namespaces {
				namespacesList = append(namespacesList, nsSuffix)
			}
			req.Namespaces = namespacesList
			s.logger.Info("UpdateSchedule: extracted namespaces from existing schedule", "namespaces", strings.Join(namespacesList, ","), "count", len(namespacesList))
		} else {
			s.logger.Info("UpdateSchedule: could not extract namespaces from existing schedule, will use empty list", "error", err)
		}
	} else {
		s.logger.Info("UpdateSchedule: using namespaces from request", "namespaces", strings.Join(req.Namespaces, ","), "count", len(req.Namespaces))
		// Obtener schedule existente (no hay Delays en CreateScheduleRequest)
		existing, err := s.GetSchedule(ctx, tenant, filterNamespace)
		if err == nil && existing != nil {
			existingSchedule = existing
		}
	}

	// IMPORTANTE: Extraer weekdays del schedule existente si no se proporcionan en el request
	// Los weekdays del schedule existente están en UTC (ya shiftados), necesitamos convertirlos de vuelta a la timezone del usuario
	if (req.SleepDays == "" || req.WakeDays == "" || req.Weekdays == "") && existingSchedule != nil {
		// Get timezones for conversion (default to America/Bogota -> UTC)
		userTZ := TZLocal
		clusterTZ := TZUTC

		// Buscar schedules sleep y wake en el schedule existente
		for _, nsInfo := range existingSchedule.Namespaces {
			if len(nsInfo.Schedule) > 0 {
				// Get cluster timezone from existing schedule
				first := nsInfo.Schedule[0]
				if first.TimeZone != "" {
					clusterTZ = first.TimeZone
				}

				// Buscar schedule sleep para extraer weekdays
				if req.SleepDays == "" && req.Weekdays == "" {
					var sleepSchedule *SleepInfoSummary
					for i := range nsInfo.Schedule {
						if nsInfo.Schedule[i].Role == "sleep" {
							sleepSchedule = &nsInfo.Schedule[i]
							break
						}
					}

					// Si no se encuentra schedule sleep separado, buscar en el schedule único
					if sleepSchedule == nil {
						for i := range nsInfo.Schedule {
							sched := &nsInfo.Schedule[i]
							if sched.Role == "sleep" || (sched.Role == "" && sched.Time != "" && sched.WakeTime == "") {
								sleepSchedule = sched
								break
							}
						}
					}

					if sleepSchedule != nil && sleepSchedule.Weekdays != "" {
						// Los weekdays están en UTC, necesitamos convertirlos de vuelta a la timezone del usuario
						// Para esto, necesitamos el tiempo de sleep para calcular el day shift
						sleepTimeUTC := sleepSchedule.Time
						if sleepTimeUTC != "" {
							// Calcular day shift usando FromClusterToUserTimezone
							offConv, err := FromClusterToUserTimezone(sleepTimeUTC, clusterTZ, userTZ)
							if err == nil {
								// Aplicar shift inverso (negativo) para convertir de UTC a user timezone
								wdSleepUser, err := ShiftWeekdaysStr(sleepSchedule.Weekdays, -offConv.DayShift)
								if err == nil {
									// Convertir de formato Kube (0-6) a formato Human (0-6 pero puede ser range o comma-separated)
									// El formato ya debería estar en formato correcto, solo necesitamos asegurarnos
									req.SleepDays = wdSleepUser
									s.logger.Info("UpdateSchedule: converted SleepDays from UTC to user timezone", "from_utc", sleepSchedule.Weekdays, "to_user", req.SleepDays, "dayShift", -offConv.DayShift, "sleepTimeUTC", sleepTimeUTC)
								} else {
									s.logger.Error(err, "failed to shift sleep weekdays", "weekdays", sleepSchedule.Weekdays, "dayShift", -offConv.DayShift)
								}
							} else {
								s.logger.Error(err, "failed to convert sleep time for weekday conversion", "time", sleepTimeUTC)
							}
						}
					}
				}

				// Buscar schedule wake para extraer weekdays
				if req.WakeDays == "" && req.Weekdays == "" {
					var wakeSchedule *SleepInfoSummary
					for i := range nsInfo.Schedule {
						if nsInfo.Schedule[i].Role == "wake" {
							wakeSchedule = &nsInfo.Schedule[i]
							break
						}
					}

					// Si no se encuentra schedule wake separado, buscar en el schedule único
					if wakeSchedule == nil {
						for i := range nsInfo.Schedule {
							sched := &nsInfo.Schedule[i]
							if sched.Role == "wake" || (sched.WakeTime != "" && sched.Role == "") {
								wakeSchedule = sched
								break
							}
						}
					}

					if wakeSchedule != nil && wakeSchedule.Weekdays != "" {
						// Los weekdays están en UTC, necesitamos convertirlos de vuelta a la timezone del usuario
						// Para esto, necesitamos el tiempo de wake para calcular el day shift
						wakeTimeUTC := wakeSchedule.Time
						if wakeTimeUTC == "" && wakeSchedule.WakeTime != "" {
							wakeTimeUTC = wakeSchedule.WakeTime
						}
						if wakeTimeUTC != "" {
							// Calcular day shift usando FromClusterToUserTimezone
							onConv, err := FromClusterToUserTimezone(wakeTimeUTC, clusterTZ, userTZ)
							if err == nil {
								// Aplicar shift inverso (negativo) para convertir de UTC a user timezone
								wdWakeUser, err := ShiftWeekdaysStr(wakeSchedule.Weekdays, -onConv.DayShift)
								if err == nil {
									req.WakeDays = wdWakeUser
									s.logger.Info("UpdateSchedule: converted WakeDays from UTC to user timezone", "from_utc", wakeSchedule.Weekdays, "to_user", req.WakeDays, "dayShift", -onConv.DayShift, "wakeTimeUTC", wakeTimeUTC)
								} else {
									s.logger.Error(err, "failed to shift wake weekdays", "weekdays", wakeSchedule.Weekdays, "dayShift", -onConv.DayShift)
								}
							} else {
								s.logger.Error(err, "failed to convert wake time for weekday conversion", "time", wakeTimeUTC)
							}
						}
					}
				}
				break
			}
		}
	}

	// Note: Delays are not in CreateScheduleRequest, will use defaults

	// IMPORTANTE: Extraer scheduleName y description del schedule existente si no se proporcionan en el request
	// Esto preserva las anotaciones cuando se actualiza solo la hora o el día
	if (req.ScheduleName == "" || req.Description == "") && existingSchedule != nil {
		s.logger.Info("UpdateSchedule: attempting to extract scheduleName and description from existing schedule")
		for _, nsInfo := range existingSchedule.Namespaces {
			for _, sched := range nsInfo.Schedule {
				// Extraer scheduleName de las anotaciones si no se proporciona
				if req.ScheduleName == "" && sched.Annotations != nil {
					if scheduleName, ok := sched.Annotations["kube-green.stratio.com/schedule-name"]; ok && scheduleName != "" {
						req.ScheduleName = scheduleName
						s.logger.Info("UpdateSchedule: extracted scheduleName from existing schedule", "scheduleName", scheduleName)
					}
				}
				// Extraer description de las anotaciones si no se proporciona
				if req.Description == "" && sched.Annotations != nil {
					if description, ok := sched.Annotations["kube-green.stratio.com/schedule-description"]; ok && description != "" {
						req.Description = description
						s.logger.Info("UpdateSchedule: extracted description from existing schedule", "description", description)
					}
				}
				// Si ya encontramos ambos, salir del loop
				if req.ScheduleName != "" && req.Description != "" {
					break
				}
			}
			// Si ya encontramos ambos, salir del loop de namespaces
			if req.ScheduleName != "" && req.Description != "" {
				break
			}
		}
	}

	// IMPORTANTE: Eliminar SleepInfos antiguos ANTES de crear los nuevos
	// Esto asegura que los cambios se reflejen correctamente, especialmente cuando cambian los weekdays
	// o cuando se cambia de un schedule único a múltiples SleepInfos (o viceversa)
	if err := s.DeleteSchedule(ctx, tenant, filterNamespace); err != nil {
		// Si no se encuentran schedules, está bien - crearemos nuevos
		if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "no schedules found") {
			s.logger.Info("Failed to delete existing schedules before update (will continue)", "error", err, "tenant", tenant, "namespace", filterNamespace)
			// Continuar de todas formas - CreateSchedule usará createOrUpdateSleepInfo que actualizará si existen
		}
	}

	// Validar que weekdays estén presentes
	// Si no están, usar valores por defecto (todos los días)
	if req.SleepDays == "" && req.Weekdays == "" {
		req.Weekdays = "0-6"
		s.logger.Info("UpdateSchedule: using default Weekdays", "weekdays", req.Weekdays)
	}
	if req.WakeDays == "" && req.SleepDays == "" && req.Weekdays == "" {
		req.Weekdays = "0-6"
		s.logger.Info("UpdateSchedule: using default Weekdays for wake", "weekdays", req.Weekdays)
	}

	req.Tenant = tenant
	s.logger.Info("UpdateSchedule: calling CreateSchedule", "tenant", tenant, "namespaces", strings.Join(req.Namespaces, ","), "off", req.Off, "on", req.On, "weekdays", req.Weekdays, "sleepDays", req.SleepDays, "wakeDays", req.WakeDays)
	return s.CreateSchedule(ctx, req)
}

// DeleteSchedule deletes all SleepInfos for a tenant
func (s *ScheduleService) DeleteSchedule(ctx context.Context, tenant string, namespaceSuffix ...string) error {
	// List all SleepInfos
	sleepInfoList := &kubegreenv1alpha1.SleepInfoList{}
	if err := s.client.List(ctx, sleepInfoList); err != nil {
		return fmt.Errorf("failed to list SleepInfos: %w", err)
	}

	var filterNamespace string
	if len(namespaceSuffix) > 0 && namespaceSuffix[0] != "" {
		filterNamespace = namespaceSuffix[0]
	}

	// Find and delete all SleepInfos for the tenant
	deletedCount := 0
	for _, si := range sleepInfoList.Items {
		// Extract tenant from namespace
		nsParts := strings.Split(si.Namespace, "-")
		if len(nsParts) < 2 {
			continue
		}

		tenantFromNS := strings.Join(nsParts[:len(nsParts)-1], "-")
		if tenantFromNS != tenant {
			continue
		}

		suffix := nsParts[len(nsParts)-1]

		// Filter by namespace suffix if provided
		if filterNamespace != "" && suffix != filterNamespace {
			continue
		}

		// Delete associated secret first (if it exists)
		secretName := fmt.Sprintf("sleepinfo-%s", si.Name)
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: si.Namespace,
			},
		}
		if err := s.client.Delete(ctx, secret); err != nil {
			// Ignore not found errors (secret might not exist)
			if client.IgnoreNotFound(err) == nil {
				s.logger.Info("Secret not found or already deleted", "secret", secretName, "namespace", si.Namespace)
			} else {
				s.logger.Error(err, "failed to delete secret", "secret", secretName, "namespace", si.Namespace)
			}
		} else {
			s.logger.Info("Associated secret deleted", "secret", secretName, "namespace", si.Namespace)
		}

		// Delete the SleepInfo
		if err := s.client.Delete(ctx, &si); err != nil {
			s.logger.Error(err, "failed to delete SleepInfo", "name", si.Name, "namespace", si.Namespace)
			continue
		}

		deletedCount++
		s.logger.Info("SleepInfo deleted", "name", si.Name, "namespace", si.Namespace)
	}

	if deletedCount == 0 {
		if filterNamespace != "" {
			return fmt.Errorf("no schedules found for tenant: %s in namespace: %s", tenant, filterNamespace)
		}
		return fmt.Errorf("no schedules found for tenant: %s", tenant)
	}

	if filterNamespace != "" {
		s.logger.Info("Deleted schedules for tenant and namespace", "tenant", tenant, "namespace", filterNamespace, "count", deletedCount)
	} else {
		s.logger.Info("Deleted schedules for tenant", "tenant", tenant, "count", deletedCount)
	}
	return nil
}

// TenantInfo represents a discovered tenant
type TenantInfo struct {
	Name       string   `json:"name"`
	Namespaces []string `json:"namespaces"`
	CreatedAt  string   `json:"createdAt,omitempty"`
}

// TenantListResponse represents the response for listing tenants
type TenantListResponse struct {
	Tenants []TenantInfo `json:"tenants"`
}

// ListTenants discovers all tenants by scanning namespaces
func (s *ScheduleService) ListTenants(ctx context.Context) (*TenantListResponse, error) {
	// List all namespaces
	namespaceList := &v1.NamespaceList{}
	if err := s.client.List(ctx, namespaceList); err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	s.logger.Info("ListTenants", "total_namespaces_found", len(namespaceList.Items))

	// Map to track tenants and their namespaces (dinámico - sin filtrar por validSuffixes)
	tenantMap := make(map[string]map[string]bool)

	for _, ns := range namespaceList.Items {
		nsName := ns.Name

		// Check if namespace matches tenant-suffix pattern
		nsParts := strings.Split(nsName, "-")
		if len(nsParts) < 2 {
			continue // Skip namespaces that don't match pattern
		}

		// Extract tenant (all parts except last)
		tenant := strings.Join(nsParts[:len(nsParts)-1], "-")
		suffix := nsParts[len(nsParts)-1]

		// NO FILTRAR por validSuffixes - aceptar TODOS los namespaces que coincidan con el patrón
		// Esto permite descubrimiento dinámico de cualquier namespace que siga el patrón {tenant}-{prefix}

		// Initialize tenant map if needed
		if tenantMap[tenant] == nil {
			tenantMap[tenant] = make(map[string]bool)
		}

		// Add namespace suffix (prefix) dinámicamente
		tenantMap[tenant][suffix] = true

		// Debug: log bdadev namespaces as they're found
		if tenant == "bdadev" {
			s.logger.Info("ListTenants", "found_bdadev_namespace", suffix, "full_namespace", nsName)
		}
	}

	s.logger.Info("ListTenants", "total_tenants_found", len(tenantMap))
	if bdadevNamespaces, ok := tenantMap["bdadev"]; ok {
		s.logger.Info("ListTenants", "bdadev_namespaces_count", len(bdadevNamespaces))
		// Log all bdadev namespaces found
		bdadevList := make([]string, 0, len(bdadevNamespaces))
		for ns := range bdadevNamespaces {
			bdadevList = append(bdadevList, ns)
		}
		s.logger.Info("ListTenants", "bdadev_namespaces_list", strings.Join(bdadevList, ","))
	}

	// Convert to response format
	tenants := make([]TenantInfo, 0, len(tenantMap))
	for tenant, namespaces := range tenantMap {
		nsList := make([]string, 0, len(namespaces))
		for ns := range namespaces {
			nsList = append(nsList, ns)
		}
		// Sort namespaces for consistent ordering
		sort.Strings(nsList)

		// Debug: log bdadev namespaces in response
		if tenant == "bdadev" {
			s.logger.Info("ListTenants", "bdadev_response_namespaces", strings.Join(nsList, ","), "count", len(nsList))
		}

		tenants = append(tenants, TenantInfo{
			Name:       tenant,
			Namespaces: nsList,
		})
	}
	// Sort tenants by name for consistent ordering
	sort.Slice(tenants, func(i, j int) bool {
		return tenants[i].Name < tenants[j].Name
	})

	return &TenantListResponse{
		Tenants: tenants,
	}, nil
}

// ServiceInfo represents a Kubernetes service/resource
type ServiceInfo struct {
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	Annotations   map[string]string `json:"annotations"`
	Labels        map[string]string `json:"labels"`
	Replicas      *int32            `json:"replicas,omitempty"`
	ReadyReplicas *int32            `json:"readyReplicas,omitempty"`
	Status        string            `json:"status,omitempty"`
}

// NamespaceServicesResponse represents services in a namespace
type NamespaceServicesResponse struct {
	Namespace string        `json:"namespace"`
	Services  []ServiceInfo `json:"services"`
}

// GetNamespaceServices lists all services (Deployments, StatefulSets, CronJobs) in a namespace
func (s *ScheduleService) GetNamespaceServices(ctx context.Context, tenant, namespaceSuffix string) (*NamespaceServicesResponse, error) {
	namespace := fmt.Sprintf("%s-%s", tenant, namespaceSuffix)

	services := make([]ServiceInfo, 0)

	// List Deployments
	deploymentList := &appsv1.DeploymentList{}
	if err := s.client.List(ctx, deploymentList, client.InNamespace(namespace)); err == nil {
		for _, dep := range deploymentList.Items {
			replicas := int32(0)
			if dep.Spec.Replicas != nil {
				replicas = *dep.Spec.Replicas
			}
			readyReplicas := dep.Status.ReadyReplicas

			status := "Running"
			if replicas == 0 {
				status = "Suspended"
			} else if readyReplicas < replicas {
				status = "Pending"
			}

			services = append(services, ServiceInfo{
				Name:          dep.Name,
				Kind:          "Deployment",
				Annotations:   dep.Annotations,
				Labels:        dep.Labels,
				Replicas:      &replicas,
				ReadyReplicas: &readyReplicas,
				Status:        status,
			})
		}
	}

	// List StatefulSets
	statefulSetList := &appsv1.StatefulSetList{}
	if err := s.client.List(ctx, statefulSetList, client.InNamespace(namespace)); err == nil {
		for _, sts := range statefulSetList.Items {
			replicas := int32(0)
			if sts.Spec.Replicas != nil {
				replicas = *sts.Spec.Replicas
			}
			readyReplicas := sts.Status.ReadyReplicas

			status := "Running"
			if replicas == 0 {
				status = "Suspended"
			} else if readyReplicas < replicas {
				status = "Pending"
			}

			services = append(services, ServiceInfo{
				Name:          sts.Name,
				Kind:          "StatefulSet",
				Annotations:   sts.Annotations,
				Labels:        sts.Labels,
				Replicas:      &replicas,
				ReadyReplicas: &readyReplicas,
				Status:        status,
			})
		}
	}

	// List CronJobs
	cronJobList := &batchv1.CronJobList{}
	if err := s.client.List(ctx, cronJobList, client.InNamespace(namespace)); err == nil {
		for _, cj := range cronJobList.Items {
			suspended := false
			if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
				suspended = true
			}

			status := "Running"
			if suspended {
				status = "Suspended"
			}

			services = append(services, ServiceInfo{
				Name:        cj.Name,
				Kind:        "CronJob",
				Annotations: cj.Annotations,
				Labels:      cj.Labels,
				Status:      status,
			})
		}
	}

	return &NamespaceServicesResponse{
		Namespace: namespace,
		Services:  services,
	}, nil
}

// SuspendedServiceInfo represents a suspended service
type SuspendedServiceInfo struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`
	SuspendedAt string `json:"suspendedAt"`
	Reason      string `json:"reason"`
	WillWakeAt  string `json:"willWakeAt,omitempty"`
}

// SuspendedServicesResponse represents suspended services for a tenant
type SuspendedServicesResponse struct {
	Tenant    string                 `json:"tenant"`
	Suspended []SuspendedServiceInfo `json:"suspended"`
}

// GetSuspendedServices lists currently suspended services for a tenant
func (s *ScheduleService) GetSuspendedServices(ctx context.Context, tenant string) (*SuspendedServicesResponse, error) {
	// List all SleepInfos for the tenant
	_, err := s.GetSchedule(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}

	suspended := make([]SuspendedServiceInfo, 0)

	// TODO: Implement logic to check actual resource states
	// This would require:
	// 1. List Deployments/StatefulSets in each namespace
	// 2. Check if replicas are 0
	// 3. Check associated SleepInfo to determine when they were suspended
	// 4. Check when they will wake up based on wake schedule

	return &SuspendedServicesResponse{
		Tenant:    tenant,
		Suspended: suspended,
	}, nil
}

// NamespaceResourceInfo represents detected resources in a namespace
type NamespaceResourceInfo struct {
	Namespace      string            `json:"namespace"`
	HasPgCluster   bool              `json:"hasPgCluster"`
	HasHdfsCluster bool              `json:"hasHdfsCluster"`
	HasPgBouncer   bool              `json:"hasPgBouncer"`
	HasVirtualizer bool              `json:"hasVirtualizer"`
	ResourceCounts ResourceCounts    `json:"resourceCounts"`
	AutoExclusions []ExclusionFilter `json:"autoExclusions"`
}

// ResourceCounts represents counts of different resource types
type ResourceCounts struct {
	Deployments  int `json:"deployments"`
	StatefulSets int `json:"statefulSets"`
	CronJobs     int `json:"cronJobs"`
	PgClusters   int `json:"pgClusters"`
	HdfsClusters int `json:"hdfsClusters"`
	PgBouncers   int `json:"pgBouncers"`
}

// GetNamespaceResources detects CRDs and other resources in a namespace
func (s *ScheduleService) GetNamespaceResources(ctx context.Context, tenant, namespaceSuffix string) (*NamespaceResourceInfo, error) {
	namespace := fmt.Sprintf("%s-%s", tenant, namespaceSuffix)

	info := &NamespaceResourceInfo{
		Namespace:      namespace,
		HasPgCluster:   false,
		HasHdfsCluster: false,
		HasPgBouncer:   false,
		HasVirtualizer: false,
		ResourceCounts: ResourceCounts{},
		AutoExclusions: []ExclusionFilter{},
	}

	// List Deployments
	deploymentList := &appsv1.DeploymentList{}
	if err := s.client.List(ctx, deploymentList, client.InNamespace(namespace)); err == nil {
		info.ResourceCounts.Deployments = len(deploymentList.Items)

		// Check for Virtualizer (apps namespace)
		for _, dep := range deploymentList.Items {
			if appID, ok := dep.Labels["cct.stratio.com/application_id"]; ok {
				if strings.Contains(appID, "virtualizer") {
					info.HasVirtualizer = true
					break
				}
			}
		}
	}

	// List StatefulSets
	statefulSetList := &appsv1.StatefulSetList{}
	if err := s.client.List(ctx, statefulSetList, client.InNamespace(namespace)); err == nil {
		info.ResourceCounts.StatefulSets = len(statefulSetList.Items)
	}

	// List CronJobs
	cronJobList := &batchv1.CronJobList{}
	if err := s.client.List(ctx, cronJobList, client.InNamespace(namespace)); err == nil {
		info.ResourceCounts.CronJobs = len(cronJobList.Items)
	}

	// Detect PgCluster CRDs
	pgClusterGVR := schema.GroupVersionResource{
		Group:    "postgres.stratio.com",
		Version:  "v1",
		Resource: "pgclusters",
	}
	pgClusterList := &unstructured.UnstructuredList{}
	pgClusterList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   pgClusterGVR.Group,
		Version: pgClusterGVR.Version,
		Kind:    "PgClusterList",
	})
	if err := s.client.List(ctx, pgClusterList, client.InNamespace(namespace)); err == nil {
		info.ResourceCounts.PgClusters = len(pgClusterList.Items)
		info.HasPgCluster = len(pgClusterList.Items) > 0
	} else {
		// Try alternative API group
		pgClusterGVR2 := schema.GroupVersionResource{
			Group:    "postgresql.cnpg.io",
			Version:  "v1",
			Resource: "clusters",
		}
		pgClusterList2 := &unstructured.UnstructuredList{}
		pgClusterList2.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   pgClusterGVR2.Group,
			Version: pgClusterGVR2.Version,
			Kind:    "ClusterList",
		})
		if err2 := s.client.List(ctx, pgClusterList2, client.InNamespace(namespace)); err2 == nil {
			info.ResourceCounts.PgClusters = len(pgClusterList2.Items)
			info.HasPgCluster = len(pgClusterList2.Items) > 0
		}
	}

	// Detect HDFSCluster CRDs
	hdfsClusterGVR := schema.GroupVersionResource{
		Group:    "hdfs.stratio.com",
		Version:  "v1",
		Resource: "hdfsclusters",
	}
	hdfsClusterList := &unstructured.UnstructuredList{}
	hdfsClusterList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   hdfsClusterGVR.Group,
		Version: hdfsClusterGVR.Version,
		Kind:    "HDFSClusterList",
	})
	if err := s.client.List(ctx, hdfsClusterList, client.InNamespace(namespace)); err == nil {
		info.ResourceCounts.HdfsClusters = len(hdfsClusterList.Items)
		info.HasHdfsCluster = len(hdfsClusterList.Items) > 0
	}

	// Detect PgBouncer CRDs
	pgBouncerGVR := schema.GroupVersionResource{
		Group:    "postgres.stratio.com",
		Version:  "v1",
		Resource: "pgbouncers",
	}
	pgBouncerList := &unstructured.UnstructuredList{}
	pgBouncerList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   pgBouncerGVR.Group,
		Version: pgBouncerGVR.Version,
		Kind:    "PgBouncerList",
	})
	if err := s.client.List(ctx, pgBouncerList, client.InNamespace(namespace)); err == nil {
		info.ResourceCounts.PgBouncers = len(pgBouncerList.Items)
		info.HasPgBouncer = len(pgBouncerList.Items) > 0
	}

	// Build auto-exclusions based on detected resources
	if info.HasPgCluster || info.HasPgBouncer {
		info.AutoExclusions = append(info.AutoExclusions, ExclusionFilter{
			MatchLabels: map[string]string{
				"app.kubernetes.io/managed-by": "postgres-operator",
			},
		})
		info.AutoExclusions = append(info.AutoExclusions, ExclusionFilter{
			MatchLabels: map[string]string{
				"postgres.stratio.com/cluster": "true",
			},
		})
		info.AutoExclusions = append(info.AutoExclusions, ExclusionFilter{
			MatchLabels: map[string]string{
				"app.kubernetes.io/part-of": "postgres",
			},
		})
	}

	if info.HasHdfsCluster {
		info.AutoExclusions = append(info.AutoExclusions, ExclusionFilter{
			MatchLabels: map[string]string{
				"app.kubernetes.io/managed-by": "hdfs-operator",
			},
		})
		info.AutoExclusions = append(info.AutoExclusions, ExclusionFilter{
			MatchLabels: map[string]string{
				"hdfs.stratio.com/cluster": "true",
			},
		})
		info.AutoExclusions = append(info.AutoExclusions, ExclusionFilter{
			MatchLabels: map[string]string{
				"app.kubernetes.io/part-of": "hdfs",
			},
		})
	}

	return info, nil
}

// NamespaceScheduleResponse represents a schedule response for a single namespace
type NamespaceScheduleResponse struct {
	Tenant     string            `json:"tenant"`
	Namespace  string            `json:"namespace"`
	SleepInfos []SleepInfoDetail `json:"sleepInfos"`
}

// SleepInfoDetail represents detailed information about a SleepInfo
type SleepInfoDetail struct {
	Name                        string            `json:"name"`
	Namespace                   string            `json:"namespace"`
	Weekdays                    string            `json:"weekdays"`
	SleepAt                     string            `json:"sleepAt,omitempty"`
	WakeUpAt                    string            `json:"wakeUpAt,omitempty"`
	TimeZone                    string            `json:"timeZone"`
	Role                        string            `json:"role,omitempty"` // "sleep" or "wake" from annotations
	SuspendDeployments          bool              `json:"suspendDeployments"`
	SuspendStatefulSets         bool              `json:"suspendStatefulSets"`
	SuspendCronJobs             bool              `json:"suspendCronJobs"`
	SuspendDeploymentsPgbouncer bool              `json:"suspendDeploymentsPgbouncer,omitempty"`
	SuspendStatefulSetsPostgres bool              `json:"suspendStatefulSetsPostgres,omitempty"`
	SuspendStatefulSetsHdfs     bool              `json:"suspendStatefulSetsHdfs,omitempty"`
	ExcludeRef                  []ExclusionFilter `json:"excludeRef,omitempty"`
	Annotations                 map[string]string `json:"annotations,omitempty"`
}

// GetNamespaceSchedule gets SleepInfos for a specific namespace
func (s *ScheduleService) GetNamespaceSchedule(ctx context.Context, tenant, namespaceSuffix string) (*NamespaceScheduleResponse, error) {
	namespace := fmt.Sprintf("%s-%s", tenant, namespaceSuffix)

	// List SleepInfos in the namespace
	sleepInfoList := &kubegreenv1alpha1.SleepInfoList{}
	if err := s.client.List(ctx, sleepInfoList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list SleepInfos: %w", err)
	}

	if len(sleepInfoList.Items) == 0 {
		return nil, fmt.Errorf("no schedules found for tenant %s in namespace %s", tenant, namespaceSuffix)
	}

	// Convert to detail format
	sleepInfos := make([]SleepInfoDetail, 0, len(sleepInfoList.Items))
	for _, si := range sleepInfoList.Items {
		detail := SleepInfoDetail{
			Name:                        si.Name,
			Namespace:                   si.Namespace,
			Weekdays:                    si.Spec.Weekdays,
			SleepAt:                     si.Spec.SleepTime,
			WakeUpAt:                    si.Spec.WakeUpTime,
			TimeZone:                    si.Spec.TimeZone,
			SuspendDeployments:          si.Spec.SuspendDeployments != nil && *si.Spec.SuspendDeployments,
			SuspendStatefulSets:         si.Spec.SuspendStatefulSets != nil && *si.Spec.SuspendStatefulSets,
			SuspendCronJobs:             si.Spec.SuspendCronjobs,
			SuspendDeploymentsPgbouncer: si.Spec.SuspendDeploymentsPgbouncer != nil && *si.Spec.SuspendDeploymentsPgbouncer,
			SuspendStatefulSetsPostgres: si.Spec.SuspendStatefulSetsPostgres != nil && *si.Spec.SuspendStatefulSetsPostgres,
			SuspendStatefulSetsHdfs:     si.Spec.SuspendStatefulSetsHdfs != nil && *si.Spec.SuspendStatefulSetsHdfs,
			Annotations:                 si.Annotations,
		}

		// Extract role from annotations
		if role, ok := si.Annotations["kube-green.stratio.com/pair-role"]; ok {
			detail.Role = role
		}

		// Convert excludeRef
		if len(si.Spec.ExcludeRef) > 0 {
			detail.ExcludeRef = make([]ExclusionFilter, 0, len(si.Spec.ExcludeRef))
			for _, ref := range si.Spec.ExcludeRef {
				detail.ExcludeRef = append(detail.ExcludeRef, ExclusionFilter{
					MatchLabels: ref.MatchLabels,
				})
			}
		}

		sleepInfos = append(sleepInfos, detail)
	}

	return &NamespaceScheduleResponse{
		Tenant:     tenant,
		Namespace:  namespaceSuffix,
		SleepInfos: sleepInfos,
	}, nil
}

// CreateNamespaceSchedule creates SleepInfos for a specific namespace using dynamic resource detection
func (s *ScheduleService) CreateNamespaceSchedule(ctx context.Context, req NamespaceScheduleRequest) error {
	// 1. Detect resources in the namespace
	resources, err := s.GetNamespaceResources(ctx, req.Tenant, req.Namespace)
	if err != nil {
		return fmt.Errorf("failed to detect resources: %w", err)
	}

	// 2. Normalize weekdays
	wdSleep := req.WeekdaysSleep
	if wdSleep == "" {
		wdSleep = "0-6"
	}
	wdSleepKube, err := HumanWeekdaysToKube(wdSleep)
	if err != nil {
		return fmt.Errorf("invalid sleep weekdays: %w", err)
	}

	wdWake := req.WeekdaysWake
	if wdWake == "" {
		wdWake = wdSleepKube
	}
	wdWakeKube, err := HumanWeekdaysToKube(wdWake)
	if err != nil {
		return fmt.Errorf("invalid wake weekdays: %w", err)
	}

	// 3. Convert times to UTC (default to America/Bogota -> UTC)
	userTZ := TZLocal

	offConv, err := ToUTCHHMM(req.Off, userTZ)
	if err != nil {
		return fmt.Errorf("invalid off time: %w", err)
	}

	onConv, err := ToUTCHHMM(req.On, userTZ)
	if err != nil {
		return fmt.Errorf("invalid on time: %w", err)
	}

	// 4. Adjust weekdays for timezone shift
	wdSleepUTC, err := ShiftWeekdaysStr(wdSleepKube, offConv.DayShift)
	if err != nil {
		return fmt.Errorf("failed to shift sleep weekdays: %w", err)
	}

	wdWakeUTC, err := ShiftWeekdaysStr(wdWakeKube, onConv.DayShift)
	if err != nil {
		return fmt.Errorf("failed to shift wake weekdays: %w", err)
	}

	// 5. Calculate staggered wake times based on delays
	onPgHDFS := onConv.TimeUTC
	onPgBouncer := onConv.TimeUTC
	onDeployments := onConv.TimeUTC

	if req.Delays != nil {
		if req.Delays.PgHdfsDelay != "" {
			delayMinutes, _ := parseDelayToMinutes(req.Delays.PgHdfsDelay)
			onPgHDFS, _ = AddMinutes(onConv.TimeUTC, delayMinutes)
		}
		if req.Delays.PgbouncerDelay != "" {
			delayMinutes, _ := parseDelayToMinutes(req.Delays.PgbouncerDelay)
			onPgBouncer, _ = AddMinutes(onConv.TimeUTC, delayMinutes)
		}
		if req.Delays.DeploymentsDelay != "" {
			delayMinutes, _ := parseDelayToMinutes(req.Delays.DeploymentsDelay)
			onDeployments, _ = AddMinutes(onConv.TimeUTC, delayMinutes)
		}
	} else {
		// Default delays (like Python script)
		onPgHDFS = onConv.TimeUTC // t0
		// Aplicar delays por defecto SOLO para datastores (staggered wake)
		onPgBouncer, _ = AddMinutes(onConv.TimeUTC, 5)   // t0+5m para PgBouncer
		onDeployments, _ = AddMinutes(onConv.TimeUTC, 7) // t0+7m para Deployments
	}

	// 6. Build excludeRefs
	excludeRefs := resources.AutoExclusions
	if len(req.Exclusions) > 0 {
		for _, excl := range req.Exclusions {
			if excl.Namespace == req.Namespace || excl.Namespace == fmt.Sprintf("%s-%s", req.Tenant, req.Namespace) {
				excludeRefs = append(excludeRefs, ExclusionFilter{
					MatchLabels: excl.Filter.MatchLabels,
				})
			}
		}
	}

	// 7. Validate scheduleName uniqueness if provided
	if req.ScheduleName != "" {
		namespace := fmt.Sprintf("%s-%s", req.Tenant, req.Namespace)
		if err := s.validateScheduleNameUniqueness(ctx, namespace, req.ScheduleName); err != nil {
			return err
		}
	}

	// Note: Virtualizer detection is available in resources.HasVirtualizer
	// but exclusion must be configured manually from frontend if needed
	// No automatic exclusion is applied

	// Convert to kubegreen FilterRef
	kubeExcludeRefs := make([]kubegreenv1alpha1.FilterRef, 0, len(excludeRefs))
	for _, excl := range excludeRefs {
		kubeExcludeRefs = append(kubeExcludeRefs, kubegreenv1alpha1.FilterRef{
			MatchLabels: excl.MatchLabels,
		})
	}

	namespace := fmt.Sprintf("%s-%s", req.Tenant, req.Namespace)

	// 7. Generate SleepInfos based on detected resources (DYNAMIC LOGIC)
	hasCRDs := resources.HasPgCluster || resources.HasHdfsCluster || resources.HasPgBouncer

	if hasCRDs {
		// Apply staggered wake logic when CRDs are detected
		if err := s.createDatastoresSleepInfosWithExclusions(ctx, req.Tenant, namespace, offConv.TimeUTC, onDeployments, onPgHDFS, onPgBouncer, wdSleepUTC, wdWakeUTC, kubeExcludeRefs, req.ScheduleName, req.Description, userTZ); err != nil {
			return fmt.Errorf("failed to create staggered sleepinfos: %w", err)
		}
	} else {
		// Simple namespace without CRDs
		suspendStatefulSets := false

		// Special case: airflowsso can have PgCluster
		if req.Namespace == "airflowsso" && resources.HasPgCluster {
			suspendStatefulSets = true
		}

		if err := s.createNamespaceSleepInfoWithExclusions(ctx, req.Tenant, namespace, req.Namespace, offConv.TimeUTC, onDeployments, wdSleepUTC, wdWakeUTC, suspendStatefulSets, kubeExcludeRefs, req.ScheduleName, req.Description, userTZ); err != nil {
			return fmt.Errorf("failed to create namespace sleepinfo: %w", err)
		}
	}

	return nil
}

// UpdateNamespaceSchedule updates SleepInfos for a specific namespace
func (s *ScheduleService) UpdateNamespaceSchedule(ctx context.Context, req NamespaceScheduleRequest) error {
	// Delete existing schedule first
	if err := s.DeleteNamespaceSchedule(ctx, req.Tenant, req.Namespace); err != nil {
		// If not found, that's okay - we'll create new
		if !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("failed to delete existing schedule: %w", err)
		}
	}

	// Create new schedule
	return s.CreateNamespaceSchedule(ctx, req)
}

// extractDelaysFromSchedule extrae los delays configurados de un schedule existente
// analizando los tiempos de los SleepInfos wake en namespaces datastores
func (s *ScheduleService) extractDelaysFromSchedule(existing *ScheduleResponse, targetNamespaces []string) *DelayConfig {
	// Buscar namespace datastores para extraer delays (solo datastores tiene staggered wake)
	var datastoresNS *NamespaceInfo
	for _, nsSuffix := range targetNamespaces {
		if nsSuffix == "datastores" {
			if nsInfo, ok := existing.Namespaces["datastores"]; ok {
				datastoresNS = &nsInfo
				break
			}
		}
	}

	if datastoresNS == nil || len(datastoresNS.Schedule) == 0 {
		// No hay datastores o no hay schedules, no hay delays que extraer
		return nil
	}

	// Buscar todos los SleepInfos wake en datastores
	var wakeSchedules []SleepInfoSummary
	for _, sched := range datastoresNS.Schedule {
		if sched.Role == "wake" {
			wakeSchedules = append(wakeSchedules, sched)
		}
	}

	if len(wakeSchedules) < 2 {
		// No hay suficientes wake schedules para calcular delays (necesitamos al menos 2)
		return nil
	}

	// Encontrar el tiempo base (el más temprano) - este es PgHDFS (t0)
	baseTime := ""
	for _, sched := range wakeSchedules {
		if baseTime == "" || sched.Time < baseTime {
			baseTime = sched.Time
		}
	}

	if baseTime == "" {
		return nil
	}

	// Calcular delays basándose en los tiempos y los recursos que manejan
	delays := &DelayConfig{}

	for _, sched := range wakeSchedules {
		if sched.Time == baseTime {
			continue // Skip el tiempo base
		}

		// Calcular diferencia en minutos
		delayMinutes := calculateTimeDifferenceMinutes(baseTime, sched.Time)
		if delayMinutes < 0 {
			// Si el delay es negativo, puede ser que el día cambió, ajustar
			delayMinutes += 24 * 60
		}

		// Identificar el tipo de delay basándose en los recursos
		hasPostgres := false
		hasHdfs := false
		hasPgbouncer := false
		hasDeployments := false

		for _, resource := range sched.Resources {
			resourceLower := strings.ToLower(resource)
			if strings.Contains(resourceLower, "postgres") {
				hasPostgres = true
			}
			if strings.Contains(resourceLower, "hdfs") {
				hasHdfs = true
			}
			if strings.Contains(resourceLower, "pgbouncer") {
				hasPgbouncer = true
			}
			if strings.Contains(resourceLower, "deployment") {
				hasDeployments = true
			}
		}

		// Mapear a los campos de DelayConfig
		// DelayConfig usa: PgbouncerDelay y DeploymentsDelay
		if hasPgbouncer && !hasDeployments && delays.PgbouncerDelay == "" {
			delays.PgbouncerDelay = formatMinutesToDelay(delayMinutes)
		} else if hasDeployments && !hasPostgres && !hasHdfs && delays.DeploymentsDelay == "" {
			delays.DeploymentsDelay = formatMinutesToDelay(delayMinutes)
		}
	}

	// Solo retornar si se encontraron delays
	if delays.PgbouncerDelay != "" || delays.DeploymentsDelay != "" {
		return delays
	}

	return nil
}

// calculateTimeDifferenceMinutes calcula la diferencia en minutos entre dos tiempos HH:MM
func calculateTimeDifferenceMinutes(time1, time2 string) int {
	parts1 := strings.Split(time1, ":")
	parts2 := strings.Split(time2, ":")
	if len(parts1) != 2 || len(parts2) != 2 {
		return 0
	}

	hour1, _ := strconv.Atoi(parts1[0])
	min1, _ := strconv.Atoi(parts1[1])
	hour2, _ := strconv.Atoi(parts2[0])
	min2, _ := strconv.Atoi(parts2[1])

	totalMinutes1 := hour1*60 + min1
	totalMinutes2 := hour2*60 + min2

	return totalMinutes2 - totalMinutes1
}

// formatMinutesToDelay formatea minutos a formato delay (e.g., "5m", "7m")
func formatMinutesToDelay(minutes int) string {
	if minutes < 0 {
		minutes += 24 * 60 // Ajustar si es negativo (cambio de día)
	}
	return fmt.Sprintf("%dm", minutes)
}

// DeleteNamespaceSchedule deletes all SleepInfos for a specific namespace
func (s *ScheduleService) DeleteNamespaceSchedule(ctx context.Context, tenant, namespaceSuffix string) error {
	namespace := fmt.Sprintf("%s-%s", tenant, namespaceSuffix)

	// List all SleepInfos in the namespace
	sleepInfoList := &kubegreenv1alpha1.SleepInfoList{}
	if err := s.client.List(ctx, sleepInfoList, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list SleepInfos: %w", err)
	}

	if len(sleepInfoList.Items) == 0 {
		return fmt.Errorf("no schedules found for tenant %s in namespace %s", tenant, namespaceSuffix)
	}

	// Delete each SleepInfo
	for _, si := range sleepInfoList.Items {
		if err := s.client.Delete(ctx, &si); err != nil {
			return fmt.Errorf("failed to delete SleepInfo %s: %w", si.Name, err)
		}
	}

	return nil
}
