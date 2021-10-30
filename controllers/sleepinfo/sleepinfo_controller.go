/*
Copyright 2021.
*/

package sleepinfo

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
)

const (
	lastScheduleKey                = "scheduled-at"
	lastOperationKey               = "operation-type"
	replicasBeforeSleepKey         = "deployment-replicas"
	suspendedCronJobBeforeSleepKey = "original-cronjobs"
	replicasBeforeSleepAnnotation  = "sleepinfo.kube-green.com/replicas-before-sleep"

	sleepOperation  = "SLEEP"
	wakeUpOperation = "WAKE_UP"
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

type OriginalDeploymentReplicas struct {
	Name     string `json:"name"`
	Replicas int32  `json:"replicas"`
}
type OriginalSuspendedCronJob struct {
	Name    string `json:"name"`
	Suspend bool   `json:"suspend"`
}
type SleepInfoData struct {
	LastSchedule                time.Time        `json:"lastSchedule"`
	CurrentOperationType        string           `json:"operationType"`
	OriginalDeploymentsReplicas map[string]int32 `json:"originalDeploymentReplicas"`
	CurrentOperationSchedule    string           `json:"-"`
	NextOperationSchedule       string           `json:"-"`
	SuspendCronjobs             bool             `json:"-"`
}

func (s SleepInfoData) isWakeUpOperation() bool {
	return s.CurrentOperationType == wakeUpOperation
}

func (s SleepInfoData) isSleepOperation() bool {
	return s.CurrentOperationType == sleepOperation
}

var sleepDelta int64 = 60

//+kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=cronjob,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.1/pkg/reconcile
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

	secretName := getSecretName(req.Name)
	secret, err := r.getSecret(ctx, secretName, req.Namespace)
	if client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to fetch namespace", "namespaceName", req.Namespace)
		return ctrl.Result{}, err
	}
	sleepInfoData, err := getSleepInfoData(sleepInfoSecret{Secret: secret}, sleepInfo)
	if err != nil {
		log.Error(err, "unable to get secret data")
		return ctrl.Result{}, err
	}
	now := r.Clock.Now()

	isToExecute, nextSchedule, requeueAfter, err := r.getNextSchedule(sleepInfoData, now, sleepDelta)
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

	deploymentList, err := r.getDeploymentsList(ctx, req.Namespace, sleepInfo)
	if err != nil {
		log.Error(err, "fails to fetch deployments")
		return ctrl.Result{}, err
	}
	cronJobList, err := r.getCronJobList(ctx, req.Namespace, sleepInfo)
	if err != nil {
		log.Error(err, "fails to fetch cronjobs")
		return ctrl.Result{}, err
	}

	resources := Resources{
		Deployments: deploymentList,
		CronJobs:    cronJobList,
	}

	if err := r.handleSleepInfoStatus(ctx, now, sleepInfo, sleepInfoData, resources); err != nil {
		log.Error(err, "unable to update sleepInfo status")
		return ctrl.Result{}, err
	}
	log.V(1).Info("update status info")

	logSecret := log.WithValues("secret", secretName)
	if err = r.upsertSecret(ctx, log, now, secretName, req.Namespace, secret, sleepInfoData, resources); err != nil {
		logSecret.Error(err, "fails to update secret")
		return ctrl.Result{
			Requeue: true,
		}, nil
	}
	if !resources.hasResources() {
		if sleepInfoData.isSleepOperation() {
			requeueAfter, err = skipWakeUpIfSleepNotPerformed(sleepInfoData.CurrentOperationSchedule, nextSchedule, now)
			if err != nil {
				log.Error(err, "fails to parse cron - 0 deployment")
				return ctrl.Result{}, nil
			}
		}

		log.WithValues("requeueAfter", requeueAfter).Info("deployment not present in namespace")
		return ctrl.Result{
			RequeueAfter: requeueAfter,
		}, nil
	}

	switch {
	case sleepInfoData.isSleepOperation():
		if err := r.handleSleep(log, ctx, resources); err != nil {
			log.Error(err, "fails to handle sleep")
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	case sleepInfoData.isWakeUpOperation():
		if err := r.handleWakeUp(log, ctx, resources, sleepInfoData); err != nil {
			log.Error(err, "fails to handle wake up")
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	default:
		return ctrl.Result{}, fmt.Errorf("operation %s not supported", sleepInfoData.CurrentOperationType)
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
			MaxConcurrentReconciles: 10,
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

func getSleepInfoData(secret sleepInfoSecret, sleepInfo *kubegreenv1alpha1.SleepInfo) (SleepInfoData, error) {
	sleepSchedule, err := sleepInfo.GetSleepSchedule()
	if err != nil {
		return SleepInfoData{}, err
	}
	wakeUpSchedule, err := sleepInfo.GetWakeUpSchedule()
	if err != nil {
		return SleepInfoData{}, err
	}

	sleepInfoData := SleepInfoData{
		CurrentOperationType:     sleepOperation,
		CurrentOperationSchedule: sleepSchedule,
		NextOperationSchedule:    wakeUpSchedule,
		SuspendCronjobs:          sleepInfo.IsCronjobsToSuspend(),
	}
	if wakeUpSchedule == "" {
		sleepInfoData.NextOperationSchedule = sleepSchedule
	}

	if secret.Secret == nil || secret.Data == nil {
		return sleepInfoData, nil
	}

	originalDeploymentReplicas, err := secret.getOriginalDeploymentReplicas()
	if err != nil {
		return SleepInfoData{}, err
	}
	sleepInfoData.OriginalDeploymentsReplicas = originalDeploymentReplicas

	lastSchedule, err := time.Parse(time.RFC3339, secret.getLastSchedule())
	if err != nil {
		return SleepInfoData{}, fmt.Errorf("fails to parse %s: %s", lastScheduleKey, err)
	}
	sleepInfoData.LastSchedule = lastSchedule

	lastOperation := secret.getLastOperation()

	if lastOperation == sleepOperation && wakeUpSchedule != "" {
		sleepInfoData.CurrentOperationSchedule = wakeUpSchedule
		sleepInfoData.NextOperationSchedule = sleepSchedule
		sleepInfoData.CurrentOperationType = wakeUpOperation
	}

	return sleepInfoData, nil
}

// handleSleepInfoStatus handles operator status
func (r SleepInfoReconciler) handleSleepInfoStatus(
	ctx context.Context,
	now time.Time,
	currentSleepInfo *kubegreenv1alpha1.SleepInfo,
	sleepInfoData SleepInfoData,
	resources Resources,
) error {
	sleepInfo := currentSleepInfo.DeepCopy()
	sleepInfo.Status.LastScheduleTime = metav1.NewTime(now)
	sleepInfo.Status.OperationType = sleepInfoData.CurrentOperationType
	if !resources.hasResources() {
		sleepInfo.Status.OperationType = ""
	}
	return r.Status().Update(ctx, sleepInfo)
}
