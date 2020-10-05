package controllers

import (
	"context"
	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ValidationReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

func (r *ValidationReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Validating CyndiPipeline")

	i, err := setup(r.Client, r.Scheme, reqLogger, request)
	defer i.closeAppDB()
	if err != nil {
		return reconcile.Result{}, err
	}

	// Request object not found, could have been deleted after reconcile request.
	if i.Instance == nil {
		return reconcile.Result{}, nil
	}

	isValid, err := i.validate()
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error validating pipeline", err)
	}

	i.Instance.Status.SyndicatedDataIsValid = isValid
	reqLogger.Info("Validation finished", "isValid", isValid)

	return i.requeue(i.getValidationInterval())
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

func (i *ReconcileIteration) getValidationInterval() int64 {
	if i.Instance.Status.InitialSyncInProgress {
		return i.ValidationParams.InitInterval
	}

	return i.ValidationParams.Interval
}
