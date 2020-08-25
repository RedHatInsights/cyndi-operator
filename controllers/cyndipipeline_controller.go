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
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/jackc/pgx"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
	"strings"
	"time"

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
)

var log = logf.Log.WithName("controller_cyndipipeline")

// CyndiPipelineReconciler reconciles a CyndiPipeline object
type CyndiPipelineReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

type HBIDBParams struct {
	Name     string
	Host     string
	Port     string
	User     string
	Password string
}

type ValidationParams struct {
	Interval                int64
	AttemptsThreshold       int64
	PercentageThreshold     int64
	InitInterval            int64
	InitAttemptsThreshold   int64
	InitPercentageThreshold int64
}

type ReconcileIteration struct {
	Instance         *cyndiv1beta1.CyndiPipeline
	Log              logr.Logger
	AppDb            *pgx.Conn
	Client           client.Client
	Scheme           *runtime.Scheme
	Now              string
	Recorder         record.EventRecorder
	HBIDBParams      HBIDBParams
	DBSchema         string
	ConnectorConfig  string
	ValidationParams ValidationParams
	ConnectCluster   string
}

const cyndipipelineFinalizer = "finalizer.cyndi.cloud.redhat.com"

// +kubebuilder:rbac:groups=cyndi.cloud.redhat.com,resources=cyndipipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cyndi.cloud.redhat.com,resources=cyndipipelines/status,verbs=get;update;patch

func (r *CyndiPipelineReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	reqLogger.Info("Reconciling CyndiPipeline")

	instance := &cyndiv1beta1.CyndiPipeline{}

	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if k8errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	i := ReconcileIteration{
		Instance: instance,
		Log:      reqLogger,
		Client:   r.Client,
		Scheme:   r.Scheme,
		Now:      time.Now().Format(time.RFC3339)}

	if err = i.setupEventRecorder(); err != nil {
		return reconcile.Result{}, err
	}

	if err = i.parseConfig(); err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error while reading cyndi configmap.", err)
	}

	if err = i.parseHBIDBSecret(); err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error while reading HBI DB secret.", err)
	}

	if err = i.connectToAppDB(); err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error while connecting to app DB.", err)
	}

	defer i.closeAppDB()

	// delete pipeline
	if i.Instance.GetDeletionTimestamp() != nil {
		if contains(i.Instance.GetFinalizers(), cyndipipelineFinalizer) {
			if err := i.finalizeCyndiPipeline(); err != nil {
				return reconcile.Result{}, i.errorWithEvent("Error running finalizer.", err)
			}

			controllerutil.RemoveFinalizer(instance, cyndipipelineFinalizer)
			err := r.Client.Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, i.errorWithEvent("Error updating resource after finalizer.", err)
			}
		}
		return reconcile.Result{}, nil
	}

	if !contains(instance.GetFinalizers(), cyndipipelineFinalizer) {
		if err := i.addFinalizer(); err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error adding finalizer to resource", err)
		}
	}

	//new pipeline
	if instance.Status.PipelineVersion == "" {
		i.refreshPipelineVersion()
		instance.Status.InitialSyncInProgress = true
	}

	dbTableExists, err := i.checkIfTableExists(i.Instance.Status.TableName)
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error checking if table exists.", err)
	}

	connectorExists, err := i.checkIfConnectorExists(i.Instance.Status.ConnectorName)
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error checking if connector exists", err)
	}

	//part, or all, of the pipeline is missing, create a new pipeline
	if dbTableExists != true || connectorExists != true {
		if instance.Status.InitialSyncInProgress != true {
			instance.Status.PreviousPipelineVersion = instance.Status.PipelineVersion
			i.refreshPipelineVersion()
		}

		err = i.createTable(i.Instance.Status.TableName)
		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error creating table", err)
		}

		err = i.createConnector()
		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error creating connector", err)
		}
	}

	pipelineIsValid, err := i.validate()
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error validating pipeline", err)
	}

	err = r.Client.Status().Update(context.TODO(), instance)
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error updating status", err)
	}

	validationFailedCountThreshold := i.ValidationParams.AttemptsThreshold
	validationInterval := i.ValidationParams.Interval
	if instance.Status.InitialSyncInProgress == true {
		validationFailedCountThreshold = i.ValidationParams.InitAttemptsThreshold
		validationInterval = i.ValidationParams.InitInterval
	}

	if pipelineIsValid != true && instance.Status.ValidationFailedCount > validationFailedCountThreshold {
		err = i.triggerRefresh()

		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error triggering refresh", err)
		}
	} else if pipelineIsValid == true {
		err = i.updateView()
		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error updating database view", err)
		}

		if instance.Status.PreviousPipelineVersion != "" {
			err = i.deleteTable(tableName(instance.Status.PreviousPipelineVersion))
			if err != nil {
				return reconcile.Result{}, i.errorWithEvent("Error deleting table", err)
			}

			err = i.deleteConnector(connectorName(instance.Status.PreviousPipelineVersion, instance.Spec.AppName))
			if err != nil {
				return reconcile.Result{}, i.errorWithEvent("Error deleting connector", err)
			}

			instance.Status.PreviousPipelineVersion = ""
		}

		instance.Status.InitialSyncInProgress = false
	} else if pipelineIsValid != true {
		//need to sleep here. Updating the validationFailedCount in the status causes an immediate requeue of Reconcile.
		//So, setting a RequeueAfter delay will not delay the Reconcile loop.
		//https://github.com/operator-framework/operator-sdk/issues/1164#issuecomment-469485711
		//A better solution might be to create a separate controller to perform the validation. When the
		//validation_controller fails n times and needs to recreate the pipeline, it can set the status of this operator
		//to trigger a refresh.
		time.Sleep(time.Second * time.Duration(validationInterval))
	}

	return i.requeue(time.Second * time.Duration(validationInterval))
}

func (i *ReconcileIteration) triggerRefresh() error {

	i.Instance.Status.PreviousPipelineVersion = i.Instance.Status.PipelineVersion
	i.Instance.Status.ValidationFailedCount = 0
	i.Instance.Status.PipelineVersion = ""

	err := i.Client.Status().Update(context.TODO(), i.Instance)
	return err
}

func (i *ReconcileIteration) requeue(delay time.Duration) (reconcile.Result, error) {
	err := i.Client.Status().Update(context.TODO(), i.Instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: delay, Requeue: true}, nil
}

func (r *CyndiPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cyndiv1beta1.CyndiPipeline{}).
		Complete(r)
}

func (i *ReconcileIteration) parseConfig() error {
	cyndiConfig := &corev1.ConfigMap{}
	err := i.Client.Get(context.TODO(), client.ObjectKey{Name: "cyndi", Namespace: i.Instance.Namespace}, cyndiConfig)
	if err != nil {
		return err
	}

	i.ConnectorConfig = cyndiConfig.Data["connector.config"]
	if i.ConnectorConfig == "" {
		return errors.New("connector.config is missing from cyndi configmap")
	}

	i.DBSchema = cyndiConfig.Data["db.schema"]
	if i.DBSchema == "" {
		return errors.New("db.schema is missing from cyndi configmap")
	}

	i.ConnectCluster = cyndiConfig.Data["connect.cluster"]
	if i.ConnectCluster == "" {
		return errors.New("connect.cluster is missing from cyndi configmap")
	}

	if i.Instance.Status.CyndiConfigVersion == "" {
		i.Instance.Status.CyndiConfigVersion = cyndiConfig.ResourceVersion
	} else if i.Instance.Status.CyndiConfigVersion != cyndiConfig.ResourceVersion {
		//cyndi configmap changed, perform a refresh to use latest values
		i.Instance.Status.CyndiConfigVersion = cyndiConfig.ResourceVersion
		if err = i.triggerRefresh(); err != nil {
			return err
		}
	}

	i.ValidationParams = ValidationParams{}

	i.ValidationParams.Interval, err =
		strconv.ParseInt(cyndiConfig.Data["validation.interval"], 10, 64)
	if err != nil || i.ValidationParams.Interval <= 0 {
		return errors.New("unable to parse validation.interval from cyndi configmap")
	}

	i.ValidationParams.AttemptsThreshold, err =
		strconv.ParseInt(cyndiConfig.Data["validation.attempts.threshold"], 10, 64)
	if err != nil || i.ValidationParams.AttemptsThreshold <= 0 {
		return errors.New("unable to parse validation.attempts.threshold from cyndi configmap")
	}

	i.ValidationParams.PercentageThreshold, err =
		strconv.ParseInt(cyndiConfig.Data["validation.percentage.threshold"], 10, 64)
	if err != nil || i.ValidationParams.PercentageThreshold <= 0 {
		return errors.New("unable to parse validation.percentage.threshold from cyndi configmap")
	}

	i.ValidationParams.InitInterval, err =
		strconv.ParseInt(cyndiConfig.Data["init.validation.interval"], 10, 64)
	if err != nil || i.ValidationParams.InitInterval <= 0 {
		return errors.New("unable to parse init.validation.interval from cyndi configmap")
	}

	i.ValidationParams.InitAttemptsThreshold, err =
		strconv.ParseInt(cyndiConfig.Data["init.validation.attempts.threshold"], 10, 64)
	if err != nil || i.ValidationParams.InitAttemptsThreshold <= 0 {
		return errors.New("unable to parse init.validation.attempts.threshold from cyndi configmap")
	}

	i.ValidationParams.InitPercentageThreshold, err =
		strconv.ParseInt(cyndiConfig.Data["init.validation.percentage.threshold"], 10, 64)
	if err != nil || i.ValidationParams.InitPercentageThreshold <= 0 {
		return errors.New("unable to parse init.validation.percentage.threshold from cyndi configmap")
	}

	return nil
}

func (i *ReconcileIteration) refreshPipelineVersion() {
	i.Instance.Status.PipelineVersion = fmt.Sprintf(
		"1_%s",
		strconv.FormatInt(time.Now().UnixNano(), 10))
	i.Instance.Status.ConnectorName = connectorName(i.Instance.Status.PipelineVersion, i.Instance.Spec.AppName)
	i.Instance.Status.TableName = tableName(i.Instance.Status.PipelineVersion)
}

func (i *ReconcileIteration) errorWithEvent(message string, err error) error {
	i.Recorder.Event(
		i.Instance,
		corev1.EventTypeWarning,
		message,
		err.Error())
	return err
}

func tableName(pipelineVersion string) string {
	return fmt.Sprintf("hosts_v%s", pipelineVersion)
}

func connectorName(pipelineVersion string, appName string) string {
	return fmt.Sprintf("syndication-pipeline-%s-%s",
		appName,
		strings.Replace(pipelineVersion, "_", "-", 1))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (i *ReconcileIteration) setupEventRecorder() error {
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(
		&typedcorev1.EventSinkImpl{
			Interface: kubeClient.CoreV1().Events(i.Instance.Namespace)})
	recorder := eventBroadcaster.NewRecorder(
		scheme.Scheme,
		corev1.EventSource{Component: "cyndipipeline"})

	i.Recorder = recorder
	return nil
}

func (i *ReconcileIteration) finalizeCyndiPipeline() error {
	err := i.deleteTable(i.Instance.Status.TableName)
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
