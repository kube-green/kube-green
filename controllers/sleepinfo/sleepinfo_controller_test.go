package controllers

import (
	"context"
	"fmt"
	"time"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("SleepInfo Controller", func() {
	const (
		sleepInfoName = "sleep-name"
		mockNow       = "2021-03-23T20:05:20.555Z"
	)

	var (
		sleepInfoReconciler SleepInfoReconciler
		testLogger          = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	)

	BeforeEach(func() {
		sleepInfoReconciler = SleepInfoReconciler{
			Clock: mockClock{
				now: mockNow,
			},
			Client: k8sClient,
			Log:    testLogger,
		}
	})

	ctx := context.Background()

	namespace := "zero-deployments"
	It("reconcile - zero deployments", func() {
		createdSleepInfo := createSleepInfo(ctx, sleepInfoName, namespace, false)

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      createdSleepInfo.Name,
				Namespace: namespace,
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("is requeued correctly", func() {
			Expect(result).Should(Equal(ctrl.Result{
				// 39445 is the difference between mocked now and next minute
				// (the next scheduled time), in milliseconds
				RequeueAfter: 39445 * time.Millisecond,
			}))
		})

		By("when reconciled correctly - SLEEP")
		sleepScheduleTime := "2021-03-23T20:05:59.000Z"
		sleepInfoReconciler = SleepInfoReconciler{
			Clock: mockClock{
				now: sleepScheduleTime,
			},
			Client: k8sClient,
			Log:    testLogger,
		}
		result, err = sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("not exists deployments", func() {
			deployments, err := listDeployments(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(deployments)).To(Equal(0))
		})

		By("without deployments, secret is not written", func() {
			secret, err := sleepInfoReconciler.getSecret(ctx, getSecretName(sleepInfoName), namespace)
			Expect(err).To(BeNil())
			Expect(secret).NotTo(BeNil())
			Expect(secret.Data).To(Equal(map[string][]byte{
				lastScheduleKey: []byte(getTime(sleepScheduleTime).Format(time.RFC3339)),
			}))
		})

		By("sleepinfo status updated correctly", func() {
			sleepInfo, err := sleepInfoReconciler.getSleepInfo(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(sleepInfo.Status).To(Equal(kubegreenv1alpha1.SleepInfoStatus{
				LastScheduleTime: metav1.NewTime(getTime(sleepScheduleTime).Local()),
			}))
		})

		By("is requeued correctly to next SLEEP", func() {
			Expect(result).Should(Equal(ctrl.Result{
				// 121000 is the difference between mocked now and next uneven minute
				// (the next scheduled time for wake up), in milliseconds
				RequeueAfter: 121000 * time.Millisecond,
			}))
		})

		By("is reconciled correctly - WAKE_UP")
		sleepInfoReconciler = SleepInfoReconciler{
			Clock: mockClock{
				now: "2021-03-23T20:07:00.000Z",
			},
			Client: k8sClient,
			Log:    testLogger,
		}
		result, err = sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("no deployment in namespace", func() {
			deployments, err := listDeployments(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(deployments)).To(Equal(0))
		})

		By("sleepinfo status updated correctly", func() {
			sleepInfo, err := sleepInfoReconciler.getSleepInfo(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(sleepInfo.Status).To(Equal(kubegreenv1alpha1.SleepInfoStatus{
				LastScheduleTime: metav1.NewTime(getTime(sleepScheduleTime).Local()),
			}))
		})

		By("without deployments, secret is not written", func() {
			secret, err := sleepInfoReconciler.getSecret(ctx, getSecretName(sleepInfoName), namespace)
			Expect(err).To(BeNil())
			Expect(secret).NotTo(BeNil())
			Expect(secret.Data).To(Equal(map[string][]byte{
				lastScheduleKey: []byte(getTime(sleepScheduleTime).Format(time.RFC3339)),
			}))
		})
	})

	It("not existent resource return without error", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "not-exists",
				Namespace: namespace,
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).Should(Equal(ctrl.Result{}))
	})

	It("not valid sleep schedule", func() {
		name := "not-valid-schedule"
		By("create SleepInfo")
		sleepInfo := &kubegreenv1alpha1.SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "kube-green.com/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				Weekdays: "",
			},
		}
		Expect(k8sClient.Create(ctx, sleepInfo)).Should(Succeed())

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err.Error()).Should(Equal("empty weekday from sleep info configuration"))
		Expect(result).Should(Equal(ctrl.Result{}))
	})

	It("reconcile - not existent namespace", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sleepInfoName,
				Namespace: "not-exists",
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("is requeued correctly")
		Expect(result).Should(Equal(ctrl.Result{}))
	})

	It("reconcile - with deployments", func() {
		namespace := "multiple-deployments"
		req, originalDeployments := setupNamespaceWithDeployments(ctx, sleepInfoName, namespace, sleepInfoReconciler)

		assertContextInfo := AssertOperation{
			testLogger:          testLogger,
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalDeployments,
		}

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(61))
		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:07:00.100Z").withRequeue(59.9))
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:08:00.000Z").withRequeue(60))
	})

	It("reconcile - deploy between sleep and wake up", func() {
		namespace := "deploy-between-sleep-and-wake-up"
		req, originalDeployments := setupNamespaceWithDeployments(ctx, sleepInfoName, namespace, sleepInfoReconciler)

		assertContextInfo := AssertOperation{
			testLogger:          testLogger,
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalDeployments,
		}

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(61))

		By("re deploy", func() {
			_, err := upsertDeployments(ctx, namespace, true)
			Expect(err).NotTo(HaveOccurred())

			By("check replicas")
			deployments, err := listDeployments(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())
			for idx, deployment := range deployments {
				Expect(deployment.Spec.Replicas).To(Equal(originalDeployments[idx].Spec.Replicas))

				annotations := deployment.GetAnnotations()
				_, ok := annotations[replicasBeforeSleepAnnotation]
				Expect(ok).To(BeFalse())
			}
		})

		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:07:00.000Z").withRequeue(60))
	})

	It("reconcile - change single deployment replicas between sleep and wake up", func() {
		namespace := "change-replicas-between-sleep-and-wake-up"
		req, originalDeployments := setupNamespaceWithDeployments(ctx, sleepInfoName, namespace, sleepInfoReconciler)

		assertContextInfo := AssertOperation{
			testLogger:          testLogger,
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalDeployments,
		}
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(61))

		By("re deploy a single deployment", func() {
			deployments, err := listDeployments(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())

			deploymentToUpdate := deployments[0].DeepCopy()
			*deploymentToUpdate.Spec.Replicas = 0
			patch := client.RawPatch(types.ApplyPatchType, []byte(deploymentToUpdate.String()))
			k8sClient.Patch(ctx, deploymentToUpdate, patch)

			updatedDeployment := appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: deploymentToUpdate.Name, Namespace: namespace}, &updatedDeployment)).To(Succeed())
			Expect(*updatedDeployment.Spec.Replicas).To(Equal(int32(0)))
		})

		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:07:00.000Z").withRequeue(60))
	})

	It("reconcile - twice consecutive sleep operation", func() {
		namespace := "twice-sleep"
		req, originalDeployments := setupNamespaceWithDeployments(ctx, sleepInfoName, namespace, sleepInfoReconciler)

		assertContextInfo := AssertOperation{
			testLogger:          testLogger,
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalDeployments,
		}
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(61))
		assertCorrectSleepOperation(
			assertContextInfo.
				withSchedule("2021-03-23T20:08:00.000Z").
				withExpectedSchedule("2021-03-23T20:05:59Z").
				withRequeue(60),
		)
	})

	It("reconcile - only sleep, wake up set to nil", func() {
		namespace := "without-wake-up"
		req, originalDeployments := setupNamespaceWithDeployments(ctx, sleepInfoName, namespace, sleepInfoReconciler, SetupOptions{
			UnsetWakeUpTime: true,
		})

		assertContextInfo := AssertOperation{
			testLogger:          testLogger,
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalDeployments,
		}
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(121))
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:08:00.000Z").withRequeue(120))

		By("re deploy", func() {
			_, err := upsertDeployments(ctx, namespace, true)
			Expect(err).NotTo(HaveOccurred())

			By("check replicas")
			deployments, err := listDeployments(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())
			for idx, deployment := range deployments {
				Expect(deployment.Spec.Replicas).To(Equal(originalDeployments[idx].Spec.Replicas))

				annotations := deployment.GetAnnotations()
				_, ok := annotations[replicasBeforeSleepAnnotation]
				Expect(ok).To(BeFalse())
			}
		})

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:10:00.000Z").withRequeue(120))
	})

	It("reconcile - sleep info not present in namespace", func() {
		namespace := "no-sleepinfo"
		err := createNamespace(ctx, namespace)
		Expect(err).ShouldNot(HaveOccurred())

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sleepInfoName,
				Namespace: namespace,
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).Should(Equal(ctrl.Result{}))
	})

	It("reconcile - sleepinfo deployed when should be triggered", func() {
		namespace := "immediately-triggered"
		createSleepInfo(ctx, sleepInfoName, namespace, false)
		originalDeployments, err := upsertDeployments(ctx, namespace, false)
		Expect(err).NotTo(HaveOccurred())

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sleepInfoName,
				Namespace: namespace,
			},
		}
		sleepInfoReconciler = SleepInfoReconciler{
			Clock: mockClock{
				now: "2021-03-23T20:06:00.000Z",
			},
			Client: k8sClient,
			Log:    testLogger,
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("is requeued after correct duration", func() {
			Expect(result).Should(Equal(ctrl.Result{
				// 60000 is the difference between mocked now and next minute
				// (the next scheduled time), in milliseconds
				RequeueAfter: 60000 * time.Millisecond,
			}))
		})

		By("replicas are set to 0 to all deployments and annotations are set correctly to deployments", func() {
			deployments, err := listDeployments(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())
			assertAllReplicasSetToZero(deployments, originalDeployments)
		})
	})
})

func createNamespace(ctx context.Context, name string) error {
	namespace := &core.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return k8sClient.Create(ctx, namespace)
}

func upsertDeployments(ctx context.Context, namespace string, updateIfAlreadyCreated bool) ([]appsv1.Deployment, error) {
	var threeReplicas int32 = 3
	var oneReplica int32 = 1
	var zeroReplicas int32 = 0
	deployments := []appsv1.Deployment{
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-1",
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &threeReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "service-1",
					},
				},
				Template: core.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "service-1",
						},
					},
					Spec: core.PodSpec{
						Containers: []core.Container{
							{
								Name:  "c1",
								Image: "davidebianchi/echo-service",
							},
						},
					},
				},
			},
		},
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-2",
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &oneReplica,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "service-2",
					},
				},
				Template: core.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "service-2",
						},
					},
					Spec: core.PodSpec{
						Containers: []core.Container{
							{
								Name:  "c2",
								Image: "davidebianchi/echo-service",
							},
						},
					},
				},
			},
		},
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zero-replicas",
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &zeroReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "zero-replicas",
					},
				},
				Template: core.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "zero-replicas",
						},
					},
					Spec: core.PodSpec{
						Containers: []core.Container{
							{
								Name:  "zero",
								Image: "davidebianchi/echo-service",
							},
						},
					},
				},
			},
		},
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zero-replicas-annotation",
				Namespace: namespace,
				Annotations: map[string]string{
					lastScheduleKey: "2021-03-23T00:00:00.000Z",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &zeroReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "zero-replicas-annotation",
					},
				},
				Template: core.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "zero-replicas-annotation",
						},
					},
					Spec: core.PodSpec{
						Containers: []core.Container{
							{
								Name:  "zero",
								Image: "davidebianchi/echo-service",
							},
						},
					},
				},
			},
		},
	}
	for _, deployment := range deployments {
		var deploymentAlreadyExists bool
		if updateIfAlreadyCreated {
			d := appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      deployment.Name,
				Namespace: namespace,
			}, &d)
			if err == nil {
				deploymentAlreadyExists = true
			}
		}
		if deploymentAlreadyExists {
			if err := k8sClient.Update(ctx, &deployment); err != nil {
				return nil, fmt.Errorf("error %s updating deployment 1", err)
			}
		} else {
			if err := k8sClient.Create(ctx, &deployment); err != nil {
				return nil, fmt.Errorf("error %s creating deployment 1", err)
			}
		}
	}
	return deployments, nil
}

func listDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	deployments := appsv1.DeploymentList{}
	err := k8sClient.List(ctx, &deployments, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return nil, err
	}
	return deployments.Items, nil
}

type mockClock struct {
	now string
}

func (m mockClock) Now() time.Time {
	parsedTime, err := time.Parse(time.RFC3339, m.now)
	Expect(err).NotTo(HaveOccurred())
	return parsedTime
}

func assertAllReplicasSetToZero(actualDeployments []appsv1.Deployment, originalDeployments []appsv1.Deployment) {
	allReplicas := []int32{}
	for idx, deployment := range actualDeployments {
		allReplicas = append(allReplicas, *deployment.Spec.Replicas)

		annotations := deployment.GetAnnotations()
		originalReplicas := *originalDeployments[idx].Spec.Replicas
		if originalReplicas == 0 {
			Expect(annotations[replicasBeforeSleepAnnotation]).To(BeEmpty())
		} else {
			Expect(annotations[replicasBeforeSleepAnnotation]).To(Equal(fmt.Sprintf("%d", *originalDeployments[idx].Spec.Replicas)))
		}
	}
	for _, replicas := range allReplicas {
		Expect(replicas).To(Equal(int32(0)))
	}
}

func getTime(mockNowRaw string) time.Time {
	now, err := time.Parse(time.RFC3339, mockNowRaw)
	Expect(err).ShouldNot(HaveOccurred())
	return now
}

type AssertOperation struct {
	testLogger          logr.Logger
	ctx                 context.Context
	req                 ctrl.Request
	namespace           string
	sleepInfoName       string
	originalDeployments []appsv1.Deployment
	scheduleTime        string
	// optional - default is equal to scheduleTime
	expectedScheduleTime string
	expectedNextRequeue  time.Duration
}

func (a AssertOperation) withSchedule(schedule string) AssertOperation {
	a.scheduleTime = schedule
	if a.expectedScheduleTime == "" {
		a.expectedScheduleTime = schedule
	}
	return a
}

func (a AssertOperation) withExpectedSchedule(schedule string) AssertOperation {
	a.expectedScheduleTime = schedule
	return a
}

func (a AssertOperation) withRequeue(requeue float64) AssertOperation {
	a.expectedNextRequeue = time.Duration(requeue*1000) * time.Millisecond
	return a
}

func assertCorrectSleepOperation(assert AssertOperation) {
	By("is requeued correctly - SLEEP")
	sleepInfoReconciler := SleepInfoReconciler{
		Clock: mockClock{
			now: assert.scheduleTime,
		},
		Client: k8sClient,
		Log:    assert.testLogger,
	}
	result, err := sleepInfoReconciler.Reconcile(assert.ctx, assert.req)
	Expect(err).NotTo(HaveOccurred())

	By("replicas are set to 0 to all deployments and annotations are set correctly to deployments", func() {
		deployments, err := listDeployments(assert.ctx, assert.namespace)
		Expect(err).NotTo(HaveOccurred())
		assertAllReplicasSetToZero(deployments, assert.originalDeployments)
	})

	By("secret is correctly set", func() {
		secret, err := sleepInfoReconciler.getSecret(assert.ctx, getSecretName(assert.sleepInfoName), assert.namespace)
		Expect(err).NotTo(HaveOccurred())
		secretData := secret.Data
		Expect(secretData).To(Equal(map[string][]byte{
			lastScheduleKey:  []byte(getTime(assert.expectedScheduleTime).Truncate(time.Second).Format(time.RFC3339)),
			lastOperationKey: []byte(sleepOperation),
		}))
	})

	By("sleepinfo status updated correctly", func() {
		sleepInfo, err := sleepInfoReconciler.getSleepInfo(assert.ctx, assert.req)
		Expect(err).NotTo(HaveOccurred())
		Expect(sleepInfo.Status).To(Equal(kubegreenv1alpha1.SleepInfoStatus{
			LastScheduleTime: metav1.NewTime(getTime(assert.expectedScheduleTime).Local()),
			OperationType:    sleepOperation,
		}))
	})

	By("is requeued after correct duration to wake up", func() {
		Expect(result).Should(Equal(ctrl.Result{
			RequeueAfter: assert.expectedNextRequeue,
		}))
	})
}

func assertCorrectWakeUpOperation(assert AssertOperation) {
	By("requeued correctly - WAKE_UP")
	sleepInfoReconciler := SleepInfoReconciler{
		Clock: mockClock{
			now: assert.scheduleTime,
		},
		Client: k8sClient,
		Log:    assert.testLogger,
	}
	result, err := sleepInfoReconciler.Reconcile(assert.ctx, assert.req)
	Expect(err).NotTo(HaveOccurred())

	By("deployment replicas correctly waked up and annotation deleted", func() {
		deployments, err := listDeployments(assert.ctx, assert.namespace)
		Expect(err).NotTo(HaveOccurred())
		for idx, deployment := range deployments {
			Expect(deployment.Spec.Replicas).To(Equal(assert.originalDeployments[idx].Spec.Replicas))

			annotations := deployment.GetAnnotations()
			_, ok := annotations[replicasBeforeSleepAnnotation]
			Expect(ok).To(BeFalse())
		}
	})

	By("secret is correctly set", func() {
		secret, err := sleepInfoReconciler.getSecret(assert.ctx, getSecretName(assert.sleepInfoName), assert.namespace)
		Expect(err).NotTo(HaveOccurred())
		secretData := secret.Data
		Expect(secretData).To(Equal(map[string][]byte{
			lastScheduleKey:  []byte(getTime(assert.expectedScheduleTime).Truncate(time.Second).Format(time.RFC3339)),
			lastOperationKey: []byte(wakeUpOperation),
		}))
	})

	By("status correctly updated", func() {
		sleepInfo, err := sleepInfoReconciler.getSleepInfo(assert.ctx, assert.req)
		Expect(err).NotTo(HaveOccurred())
		Expect(sleepInfo.Status).To(Equal(kubegreenv1alpha1.SleepInfoStatus{
			LastScheduleTime: metav1.NewTime(getTime(assert.expectedScheduleTime).Round(time.Second).Local()),
			OperationType:    wakeUpOperation,
		}))
	})

	By("is requeued after correct duration to sleep", func() {
		Expect(result).Should(Equal(ctrl.Result{
			RequeueAfter: assert.expectedNextRequeue,
		}))
	})
}

type SetupOptions struct {
	UnsetWakeUpTime bool
}

func setupNamespaceWithDeployments(ctx context.Context, sleepInfoName, namespace string, reconciler SleepInfoReconciler, opts ...SetupOptions) (ctrl.Request, []appsv1.Deployment) {
	var unsetWakeUpTime bool
	if len(opts) != 0 {
		unsetWakeUpTime = opts[0].UnsetWakeUpTime
	}
	createSleepInfo(ctx, sleepInfoName, namespace, unsetWakeUpTime)

	By("create deployments")
	originalDeployments, err := upsertDeployments(ctx, namespace, false)
	Expect(err).NotTo(HaveOccurred())

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      sleepInfoName,
			Namespace: namespace,
		},
	}
	result, err := reconciler.Reconcile(ctx, req)
	Expect(err).NotTo(HaveOccurred())

	By("is requeued after correct duration", func() {
		Expect(result).Should(Equal(ctrl.Result{
			// 39445 is the difference between mocked now and next minute
			// (the next scheduled time), in milliseconds
			RequeueAfter: 39445 * time.Millisecond,
		}))
	})

	By("replicas not changed", func() {
		deploymentsNotChanged, err := listDeployments(ctx, namespace)
		Expect(err).NotTo(HaveOccurred())
		for i, deployment := range deploymentsNotChanged {
			Expect(*deployment.Spec.Replicas).To(Equal(*originalDeployments[i].Spec.Replicas))
		}
	})

	return req, originalDeployments
}

func createSleepInfo(ctx context.Context, sleepInfoName, namespace string, unsetWakeUpTime bool) kubegreenv1alpha1.SleepInfo {
	var (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Expect(createNamespace(ctx, namespace)).NotTo(HaveOccurred())
	sleepInfo := &kubegreenv1alpha1.SleepInfo{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SleepInfo",
			APIVersion: "kube-green.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sleepInfoName,
			Namespace: namespace,
		},
		Spec: kubegreenv1alpha1.SleepInfoSpec{
			Weekdays:   "*",
			SleepTime:  "*:*/2",    // every even minute
			WakeUpTime: "*:1-59/2", // every uneven minute
		},
	}
	if unsetWakeUpTime {
		sleepInfo.Spec.WakeUpTime = ""
	}

	Expect(k8sClient.Create(ctx, sleepInfo)).Should(Succeed())

	sleepInfoLookupKey := types.NamespacedName{Name: sleepInfoName, Namespace: namespace}
	createdSleepInfo := &kubegreenv1alpha1.SleepInfo{}

	// We'll need to retry getting this newly created SleepInfo, given that creation may not immediately happen.
	Eventually(func() bool {
		err := k8sClient.Get(ctx, sleepInfoLookupKey, createdSleepInfo)
		Expect(err).NotTo(HaveOccurred())
		return true
	}, timeout, interval).Should(BeTrue())

	return *createdSleepInfo
}
