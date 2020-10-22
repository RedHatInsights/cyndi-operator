package controllers

import (
	"context"
	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"cyndi-operator/controllers/config"
	"cyndi-operator/controllers/database"

	"github.com/go-logr/logr"
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

	Recorder  record.EventRecorder
	Scheme    *runtime.Scheme
	Log       logr.Logger
	Client    client.Client
	Clientset *kubernetes.Clientset

	config      *config.CyndiConfiguration
	HBIDBParams config.DBParams
	AppDBParams config.DBParams

	AppDb       *database.AppDatabase
	InventoryDb *database.Database

	Now string
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

func (i *ReconcileIteration) debug(message string) {
	i.Log.V(1).Info(message)
}
