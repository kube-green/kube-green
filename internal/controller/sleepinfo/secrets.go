package sleepinfo

import (
	"context"
	"fmt"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SleepInfoReconciler) getSecret(ctx context.Context, secretName, namespaceName string) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{
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
		newSecret.Data = map[string][]byte{
			originalJSONPatchDataKey: data,
		}
	}

	if secret == nil {
		if err := r.Client.Create(ctx, newSecret); err != nil {
			return err
		}
		logger.Info("secret created")
	} else {
		if err := r.Client.Update(ctx, newSecret); err != nil {
			return err
		}
		logger.Info("secret updated")
	}
	return nil
}
