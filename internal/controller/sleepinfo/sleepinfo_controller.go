/*
Copyright 2021.
*/

package sleepinfo

import (
	"context"
	"fmt"
	"strings"
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
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	lastScheduleKey               = "scheduled-at"
	lastOperationKey              = "operation-type"
	originalJSONPatchDataKey      = "original-resource-info"
	replicasBeforeSleepAnnotation = "sleepinfo.kube-green.com/replicas-before-sleep"

	sleepOperation  = "SLEEP"
	wakeUpOperation = "WAKE_UP"

	manualActionAnnotation   = "kube-green.stratio.com/manual-action"
	manualActionTimeAnnotion = "kube-green.stratio.com/manual-at"

	manualActionTTL = 5 * time.Minute
)

// SleepInfoReconciler reconciles a SleepInfo object
type SleepInfoReconciler struct {
	client.Client
	Clock
	Log                     logr.Logger
	Scheme                  *runtime.Scheme
	Metrics                 metrics.Metrics
	SleepDelta              int64
	ManagerName             string
	MaxConcurrentReconciles int
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

// Clock knows how to get the current time.
// It can be used to fake out timing for testing.
type Clock interface {
	Now() time.Time
}

// +kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kube-green.com,resources=sleepinfos/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *SleepInfoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("sleepinfo", req.NamespacedName)

	sleepInfo, err := r.getSleepInfo(ctx, req)
	if err != nil {
		log.V(8).Info("unable to fetch sleepInfo", "err", err.Error())
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
	now := r.Now()

	manualAction := ""
	manualActionAt := ""
	manualActionValid := false
	manualActionShouldClear := false
	if sleepInfo.Annotations != nil {
		manualAction = strings.ToLower(strings.TrimSpace(sleepInfo.Annotations[manualActionAnnotation]))
		manualActionAt = strings.TrimSpace(sleepInfo.Annotations[manualActionTimeAnnotion])
	}

	isToExecute, nextSchedule, requeueAfter, err := r.getNextSchedule(log, sleepInfoData, now)
	if err != nil {
		log.Error(err, "unable to update deployment with 0 replicas")
		return ctrl.Result{}, err
	}

	if manualAction == "sleep" || manualAction == "wake" {
		manualActionValid = true
		if manualActionAt != "" {
			parsedAt, err := time.Parse(time.RFC3339, manualActionAt)
			if err != nil {
				log.Info("manual action timestamp invalid, ignoring manual action", "manualAt", manualActionAt, "sleepinfo", sleepInfo.Name)
				manualActionValid = false
				manualActionShouldClear = true
			} else if now.Sub(parsedAt) > manualActionTTL {
				log.Info("manual action expired, ignoring manual action", "manualAt", manualActionAt, "sleepinfo", sleepInfo.Name)
				manualActionValid = false
				manualActionShouldClear = true
			}
		}
	}

	if manualActionValid {
		if manualAction == "sleep" {
			sleepInfoData.CurrentOperationType = sleepOperation
		} else {
			sleepInfoData.CurrentOperationType = wakeUpOperation
		}
		isToExecute = true
		log.Info("manual action requested", "action", manualAction, "sleepinfo", sleepInfo.Name)
	}
	scheduleLog := log.WithValues("now", r.Now(), "next run", nextSchedule, "requeue", requeueAfter)

	if !isToExecute {
		scheduleLog.Info("skip execution")
		return ctrl.Result{
			RequeueAfter: requeueAfter,
		}, nil
	}
	scheduleLog.WithValues("last schedule", now, "status", sleepInfo.Status).Info("last schedule value")

	// EXTENSIÓN: Si es operación WAKE_UP, buscar restore patches de SleepInfos relacionados
	restorePatches := sleepInfoData.OriginalGenericResourceInfo
	if sleepInfoData.IsWakeUpOperation() {
		relatedPatches, err := getRelatedRestorePatches(ctx, r.Client, log, sleepInfo, req.Namespace)
		if err != nil {
			log.Error(err, "failed to get related restore patches, using current ones")
		} else if relatedPatches != nil && len(relatedPatches) > 0 {
			// Combinar restore patches: primero los relacionados, luego los del actual (el actual tiene prioridad)
			log.Info("usando restore patches de SleepInfo relacionado", "count", len(relatedPatches))
			if restorePatches == nil {
				restorePatches = make(map[string]jsonpatch.RestorePatches)
			}
			for key, patches := range relatedPatches {
				if _, exists := restorePatches[key]; !exists {
					restorePatches[key] = patches
				}
			}
		}
	}
	if sleepInfoData.IsWakeUpOperation() && len(restorePatches) == 0 {
		backupPatches, err := r.getEmergencyRestorePatches(ctx, sleepInfo, req.Namespace)
		if err != nil {
			log.Error(err, "failed to get emergency restore patches")
		} else if backupPatches != nil && len(backupPatches) > 0 {
			restorePatches = backupPatches
			log.Info("using emergency restore patches", "count", len(backupPatches))
		}
	}

	// EXTENSIÓN: Agregar patches dinámicos para PgCluster, HDFSCluster, OsCluster y KafkaCluster según operación
	// Los patches de anotaciones dependen de si es SLEEP (shutdown=true) o WAKE (shutdown=false)
	sleepInfoWithPatches := sleepInfo.DeepCopy()
	if sleepInfoData.IsSleepOperation() {
		if sleepInfo.IsPostgresToSuspend() {
			sleepInfoWithPatches.Spec.Patches = append(sleepInfoWithPatches.Spec.Patches, kubegreenv1alpha1.PgclusterSleepPatch)
			log.Info("added pgcluster sleep patch", "sleepinfo", sleepInfo.GetName(), "namespace", req.Namespace)
		}
		if sleepInfo.IsHdfsToSuspend() {
			sleepInfoWithPatches.Spec.Patches = append(sleepInfoWithPatches.Spec.Patches, kubegreenv1alpha1.HdfsclusterSleepPatch)
			log.Info("added hdfscluster sleep patch", "sleepinfo", sleepInfo.GetName(), "namespace", req.Namespace)
		}
		if sleepInfo.IsOpenSearchToSuspend() {
			sleepInfoWithPatches.Spec.Patches = append(sleepInfoWithPatches.Spec.Patches, kubegreenv1alpha1.OsclusterSleepPatch)
			log.Info("added oscluster sleep patch", "sleepinfo", sleepInfo.GetName(), "namespace", req.Namespace)
		}
		if sleepInfo.IsKafkaToSuspend() {
			sleepInfoWithPatches.Spec.Patches = append(sleepInfoWithPatches.Spec.Patches, kubegreenv1alpha1.KafkaclusterSleepPatch)
			log.Info("added kafkacluster sleep patch", "sleepinfo", sleepInfo.GetName(), "namespace", req.Namespace)
		}
	} else if sleepInfoData.IsWakeUpOperation() {
		if sleepInfo.IsPostgresToSuspend() {
			sleepInfoWithPatches.Spec.Patches = append(sleepInfoWithPatches.Spec.Patches, kubegreenv1alpha1.PgclusterWakePatch)
			log.Info("added pgcluster wake patch", "sleepinfo", sleepInfo.GetName(), "namespace", req.Namespace)
		}
		if sleepInfo.IsHdfsToSuspend() {
			sleepInfoWithPatches.Spec.Patches = append(sleepInfoWithPatches.Spec.Patches, kubegreenv1alpha1.HdfsclusterWakePatch)
			log.Info("added hdfscluster wake patch", "sleepinfo", sleepInfo.GetName(), "namespace", req.Namespace)
		}
		if sleepInfo.IsOpenSearchToSuspend() {
			sleepInfoWithPatches.Spec.Patches = append(sleepInfoWithPatches.Spec.Patches, kubegreenv1alpha1.OsclusterWakePatch)
			log.Info("added oscluster wake patch", "sleepinfo", sleepInfo.GetName(), "namespace", req.Namespace)
		}
		if sleepInfo.IsKafkaToSuspend() {
			sleepInfoWithPatches.Spec.Patches = append(sleepInfoWithPatches.Spec.Patches, kubegreenv1alpha1.KafkaclusterWakePatch)
			log.Info("added kafkacluster wake patch", "sleepinfo", sleepInfo.GetName(), "namespace", req.Namespace)
		}
	}

	resources, err := jsonpatch.NewResources(ctx, resource.ResourceClient{
		Client:           r.Client,
		SleepInfo:        sleepInfoWithPatches,
		Log:              log,
		FieldManagerName: r.ManagerName,
	}, req.Namespace, restorePatches)
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

		logMsg := "deployments, statefulsets and cronjobs not present in namespace"
		if !sleepInfo.IsCronjobsToSuspend() && !sleepInfo.IsDeploymentsToSuspend() && !sleepInfo.IsStatefulSetsToSuspend() {
			logMsg = "deployments, statefulsets and cronjobs are not to suspend"
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

	if manualActionValid || manualActionShouldClear {
		if err := r.clearManualAction(ctx, sleepInfo); err != nil {
			log.Error(err, "failed to clear manual action annotation")
		}
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

	pred := predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return false
			}
			if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
				return true
			}
			oldAnn := e.ObjectOld.GetAnnotations()
			newAnn := e.ObjectNew.GetAnnotations()
			oldAction := ""
			newAction := ""
			if oldAnn != nil {
				oldAction = strings.ToLower(strings.TrimSpace(oldAnn[manualActionAnnotation]))
			}
			if newAnn != nil {
				newAction = strings.ToLower(strings.TrimSpace(newAnn[manualActionAnnotation]))
			}
			if newAction != oldAction && (newAction == "sleep" || newAction == "wake") {
				return true
			}
			if newAction != "" && oldAnn != nil && newAnn != nil &&
				newAnn[manualActionTimeAnnotion] != oldAnn[manualActionTimeAnnotion] {
				return true
			}
			return false
		},
		DeleteFunc: func(event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubegreenv1alpha1.SleepInfo{}).
		Named("kubegreen-sleepinfo").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.MaxConcurrentReconciles,
		}).
		WithEventFilter(pred).
		Complete(r)
}

func (r *SleepInfoReconciler) clearManualAction(ctx context.Context, sleepInfo *kubegreenv1alpha1.SleepInfo) error {
	key := client.ObjectKeyFromObject(sleepInfo)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &kubegreenv1alpha1.SleepInfo{}
		if err := r.Get(ctx, key, latest); err != nil {
			return err
		}
		if latest.Annotations == nil {
			return nil
		}
		if _, exists := latest.Annotations[manualActionAnnotation]; !exists {
			return nil
		}
		delete(latest.Annotations, manualActionAnnotation)
		delete(latest.Annotations, manualActionTimeAnnotion)
		return r.Update(ctx, latest)
	})
}

func (r *SleepInfoReconciler) getSleepInfo(ctx context.Context, req ctrl.Request) (*kubegreenv1alpha1.SleepInfo, error) {
	sleepInfo := &kubegreenv1alpha1.SleepInfo{}
	if err := r.Get(ctx, req.NamespacedName, sleepInfo); err != nil {
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
