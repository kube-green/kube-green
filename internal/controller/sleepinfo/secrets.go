package sleepinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/jsonpatch"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SleepInfoReconciler) getSecret(ctx context.Context, secretName, namespaceName string) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: namespaceName,
		Name:      secretName,
	}, secret)
	if err != nil {
		r.Log.Info("failed to get secret", "name", secretName, "namespace", namespaceName, "error", err)
		return nil, err
	}
	return secret, nil
}

func getSecretName(name string) string {
	return fmt.Sprintf("sleepinfo-%s", name)
}

func getRestoreSecretName(name string) string {
	return fmt.Sprintf("sleepinfo-restore-%s", name)
}

func (r SleepInfoReconciler) upsertSecret(
	ctx context.Context,
	logger logr.Logger,
	now time.Time,
	secretName, namespace string,
	sleepInfo *kubegreenv1alpha1.SleepInfo,
	secret *v1.Secret,
	sleepInfoData SleepInfoData,
	resources resource.Resource,
) error {
	logger.Info("manage secret", "name", secretName)

	var newSecret = &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": r.ManagerName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: kubegreenv1alpha1.GroupVersion.String(),
					Kind:       "SleepInfo",
					Name:       sleepInfo.Name,
					UID:        sleepInfo.UID,
				},
			},
		},
		Data:       make(map[string][]byte),
		StringData: make(map[string]string),
	}
	newSecret.StringData[lastScheduleKey] = now.Format(time.RFC3339)
	if resources.HasResource() {
		newSecret.StringData[lastOperationKey] = sleepInfoData.CurrentOperationType
	}

	if resources.HasResource() && sleepInfoData.IsSleepOperation() {
		data, err := resources.GetOriginalInfoToSave()
		if err != nil {
			logger.Error(err, "failed to get original resource info to save")
			return err
		}
		mergedData := data
		newCount := countRestorePatchesFromBytes(data)
		prevCount := countRestorePatchesFromMap(sleepInfoData.OriginalGenericResourceInfo)
		if newCount < prevCount {
			logger.Info("restore patches reduced after sleep, preserving previous restore", "newCount", newCount, "prevCount", prevCount)
			if err := r.setRestoreIncomplete(ctx, sleepInfo, true); err != nil {
				logger.Error(err, "failed to set restore incomplete annotation")
			}
		} else if prevCount > 0 {
			if err := r.setRestoreIncomplete(ctx, sleepInfo, false); err != nil {
				logger.Error(err, "failed to clear restore incomplete annotation")
			}
		}
		if len(sleepInfoData.OriginalGenericResourceInfo) > 0 {
			// Preserve previous restore patches for resources not captured in this sleep.
			var current map[string]jsonpatch.RestorePatches
			if len(data) > 0 {
				if err := json.Unmarshal(data, &current); err != nil {
					logger.Error(err, "failed to unmarshal restore patches, falling back to previous data")
					current = nil
				}
			}
			if current == nil {
				current = map[string]jsonpatch.RestorePatches{}
			}
			mergedCount := 0
			for kind, patches := range sleepInfoData.OriginalGenericResourceInfo {
				if _, ok := current[kind]; !ok {
					current[kind] = patches
					mergedCount += len(patches)
					continue
				}
				for name, patch := range patches {
					if _, ok := current[kind][name]; !ok {
						current[kind][name] = patch
						mergedCount++
					}
				}
			}
			if mergedCount > 0 {
				logger.Info("preserved restore patches from previous secret", "count", mergedCount)
			}
			if serialized, err := json.Marshal(current); err == nil {
				mergedData = serialized
			} else {
				logger.Error(err, "failed to serialize merged restore patches, using current data")
			}
		}
		newSecret.Data = map[string][]byte{
			originalJSONPatchDataKey: mergedData,
		}
	} else if secret != nil && secret.Data != nil {
		// Preserve restore info on non-sleep operations (e.g. wake/manual).
		if data, ok := secret.Data[originalJSONPatchDataKey]; ok && len(data) > 0 {
			newSecret.Data = map[string][]byte{
				originalJSONPatchDataKey: data,
			}
		}
	}

	if secret == nil {
		if err := r.Create(ctx, newSecret); err != nil {
			return err
		}
		logger.Info("secret created")
	} else {
		if err := r.Update(ctx, newSecret); err != nil {
			return err
		}
		logger.Info("secret updated")
	}

	if data, ok := newSecret.Data[originalJSONPatchDataKey]; ok && len(data) > 0 {
		if err := r.upsertRestoreSecret(ctx, namespace, sleepInfo, data); err != nil {
			logger.Error(err, "failed to upsert emergency restore secret")
		}
	}
	return nil
}

func (r *SleepInfoReconciler) upsertRestoreSecret(ctx context.Context, namespace string, sleepInfo *kubegreenv1alpha1.SleepInfo, data []byte) error {
	secretName := getRestoreSecretName(sleepInfo.Name)
	restoreSecret := &v1.Secret{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      secretName,
	}, restoreSecret)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return err
	}

	newSecret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":             r.ManagerName,
				"kube-green.stratio.com/emergency-restore": "true",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: kubegreenv1alpha1.GroupVersion.String(),
					Kind:       "SleepInfo",
					Name:       sleepInfo.Name,
					UID:        sleepInfo.UID,
				},
			},
		},
		Data: map[string][]byte{
			originalJSONPatchDataKey: data,
		},
		StringData: map[string]string{
			"saved-at": time.Now().Format(time.RFC3339),
		},
	}

	if err != nil && client.IgnoreNotFound(err) == nil {
		return r.Create(ctx, newSecret)
	}
	if restoreSecret.Data != nil {
		if existing := restoreSecret.Data[originalJSONPatchDataKey]; len(existing) > 0 {
			existingCount := countRestorePatchesFromBytes(existing)
			newCount := countRestorePatchesFromBytes(data)
			if newCount < existingCount {
				// Keep the most complete emergency restore data.
				newSecret.Data[originalJSONPatchDataKey] = existing
			}
		}
	}
	newSecret.ResourceVersion = restoreSecret.ResourceVersion
	return r.Update(ctx, newSecret)
}

func (r *SleepInfoReconciler) getEmergencyRestorePatches(ctx context.Context, sleepInfo *kubegreenv1alpha1.SleepInfo, namespace string) (map[string]jsonpatch.RestorePatches, error) {
	secretName := getRestoreSecretName(sleepInfo.Name)
	secret := &v1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, secret); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	if secret == nil || secret.Data == nil {
		return nil, nil
	}
	return jsonpatch.GetOriginalInfoToRestore(secret.Data[originalJSONPatchDataKey])
}

func countRestorePatchesFromBytes(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	parsed := map[string]jsonpatch.RestorePatches{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return 0
	}
	return countRestorePatchesFromMap(parsed)
}

func countRestorePatchesFromMap(data map[string]jsonpatch.RestorePatches) int {
	total := 0
	for _, patches := range data {
		total += len(patches)
	}
	return total
}

func (r *SleepInfoReconciler) setRestoreIncomplete(ctx context.Context, sleepInfo *kubegreenv1alpha1.SleepInfo, incomplete bool) error {
	key := client.ObjectKeyFromObject(sleepInfo)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &kubegreenv1alpha1.SleepInfo{}
		if err := r.Get(ctx, key, latest); err != nil {
			return err
		}
		if latest.Annotations == nil {
			latest.Annotations = map[string]string{}
		}
		const (
			restoreIncompleteKey = "kube-green.stratio.com/restore-incomplete"
			restoreIncompleteAt  = "kube-green.stratio.com/restore-incomplete-at"
		)
		if incomplete {
			latest.Annotations[restoreIncompleteKey] = "true"
			latest.Annotations[restoreIncompleteAt] = time.Now().Format(time.RFC3339)
		} else {
			delete(latest.Annotations, restoreIncompleteKey)
			delete(latest.Annotations, restoreIncompleteAt)
		}
		return r.Update(ctx, latest)
	})
}
