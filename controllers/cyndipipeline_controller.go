/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	k8errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"cyndi-operator/controllers/config"
	. "cyndi-operator/controllers/config"
	connect "cyndi-operator/controllers/connect"
	"cyndi-operator/controllers/database"
	"cyndi-operator/controllers/utils"
)

// CyndiPipelineReconciler reconciles a CyndiPipeline object
type CyndiPipelineReconciler struct {
	Client    client.Client
	Clientset *kubernetes.Clientset
	Scheme    *runtime.Scheme
	Log       logr.Logger
}

const cyndipipelineFinalizer = "finalizer.cyndi.cloud.redhat.com"

var log = logf.Log.WithName("controller_cyndipipeline")

func (r *CyndiPipelineReconciler) setup(reqLogger logr.Logger, request ctrl.Request) (ReconcileIteration, error) {

	i := ReconcileIteration{}

	instance, err := utils.FetchCyndiPipeline(r.Client, request.NamespacedName)
	if err != nil {
		if k8errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return i, nil
		}
		// Error reading the object - requeue the request.
		return i, err
	}

	i = ReconcileIteration{
		Instance:  instance,
		Client:    r.Client,
		Clientset: r.Clientset,
		Scheme:    r.Scheme,
		Log:       reqLogger,
		Now:       time.Now().Format(time.RFC3339)}

	if err = i.setupEventRecorder(); err != nil {
		return i, err
	}

	if err = i.parseConfig(); err != nil {
		return i, i.errorWithEvent("Error while reading cyndi configmap.", err)
	}

	if i.HBIDBParams, err = config.LoadSecret(i.Client, i.Instance.Namespace, "host-inventory-db"); err != nil {
		return i, i.errorWithEvent("Error while reading HBI DB secret.", err)
	}

	if i.AppDBParams, err = config.LoadSecret(i.Client, i.Instance.Namespace, fmt.Sprintf("%s-db", i.Instance.Spec.AppName)); err != nil {
		return i, i.errorWithEvent("Error while reading HBI DB secret.", err)
	}

	i.AppDb = database.NewAppDatabase(&i.AppDBParams)

	if err = i.AppDb.Connect(); err != nil {
		return i, i.errorWithEvent("Error while connecting to app DB.", err)
	}

	return i, nil
}

// +kubebuilder:rbac:groups=cyndi.cloud.redhat.com,resources=cyndipipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cyndi.cloud.redhat.com,resources=cyndipipelines/status,verbs=get;update;patch

func (r *CyndiPipelineReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CyndiPipeline")

	i, err := r.setup(reqLogger, request)
	defer i.Close()

	if err != nil {
		return reconcile.Result{}, err
	}

	// Request object not found, could have been deleted after reconcile request.
	if i.Instance == nil {
		return reconcile.Result{}, nil
	}

	// delete pipeline
	if i.Instance.GetDeletionTimestamp() != nil {
		if utils.ContainsString(i.Instance.GetFinalizers(), cyndipipelineFinalizer) {
			if err := i.finalizeCyndiPipeline(); err != nil {
				return reconcile.Result{}, i.errorWithEvent("Error running finalizer.", err)
			}

			controllerutil.RemoveFinalizer(i.Instance, cyndipipelineFinalizer)
			err := r.Client.Update(context.TODO(), i.Instance)
			if err != nil {
				return reconcile.Result{}, i.errorWithEvent("Error updating resource after finalizer.", err)
			}
		}
		return reconcile.Result{}, nil
	}

	if !utils.ContainsString(i.Instance.GetFinalizers(), cyndipipelineFinalizer) {
		if err := i.addFinalizer(); err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error adding finalizer to resource", err)
		}
	}

	//new pipeline
	if i.Instance.Status.PipelineVersion == "" {
		i.refreshPipelineVersion()
		i.Instance.Status.InitialSyncInProgress = true
	}

	dbTableExists, err := i.AppDb.CheckIfTableExists(i.Instance.Status.TableName)
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error checking if table exists.", err)
	}

	connectorExists, err := connect.CheckIfConnectorExists(i.Client, i.Instance.Status.ConnectorName, i.Instance.Namespace)
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error checking if connector exists", err)
	}

	//part, or all, of the pipeline is missing, create a new pipeline
	if dbTableExists != true || connectorExists != true {
		if i.Instance.Status.InitialSyncInProgress != true {
			i.Instance.Status.PreviousPipelineVersion = i.Instance.Status.PipelineVersion
			i.refreshPipelineVersion()
		}

		err = i.AppDb.CreateTable(i.Instance.Status.TableName, i.config.DBTableInitScript)
		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error creating table", err)
		}

		err = i.createConnector()
		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error creating connector", err)
		}
	}

	err = r.Client.Status().Update(context.TODO(), i.Instance)
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error updating status", err)
	}

	if i.Instance.Status.SyndicatedDataIsValid != true && i.Instance.Status.ValidationFailedCount > i.getValidationConfig().AttemptsThreshold {
		err = i.triggerRefresh()

		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error triggering refresh", err)
		}
	} else if i.Instance.Status.SyndicatedDataIsValid == true {
		err = i.AppDb.UpdateView(i.Instance.Status.TableName) // TODO: only update when outdated?
		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error updating database view", err)
		}

		// remove previous pipeline artifacts
		if i.Instance.Status.PreviousPipelineVersion != "" {
			err = i.AppDb.DeleteTable(tableName(i.Instance.Status.PreviousPipelineVersion))
			if err != nil {
				return reconcile.Result{}, i.errorWithEvent("Error deleting table", err)
			}

			err = connect.DeleteConnector(i.Client, connectorName(i.Instance.Status.PreviousPipelineVersion, i.Instance.Spec.AppName), i.Instance.Namespace)
			if err != nil {
				return reconcile.Result{}, i.errorWithEvent("Error deleting connector", err)
			}

			i.Instance.Status.PreviousPipelineVersion = ""
		}

		i.Instance.Status.InitialSyncInProgress = false
		err = i.Client.Status().Update(context.TODO(), i.Instance)
		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error updating status", err)
		}
	}

	return reconcile.Result{}, nil
}

func (i *ReconcileIteration) triggerRefresh() error {
	i.Log.Info("Refreshing CyndiPipeline")

	i.Instance.Status.PreviousPipelineVersion = i.Instance.Status.PipelineVersion
	i.Instance.Status.ValidationFailedCount = 0
	i.Instance.Status.PipelineVersion = ""

	err := i.Client.Status().Update(context.TODO(), i.Instance)
	return err
}

func (r *CyndiPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cyndiv1beta1.CyndiPipeline{}).
		Complete(r)
}

func (i *ReconcileIteration) refreshPipelineVersion() {
	i.Instance.Status.PipelineVersion = fmt.Sprintf(
		"1_%s",
		strconv.FormatInt(time.Now().UnixNano(), 10))
	i.Instance.Status.ConnectorName = connectorName(i.Instance.Status.PipelineVersion, i.Instance.Spec.AppName)
	i.Instance.Status.TableName = tableName(i.Instance.Status.PipelineVersion)
	i.Instance.Status.SyndicatedDataIsValid = false
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

func tableName(pipelineVersion string) string {
	return fmt.Sprintf("hosts_v%s", pipelineVersion)
}

func connectorName(pipelineVersion string, appName string) string {
	return fmt.Sprintf("syndication-pipeline-%s-%s",
		appName,
		strings.Replace(pipelineVersion, "_", "-", 1))
}

func (i *ReconcileIteration) setupEventRecorder() error {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(
		&typedcorev1.EventSinkImpl{
			Interface: i.Clientset.CoreV1().Events(i.Instance.Namespace)})
	recorder := eventBroadcaster.NewRecorder(
		scheme.Scheme,
		corev1.EventSource{Component: "cyndipipeline"})

	i.Recorder = recorder
	return nil
}

func (i *ReconcileIteration) finalizeCyndiPipeline() error {
	err := i.AppDb.DeleteTable(i.Instance.Status.TableName)
	if err != nil {
		return err
	}
	i.Log.Info("Successfully finalized CyndiPipeline")
	return nil
}

func (i *ReconcileIteration) addFinalizer() error {
	i.Log.Info("Adding Finalizer for the CyndiPipeline")
	controllerutil.AddFinalizer(i.Instance, cyndipipelineFinalizer)

	err := i.Client.Update(context.TODO(), i.Instance)
	if err != nil {
		return i.errorWithEvent("Failed to update CyndiPipeline with finalizer", err)
	}
	return nil
}

func (i *ReconcileIteration) createConnector() error {
	var config = connect.ConnectorConfiguration{
		AppName:      i.Instance.Spec.AppName,
		InsightsOnly: i.Instance.Spec.InsightsOnly,
		Cluster:      i.config.ConnectCluster,
		Topic:        i.config.Topic,
		TableName:    i.Instance.Status.TableName,
		DB:           i.AppDBParams,
		TasksMax:     i.config.ConnectorTasksMax,
		BatchSize:    i.config.ConnectorBatchSize,
		MaxAge:       i.config.ConnectorMaxAge,
		Template:     i.config.ConnectorTemplate,
	}

	return connect.CreateConnector(i.Client, i.Instance.Status.ConnectorName, i.Instance.Namespace, config, i.Instance, i.Scheme)
}

func (i *ReconcileIteration) getValidationConfig() ValidationConfiguration {
	if i.Instance.Status.InitialSyncInProgress == true {
		return i.config.ValidationConfigInit
	}

	return i.config.ValidationConfig
}
