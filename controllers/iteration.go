package controllers

import (
	"context"
	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"cyndi-operator/controllers/config"
	"cyndi-operator/controllers/database"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ReconcileIteration struct {
	Instance *cyndiv1beta1.CyndiPipeline
	// Do not alter this copy
	// Used for tracking of whether Reconcile actually changed the state or not
	OriginalInstance *cyndiv1beta1.CyndiPipeline

	Recorder  record.EventRecorder
	Scheme    *runtime.Scheme
	Log       logr.Logger
	Client    client.Client
	Clientset *kubernetes.Clientset

	config      *config.CyndiConfiguration
	HBIDBParams config.DBParams
	AppDBParams config.DBParams

	AppDb       *database.AppDatabase
	InventoryDb database.Database

	Now string

	GetRequeueInterval func(i *ReconcileIteration) (result int64)
}

func (i *ReconcileIteration) Close() {
	if i.AppDb != nil {
		i.AppDb.Close()
	}

	if i.InventoryDb != nil {
		i.InventoryDb.Close()
	}
}

func (i *ReconcileIteration) UpdateStatus() (ctrl.Result, error) {
	if err := i.Client.Status().Update(context.TODO(), i.Instance); err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error updating pipeline status", err)
	}

	return reconcile.Result{}, nil
}

func (i *ReconcileIteration) errorWithEvent(message string, err error) error {
	i.Log.Error(err, "Caught error")

	i.Recorder.Event(
		i.Instance,
		corev1.EventTypeWarning,
		message,
		err.Error())
	return err
}

func (i *ReconcileIteration) debug(message string, keysAndValues ...interface{}) {
	i.Log.V(1).Info(message, keysAndValues...)
}

func (i *ReconcileIteration) getValidationConfig() config.ValidationConfiguration {
	if i.Instance.Status.InitialSyncInProgress == true {
		return i.config.ValidationConfigInit
	}

	return i.config.ValidationConfig
}

func (i *ReconcileIteration) updateStatusAndRequeue() (reconcile.Result, error) {
	// Update Status.ActiveTableName to reflect the active table regardless of what happened in this Reconcile() invocation
	if table, err := i.AppDb.GetCurrentTable(); err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error determining current table", err)
	} else {
		if table == nil {
			i.Instance.Status.ActiveTableName = ""
		} else {
			i.Instance.Status.ActiveTableName = *table
		}
	}

	// Only issue status update if Reconcile actually modified Status
	// This prevents write conflicts between the controllers
	if !cmp.Equal(i.Instance.Status, i.OriginalInstance.Status) {
		i.debug("Updating status")

		if err := i.Client.Status().Update(context.TODO(), i.Instance); err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error updating pipeline status", err)
		}
	}

	delay := time.Second * time.Duration(i.GetRequeueInterval(i))
	i.debug("RequeueAfter", "delay", delay)
	return reconcile.Result{RequeueAfter: delay}, nil
}
