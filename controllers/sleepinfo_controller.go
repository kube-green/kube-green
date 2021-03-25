/*
Copyright 2021.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
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

	sleepInfo := &kubegreenv1alpha1.SleepInfo{}
	if err := r.Client.Get(ctx, req.NamespacedName, sleepInfo); err != nil {
		log.Error(err, "unable to fetch sleepInfo")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	nextSchedule, requeueAfter, err := r.getNextSchedule(sleepInfo)
	if err != nil {
		log.Error(err, "unable to update deployment with 0 replicas")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, err
	}
	sleepInfo.Status.NextScheduleTime = v1.NewTime(nextSchedule)
	if err := r.Status().Update(ctx, sleepInfo); err != nil {
	}

	deploymentList, err := r.getDeploymentsByNamespace(ctx, req.Namespace)
	if err != nil {
		log.Error(err, "fails to fetch deployments")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, err
	}

	if len(deploymentList) != 0 {
		return ctrl.Result{
			RequeueAfter: requeueAfter,
		}, nil
	}

	err = r.updateDeploymentsWithZeroReplicas(ctx, deploymentList)
	if err != nil {
		log.Error(err, "fails to update deployments")
		return ctrl.Result{
			Requeue: true,
		}, err
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&kubegreenv1alpha1.SleepInfo{}).
		Complete(r)
}

func (r *SleepInfoReconciler) getNextSchedule(sleepInfo *kubegreenv1alpha1.SleepInfo) (time.Time, time.Duration, error) {
	sched, err := cron.ParseStandard(sleepInfo.Spec.SleepSchedule)
	if err != nil {
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return time.Time{}, 0, fmt.Errorf("sleep schedule not valid: %s", err)
	}
	now := r.Clock.Now()
	nextSchedule := sched.Next(now)

	// The 1 second added is due to avoid the operator to requeue twice spaced by some milliseconds.
	// TODO: add a better algorithm to correctly set requeue.
	requeueAfter := nextSchedule.Sub(now) + 1*time.Second

	return nextSchedule, requeueAfter, nil
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

func (r *SleepInfoReconciler) updateDeploymentsWithZeroReplicas(ctx context.Context, deployments []appsv1.Deployment) error {
	for _, deployment := range deployments {
		d := deployment.DeepCopy()
		*d.Spec.Replicas = 0
		if err := r.Client.Update(ctx, d); err != nil {
			return client.IgnoreNotFound(err)
		}
	}
	return nil
}
