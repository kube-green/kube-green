package sleepinfo

import (
	"context"
	"fmt"
	"time"

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
	secret *v1.Secret,
	sleepInfoData SleepInfoData,
	resources Resources,
) error {
	logger.Info("update secret")

	var newSecret = &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data:       make(map[string][]byte),
		StringData: make(map[string]string),
	}
	newSecret.StringData[lastScheduleKey] = now.Format(time.RFC3339)
	if resources.hasResources() {
		newSecret.StringData[lastOperationKey] = sleepInfoData.CurrentOperationType
	}

	if resources.hasResources() && sleepInfoData.IsSleepOperation() {
		data, err := resources.getOriginalResourceInfoToSave(sleepInfoData)
		if err != nil {
			logger.Error(err, "failed to get original resource info to save")
			return err
		}
		newSecret.Data = data
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
