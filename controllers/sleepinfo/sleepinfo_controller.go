/*
Copyright 2021.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
)

const (
	lastScheduledAnnotation       = "sleepinfo.kube-green.com/scheduled-at"
	lastOperationAnnotation       = "sleepinfo.kube-green.com/operation"
	replicasBeforeSleepAnnotation = "sleepinfo.kube-green.com/replicas-before-sleep"

	sleepOperation   = "SLEEP"
	restoreOperation = "RESTORE"
)

// SleepInfoReconciler reconciles a SleepInfo object
type SleepInfoReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Clock
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

// clock knows how to get the current time.
// It can be used to fake out timing for testing.
type Clock interface {
	Now() time.Time
}

//+kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SleepInfo object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *SleepInfoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("sleepinfo", req.NamespacedName)

	sleepInfo, err := r.getSleepInfo(ctx, req)
	if err != nil {
		log.Error(err, "unable to fetch sleepInfo")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	namespace, err := r.getNamespace(ctx, req.Namespace)
	if err != nil {
		log.Error(err, "unable to fetch namespace", "namespaceName", req.Namespace)
		return ctrl.Result{}, err
	}

	lastSchedule, err := r.getLastScheduledAnnotation(namespace)
	if err != nil {
		log.Error(err, "last schedule is invalid")
		return ctrl.Result{}, err
	}
	currentOperation, currentOperationCronSchedule, nextOperationCronSchedule := r.getCurrentOperation(namespace, sleepInfo)
	log.WithValues("operation", currentOperation)

	now := r.Clock.Now()
	isToExecute, nextSchedule, requeueAfter, err := r.getNextSchedule(currentOperationCronSchedule, nextOperationCronSchedule, lastSchedule, now)
	if err != nil {
		log.Error(err, "unable to update deployment with 0 replicas")
		return ctrl.Result{}, err
	}
	scheduleLog := log.WithValues("now", r.Now(), "next run", nextSchedule, "requeue", requeueAfter)

	if !isToExecute {
		scheduleLog.Info("skip execution")
		return ctrl.Result{
			RequeueAfter: requeueAfter,
		}, nil
	}

	// Handle operator status
	scheduleLog.WithValues("last schedule", now, "status", sleepInfo.Status).Info("last schedule value")
	sleepInfo.Status.LastScheduleTime = metav1.NewTime(now)
	sleepInfo.Status.OperationType = currentOperation
	if err := r.Status().Update(ctx, sleepInfo); err != nil {
		log.Error(err, "unable to update sleepInfo status")
		return ctrl.Result{}, err
	}

	deploymentList, err := r.getDeploymentsByNamespace(ctx, req.Namespace)
	if err != nil {
		log.Error(err, "fails to fetch deployments")
		return ctrl.Result{}, err
	}

	if len(deploymentList) == 0 {
		// TODO: skip only if current operation is SLEEP - add test
		if currentOperation == sleepOperation {
			requeueAfter, err = skipRestoreIfSleepNotPerformed(currentOperationCronSchedule, nextSchedule, now)
			if err != nil {
				log.Error(err, "fails to parse cron - 0 deployment")
				return ctrl.Result{}, nil
			}
		}

		log.Info("deployment not present in namespace")
		return ctrl.Result{
			RequeueAfter: requeueAfter,
		}, nil
	}

	switch currentOperation {
	case sleepOperation:
		if err := r.handleSleep(log, ctx, deploymentList); err != nil {
			log.Error(err, "fails to handle sleep")
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	case restoreOperation:
		if err := r.handleRestore(log, ctx, deploymentList); err != nil {
			log.Error(err, "fails to handle restore")
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	default:
		return ctrl.Result{}, fmt.Errorf("operation %s not supported", currentOperation)
	}

	namespaceAnnotations := namespace.GetAnnotations()
	if namespaceAnnotations == nil {
		namespaceAnnotations = map[string]string{}
	}
	namespaceAnnotations[lastScheduledAnnotation] = now.Format(time.RFC3339)
	namespaceAnnotations[lastOperationAnnotation] = currentOperation
	namespace.SetAnnotations(namespaceAnnotations)
	log.Info("update namespace", "namespace", req.Namespace)
	if err := r.Client.Update(ctx, namespace); err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Info("namespace not found", "namespace name", req.Namespace)
			return ctrl.Result{}, nil
		}
		log.Error(err, "fails to update namespace", "namespace name", req.Namespace)
		return ctrl.Result{
			Requeue: true,
		}, nil
	}

	return ctrl.Result{
		RequeueAfter: requeueAfter,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SleepInfoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Clock == nil {
		r.Clock = realClock{}
	}

	// TODO: validate schedule

	pred := predicate.GenerationChangedPredicate{}
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubegreenv1alpha1.SleepInfo{}).
		WithEventFilter(pred).
		// Owns(). // TODO: handle secrets
		Complete(r)
}

func (r *SleepInfoReconciler) getDeploymentsByNamespace(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	listOptions := &client.ListOptions{
		Namespace: namespace,
	}
	deployments := appsv1.DeploymentList{}
	if err := r.Client.List(ctx, &deployments, listOptions); err != nil {
		return deployments.Items, client.IgnoreNotFound(err)
	}
	return deployments.Items, nil
}

func (r *SleepInfoReconciler) getLastScheduledAnnotation(namespace *v1.Namespace) (time.Time, error) {
	annotations := namespace.GetAnnotations()
	if annotations == nil || annotations[lastScheduledAnnotation] == "" {
		return time.Time{}, nil
	}
	lastScheduleRaw := annotations[lastScheduledAnnotation]
	return time.Parse(time.RFC3339, lastScheduleRaw)
}

func (r *SleepInfoReconciler) getCurrentOperation(namespace *v1.Namespace, sleepInfo *kubegreenv1alpha1.SleepInfo) (string, string, string) {
	annotations := namespace.GetAnnotations()
	lastOperation := annotations[lastOperationAnnotation]
	if lastOperation == sleepOperation {
		return restoreOperation, sleepInfo.Spec.RestoreSchedule, sleepInfo.Spec.SleepSchedule
	}
	return sleepOperation, sleepInfo.Spec.SleepSchedule, sleepInfo.Spec.RestoreSchedule
}

func (r *SleepInfoReconciler) getNamespace(ctx context.Context, namespaceName string) (*v1.Namespace, error) {
	namespace := &v1.Namespace{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Name: namespaceName,
	}, namespace)
	if err != nil {
		return nil, err
	}
	return namespace, nil
}

func (r *SleepInfoReconciler) getSleepInfo(ctx context.Context, req ctrl.Request) (*kubegreenv1alpha1.SleepInfo, error) {
	sleepInfo := &kubegreenv1alpha1.SleepInfo{}
	if err := r.Client.Get(ctx, req.NamespacedName, sleepInfo); err != nil {
		return nil, err
	}
	return sleepInfo, nil
}

func skipRestoreIfSleepNotPerformed(currentOperationCronSchedule string, nextSchedule, now time.Time) (time.Duration, error) {
	nextOpSched, err := getCronParsed(currentOperationCronSchedule)
	if err != nil {
		return 0, fmt.Errorf("fails to parse cron current schedule: %s", err)
	}
	requeueAfter := getRequeueAfter(nextOpSched.Next(nextSchedule), now)

	return requeueAfter, nil
}
