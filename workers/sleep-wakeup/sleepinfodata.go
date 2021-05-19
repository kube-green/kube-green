package sleepwakeup

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	replicasBeforeSleepKey = "deployment-replicas"
)

type OriginalDeploymentReplicas struct {
	Name     string `json:"name"`
	Replicas int32  `json:"replicas"`
}

type SleepInfoData struct {
	OriginalDeploymentsReplicas []OriginalDeploymentReplicas `json:"originalDeploymentReplicas"`
}

func (c Client) getSleepInfoData(ctx context.Context) (SleepInfoData, error) {
	secretData := SleepInfoData{}

	secret, err := c.getSecret(ctx, c.secretName, c.namespaceName)
	if err != nil {
		return secretData, err
	}
	if secret == nil || secret.Data == nil {
		return secretData, fmt.Errorf("invalid empty secret")
	}

	data := secret.Data
	originalDeploymentsReplicas := []OriginalDeploymentReplicas{}
	if data[replicasBeforeSleepKey] == nil || len(data[replicasBeforeSleepKey]) == 0 {
		return secretData, fmt.Errorf("invalid empty %s field", replicasBeforeSleepKey)
	}

	if err := json.Unmarshal(data[replicasBeforeSleepKey], &originalDeploymentsReplicas); err != nil {
		return SleepInfoData{}, err
	}
	secretData.OriginalDeploymentsReplicas = originalDeploymentsReplicas

	return secretData, nil
}

func (c Client) upsertSecret(ctx context.Context, originalDeploymentReplicas []OriginalDeploymentReplicas) error {
	secret, err := c.getSecret(ctx, c.secretName, c.namespaceName)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fails to get secret %s: %s", c.secretName, err)
	}

	rawReplicasData, err := json.Marshal(originalDeploymentReplicas)
	if err != nil {
		return err
	}

	if secret.Name != "" {
		secret.Data = map[string][]byte{
			replicasBeforeSleepKey: rawReplicasData,
		}
		_, err := c.k8s.CoreV1().Secrets(c.namespaceName).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("fails to update secret %s in namespace %s: %s", c.secretName, c.namespaceName, err)
		}
		return nil
	}

	var newSecret = &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.secretName,
			Namespace: c.namespaceName,
		},
		Data: map[string][]byte{
			replicasBeforeSleepKey: rawReplicasData,
		},
	}
	_, err = c.k8s.CoreV1().Secrets(c.namespaceName).Create(ctx, newSecret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("fails to create secret %s in namespace %s: %s", c.secretName, c.namespaceName, err)
	}
	return nil
}
