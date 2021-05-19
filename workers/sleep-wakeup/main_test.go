package sleepwakeup

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var _ = Describe("sleep", func() {
	ctx := context.Background()

	It("0 deployments", func() {
		namespace := "zero-deployments"
		client := newIntegrationClient(sleepOperation, namespace)

		createNamespace(ctx, client, namespace)

		err := handleOperations(ctx, client)
		Expect(err).ToNot(HaveOccurred())

		By("secret correctly created")
		data, err := client.getSleepInfoData(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(Equal(SleepInfoData{
			OriginalDeploymentsReplicas: []OriginalDeploymentReplicas{},
		}))
	})

	It("deployment with 0 replicas", func() {
		namespace := "deployment-with-zero-replicas"
		client := newIntegrationClient(sleepOperation, namespace)

		createNamespace(ctx, client, namespace)
		createDeployment(ctx, client, namespace, getDeploymentObj("service-1", 0))

		err := handleOperations(ctx, client)
		Expect(err).ToNot(HaveOccurred())

		By("secret correctly created")
		data, err := client.getSleepInfoData(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(Equal(SleepInfoData{
			OriginalDeploymentsReplicas: []OriginalDeploymentReplicas{},
		}))
	})

	It("multiple deployments", func() {
		namespace := "sleep-deployments-multiple"
		client := newIntegrationClient(sleepOperation, namespace)

		createNamespace(ctx, client, namespace)
		createDeployment(ctx, client, namespace, getDeploymentObj("service-1", 1))
		createDeployment(ctx, client, namespace, getDeploymentObj("service-2", 5))

		err := handleOperations(ctx, client)
		Expect(err).ToNot(HaveOccurred())

		By("secret correctly created")
		data, err := client.getSleepInfoData(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(Equal(SleepInfoData{
			OriginalDeploymentsReplicas: []OriginalDeploymentReplicas{
				{
					Name:     "service-1",
					Replicas: 1,
				},
				{
					Name:     "service-2",
					Replicas: 5,
				},
			},
		}))
		assertCorrectDeployments(ctx, client, []OriginalDeploymentReplicas{
			{
				Name:     "service-1",
				Replicas: 0,
			},
			{
				Name:     "service-2",
				Replicas: 0,
			},
		})
	})

	It("fails to get list of deployments", func() {
		namespace := "fails"
		client := newIntegrationClient(sleepOperation, namespace)

		ctx, cancelFn := context.WithCancel(ctx)
		cancelFn()
		err := handleOperations(ctx, client)
		Expect(err.Error()).To(Equal("fails to list deployment for namespace fails: context canceled"))
	})

	It("already existent secret", func() {
		namespace := "existent-secret"
		client := newIntegrationClient(sleepOperation, namespace)

		createNamespace(ctx, client, namespace)
		createDeployment(ctx, client, namespace, getDeploymentObj("service-1", 1))
		createSecret(ctx, client, namespace, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: client.secretName,
			},
		})

		err := handleOperations(ctx, client)
		Expect(err).ToNot(HaveOccurred())

		By("secret correctly created")
		data, err := client.getSleepInfoData(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(Equal(SleepInfoData{
			OriginalDeploymentsReplicas: []OriginalDeploymentReplicas{
				{
					Name:     "service-1",
					Replicas: 1,
				},
			},
		}))
	})

	It("two consecutive sleep operation", func() {
		namespace := "sleep-consecutive-sleep-operation"
		client := newIntegrationClient(sleepOperation, namespace)

		createNamespace(ctx, client, namespace)
		createDeployment(ctx, client, namespace, getDeploymentObj("service-1", 1))
		createDeployment(ctx, client, namespace, getDeploymentObj("service-2", 5))

		By("first sleep", func() {
			err := handleOperations(ctx, client)
			Expect(err).ToNot(HaveOccurred())
			By("secret correctly created")
			data, err := client.getSleepInfoData(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(Equal(SleepInfoData{
				OriginalDeploymentsReplicas: []OriginalDeploymentReplicas{
					{
						Name:     "service-1",
						Replicas: 1,
					},
					{
						Name:     "service-2",
						Replicas: 5,
					},
				},
			}))
			assertCorrectDeployments(ctx, client, []OriginalDeploymentReplicas{
				{
					Name:     "service-1",
					Replicas: 0,
				},
				{
					Name:     "service-2",
					Replicas: 0,
				},
			})
		})

		By("second sleep", func() {
			err := handleOperations(ctx, client)
			Expect(err).ToNot(HaveOccurred())
			By("secret correctly created")
			data, err := client.getSleepInfoData(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(Equal(SleepInfoData{
				OriginalDeploymentsReplicas: []OriginalDeploymentReplicas{
					{
						Name:     "service-1",
						Replicas: 1,
					},
					{
						Name:     "service-2",
						Replicas: 5,
					},
				},
			}))
			assertCorrectDeployments(ctx, client, []OriginalDeploymentReplicas{
				{
					Name:     "service-1",
					Replicas: 0,
				},
				{
					Name:     "service-2",
					Replicas: 0,
				},
			})
		})

		By("third sleep", func() {
			err := handleOperations(ctx, client)
			Expect(err).ToNot(HaveOccurred())
			By("secret correctly created")
			data, err := client.getSleepInfoData(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(Equal(SleepInfoData{
				OriginalDeploymentsReplicas: []OriginalDeploymentReplicas{
					{
						Name:     "service-1",
						Replicas: 1,
					},
					{
						Name:     "service-2",
						Replicas: 5,
					},
				},
			}))
			assertCorrectDeployments(ctx, client, []OriginalDeploymentReplicas{
				{
					Name:     "service-1",
					Replicas: 0,
				},
				{
					Name:     "service-2",
					Replicas: 0,
				},
			})
		})
	})
})

var _ = Describe("wake up", func() {
	ctx := context.Background()

	It("fails if secret not found", func() {
		namespace := "wakeup-no-secret"
		client := newIntegrationClient(wakeUpOperation, namespace)

		createNamespace(ctx, client, namespace)

		err := handleOperations(ctx, client)
		Expect(err.Error()).To(Equal(fmt.Sprintf("secrets \"%s\" not found", client.secretName)))
	})

	It("fails if secret is empty", func() {
		namespace := "wakeup-empty-secret"
		client := newIntegrationClient(wakeUpOperation, namespace)

		createNamespace(ctx, client, namespace)
		createSecret(ctx, client, namespace, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: client.secretName,
			},
		})

		err := handleOperations(ctx, client)
		Expect(err.Error()).To(Equal("invalid empty secret"))
	})

	It("fails if secret field is empty", func() {
		namespace := "wakeup-secret-empty-original-replicas"
		client := newIntegrationClient(wakeUpOperation, namespace)

		createNamespace(ctx, client, namespace)
		createSecret(ctx, client, namespace, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: client.secretName,
			},
			Data: map[string][]byte{
				replicasBeforeSleepKey: nil,
			},
		})

		err := handleOperations(ctx, client)
		Expect(err.Error()).To(Equal(fmt.Sprintf("invalid empty %s field", replicasBeforeSleepKey)))
	})

	It("fails if secret not contains original replicas", func() {
		namespace := "wakeup-secret-no-original-replicas"
		client := newIntegrationClient(wakeUpOperation, namespace)

		createNamespace(ctx, client, namespace)
		createSecret(ctx, client, namespace, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: client.secretName,
			},
			StringData: map[string]string{
				replicasBeforeSleepKey: "null",
			},
		})

		err := handleOperations(ctx, client)
		Expect(err.Error()).To(Equal(fmt.Sprintf("original deployment replicas not found in secret %s in namespace %s", client.secretName, client.namespaceName)))
	})

	It("correctly update replicas", func() {
		namespace := "wakeup-multi-deployments"
		client := newIntegrationClient(wakeUpOperation, namespace)

		createNamespace(ctx, client, namespace)
		createDeployment(ctx, client, namespace, getDeploymentObj("service-1", 0))
		createDeployment(ctx, client, namespace, getDeploymentObj("service-2", 0))

		o := []OriginalDeploymentReplicas{
			{
				Name:     "service-1",
				Replicas: 1,
			},
			{
				Name:     "service-2",
				Replicas: 5,
			},
		}
		data, err := json.Marshal(o)
		Expect(err).NotTo(HaveOccurred())
		createSecret(ctx, client, namespace, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: client.secretName,
			},
			Data: map[string][]byte{
				replicasBeforeSleepKey: data,
			},
		})

		err = handleOperations(ctx, client)
		Expect(err).NotTo(HaveOccurred())

		deployments, err := client.listDeployments(ctx, namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(deployments.Items).To(HaveLen(2))
		Expect(deployments.Items[0].Name).To(Equal("service-1"))
		Expect(*deployments.Items[0].Spec.Replicas).To(Equal(int32(1)))
		Expect(deployments.Items[1].Name).To(Equal("service-2"))
		Expect(*deployments.Items[1].Spec.Replicas).To(Equal(int32(5)))

		By("secret correctly created")
		sleepInfoData, err := client.getSleepInfoData(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(sleepInfoData).To(Equal(SleepInfoData{
			OriginalDeploymentsReplicas: nil,
		}))
	})

	It("correctly update replicas - deployments with replicas not set to 0", func() {
		namespace := "wakeup-multi-deployments-with-no-set-to-zero"
		client := newIntegrationClient(wakeUpOperation, namespace)

		createNamespace(ctx, client, namespace)
		createDeployment(ctx, client, namespace, getDeploymentObj("service-1", 0))
		createDeployment(ctx, client, namespace, getDeploymentObj("service-2", 2))

		o := []OriginalDeploymentReplicas{
			{
				Name:     "service-1",
				Replicas: 1,
			},
			{
				Name:     "service-2",
				Replicas: 5,
			},
		}
		data, err := json.Marshal(o)
		Expect(err).NotTo(HaveOccurred())
		createSecret(ctx, client, namespace, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: client.secretName,
			},
			Data: map[string][]byte{
				replicasBeforeSleepKey: data,
			},
		})

		err = handleOperations(ctx, client)
		Expect(err).NotTo(HaveOccurred())

		deployments, err := client.listDeployments(ctx, namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(deployments.Items).To(HaveLen(2))
		Expect(deployments.Items[0].Name).To(Equal("service-1"))
		Expect(*deployments.Items[0].Spec.Replicas).To(Equal(int32(1)))
		Expect(deployments.Items[1].Name).To(Equal("service-2"))
		Expect(*deployments.Items[1].Spec.Replicas).To(Equal(int32(2)))

		By("secret correctly created")
		sleepInfoData, err := client.getSleepInfoData(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(sleepInfoData).To(Equal(SleepInfoData{
			OriginalDeploymentsReplicas: nil,
		}))
	})
})

func createNamespace(ctx context.Context, client Client, name string) {
	_, err := client.k8s.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func createDeployment(ctx context.Context, client Client, namespaceName string, deployment *appsv1.Deployment) {
	_, err := client.k8s.AppsV1().Deployments(namespaceName).Create(ctx, deployment, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func createSecret(ctx context.Context, client Client, namespaceName string, secret *v1.Secret) {
	_, err := client.k8s.CoreV1().Secrets(namespaceName).Create(ctx, secret, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func newIntegrationClient(operationType, namespaceName string) Client {
	apiURL := testEnv.ControlPlane.APIURL()
	k8s := kubernetes.NewForConfigOrDie(&rest.Config{
		Host:    apiURL.Host,
		APIPath: apiURL.Path,
	})
	return Client{
		k8s: k8s,

		secretName:    "my-secret",
		namespaceName: namespaceName,
		operationType: operationType,
	}
}

func getDeploymentObj(serviceName string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": serviceName,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": serviceName,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "c1",
							Image: "davidebianchi/echo-service",
						},
					},
				},
			},
		},
	}
}

func assertCorrectDeployments(ctx context.Context, client Client, deploymentReplicas []OriginalDeploymentReplicas) {
	deployments, err := client.listDeployments(ctx, client.namespaceName)
	Expect(err).NotTo(HaveOccurred())
	Expect(deployments.Items).To(HaveLen(2))
	for i, d := range deployments.Items {
		Expect(d.Name).To(Equal(deploymentReplicas[i].Name))
		Expect(*d.Spec.Replicas).To(Equal(deploymentReplicas[i].Replicas))
	}
}
