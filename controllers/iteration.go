package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	cyndi "github.com/RedHatInsights/cyndi-operator/api/v1alpha1"
	"github.com/RedHatInsights/cyndi-operator/controllers/config"
	"github.com/RedHatInsights/cyndi-operator/controllers/database"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ReconcileIteration struct {
	Instance *cyndi.CyndiPipeline
	// Do not alter this copy
	// Used for tracking of whether Reconcile actually changed the state or not
	OriginalInstance *cyndi.CyndiPipeline
	ctx              context.Context

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

// logs the error and produces an error log message
func (i *ReconcileIteration) error(err error, prefixes ...string) error {
	msg := err.Error()

	if len(prefixes) > 0 {
		prefix := strings.Join(prefixes[:], ", ")
		msg = fmt.Sprintf("%s: %s", prefix, msg)
	}

	i.Log.Error(err, msg)

	i.eventWarning("Failed", msg)
	return err
}

func (i *ReconcileIteration) eventNormal(reason, messageFmt string, args ...interface{}) {
	i.Recorder.Eventf(i.Instance, corev1.EventTypeNormal, reason, messageFmt, args...)
}

func (i *ReconcileIteration) eventWarning(reason, messageFmt string, args ...interface{}) {
	i.Recorder.Eventf(i.Instance, corev1.EventTypeWarning, reason, messageFmt, args...)
}

func (i *ReconcileIteration) debug(message string, keysAndValues ...interface{}) {
	i.Log.V(1).Info(message, keysAndValues...)
}

func (i *ReconcileIteration) getValidationConfig() config.ValidationConfiguration {
	if i.Instance.Status.InitialSyncInProgress {
		return i.config.ValidationConfigInit
	}

	return i.config.ValidationConfig
}

func (i *ReconcileIteration) updateStatusAndRequeue() (reconcile.Result, error) {
	// Update Status.ActiveTableName to reflect the active table regardless of what happened in this Reconcile() invocation
	if table, err := i.AppDb.GetCurrentTable(); err != nil {
		return reconcile.Result{}, i.error(err, "Error determining current table")
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
			if errors.IsConflict(err) {
				i.Log.Error(err, "Status conflict")
				return reconcile.Result{}, err
			}

			return reconcile.Result{}, i.error(err, "Error updating pipeline status")
		}
	}

	delay := time.Second * time.Duration(i.GetRequeueInterval(i))
	i.debug("RequeueAfter", "delay", delay)
	return reconcile.Result{RequeueAfter: delay}, nil
}
