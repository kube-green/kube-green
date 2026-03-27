package sleepinfo

import (
	"context"
	"fmt"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/jsonpatch"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Anotación para relacionar SleepInfos (pair-id: identificador común, pair-role: sleep/wake)
	pairIDAnnotation   = "kube-green.stratio.com/pair-id"
	pairRoleAnnotation = "kube-green.stratio.com/pair-role"
	pairRoleSleep      = "sleep"
	pairRoleWake       = "wake"
)

// getRelatedRestorePatches busca restore patches de SleepInfos relacionados mediante anotaciones pair-id
// Esta función permite que un SleepInfo de "wake" encuentre los restore patches guardados por un SleepInfo de "sleep"
func getRelatedRestorePatches(
	ctx context.Context,
	c client.Client,
	logger logr.Logger,
	currentSleepInfo *kubegreenv1alpha1.SleepInfo,
	namespace string,
) (map[string]jsonpatch.RestorePatches, map[string]jsonpatch.SleptResourceGenerations, error) {
	// Si el SleepInfo actual no tiene anotación pair-id, no hay relación
	pairID := currentSleepInfo.GetAnnotations()[pairIDAnnotation]
	if pairID == "" {
		return nil, nil, nil
	}

	currentRole := currentSleepInfo.GetAnnotations()[pairRoleAnnotation]
	// Solo buscamos restore patches si somos un "wake" buscando el "sleep"
	if currentRole != pairRoleWake {
		return nil, nil, nil
	}

	logger.Info("buscando restore patches de SleepInfo relacionado", "pair-id", pairID, "rol-actual", currentRole)

	// Buscar todos los SleepInfos en el namespace
	sleepInfoList := &kubegreenv1alpha1.SleepInfoList{}
	if err := c.List(ctx, sleepInfoList, client.InNamespace(namespace)); err != nil {
		return nil, nil, fmt.Errorf("failed to list SleepInfos: %w", err)
	}

	// Encontrar el SleepInfo relacionado con rol "sleep" y mismo pair-id
	var relatedSleepInfo *kubegreenv1alpha1.SleepInfo
	for i := range sleepInfoList.Items {
		si := &sleepInfoList.Items[i]
		if si.Name == currentSleepInfo.Name {
			continue // Skip el actual
		}
		if si.GetAnnotations()[pairIDAnnotation] == pairID &&
			si.GetAnnotations()[pairRoleAnnotation] == pairRoleSleep {
			relatedSleepInfo = si
			break
		}
	}

	if relatedSleepInfo == nil {
		logger.V(8).Info("no se encontró SleepInfo relacionado con rol 'sleep'", "pair-id", pairID)
		return nil, nil, nil
	}

	logger.Info("SleepInfo relacionado encontrado", "nombre", relatedSleepInfo.Name, "pair-id", pairID)

	// Obtener el secret del SleepInfo relacionado
	relatedSecretName := getSecretName(relatedSleepInfo.Name)
	relatedSecret := &v1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      relatedSecretName,
	}, relatedSecret); err != nil {
		logger.V(8).Info("no se encontró secret del SleepInfo relacionado", "secret", relatedSecretName, "error", err)
		return nil, nil, nil // No es un error crítico, simplemente no hay restore patches
	}

	// Extraer restore patches del secret relacionado
	if relatedSecret.Data == nil {
		return nil, nil, nil
	}

	restorePatches, err := jsonpatch.GetOriginalInfoToRestore(relatedSecret.Data[originalJSONPatchDataKey])
	if err != nil {
		logger.Error(err, "failed to parse restore patches from related SleepInfo secret", "secret", relatedSecretName)
		return nil, nil, nil // No es un error crítico
	}
	sleptGenerations, err := jsonpatch.GetSleepGenerationsToRestore(relatedSecret.Data[sleptGenerationsDataKey])
	if err != nil {
		logger.Error(err, "failed to parse slept generations from related SleepInfo secret", "secret", relatedSecretName)
		return restorePatches, nil, nil
	}

	if len(restorePatches) > 0 {
		logger.Info("restore patches encontrados en SleepInfo relacionado",
			"sleepinfo-relacionado", relatedSleepInfo.Name,
			"patches-count", len(restorePatches))
	}
	if len(sleptGenerations) > 0 {
		logger.Info("sleep generations encontradas en SleepInfo relacionado",
			"sleepinfo-relacionado", relatedSleepInfo.Name,
			"generation-count", len(sleptGenerations))
	}

	return restorePatches, sleptGenerations, nil
}
