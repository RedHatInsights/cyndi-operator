package controllers

import (
	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"cyndi-operator/controllers/config"
	"cyndi-operator/controllers/database"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
