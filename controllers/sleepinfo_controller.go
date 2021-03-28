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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
)

const (
	lastScheduledAnnotation = "kube-green.com/scheduled-at"
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

	deploymentList, err := r.getDeploymentsByNamespace(ctx, req.Namespace)
	if err != nil {
		log.Error(err, "fails to fetch deployments")
		return ctrl.Result{}, err
	}

	lastSchedule, err := r.getLastScheduledAnnotation(deploymentList)
	if err != nil {
		log.Error(err, "last schedule is invalid")
		return ctrl.Result{}, err
	}
	if lastSchedule != nil {
		log.WithValues("last schedule", lastSchedule, "status", sleepInfo.Status).Info("last schedule value")
		sleepInfo.Status.LastScheduleTime = metav1.NewTime(*lastSchedule)
		if err := r.Status().Update(ctx, sleepInfo); err != nil {
			log.Error(err, "unable to update sleepInfo status")
			return ctrl.Result{}, err
		}
	}

	now := r.Clock.Now()
	nextSchedule, requeueAfter, err := r.getNextSchedule(sleepInfo, now)
	if err != nil {
		log.Error(err, "unable to update deployment with 0 replicas")
		return ctrl.Result{}, err
	}
	log = log.WithValues("now", r.Now(), "next run", nextSchedule, "requeue", requeueAfter)

	if len(deploymentList) == 0 {
		log.Info("deployment not present in namespace")
		return ctrl.Result{
			RequeueAfter: requeueAfter,
		}, nil
	}

	err = r.updateDeploymentsWithZeroReplicas(ctx, deploymentList, now)
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

func (r *SleepInfoReconciler) getNextSchedule(sleepInfo *kubegreenv1alpha1.SleepInfo, now time.Time) (time.Time, time.Duration, error) {
	sched, err := cron.ParseStandard(sleepInfo.Spec.SleepSchedule)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("sleep schedule not valid: %s", err)
	}

	var earliestTime time.Time
	if !sleepInfo.Status.LastScheduleTime.IsZero() {
		earliestTime = sleepInfo.Status.LastScheduleTime.Time
	} else {
		earliestTime = now
	}
	nextSchedule := sched.Next(earliestTime)
	if nextSchedule.Before(now) {
		nextSchedule = sched.Next(now)
	}
	requeueAfter := nextSchedule.Sub(now) + 1*time.Second

	// The 1 second added is due to avoid the operator to requeue twice spaced by some milliseconds.
	// TODO: add a better algorithm to correctly set requeue.
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

func (r *SleepInfoReconciler) getLastScheduledAnnotation(deployments []appsv1.Deployment) (*time.Time, error) {
	var mostRecentTime *time.Time = nil
	for _, deployment := range deployments {
		timeString := deployment.GetAnnotations()[lastScheduledAnnotation]
		if timeString == "" {
			continue
		}
		timeParsed, err := time.Parse(time.RFC3339, timeString)
		if err != nil {
			return nil, fmt.Errorf("last schedule invalid for deployment %s: %s", deployment.Name, err)
		}
		if mostRecentTime == nil || mostRecentTime.Before(timeParsed) {
			mostRecentTime = &timeParsed
		}
	}
	return mostRecentTime, nil
}

func (r *SleepInfoReconciler) updateDeploymentsWithZeroReplicas(ctx context.Context, deployments []appsv1.Deployment, now time.Time) error {
	for _, deployment := range deployments {
		if *deployment.Spec.Replicas == 0 {
			continue
		}
		d := deployment.DeepCopy()
		*d.Spec.Replicas = 0
		annotations := d.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[lastScheduledAnnotation] = now.Format(time.RFC3339)
		d.SetAnnotations(annotations)
		if err := r.Client.Update(ctx, d); err != nil {
			// multiple update to a deployment could cause conflict. We ignore them.
			// || errors.IsConflict(err)
			if client.IgnoreNotFound(err) == nil {
				return nil
			}
			return err
		}
	}
	return nil
}
