package sleepinfo

import (
	"context"
	"fmt"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/internal/testutil"
	"github.com/kube-green/kube-green/controllers/sleepinfo/cronjobs"
	"github.com/kube-green/kube-green/controllers/sleepinfo/deployments"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type setupOptions struct {
	excludeRef         []kubegreenv1alpha1.ExcludeRef
	unsetWakeUpTime    bool
	suspendCronjobs    bool
	insertCronjobs     bool
	suspendDeployments *bool
}

type originalResources struct {
	deploymentList []appsv1.Deployment
	cronjobList    []unstructured.Unstructured

	sleepInfo kubegreenv1alpha1.SleepInfo
}

func setupNamespaceWithResources(ctx context.Context, sleepInfoName, namespace string, reconciler SleepInfoReconciler, now string, opts setupOptions) (ctrl.Request, originalResources) {
	cleanupNamespace(reconciler.Client, namespace)

	sleepInfo := createSleepInfo(ctx, sleepInfoName, namespace, opts)

	By("create deployments")
	originalDeployments := upsertDeployments(ctx, namespace, false)

	var originalCronJobs []unstructured.Unstructured
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
			Expect(isCronJobSuspended(cj)).To(Equal(isCronJobSuspended(originalCronJobs[i])))
		}
	})

	return req, originalResources{
		deploymentList: originalDeployments,
		cronjobList:    originalCronJobs,

		sleepInfo: sleepInfo,
	}
}

func upsertDeployments(ctx context.Context, namespace string, updateIfAlreadyCreated bool) []appsv1.Deployment {
	var threeReplicas int32 = 3
	var oneReplica int32 = 1
	var zeroReplicas int32 = 0
	deployments := []appsv1.Deployment{
		deployments.GetMock(deployments.MockSpec{
			Name:      "service-1",
			Namespace: namespace,
			Replicas:  &threeReplicas,
		}),
		deployments.GetMock(deployments.MockSpec{
			Name:      "service-2",
			Namespace: namespace,
			Replicas:  &oneReplica,
		}),
		deployments.GetMock(deployments.MockSpec{
			Name:      "zero-replicas",
			Namespace: namespace,
			Replicas:  &zeroReplicas,
		}),
		deployments.GetMock(deployments.MockSpec{
			Name:      "zero-replicas-annotation",
			Namespace: namespace,
			Replicas:  &zeroReplicas,
			PodAnnotations: map[string]string{
				lastScheduleKey: "2021-03-23T00:00:00.000Z",
			},
		}),
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
			patch := client.MergeFrom(&d)
			if err := k8sClient.Patch(ctx, &deployment, patch); err != nil {
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

	Expect(testutil.CreateNamespace(ctx, k8sClient, namespace)).NotTo(HaveOccurred())
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
			SleepTime:  "*:05", // at minute 5
			WakeUpTime: "*:20", // at minute 20
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
	if opts.suspendDeployments != nil {
		sleepInfo.Spec.SuspendDeployments = opts.suspendDeployments
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

func upsertCronJobs(ctx context.Context, namespace string, updateIfAlreadyCreated bool) []unstructured.Unstructured {
	suspendTrue := true
	suspendFalse := false

	restMapping, err := k8sClient.RESTMapper().RESTMapping(schema.GroupKind{
		Group: "batch",
		Kind:  "CronJob",
	})
	Expect(err).NotTo(HaveOccurred())

	version := getCronJobAPIVersion()

	cronJobs := []unstructured.Unstructured{
		cronjobs.GetMock(cronjobs.MockSpec{
			Name:      "cronjob-1",
			Namespace: namespace,
			Version:   version,
		}),
		cronjobs.GetMock(cronjobs.MockSpec{
			Name:      "cronjob-2",
			Namespace: namespace,
			Suspend:   &suspendFalse,
			Version:   version,
		}),
		cronjobs.GetMock(cronjobs.MockSpec{
			Name:      "cronjob-suspended",
			Namespace: namespace,
			Suspend:   &suspendTrue,
			Version:   version,
		}),
	}
	for _, cronJob := range cronJobs {
		var alreadyExists bool
		obj := unstructured.Unstructured{}
		if updateIfAlreadyCreated {
			obj.SetGroupVersionKind(restMapping.GroupVersionKind)

			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      cronJob.GetName(),
				Namespace: namespace,
			}, &obj)
			if err == nil {
				alreadyExists = true
			}
		}
		if alreadyExists {
			patch := client.MergeFrom(&obj)
			if err := k8sClient.Patch(ctx, &cronJob, patch); err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
		} else {
			if err := k8sClient.Create(ctx, &cronJob); err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
		}
	}
	return cronJobs
}

func listCronJobs(ctx context.Context, namespace string) []unstructured.Unstructured {
	restMapping, err := k8sClient.RESTMapper().RESTMapping(schema.GroupKind{
		Group: "batch",
		Kind:  "CronJob",
	})
	Expect(err).NotTo(HaveOccurred())

	u := unstructured.UnstructuredList{}
	u.SetGroupVersionKind(restMapping.GroupVersionKind)

	err = k8sClient.List(ctx, &u, &client.ListOptions{
		Namespace: namespace,
	})
	Expect(err).NotTo(HaveOccurred())

	return u.Items
}

func isCronJobSuspended(cronJob unstructured.Unstructured) bool {
	suspend, found, err := unstructured.NestedBool(cronJob.Object, "spec", "suspend")
	if err != nil {
		Fail(fmt.Sprintf("CronJob suspend error %s", err))
	}
	if !found {
		return false
	}
	return suspend
}

func cleanupNamespace(k8sClient client.Client, namespace string) {
	var (
		timeout  = time.Second * 20
		interval = time.Millisecond * 250
	)

	err := k8sClient.Delete(context.Background(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})
	Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())

	Eventually(func() bool {
		err := k8sClient.Get(context.Background(), types.NamespacedName{
			Name: namespace,
		}, &v1.Namespace{})
		return apierrors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}

func getCronJobAPIVersion() string {
	restMapping, err := k8sClient.RESTMapper().RESTMapping(schema.GroupKind{
		Group: "batch",
		Kind:  "CronJob",
	})
	Expect(err).NotTo(HaveOccurred())
	return restMapping.GroupVersionKind.Version
}
