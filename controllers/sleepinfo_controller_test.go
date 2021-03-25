package controllers

import (
	"context"
	"fmt"
	"time"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
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
				SleepSchedule: "* * * * *",
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

		Expect(createdSleepInfo.Spec.SleepSchedule).Should(Equal("* * * * *"))
	})

	It("reconcile", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sleepInfoName,
				Namespace: sleepInfoNamespace,
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("is requeue correctly")
		Expect(result).Should(Equal(ctrl.Result{
			// 39445 is the difference between mocked now and next minute
			// (the next scheduled time), in milliseconds
			RequeueAfter: (39445 + 1000) * time.Millisecond,
		}))
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

	It("not valid schedule", func() {
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
		Expect(err.Error()).Should(Equal("sleep schedule not valid: expected exactly 5 fields, found 4: [* * * *]"))
		Expect(result).Should(Equal(ctrl.Result{}))
	})

	It("reconcile - with deployments", func() {
		By("create deployments")
		Expect(createDeployments(ctx, sleepInfoNamespace)).NotTo(HaveOccurred())

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      sleepInfoName,
				Namespace: sleepInfoNamespace,
			},
		}
		result, err := sleepInfoReconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("is requeue correctly")
		Expect(result).Should(Equal(ctrl.Result{
			// 39445 is the difference between mocked now and next minute
			// (the next scheduled time), in milliseconds
			RequeueAfter: (39445 + 1000) * time.Millisecond,
		}))
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

func createDeployments(ctx context.Context, namespace string) error {
	var threeReplicas int32 = 3
	var oneReplica int32 = 1
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
	}
	if err := k8sClient.Create(ctx, &deployments[0]); err != nil {
		return fmt.Errorf("error %s creating deployment 1", err)
	}
	if err := k8sClient.Create(ctx, &deployments[1]); err != nil {
		return fmt.Errorf("error %s creating deployment 2", err)
	}
	return nil
}

type mockClock struct {
	now string
}

func (m mockClock) Now() time.Time {
	parsedTime, err := time.Parse(time.RFC3339, m.now)
	Expect(err).NotTo(HaveOccurred())
	return parsedTime
}
