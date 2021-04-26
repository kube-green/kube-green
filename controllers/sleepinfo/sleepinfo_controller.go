/*
Copyright 2021.
*/

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	lastScheduleKey               = "scheduled-at"
	lastOperationKey              = "operation-type"
	replicasBeforeSleepKey        = "deployment-replicas"
	replicasBeforeSleepAnnotation = "sleepinfo.kube-green.com/replicas-before-sleep"

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
type SleepInfoData struct {
	LastSchedule                time.Time        `json:"lastSchedule"`
	CurrentOperationType        string           `json:"operationType"`
	OriginalDeploymentsReplicas map[string]int32 `json:"originalDeploymentReplicas"`
	CurrentOperationSchedule    string           `json:"-"`
	NextOperationSchedule       string           `json:"-"`
}

func (s SleepInfoData) isWakeUpOperation() bool {
	return s.CurrentOperationType == wakeUpOperation
}

func (s SleepInfoData) isSleepOperation() bool {
	return s.CurrentOperationType == sleepOperation
}

//+kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

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

	isToExecute, nextSchedule, requeueAfter, err := r.getNextSchedule(sleepInfoData, now)
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

	deploymentList, err := r.getDeploymentsByNamespace(ctx, req.Namespace)
	if err != nil {
		log.Error(err, "fails to fetch deployments")
		return ctrl.Result{}, err
	}

	if err := r.handleSleepInfoStatus(ctx, now, sleepInfo, sleepInfoData, deploymentList); err != nil {
		log.Error(err, "unable to update sleepInfo status")
		return ctrl.Result{}, err
	}

	logSecret := log.WithValues("secret", secretName)
	if len(deploymentList) == 0 {
		if err = r.upsertSecret(ctx, log, now, secretName, req.Namespace, secret, sleepInfoData, deploymentList); err != nil {
			logSecret.Error(err, "fails to update secret")
			return ctrl.Result{
				Requeue: true,
			}, nil
		}

		if sleepInfoData.isSleepOperation() {
			requeueAfter, err = skipWakeUpIfSleepNotPerformed(sleepInfoData.CurrentOperationSchedule, nextSchedule, now)
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

	if err = r.upsertSecret(ctx, log, now, secretName, req.Namespace, secret, sleepInfoData, deploymentList); err != nil {
		logSecret.Error(err, "fails to update secret")
		return ctrl.Result{
			Requeue: true,
		}, nil
	}

	switch {
	case sleepInfoData.isSleepOperation():
		if err := r.handleSleep(log, ctx, deploymentList); err != nil {
			log.Error(err, "fails to handle sleep")
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	case sleepInfoData.isWakeUpOperation():
		if err := r.handleWakeUp(log, ctx, deploymentList, sleepInfoData); err != nil {
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

	// TODO: validate schedule

	pred := predicate.GenerationChangedPredicate{}
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubegreenv1alpha1.SleepInfo{}).
		WithEventFilter(pred).
		// Owns(). // TODO: handle secrets property
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

func (r *SleepInfoReconciler) getSecret(ctx context.Context, secretName, namespaceName string) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: namespaceName,
		Name:      secretName,
	}, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
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

func getSleepInfoData(secret *v1.Secret, sleepInfo *kubegreenv1alpha1.SleepInfo) (SleepInfoData, error) {
	sleepSchedule, err := getScheduleFromWeekdayAndTime(sleepInfo.Spec.Weekdays, sleepInfo.Spec.SleepTime)
	if err != nil {
		return SleepInfoData{}, err
	}
	var wakeUpSchedule string
	if sleepInfo.Spec.WakeUpTime != "" {
		wakeUpSchedule, err = getScheduleFromWeekdayAndTime(sleepInfo.Spec.Weekdays, sleepInfo.Spec.WakeUpTime)
		if err != nil {
			return SleepInfoData{}, err
		}
	}

	secretData := SleepInfoData{
		CurrentOperationType:     sleepOperation,
		CurrentOperationSchedule: sleepSchedule,
		NextOperationSchedule:    wakeUpSchedule,
	}
	if wakeUpSchedule == "" {
		secretData.NextOperationSchedule = sleepSchedule
	}

	if secret == nil || secret.Data == nil {
		return secretData, nil
	}

	data := secret.Data
	originalDeploymentsReplicas := []OriginalDeploymentReplicas{}
	if data[replicasBeforeSleepKey] != nil {
		if err := json.Unmarshal(data[replicasBeforeSleepKey], &originalDeploymentsReplicas); err != nil {
			return SleepInfoData{}, err
		}
		originalDeploymentsReplicasData := map[string]int32{}
		for _, replicaInfo := range originalDeploymentsReplicas {
			originalDeploymentsReplicasData[replicaInfo.Name] = replicaInfo.Replicas
		}
		secretData.OriginalDeploymentsReplicas = originalDeploymentsReplicasData
	}

	lastSchedule, err := time.Parse(time.RFC3339, string(data[lastScheduleKey]))
	if err != nil {
		return SleepInfoData{}, fmt.Errorf("fails to parse %s: %s", lastScheduleKey, err)
	}
	secretData.LastSchedule = lastSchedule

	lastOperation := string(data[lastOperationKey])

	if lastOperation == sleepOperation && wakeUpSchedule != "" {
		secretData.CurrentOperationSchedule = wakeUpSchedule
		secretData.NextOperationSchedule = sleepSchedule
		secretData.CurrentOperationType = wakeUpOperation
	}

	return secretData, nil
}

func getSecretName(name string) string {
	return fmt.Sprintf("sleepinfo-%s", name)
}

func getScheduleFromWeekdayAndTime(weekday string, hourAndMinute string) (string, error) {
	if weekday == "" {
		return "", fmt.Errorf("empty weekday from sleep info configuration")
	}

	splittedTime := strings.Split(hourAndMinute, ":")
	if len(splittedTime) != 2 {
		return "", fmt.Errorf("time should be of format HH:mm, actual: %s", hourAndMinute)
	}
	return fmt.Sprintf("%s %s * * %s", splittedTime[1], splittedTime[0], weekday), nil
}

// handleSleepInfoStatus handles operator status
func (r SleepInfoReconciler) handleSleepInfoStatus(
	ctx context.Context,
	now time.Time,
	currentSleepInfo *kubegreenv1alpha1.SleepInfo,
	sleepInfoData SleepInfoData,
	deploymentList []appsv1.Deployment,
) error {
	sleepInfo := currentSleepInfo.DeepCopy()
	sleepInfo.Status.LastScheduleTime = metav1.NewTime(now)
	sleepInfo.Status.OperationType = sleepInfoData.CurrentOperationType
	if len(deploymentList) == 0 {
		sleepInfo.Status.OperationType = ""
	}
	return r.Status().Update(ctx, sleepInfo)
}

func (r SleepInfoReconciler) upsertSecret(
	ctx context.Context,
	logger logr.Logger,
	now time.Time,
	secretName, namespace string,
	secret *v1.Secret,
	sleepInfoData SleepInfoData,
	deploymentList []appsv1.Deployment,
) error {
	logger.Info("update secret")

	var newSecret = &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: make(map[string][]byte),
	}
	if secret != nil {
		newSecret = secret.DeepCopy()
	}
	if newSecret.StringData == nil {
		newSecret.StringData = map[string]string{}
	}
	newSecret.StringData[lastScheduleKey] = now.Format(time.RFC3339)
	newSecret.StringData[lastOperationKey] = sleepInfoData.CurrentOperationType
	if len(deploymentList) == 0 {
		delete(newSecret.StringData, lastOperationKey)
	}

	if len(deploymentList) != 0 && sleepInfoData.isSleepOperation() {
		originalDeploymentsReplicas := []OriginalDeploymentReplicas{}
		for _, deployment := range deploymentList {
			replica, ok := sleepInfoData.OriginalDeploymentsReplicas[deployment.Name]
			originalReplicas := *deployment.Spec.Replicas
			if ok && replica != 0 {
				originalReplicas = replica
			}
			if originalReplicas == 0 {
				continue
			}
			originalDeploymentsReplicas = append(originalDeploymentsReplicas, OriginalDeploymentReplicas{
				Name:     deployment.Name,
				Replicas: originalReplicas,
			})
		}
		originalReplicasToSave, err := json.Marshal(originalDeploymentsReplicas)
		if err != nil {
			return err
		}
		newSecret.Data[replicasBeforeSleepKey] = originalReplicasToSave
	}
	if sleepInfoData.isWakeUpOperation() {
		delete(newSecret.Data, replicasBeforeSleepKey)
	}

	if secret == nil {
		if err := r.Client.Create(ctx, newSecret); err != nil {
			return err
		}
		logger.Info("secret created")
	} else {
		if err := r.Client.Update(ctx, newSecret); err != nil {
			return err
		}
		logger.Info("secret updated")
	}
	return nil
}
