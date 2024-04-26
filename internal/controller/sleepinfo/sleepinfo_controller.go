/*
Copyright 2021.
*/

package sleepinfo

import (
	"context"
	"fmt"
	"time"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/jsonpatch"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/metrics"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	lastScheduleKey               = "scheduled-at"
	lastOperationKey              = "operation-type"
	originalJSONPatchDataKey      = "original-resource-info"
	replicasBeforeSleepAnnotation = "sleepinfo.kube-green.com/replicas-before-sleep"

	sleepOperation  = "SLEEP"
	wakeUpOperation = "WAKE_UP"

	fieldManagerName = "kube-green"
)

// SleepInfoReconciler reconciles a SleepInfo object
type SleepInfoReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Clock
	Metrics    metrics.Metrics
	SleepDelta int64
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
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
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
		if apierrors.IsNotFound(err) {
			r.Metrics.CurrentSleepInfo.Delete(prometheus.Labels{
				"name":      req.Name,
				"namespace": req.Namespace,
			})
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	r.Metrics.CurrentSleepInfo.With(prometheus.Labels{
		"name":      req.Name,
		"namespace": req.Namespace,
	}).Set(1)

	secretName := getSecretName(req.Name)
	secret, err := r.getSecret(ctx, secretName, req.Namespace)
	if client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to fetch namespace", "namespaceName", req.Namespace)
		return ctrl.Result{}, err
	}
	sleepInfoData, err := getSleepInfoData(secret, sleepInfo)
	if err != nil {
		log.Error(err, "unable to get secret data")
		return ctrl.Result{}, err
	}
	now := r.Clock.Now()

	isToExecute, nextSchedule, requeueAfter, err := r.getNextSchedule(log, sleepInfoData, now)
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
	scheduleLog.WithValues("last schedule", now, "status", sleepInfo.Status).Info("last schedule value")

	resources, err := jsonpatch.NewResources(ctx, resource.ResourceClient{
		Client:           r.Client,
		SleepInfo:        sleepInfo,
		Log:              log,
		FieldManagerName: fieldManagerName,
	}, req.Namespace, sleepInfoData.OriginalGenericResourceInfo)
	if err != nil {
		log.Error(err, "fails to get resources")
		return ctrl.Result{}, err
	}

	if err := r.handleSleepInfoStatus(ctx, now, sleepInfo, sleepInfoData.CurrentOperationType, resources); err != nil {
		log.Error(err, "unable to update sleepInfo status")
		return ctrl.Result{}, err
	}
	log.V(8).Info("update status info")

	logSecret := log.WithValues("secret", secretName)
	if !resources.HasResource() {
		if err = r.upsertSecret(ctx, log, now, secretName, req.Namespace, sleepInfo, secret, sleepInfoData, resources); err != nil {
			logSecret.Error(err, "fails to update secret")
			return ctrl.Result{
				Requeue: true,
			}, nil
		}

		if sleepInfoData.IsSleepOperation() {
			requeueAfter, err = skipWakeUpIfSleepNotPerformed(sleepInfoData.CurrentOperationSchedule, nextSchedule, now)
			if err != nil {
				log.Error(err, "fails to parse cron - 0 deployment")
				return ctrl.Result{}, nil
			}
		}

		logMsg := "deployments and cronjobs not present in namespace"
		if !sleepInfo.IsCronjobsToSuspend() && !sleepInfo.IsDeploymentsToSuspend() {
			logMsg = "deployments and cronjobs are not to suspend"
		}
		log.WithValues("requeueAfter", requeueAfter).Info(logMsg)

		return ctrl.Result{
			RequeueAfter: requeueAfter,
		}, nil
	}

	switch {
	case sleepInfoData.IsSleepOperation():
		if err := resources.Sleep(ctx); err != nil {
			log.Error(err, "fails to handle sleep")
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	case sleepInfoData.IsWakeUpOperation():
		if err := resources.WakeUp(ctx); err != nil {
			log.Error(err, "fails to handle wake up")
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	default:
		return ctrl.Result{}, fmt.Errorf("operation %s not supported", sleepInfoData.CurrentOperationType)
	}

	if err = r.upsertSecret(ctx, log, now, secretName, req.Namespace, sleepInfo, secret, sleepInfoData, resources); err != nil {
		logSecret.Error(err, "fails to update secret")
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

	pred := predicate.GenerationChangedPredicate{}
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubegreenv1alpha1.SleepInfo{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 20,
		}).
		WithEventFilter(pred).
		Complete(r)
}

func (r *SleepInfoReconciler) getSleepInfo(ctx context.Context, req ctrl.Request) (*kubegreenv1alpha1.SleepInfo, error) {
	sleepInfo := &kubegreenv1alpha1.SleepInfo{}
	if err := r.Client.Get(ctx, req.NamespacedName, sleepInfo); err != nil {
		return nil, err
	}
	return sleepInfo, nil
}

func skipWakeUpIfSleepNotPerformed(currentOperationCronSchedule string, nextSchedule, now time.Time) (time.Duration, error) {
	nextOpSched, err := getCronParsed(currentOperationCronSchedule)
	if err != nil {
		return 0, fmt.Errorf("fails to parse cron current schedule: %s", err)
	}
	requeueAfter := getRequeueAfter(nextOpSched.Next(nextSchedule), now)

	return requeueAfter, nil
}

// handleSleepInfoStatus handles operator status
func (r SleepInfoReconciler) handleSleepInfoStatus(
	ctx context.Context,
	now time.Time,
	currentSleepInfo *kubegreenv1alpha1.SleepInfo,
	currentOperationType string,
	resources resource.Resource,
) error {
	sleepInfo := currentSleepInfo.DeepCopy()
	sleepInfo.Status.LastScheduleTime = metav1.NewTime(now)
	sleepInfo.Status.OperationType = currentOperationType
	if !resources.HasResource() {
		sleepInfo.Status.OperationType = ""
	}
	return r.Status().Update(ctx, sleepInfo)
}
