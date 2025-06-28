package sleepinfo

import (
	"fmt"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/cronjobs"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/deployments"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/jsonpatch"

	v1 "k8s.io/api/core/v1"
)

type SleepInfoData struct {
	LastSchedule                time.Time
	CurrentOperationType        string
	CurrentOperationSchedule    string
	NextOperationSchedule       string
	OriginalGenericResourceInfo map[string]jsonpatch.RestorePatches
}

func (s SleepInfoData) IsWakeUpOperation() bool {
	return s.CurrentOperationType == wakeUpOperation
}

func (s SleepInfoData) IsSleepOperation() bool {
	return s.CurrentOperationType == sleepOperation
}

func getSleepInfoData(secret *v1.Secret, sleepInfo *kubegreenv1alpha1.SleepInfo) (SleepInfoData, error) {
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
	}
	if wakeUpSchedule == "" {
		sleepInfoData.NextOperationSchedule = sleepSchedule
	}

	if secret == nil || secret.Data == nil {
		return sleepInfoData, nil
	}
	data := secret.Data

	if sleepInfoData.OriginalGenericResourceInfo, err = jsonpatch.GetOriginalInfoToRestore(data[originalJSONPatchDataKey]); err != nil {
		return SleepInfoData{}, fmt.Errorf("fails to set original resource info to restore in SleepInfo %s: %s", sleepInfo.Name, err)
	}
	// This will convert old secret format, where the original deployment and
	// cronjob states were stored in a different key
	sleepInfoData.OriginalGenericResourceInfo, err = convertOldSecretDataToNewFormat(sleepInfoData.OriginalGenericResourceInfo, data)
	if err != nil {
		return SleepInfoData{}, err
	}

	lastSchedule, err := time.Parse(time.RFC3339, string(data[lastScheduleKey]))
	if err != nil {
		return SleepInfoData{}, fmt.Errorf("fails to parse %s: %s", lastScheduleKey, err)
	}
	sleepInfoData.LastSchedule = lastSchedule

	lastOperation := string(data[lastOperationKey])

	if lastOperation == sleepOperation && wakeUpSchedule != "" {
		sleepInfoData.CurrentOperationSchedule = wakeUpSchedule
		sleepInfoData.NextOperationSchedule = sleepSchedule
		sleepInfoData.CurrentOperationType = wakeUpOperation
	}

	return sleepInfoData, nil
}

const (
	replicasBeforeSleepKey   = "deployment-replicas"
	originalCronjobStatusKey = "cronjobs-info"
)

func convertOldSecretDataToNewFormat(originalGenericResourceInfo map[string]jsonpatch.RestorePatches, secretData map[string][]byte) (map[string]jsonpatch.RestorePatches, error) {
	if originalGenericResourceInfo == nil {
		originalGenericResourceInfo = make(map[string]jsonpatch.RestorePatches)
	}
	if replicas, ok := secretData[replicasBeforeSleepKey]; ok {
		data, err := deployments.GetOriginalInfoToRestore(replicas)
		if err != nil {
			return nil, fmt.Errorf("fails to set original deployment replicas info to restore: %s", err)
		}
		originalGenericResourceInfo[kubegreenv1alpha1.DeploymentTarget.String()] = data
	}
	if cronjobStatus, ok := secretData[originalCronjobStatusKey]; ok {
		data, err := cronjobs.GetOriginalInfoToRestore(cronjobStatus)
		if err != nil {
			return nil, fmt.Errorf("fails to set original cronjob status info to restore: %s", err)
		}
		originalGenericResourceInfo[kubegreenv1alpha1.CronJobTarget.String()] = data
	}
	return originalGenericResourceInfo, nil
}
