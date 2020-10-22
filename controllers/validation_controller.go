package controllers

import (
	"context"
	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"cyndi-operator/controllers/database"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
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

	problem, err := i.validateDependencies()
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error validating dependencies", err)
	} else if problem != nil {
		i.Log.Info("Refreshing pipeline due to invalid dependency", "reason", problem)
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

func (r *ValidationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cyndiv1beta1.CyndiPipeline{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func (i *ReconcileIteration) requeue(interval int64) (reconcile.Result, error) {
	err := i.Client.Status().Update(context.TODO(), i.Instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	delay := time.Second * time.Duration(interval)
	return reconcile.Result{RequeueAfter: delay, Requeue: true}, nil
}
