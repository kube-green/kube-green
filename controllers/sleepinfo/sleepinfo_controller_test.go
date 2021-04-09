package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
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
		sleepInfoNamespace = "my-namespace"
		sleepInfoName      = "sleep-name"
		mockNow            = "2021-03-23T20:05:20.555Z"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		sleepInfoReconciler      SleepInfoReconciler
		testLogger                                 = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
		expectedEmptyAnnotations map[string]string = nil
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

	It("create SleepInfo resource", func() {
		Expect(createNamespace(ctx, sleepInfoNamespace)).NotTo(HaveOccurred())
		sleepInfo := &kubegreenv1alpha1.SleepInfo{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SleepInfo",
				APIVersion: "kube-green.com/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      sleepInfoName,
				Namespace: sleepInfoNamespace,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				SleepSchedule:   "*/2 * * * *",    // every even minute
				RestoreSchedule: "1-59/2 * * * *", // every uneven minute
			},
		}
		Expect(k8sClient.Create(ctx, sleepInfo)).Should(Succeed())

		sleepInfoLookupKey := types.NamespacedName{Name: sleepInfoName, Namespace: sleepInfoNamespace}
		createdSleepInfo := &kubegreenv1alpha1.SleepInfo{}

		// We'll need to retry getting this newly created SleepInfo, given that creation may not immediately happen.
		Eventually(func() bool {
			err := k8sClient.Get(ctx, sleepInfoLookupKey, createdSleepInfo)
			Expect(err).NotTo(HaveOccurred())
			return true
		}, timeout, interval).Should(BeTrue())

		Expect(createdSleepInfo.Spec.SleepSchedule).Should(Equal("*/2 * * * *"))
		Expect(createdSleepInfo.Spec.RestoreSchedule).Should(Equal("1-59/2 * * * *"))
	})

	It("reconcile - zero deployments", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sleepInfoName,
				Namespace: sleepInfoNamespace,
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
			deployments, err := listDeployments(ctx, sleepInfoNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(deployments)).To(Equal(0))
		})

		By("without deployments, namespace annotations are not written", func() {
			namespace, err := sleepInfoReconciler.getNamespace(ctx, sleepInfoNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(namespace.GetAnnotations()).To(Equal(expectedEmptyAnnotations))
		})

		By("sleepinfo status updated correctly", func() {
			sleepInfo, err := sleepInfoReconciler.getSleepInfo(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			lastSchedule, err := time.Parse(time.RFC3339, sleepScheduleTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(sleepInfo.Status).To(Equal(kubegreenv1alpha1.SleepInfoStatus{
				LastScheduleTime: metav1.NewTime(lastSchedule.Local()),
				// FIXME: sleep operation not really performed!
				OperationType: sleepOperation,
			}))
		})

		By("is requeued correctly to next SLEEP", func() {
			Expect(result).Should(Equal(ctrl.Result{
				// 121000 is the difference between mocked now and next uneven minute
				// (the next scheduled time for restore), in milliseconds
				RequeueAfter: 121000 * time.Millisecond,
			}))
		})

		By("is reconciled correctly - RESTORE")
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
			deployments, err := listDeployments(ctx, sleepInfoNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(deployments)).To(Equal(0))
		})

		By("sleepinfo status updated correctly", func() {
			sleepInfo, err := sleepInfoReconciler.getSleepInfo(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			lastSchedule, err := time.Parse(time.RFC3339, sleepScheduleTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(sleepInfo.Status).To(Equal(kubegreenv1alpha1.SleepInfoStatus{
				LastScheduleTime: metav1.NewTime(lastSchedule.Local()),
				// FIXME: sleep operation not really performed!
				OperationType: sleepOperation,
			}))
		})

		By("without deployments, namespace annotations are not written", func() {
			namespace, err := sleepInfoReconciler.getNamespace(ctx, sleepInfoNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(namespace.GetAnnotations()).To(Equal(expectedEmptyAnnotations))
		})
	})

	It("not existent resource return without error", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "not-exists",
				Namespace: sleepInfoNamespace,
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
				Namespace: sleepInfoNamespace,
			},
			Spec: kubegreenv1alpha1.SleepInfoSpec{
				SleepSchedule: "* * * *",
			},
		}
		Expect(k8sClient.Create(ctx, sleepInfo)).Should(Succeed())

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: sleepInfoNamespace,
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err.Error()).Should(Equal("current schedule not valid: expected exactly 5 fields, found 4: [* * * *]"))
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

		By("is requeue correctly")
		Expect(result).Should(Equal(ctrl.Result{}))
	})

	It("reconcile - with deployments", func() {
		// TODO: create a new namespace

		By("create deployments")
		originalDeployments, err := createDeployments(ctx, sleepInfoNamespace)
		Expect(err).NotTo(HaveOccurred())

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sleepInfoName,
				Namespace: sleepInfoNamespace,
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("is requeued after correct duration", func() {
			Expect(result).Should(Equal(ctrl.Result{
				// 39445 is the difference between mocked now and next minute
				// (the next scheduled time), in milliseconds
				RequeueAfter: 39445 * time.Millisecond,
			}))
		})

		By("replicas not changed", func() {
			deploymentsNotChanged, err := listDeployments(ctx, sleepInfoNamespace)
			Expect(err).NotTo(HaveOccurred())
			for _, deployment := range deploymentsNotChanged {
				if deployment.Name == "zero-replicas" || deployment.Name == "zero-replicas-annotation" {
					Expect(*deployment.Spec.Replicas).To(Equal(int32(0)))
				} else {
					Expect(*deployment.Spec.Replicas).NotTo(Equal(int32(0)))
				}
			}
		})

		By("is requeued correctly - SLEEP")
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

		By("replicas are set to 0 to all deployments and annotations are set correctly to deployments", func() {
			deployments, err := listDeployments(ctx, sleepInfoNamespace)
			Expect(err).NotTo(HaveOccurred())
			allReplicas := []int32{}
			for idx, deployment := range deployments {
				allReplicas = append(allReplicas, *deployment.Spec.Replicas)

				annotations := deployment.GetAnnotations()
				Expect(annotations[replicasBeforeSleepAnnotation]).To(Equal(fmt.Sprintf("%d", *originalDeployments[idx].Spec.Replicas)))
			}
			for _, replicas := range allReplicas {
				Expect(replicas).To(Equal(int32(0)))
			}
		})

		By("namespace annotations are correctly set", func() {
			namespace, err := sleepInfoReconciler.getNamespace(ctx, sleepInfoNamespace)
			Expect(err).NotTo(HaveOccurred())
			annotations := namespace.GetAnnotations()
			Expect(annotations).To(Equal(map[string]string{
				lastScheduledAnnotation: "2021-03-23T20:05:59Z",
				lastOperationAnnotation: sleepOperation,
			}))
		})

		By("sleepinfo status updated correctly", func() {
			sleepInfo, err := sleepInfoReconciler.getSleepInfo(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			lastSchedule, err := time.Parse(time.RFC3339, sleepScheduleTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(sleepInfo.Status).To(Equal(kubegreenv1alpha1.SleepInfoStatus{
				LastScheduleTime: metav1.NewTime(lastSchedule.Local()),
				OperationType:    sleepOperation,
			}))
		})

		By("is requeued after correct duration to restore", func() {
			Expect(result).Should(Equal(ctrl.Result{
				// 61000 is the difference between mocked now and next uneven minute
				// (the next scheduled time for restore), in milliseconds
				RequeueAfter: 61000 * time.Millisecond,
			}))
		})

		By("requeued correctly - RESTORE")
		sleepInfoReconciler = SleepInfoReconciler{
			Clock: mockClock{
				now: "2021-03-23T20:07:00.100Z",
			},
			Client: k8sClient,
			Log:    testLogger,
		}
		result, err = sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("deployment replicas correctly restored and annotation deleted", func() {
			// TODO:
			deployments, err := listDeployments(ctx, sleepInfoNamespace)
			Expect(err).NotTo(HaveOccurred())
			for idx, deployment := range deployments {
				Expect(deployment.Spec.Replicas).To(Equal(originalDeployments[idx].Spec.Replicas))
			}
		})

		By("status correctly updated", func() {
			// TODO:
		})

		By("is requeued after correct duration to sleep", func() {
			// Expect(result).Should(Equal(ctrl.Result{
			// 	// 61000 is the difference between mocked now and next uneven minute
			// 	// (the next scheduled time for restore), in milliseconds
			// 	RequeueAfter: 61000 * time.Millisecond,
			// }))
		})

		By("requeued correctly - SLEEP")
		sleepInfoReconciler = SleepInfoReconciler{
			Clock: mockClock{
				now: "2021-03-23T20:07:00.100Z",
			},
			Client: k8sClient,
			Log:    testLogger,
		}
		result, err = sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("deployment replicas correctly restored and annotation deleted", func() {
			// TODO:
		})

		By("status correctly updated", func() {
			// TODO:
		})

		By("is requeued after correct duration to sleep", func() {
			// Expect(result).Should(Equal(ctrl.Result{
			// 	// 61000 is the difference between mocked now and next uneven minute
			// 	// (the next scheduled time for restore), in milliseconds
			// 	RequeueAfter: 61000 * time.Millisecond,
			// }))
		})
	})
})

func TestIsTimeInDeltaMs(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		t1       time.Time
		t2       time.Time
		expected bool
		delta    time.Duration
	}{
		{
			name:     "t1 > t2 30s - delta 60s",
			t1:       now,
			t2:       now.Add(60 * time.Second),
			delta:    time.Second * 60,
			expected: true,
		},
		{
			name:     "t1 > t2 1ms - delta 1ms",
			t1:       now,
			t2:       now.Add(1 * time.Millisecond),
			delta:    time.Millisecond * 1,
			expected: true,
		},
		{
			name:     "t1 > t2 31s - delta 30s",
			t1:       now,
			t2:       now.Add(31 * time.Second),
			delta:    time.Second * 30,
			expected: false,
		},
		{
			name:     "t1 > t2 30s - delta 60s",
			t1:       now.Add(60 * time.Second),
			t2:       now,
			delta:    time.Second * 60,
			expected: true,
		},
		{
			name:     "t1 < t2 31s - delta 30s",
			t1:       now.Add(31 * time.Second),
			t2:       now,
			delta:    time.Second * 30,
			expected: false,
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("name, %s", test.name), func(t *testing.T) {
			output := isTimeInDelta(test.t1, test.t2, test.delta)
			require.Equal(t, test.expected, output)
		})
	}
}

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

func createDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
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
					lastScheduledAnnotation: "2021-03-23T00:00:00.000Z",
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
		if err := k8sClient.Create(ctx, &deployment); err != nil {
			return nil, fmt.Errorf("error %s creating deployment 1", err)
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
