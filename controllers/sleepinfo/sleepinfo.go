package sleepinfo

import (
	"context"
	"fmt"
	"time"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Get sleep info from namespace
func (r *SleepInfoReconciler) getSleepInfo(ctx context.Context, req ctrl.Request) (*kubegreenv1alpha1.SleepInfo, error) {
	sleepInfo := &kubegreenv1alpha1.SleepInfo{}
	if err := r.Client.Get(ctx, req.NamespacedName, sleepInfo); err != nil {
		return nil, err
	}
	return sleepInfo, nil
}

type SleepInfoData struct {
	LastSchedule                time.Time
	CurrentOperationType        string
	OriginalDeploymentsReplicas map[string]int32
	OriginalCronJobSuspendState map[string]bool
	CurrentOperationSchedule    string
	NextOperationSchedule       string
	SuspendCronjobs             bool
}

func (s SleepInfoData) isWakeUpOperation() bool {
	return s.CurrentOperationType == wakeUpOperation
}

func (s SleepInfoData) isSleepOperation() bool {
	return s.CurrentOperationType == sleepOperation
}

// Get sleep info data merging data saved in secret and data in CRD SleepInfo
func getSleepInfoData(secret sleepInfoSecret, sleepInfo *kubegreenv1alpha1.SleepInfo) (SleepInfoData, error) {
	sleepSchedule, err := sleepInfo.GetSleepSchedule()
	if err != nil {
		return SleepInfoData{}, err
	}
	wakeUpSchedule, err := sleepInfo.GetWakeUpSchedule()
	if err != nil {
		return SleepInfoData{}, err
	}

	sleepInfoData := SleepInfoData{
		CurrentOperationType:     sleepOperation,
		CurrentOperationSchedule: sleepSchedule,
		NextOperationSchedule:    wakeUpSchedule,
		SuspendCronjobs:          sleepInfo.IsCronjobsToSuspend(),
	}
	if wakeUpSchedule == "" {
		sleepInfoData.NextOperationSchedule = sleepSchedule
	}

	if secret.Secret == nil || secret.Data == nil {
		return sleepInfoData, nil
	}

	originalDeploymentReplicas, err := secret.getOriginalDeploymentReplicas()
	if err != nil {
		return SleepInfoData{}, err
	}
	sleepInfoData.OriginalDeploymentsReplicas = originalDeploymentReplicas

	originalCronJobSuspendedState, err := secret.getOriginalCronJobSuspendedState()
	if err != nil {
		return SleepInfoData{}, err
	}
	sleepInfoData.OriginalCronJobSuspendState = originalCronJobSuspendedState

	lastSchedule, err := time.Parse(time.RFC3339, secret.getLastSchedule())
	if err != nil {
		return SleepInfoData{}, fmt.Errorf("fails to parse %s: %s", lastScheduleKey, err)
	}
	sleepInfoData.LastSchedule = lastSchedule

	lastOperation := secret.getLastOperation()

	if lastOperation == sleepOperation && wakeUpSchedule != "" {
		sleepInfoData.CurrentOperationSchedule = wakeUpSchedule
		sleepInfoData.NextOperationSchedule = sleepSchedule
		sleepInfoData.CurrentOperationType = wakeUpOperation
	}

	return sleepInfoData, nil
}
