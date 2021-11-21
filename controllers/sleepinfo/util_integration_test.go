package sleepinfo

import (
	"context"
	"time"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type setupOptions struct {
	excludeRef      []kubegreenv1alpha1.ExcludeRef
	unsetWakeUpTime bool
	suspendCronjobs bool
	insertCronjobs  bool
}

type originalResources struct {
	deploymentList []appsv1.Deployment
	cronjobList    []batchv1.CronJob
}

func setupNamespaceWithResources(ctx context.Context, sleepInfoName, namespace string, reconciler SleepInfoReconciler, now string, opts setupOptions) (ctrl.Request, originalResources) {
	createSleepInfo(ctx, sleepInfoName, namespace, opts)

	By("create deployments")
	originalDeployments := upsertDeployments(ctx, namespace, false)

	var originalCronJobs []batchv1.CronJob
	if opts.insertCronjobs {
		By("create cronjobs")
		originalCronJobs = upsertCronJobs(ctx, namespace, false)
	}

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
			RequeueAfter: sleepRequeue(now),
		}))
	})

	By("replicas not changed", func() {
		deploymentsNotChanged := listDeployments(ctx, namespace)
		for i, deployment := range deploymentsNotChanged {
			Expect(*deployment.Spec.Replicas).To(Equal(*originalDeployments[i].Spec.Replicas))
		}
	})

	By("cron jobs not suspended", func() {
		cronJobsNotChanged := listCronJobs(ctx, namespace)
		for i, cj := range cronJobsNotChanged {
			Expect(*cj.Spec.Suspend).To(Equal(*originalCronJobs[i].Spec.Suspend))
		}
	})

	return req, originalResources{
		deploymentList: originalDeployments,
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

func upsertDeployments(ctx context.Context, namespace string, updateIfAlreadyCreated bool) []appsv1.Deployment {
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
		d := appsv1.Deployment{}
		if updateIfAlreadyCreated {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      deployment.Name,
				Namespace: namespace,
			}, &d)
			if err == nil {
				deploymentAlreadyExists = true
			}
		}
		if deploymentAlreadyExists {
			patch := client.MergeFrom(d.DeepCopy())
			d.Spec.Replicas = deployment.Spec.Replicas
			if err := k8sClient.Patch(ctx, &d, patch); err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
		} else {
			if err := k8sClient.Create(ctx, &deployment); err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
		}
	}
	return deployments
}

func listDeployments(ctx context.Context, namespace string) []appsv1.Deployment {
	deployments := appsv1.DeploymentList{}
	err := k8sClient.List(ctx, &deployments, &client.ListOptions{
		Namespace: namespace,
	})
	Expect(err).NotTo(HaveOccurred())
	return deployments.Items
}

func createSleepInfo(ctx context.Context, sleepInfoName, namespace string, opts setupOptions) kubegreenv1alpha1.SleepInfo {
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
			SleepTime:  "*:05", // every 5 minute
			WakeUpTime: "*:20", // every 20 minute
		},
	}
	if opts.unsetWakeUpTime {
		sleepInfo.Spec.WakeUpTime = ""
	}
	if len(opts.excludeRef) != 0 {
		sleepInfo.Spec.ExcludeRef = opts.excludeRef
	}
	if opts.suspendCronjobs {
		sleepInfo.Spec.SuspendCronjobs = opts.suspendCronjobs
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

func sleepRequeue(now string) time.Duration {
	parsedTime, _ := time.Parse(time.RFC3339, now)
	hour := parsedTime.Hour()
	if parsedTime.Minute() > 5 {
		hour += 1
	}
	return time.Duration((time.Date(2021, time.March, 23, hour, 5, 0, 0, time.UTC).UnixNano() - parsedTime.UnixNano()))
}

func wakeUpRequeue(now string) time.Duration {
	parsedTime, _ := time.Parse(time.RFC3339, now)
	hour := parsedTime.Hour()
	if parsedTime.Minute() > 20 {
		hour += 1
	}
	return time.Duration((time.Date(2021, time.March, 23, hour, 20, 0, 0, time.UTC).UnixNano() - parsedTime.UnixNano()))
}

func findDeployByName(deployments []appsv1.Deployment, nameToFind string) *appsv1.Deployment {
	for _, deployment := range deployments {
		if deployment.Name == nameToFind {
			return deployment.DeepCopy()
		}
	}
	return nil
}

func contains(s []string, v string) bool {
	for _, a := range s {
		if a == v {
			return true
		}
	}
	return false
}

func upsertCronJobs(ctx context.Context, namespace string, updateIfAlreadyCreated bool) []batchv1.CronJob {
	return nil
}

func listCronJobs(ctx context.Context, namespace string) []batchv1.CronJob {
	return nil
}
