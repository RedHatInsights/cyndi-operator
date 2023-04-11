package controllers

import (
	"context"
	"fmt"

	cyndi "github.com/RedHatInsights/cyndi-operator/api/v1alpha1"
	"github.com/RedHatInsights/cyndi-operator/controllers/database"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ValidationReconciler struct {
	CyndiPipelineReconciler

	// if true the Reconciler will check for pipeline state deviation
	// should always be true except for tests
	CheckResourceDeviation bool
}

func (r *ValidationReconciler) setup(reqLogger logr.Logger, request ctrl.Request, ctx context.Context) (ReconcileIteration, error) {
	i, err := r.CyndiPipelineReconciler.setup(reqLogger, request, ctx)

	if err != nil || i.Instance == nil {
		return i, err
	}

	i.GetRequeueInterval = func(i *ReconcileIteration) int64 {
		return i.getValidationConfig().Interval
	}

	i.InventoryDb = database.NewBaseDatabase(&i.HBIDBParams)

	if err = i.InventoryDb.Connect(); err != nil {
		return i, err
	}

	return i, err
}

func (r *ValidationReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	reqLogger := r.Log.WithValues("Pipeline", request.Name, "Namespace", request.Namespace)

	i, err := r.setup(reqLogger, request, ctx)
	if err != nil {
		return reconcile.Result{}, i.error(err)
	}
	defer i.Close()

	// nothing to validate
	if i.Instance == nil || i.Instance.GetState() == cyndi.STATE_REMOVED || i.Instance.GetState() == cyndi.STATE_NEW {
		return reconcile.Result{}, nil
	}

	reqLogger.Info("Validating CyndiPipeline")

	if r.CheckResourceDeviation {
		problem, err := i.checkForDeviation()
		if err != nil {
			return reconcile.Result{}, i.error(err, "Error checking for state deviation")
		} else if problem != nil {
			i.probeStateDeviationRefresh(problem.Error())
			i.Instance.TransitionToNew()
			return i.updateStatusAndRequeue()
		}
	}

	isValid, mismatchRatio, mismatchCount, hostCount, err := i.validate()
	if err != nil {
		return reconcile.Result{}, i.error(err, "Error validating pipeline")
	}

	reqLogger.Info("Validation finished", "isValid", isValid)

	if isValid {
		msg := fmt.Sprintf("%v hosts (%.2f%%) do not match", mismatchCount, mismatchRatio*100)

		if i.Instance.GetState() == cyndi.STATE_INVALID {
			i.eventNormal("Valid", "Pipeline is valid again")
		}

		i.Instance.SetValid(
			metav1.ConditionTrue,
			"ValidationSucceeded",
			fmt.Sprintf("Validation succeeded - %s", msg),
			hostCount,
		)
	} else {
		msg := fmt.Sprintf("Validation failed - %v hosts (%.2f%%) do not match", mismatchCount, mismatchRatio*100)

		i.Recorder.Event(i.Instance, corev1.EventTypeWarning, "ValidationFailed", msg)
		i.Instance.SetValid(
			metav1.ConditionFalse,
			"ValidationFailed",
			msg,
			hostCount,
		)
	}

	return i.updateStatusAndRequeue()
}

func eventFilterPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
				return true // Pipeline definition changed
			}

			oldPipeline, ok1 := e.ObjectOld.(*cyndi.CyndiPipeline)
			newPipeline, ok2 := e.ObjectNew.(*cyndi.CyndiPipeline)

			if ok1 && ok2 && !oldPipeline.Status.InitialSyncInProgress && newPipeline.Status.InitialSyncInProgress {
				return true // pipeline refresh happened - validate the new pipeline
			}

			return false
		},
	}
}

func (r *ValidationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("cyndi-validation").
		For(&cyndi.CyndiPipeline{}).
		WithEventFilter(eventFilterPredicate()).
		Complete(r)
}

func NewValidationReconciler(client client.Client, clientset *kubernetes.Clientset, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder, checkResourceDeviation bool) *ValidationReconciler {
	return &ValidationReconciler{
		CyndiPipelineReconciler: *NewCyndiReconciler(client, clientset, scheme, log, recorder),
		CheckResourceDeviation:  checkResourceDeviation,
	}
}
