package controllers

import (
	"context"
	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"cyndi-operator/controllers/database"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

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

func (r *ValidationReconciler) setup(reqLogger logr.Logger, request ctrl.Request) (ReconcileIteration, error) {
	i, err := r.CyndiPipelineReconciler.setup(reqLogger, request)

	if err != nil || i.Instance == nil {
		return i, err
	}

	i.GetRequeueInterval = func(i *ReconcileIteration) int64 {
		return i.getValidationConfig().Interval
	}

	i.InventoryDb = database.NewBaseDatabase(&i.HBIDBParams)

	if err = i.InventoryDb.Connect(); err != nil {
		return i, i.errorWithEvent("Error while connecting to HBI DB.", err)
	}

	return i, err
}

func (r *ValidationReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	i, err := r.setup(reqLogger, request)
	defer i.Close()

	if err != nil {
		return reconcile.Result{}, err
	}

	// nothing to validate
	if i.Instance == nil || i.Instance.GetState() == cyndiv1beta1.STATE_REMOVED || i.Instance.GetState() == cyndiv1beta1.STATE_NEW {
		return reconcile.Result{}, nil
	}

	reqLogger.Info("Validating CyndiPipeline")

	if r.CheckResourceDeviation {
		problem, err := i.checkForDeviation()
		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error validating dependencies", err)
		} else if problem != nil {
			i.Log.Info("Refreshing pipeline due to state deviation", "reason", problem)
			i.Instance.TransitionToNew()
			return i.updateStatusAndRequeue()
		}
	}

	isValid, mismatchRatio, mismatchCount, hostCount, err := i.validate()
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error validating pipeline", err)
	}

	reqLogger.Info("Validation finished", "isValid", isValid)

	if isValid {
		i.Instance.SetValid(
			metav1.ConditionTrue,
			"ValidationSucceeded",
			fmt.Sprintf(
				"Validation succeeded - %v hosts (%.2f%%) do not match which is below the threshold for invalid pipeline (%d%%)",
				mismatchCount,
				mismatchRatio*100,
				i.getValidationConfig().PercentageThreshold,
			),
			hostCount,
		)
	} else {
		i.Instance.SetValid(
			metav1.ConditionFalse,
			"ValidationFailed",
			fmt.Sprintf(
				"Validation failed - %v hosts (%.2f%%) do not match which is above the threshold for invalid pipeline (%d%%)",
				mismatchCount,
				mismatchRatio*100,
				i.getValidationConfig().PercentageThreshold,
			),
			hostCount,
		)
	}

	return i.updateStatusAndRequeue()
}

func eventFilterPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.MetaOld.GetGeneration() != e.MetaNew.GetGeneration() {
				return true // Pipeline definition changed
			}

			old, ok1 := e.ObjectOld.(*cyndiv1beta1.CyndiPipeline)
			new, ok2 := e.ObjectNew.(*cyndiv1beta1.CyndiPipeline)

			if ok1 && ok2 && old.Status.InitialSyncInProgress == false && new.Status.InitialSyncInProgress == true {
				return true // pipeline refresh happened - validate the new pipeline
			}

			return false
		},
	}
}

func (r *ValidationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cyndiv1beta1.CyndiPipeline{}).
		WithEventFilter(eventFilterPredicate()).
		Complete(r)
}

func NewValidationReconciler(client client.Client, clientset *kubernetes.Clientset, scheme *runtime.Scheme, log logr.Logger, checkResourceDeviation bool) *ValidationReconciler {
	return &ValidationReconciler{
		CyndiPipelineReconciler: CyndiPipelineReconciler{
			Client:    client,
			Clientset: clientset,
			Log:       log,
			Scheme:    scheme,
		},
		CheckResourceDeviation: checkResourceDeviation,
	}
}
