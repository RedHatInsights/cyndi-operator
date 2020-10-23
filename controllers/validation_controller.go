package controllers

import (
	"context"
	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"cyndi-operator/controllers/database"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ValidationReconciler struct {
	CyndiPipelineReconciler
}

func (r *ValidationReconciler) setup(reqLogger logr.Logger, request ctrl.Request) (ReconcileIteration, error) {
	i, err := r.CyndiPipelineReconciler.setup(reqLogger, request)

	if err != nil || i.Instance == nil {
		return i, err
	}

	i.InventoryDb = database.NewDatabase(&i.HBIDBParams)

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
	if i.Instance == nil || i.Instance.GetState() == cyndiv1beta1.STATE_REMOVED {
		return reconcile.Result{}, nil
	}

	// new pipeline, nothing to validate yet
	if i.Instance.GetState() == cyndiv1beta1.STATE_NEW {
		return reconcile.Result{Requeue: true}, nil
	}

	reqLogger.Info("Validating CyndiPipeline")

	problem, err := i.checkForDeviation()
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error validating dependencies", err)
	} else if problem != nil {
		i.Log.Info("Refreshing pipeline due to state deviation", "reason", problem)
		i.Instance.TransitionToNew()
		return i.requeue(i.config.ValidationConfigInit.Interval)
	}

	isValid, err := i.validate()
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error validating pipeline", err)
	}

	reqLogger.Info("Validation finished", "isValid", isValid)

	if isValid {
		if err = i.Instance.TransitionToValid(); err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error making state transition", err)
		}
	} else {
		i.Instance.Status.SyndicatedDataIsValid = false
	}

	return i.requeue(i.getValidationConfig().Interval)
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

func (i *ReconcileIteration) requeue(interval int64) (reconcile.Result, error) {
	err := i.Client.Status().Update(context.TODO(), i.Instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	delay := time.Second * time.Duration(interval)
	i.debug("RequeueAfter", "delay", delay)
	return reconcile.Result{RequeueAfter: delay, Requeue: true}, nil
}
