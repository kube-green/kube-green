package sleepwakeup

import (
	"context"
	"fmt"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	sleepOperation  = "SLEEP"
	wakeUpOperation = "WAKE_UP"
)

func main() {
	client := NewClient()

	ctx := context.Background()

	err := handleOperations(ctx, client)
	if err != nil {
		fmt.Printf("error handle operation %s to namespace %s for secret %s: %s", client.operationType, client.namespaceName, client.secretName, err.Error())
		os.Exit(1)
	}
	fmt.Printf("correct execution of operation %s to namespace %s for secret %s", client.operationType, client.namespaceName, client.secretName)
}

func handleOperations(ctx context.Context, client Client) error {
	switch client.operationType {
	case sleepOperation:
		return client.sleepDeployments(ctx)
	case wakeUpOperation:
		return client.wakeUpDeployments(ctx)
	default:
		return fmt.Errorf("invalid operation type: %s", client.operationType)
	}
}

// TODO: add a logger to use instead of fmt
type Client struct {
	k8s kubernetes.Interface

	secretName    string
	namespaceName string
	operationType string
}

func NewClient() Client {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	clientset := kubernetes.NewForConfigOrDie(config)

	namespaceName := os.Getenv("NAMESPACE_NAME")
	if namespaceName == "" {
		panic("namespace name not set")
	}

	secretName := os.Getenv("SECRET_NAME")
	if secretName == "" {
		panic("secret name not set")
	}

	operationType := os.Getenv("OPERATION_TYPE")
	if secretName == "" {
		panic("operation type not set")
	}

	return Client{
		k8s: clientset,

		secretName:    secretName,
		namespaceName: namespaceName,
		operationType: operationType,
	}
}

func (c Client) getSecret(ctx context.Context, secretName, namespaceName string) (*v1.Secret, error) {
	return c.k8s.CoreV1().Secrets(namespaceName).Get(ctx, secretName, metav1.GetOptions{})
}

func (c Client) listDeployments(ctx context.Context, namespaceName string) (*appsv1.DeploymentList, error) {
	return c.k8s.AppsV1().Deployments(namespaceName).List(ctx, metav1.ListOptions{
		Limit: 500,
	})
}

func (c Client) updateReplicas(ctx context.Context, deploymentName string, replicas int32) error {
	_, err := c.k8s.AppsV1().Deployments(c.namespaceName).UpdateScale(ctx, deploymentName, &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: c.namespaceName,
		},
		Spec: autoscalingv1.ScaleSpec{
			Replicas: replicas,
		},
	}, metav1.UpdateOptions{})

	return err
}

func (c Client) getReplicas(ctx context.Context, deploymentName string) (*autoscalingv1.Scale, error) {
	return c.k8s.AppsV1().Deployments(c.namespaceName).GetScale(ctx, deploymentName, metav1.GetOptions{})
}
