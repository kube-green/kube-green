package sleepinfo

import (
	"context"
	"encoding/json"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/internal/testutil"
	"github.com/kube-green/kube-green/controllers/sleepinfo/cronjobs"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("SleepInfo Controller", func() {
	const (
		sleepInfoName = "sleep-name"
		mockNow       = "2021-03-23T20:01:20.555Z"
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
		}
	})

	ctx := context.Background()
	ctx = log.IntoContext(ctx, testLogger)

	namespace := "zero-deployments"
	It("reconcile - zero deployments", func() {
		createdSleepInfo := createSleepInfo(ctx, sleepInfoName, namespace, setupOptions{})

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
				RequeueAfter: sleepRequeue(mockNow),
			}))
		})

		By("when reconciled correctly - SLEEP")
		sleepScheduleTime := "2021-03-23T20:05:59.000Z"
		sleepInfoReconciler = SleepInfoReconciler{
			Clock: mockClock{
				now: sleepScheduleTime,
			},
			Client: k8sClient,
		}
		result, err = sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("not exists deployments", func() {
			deployments := listDeployments(ctx, namespace)
			Expect(len(deployments)).To(Equal(0))
		})

		By("without deployments, secret written with only last schedule", func() {
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
				// sleep is: now + next wake up + next sleep
				RequeueAfter: wakeUpRequeue(sleepScheduleTime) + sleepRequeue("2021-03-23T20:20:00.000Z"),
			}))
		})

		By("is reconciled correctly - WAKE_UP")
		sleepInfoReconciler = SleepInfoReconciler{
			Clock: mockClock{
				now: "2021-03-23T20:07:00.000Z",
			},
			Client: k8sClient,
		}
		result, err = sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("no deployment in namespace", func() {
			deployments := listDeployments(ctx, namespace)
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
		namespace := "not-valid-sleep-schedule"
		err := testutil.CreateNamespace(ctx, k8sClient, namespace)
		Expect(err).NotTo(HaveOccurred())

		By("create SleepInfo")
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
				Weekdays:  "1",
				SleepTime: "",
			},
		}
		Expect(k8sClient.Create(ctx, sleepInfo)).Should(Succeed())

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sleepInfoName,
				Namespace: namespace,
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err.Error()).Should(Equal("time should be of format HH:mm, actual: "))
		Expect(result).Should(Equal(ctrl.Result{}))
	})

	It("not valid wake up schedule", func() {
		namespace := "not-valid-wake-up-schedule"
		err := testutil.CreateNamespace(ctx, k8sClient, namespace)
		Expect(err).NotTo(HaveOccurred())

		By("create SleepInfo")
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
				Weekdays:   "1",
				SleepTime:  "*:*",
				WakeUpTime: "*",
			},
		}
		Expect(k8sClient.Create(ctx, sleepInfo)).Should(Succeed())

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sleepInfoName,
				Namespace: namespace,
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err.Error()).Should(Equal("time should be of format HH:mm, actual: *"))
		Expect(result).Should(Equal(ctrl.Result{}))
	})

	It("not valid weekday", func() {
		namespace := "not-valid-weekday"
		err := testutil.CreateNamespace(ctx, k8sClient, namespace)
		Expect(err).NotTo(HaveOccurred())

		By("create SleepInfo")
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
				Weekdays: "",
			},
		}
		Expect(k8sClient.Create(ctx, sleepInfo)).Should(Succeed())

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sleepInfoName,
				Namespace: namespace,
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err.Error()).Should(Equal("empty weekdays from SleepInfo configuration"))
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
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{})

		assertContextInfo := AssertOperation{
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalResources.deploymentList,
		}

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60 + 1))
		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:19:50.100Z").withRequeue(45*60 + 9.9))
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T21:05:00.000Z").withRequeue(15 * 60))
	})

	It("reconcile - deploy between sleep and wake up", func() {
		namespace := "deploy-between-sleep-and-wake-up"
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{})

		assertContextInfo := AssertOperation{
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalResources.deploymentList,
		}

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60 + 1))

		By("re deploy", func() {
			upsertDeployments(ctx, namespace, true)

			By("check replicas")
			deployments := listDeployments(ctx, namespace)
			for _, deployment := range deployments {
				originalDeployment := findDeployByName(originalResources.deploymentList, deployment.Name)
				Expect(deployment.Spec.Replicas).To(Equal(originalDeployment.Spec.Replicas))
			}
		})

		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:20:00.000Z").withRequeue(45 * 60))
	})

	It("reconcile - change single deployment replicas between sleep and wake up", func() {
		namespace := "change-replicas-between-sleep-and-wake-up"
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{})

		assertContextInfo := AssertOperation{
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalResources.deploymentList,
		}
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60 + 1))

		By("re deploy a single deployment", func() {
			deployments := listDeployments(ctx, namespace)

			deploymentToUpdate := deployments[0].DeepCopy()
			patch := client.MergeFrom(deploymentToUpdate)
			*deploymentToUpdate.Spec.Replicas = 0
			err := k8sClient.Patch(ctx, deploymentToUpdate, patch)
			Expect(err).ToNot(HaveOccurred())

			updatedDeployment := appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: deploymentToUpdate.Name, Namespace: namespace}, &updatedDeployment)).To(Succeed())
			Expect(*updatedDeployment.Spec.Replicas).To(Equal(int32(0)))
		})

		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:20:00.000Z").withRequeue(45 * 60))
	})

	It("reconcile - twice consecutive sleep operation", func() {
		namespace := "twice-sleep"
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{})

		assertContextInfo := AssertOperation{
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalResources.deploymentList,
		}
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60 + 1))
		assertCorrectSleepOperation(
			assertContextInfo.
				withSchedule("2021-03-23T21:05:00.000Z").
				withExpectedSchedule("2021-03-23T20:05:59Z").
				withRequeue(15 * 60),
		)
	})

	It("reconcile - only sleep, wake up set to nil", func() {
		namespace := "without-wake-up"
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{
			unsetWakeUpTime: true,
		})

		assertContextInfo := AssertOperation{
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalResources.deploymentList,
		}
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(59*60 + 1))
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T21:05:00.000Z").withRequeue(60 * 60))

		By("re deploy", func() {
			upsertDeployments(ctx, namespace, true)

			By("check replicas")
			deployments := listDeployments(ctx, namespace)
			for _, deployment := range deployments {
				originalDeployment := findDeployByName(originalResources.deploymentList, deployment.Name)
				Expect(deployment.Spec.Replicas).To(Equal(originalDeployment.Spec.Replicas))
			}
		})

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T22:04:00.000Z").withRequeue(61 * 60))
	})

	It("reconcile - sleep info not present in namespace", func() {
		namespace := "no-sleepinfo"
		err := testutil.CreateNamespace(ctx, k8sClient, namespace)
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
		createSleepInfo(ctx, sleepInfoName, namespace, setupOptions{})
		originalDeployments := upsertDeployments(ctx, namespace, false)

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sleepInfoName,
				Namespace: namespace,
			},
		}
		sleepInfoReconciler = SleepInfoReconciler{
			Clock: mockClock{
				now: "2021-03-23T20:05:59.999Z",
			},
			Client: k8sClient,
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("replicas are set to 0 to all deployments", func() {
			deployments := listDeployments(ctx, namespace)
			assertAllReplicasSetToZero(deployments, originalDeployments)
		})

		By("is requeued after correct duration", func() {
			Expect(result).Should(Equal(ctrl.Result{
				RequeueAfter: wakeUpRequeue("2021-03-23T20:05:59.999Z"),
			}))
		})
	})

	It("reconcile - create deployment between sleep and wake up", func() {
		namespace := "create-deployment-between-sleep-and-wake-up"
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{})

		assertContextInfo := AssertOperation{
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalResources.deploymentList,
		}

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60 + 1))

		createdServiceName := "service-new"
		By("create deployment", func() {
			fiveReplicas := int32(5)
			deployToCreate := appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      createdServiceName,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &fiveReplicas,
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
			}
			err := k8sClient.Create(ctx, deployToCreate.DeepCopy())
			Expect(err).NotTo(HaveOccurred())

			newOriginalDeployments := append(originalResources.deploymentList, deployToCreate)
			assertContextInfo.originalDeployments = newOriginalDeployments

			By("check replicas")
			deployments := listDeployments(ctx, namespace)
			for _, deployment := range deployments {
				if deployment.Name == createdServiceName {
					Expect(*deployment.Spec.Replicas).To(Equal(fiveReplicas))
					continue
				}
				Expect(*deployment.Spec.Replicas).To(Equal(int32(0)), deployment.Name)
			}
		})

		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:20:00.000Z").withRequeue(45 * 60))
	})

	It("reconcile - with deployments to exclude", func() {
		namespace := "multiple-deployments-exclude"
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{
			excludeRef: []kubegreenv1alpha1.ExcludeRef{
				{
					ApiVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "service-1",
				},
				{
					ApiVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "zero-replicas",
				},
			},
		})

		assertContextInfo := AssertOperation{
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalResources.deploymentList,
			excludedDeployment:  []string{"service-1", "zero-replicas"},
		}

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60 + 1))
		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:19:50.100Z").withRequeue(45*60 + 9.9))
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T21:05:00.000Z").withRequeue(15 * 60))
	})

	It("reconcile - with deployments and cron jobs", func() {
		namespace := "deployments-cronjobs-suspend"
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{
			suspendCronjobs: true,
			insertCronjobs:  true,
		})

		assertContextInfo := AssertOperation{
			ctx:           ctx,
			req:           req,
			namespace:     namespace,
			sleepInfoName: sleepInfoName,

			originalDeployments: originalResources.deploymentList,

			originalCronJobs: originalResources.cronjobList,
			suspendCronjobs:  true,
		}

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60 + 1))
		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:19:50.100Z").withRequeue(45*60 + 9.9))
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T21:05:00.000Z").withRequeue(15 * 60))
	})

	It("reconcile - with deployments - suspend cron jobs active but not cron job in namespace", func() {
		namespace := "deployments-cron-job-in-namespace"
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{
			suspendCronjobs: true,
		})

		assertContextInfo := AssertOperation{
			ctx:           ctx,
			req:           req,
			namespace:     namespace,
			sleepInfoName: sleepInfoName,

			originalDeployments: originalResources.deploymentList,

			suspendCronjobs:  true,
			originalCronJobs: originalResources.cronjobList,
		}

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60 + 1))
		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:19:50.100Z").withRequeue(45*60 + 9.9))
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T21:05:00.000Z").withRequeue(15 * 60))
	})

	It("reconcile - with deployments and cron jobs not to suspend", func() {
		namespace := "deployments-sleep-cronjobs-not-suspend"
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{
			insertCronjobs: true,
		})

		assertContextInfo := AssertOperation{
			ctx:           ctx,
			req:           req,
			namespace:     namespace,
			sleepInfoName: sleepInfoName,

			originalDeployments: originalResources.deploymentList,

			originalCronJobs: originalResources.cronjobList,
		}

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60 + 1))
		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:19:50.100Z").withRequeue(45*60 + 9.9))
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T21:05:00.000Z").withRequeue(15 * 60))
	})

	It("reconcile - with deployments and cron job to exclude", func() {
		namespace := "multiple-cronjob-exclude"
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{
			suspendCronjobs: true,
			insertCronjobs:  true,
			excludeRef: []kubegreenv1alpha1.ExcludeRef{
				{
					ApiVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "service-1",
				},
				{
					ApiVersion: "batch/v1",
					Kind:       "CronJob",
					Name:       "cronjob-2",
				},
			},
		})

		assertContextInfo := AssertOperation{
			ctx:                 ctx,
			req:                 req,
			namespace:           namespace,
			sleepInfoName:       sleepInfoName,
			originalDeployments: originalResources.deploymentList,
			excludedDeployment:  []string{"service-1"},

			suspendCronjobs:  true,
			originalCronJobs: originalResources.cronjobList,
			excludedCronJob:  []string{"cronjob-2"},
		}

		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60 + 1))
		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:19:50.100Z").withRequeue(45*60 + 9.9))
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T21:05:00.000Z").withRequeue(15 * 60))
	})

	It("reconcile - with deployments and cron jobs - deploy between sleep and wakeup", func() {
		namespace := "deployments-cronjobs-suspend-redeploy"
		req, originalResources := setupNamespaceWithResources(ctx, sleepInfoName, namespace, sleepInfoReconciler, mockNow, setupOptions{
			suspendCronjobs: true,
			insertCronjobs:  true,
		})

		assertContextInfo := AssertOperation{
			ctx:           ctx,
			req:           req,
			namespace:     namespace,
			sleepInfoName: sleepInfoName,

			originalDeployments: originalResources.deploymentList,

			originalCronJobs: originalResources.cronjobList,
			suspendCronjobs:  true,
		}
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T20:05:59.000Z").withRequeue(14*60 + 1))

		By("re deploy", func() {
			upsertDeployments(ctx, namespace, true)
			upsertCronJobs(ctx, namespace, true)

			By("check replicas")
			deployments := listDeployments(ctx, namespace)
			for _, deployment := range deployments {
				originalDeployment := findDeployByName(originalResources.deploymentList, deployment.Name)
				Expect(deployment.Spec.Replicas).To(Equal(originalDeployment.Spec.Replicas))
			}

			By("check cron jobs status")
			cronJobs := listCronJobs(ctx, namespace)
			for _, cronJob := range cronJobs {
				originalCronJob := findResourceByName(originalResources.cronjobList, cronJob.GetName())
				Expect(isCronJobSuspended(cronJob)).To(Equal(isCronJobSuspended(*originalCronJob)))
			}
		})

		assertCorrectWakeUpOperation(assertContextInfo.withSchedule("2021-03-23T20:19:50.100Z").withRequeue(45*60 + 9.9))
		assertCorrectSleepOperation(assertContextInfo.withSchedule("2021-03-23T21:05:00.000Z").withRequeue(15 * 60))
	})
})

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
	for _, deployment := range actualDeployments {
		allReplicas = append(allReplicas, *deployment.Spec.Replicas)
	}
	for _, replicas := range allReplicas {
		Expect(replicas).To(Equal(int32(0)))
	}
}

func assertAllCronJobsSuspended(actualCronJobs []unstructured.Unstructured, originalCronJobs []unstructured.Unstructured) {
	allSuspended := []bool{}
	for _, cronJob := range actualCronJobs {
		allSuspended = append(allSuspended, isCronJobSuspended(cronJob))
	}
	for _, suspended := range allSuspended {
		Expect(suspended).To(BeTrue())
	}
}

func getTime(mockNowRaw string) time.Time {
	now, err := time.Parse(time.RFC3339, mockNowRaw)
	Expect(err).ShouldNot(HaveOccurred())
	return now
}

type AssertOperation struct {
	ctx           context.Context
	req           ctrl.Request
	namespace     string
	sleepInfoName string
	scheduleTime  string
	// optional - default is equal to scheduleTime
	expectedScheduleTime string
	expectedNextRequeue  time.Duration

	originalDeployments []appsv1.Deployment
	excludedDeployment  []string

	suspendCronjobs  bool
	originalCronJobs []unstructured.Unstructured
	excludedCronJob  []string
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
	}
	result, err := sleepInfoReconciler.Reconcile(assert.ctx, assert.req)
	Expect(err).NotTo(HaveOccurred())

	By("replicas are set to 0 to all deployments set to sleep", func() {
		deployments := listDeployments(assert.ctx, assert.namespace)
		if len(assert.excludedDeployment) == 0 {
			assertAllReplicasSetToZero(deployments, assert.originalDeployments)
		} else {
			for _, deployment := range deployments {
				if contains(assert.excludedDeployment, deployment.Name) {
					originalDeployment := findDeployByName(assert.originalDeployments, deployment.Name)
					Expect(*deployment.Spec.Replicas).To(Equal(*originalDeployment.Spec.Replicas))
					continue
				}
				Expect(*deployment.Spec.Replicas).To(Equal(int32(0)))
			}
		}
	})

	By("cron jobs are correctly suspended", func() {
		cronJobs := listCronJobs(assert.ctx, assert.namespace)
		if assert.suspendCronjobs {
			if len(assert.excludedCronJob) == 0 {
				assertAllCronJobsSuspended(cronJobs, assert.originalCronJobs)
			} else {
				for _, cronJob := range cronJobs {
					originalCronJob := findResourceByName(assert.originalCronJobs, cronJob.GetName())
					originalUnstructuredCronJob, err := runtime.DefaultUnstructuredConverter.ToUnstructured(originalCronJob)
					Expect(err).NotTo(HaveOccurred())
					if contains(assert.excludedCronJob, cronJob.GetName()) {
						Expect(isCronJobSuspended(cronJob)).To(Equal(isCronJobSuspended(unstructured.Unstructured{
							Object: originalUnstructuredCronJob,
						})))
						continue
					}
					Expect(isCronJobSuspended(cronJob)).To(BeTrue())
				}
			}
		} else {
			for _, cronJob := range cronJobs {
				originalCronJob := findResourceByName(assert.originalCronJobs, cronJob.GetName())
				originalUnstructuredCronJob, err := runtime.DefaultUnstructuredConverter.ToUnstructured(originalCronJob)
				Expect(err).NotTo(HaveOccurred())
				Expect(isCronJobSuspended(cronJob)).To(Equal(isCronJobSuspended(unstructured.Unstructured{
					Object: originalUnstructuredCronJob,
				})))
			}
		}
	})

	By("secret is correctly set", func() {
		secret, err := sleepInfoReconciler.getSecret(assert.ctx, getSecretName(assert.sleepInfoName), assert.namespace)
		Expect(err).NotTo(HaveOccurred())
		secretData := secret.Data

		type ExpectedReplicas struct {
			Name     string `json:"name"`
			Replicas int32  `json:"replicas"`
		}
		var originalReplicas []ExpectedReplicas
		for _, deployment := range assert.originalDeployments {
			if *deployment.Spec.Replicas == 0 || contains(assert.excludedDeployment, deployment.Name) {
				continue
			}
			originalReplicas = append(originalReplicas, ExpectedReplicas{
				Name:     deployment.Name,
				Replicas: *deployment.Spec.Replicas,
			})
		}
		var expectedReplicas = []byte{}
		expectedReplicas, err = json.Marshal(originalReplicas)
		Expect(err).NotTo(HaveOccurred())

		expectedSecretData := map[string][]byte{
			lastScheduleKey:        []byte(getTime(assert.expectedScheduleTime).Truncate(time.Second).Format(time.RFC3339)),
			lastOperationKey:       []byte(sleepOperation),
			replicasBeforeSleepKey: expectedReplicas,
		}

		if assert.suspendCronjobs {
			originalStatus := []cronjobs.OriginalCronJobStatus{}
			for _, cronJob := range assert.originalCronJobs {
				if isCronJobSuspended(cronJob) {
					continue
				}
				if contains(assert.excludedCronJob, cronJob.GetName()) {
					continue
				}
				originalStatus = append(originalStatus, cronjobs.OriginalCronJobStatus{
					Name:    cronJob.GetName(),
					Suspend: false,
				})
			}
			expectedStatus, err := json.Marshal(originalStatus)
			Expect(err).NotTo(HaveOccurred())

			expectedSecretData[originalCronjobStatusKey] = expectedStatus
		}

		Expect(secretData).To(Equal(expectedSecretData))
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
	}
	result, err := sleepInfoReconciler.Reconcile(assert.ctx, assert.req)
	Expect(err).NotTo(HaveOccurred())

	By("deployment replicas correctly waked up", func() {
		deployments := listDeployments(assert.ctx, assert.namespace)
		for _, deployment := range deployments {
			originalDeployment := findDeployByName(assert.originalDeployments, deployment.Name)
			Expect(deployment.Spec.Replicas).To(Equal(originalDeployment.Spec.Replicas))
		}
	})

	By("cron jobs correctly waked up", func() {
		cronJobs := listCronJobs(assert.ctx, assert.namespace)
		for _, cronJob := range cronJobs {
			originalCronJob := findResourceByName(assert.originalCronJobs, cronJob.GetName())
			originalUnstructuredCronJob, err := runtime.DefaultUnstructuredConverter.ToUnstructured(originalCronJob)
			Expect(err).NotTo(HaveOccurred())
			Expect(isCronJobSuspended(cronJob)).To(Equal(isCronJobSuspended(unstructured.Unstructured{
				Object: originalUnstructuredCronJob,
			})))
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
