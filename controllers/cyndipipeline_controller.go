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
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cyndi "cyndi-operator/api/v1alpha1"
	"cyndi-operator/controllers/config"
	connect "cyndi-operator/controllers/connect"
	"cyndi-operator/controllers/database"
	"cyndi-operator/controllers/probes"
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
		Instance:         instance,
		OriginalInstance: instance.DeepCopy(),
		Client:           r.Client,
		Clientset:        r.Clientset,
		Scheme:           r.Scheme,
		Log:              reqLogger,
		Now:              time.Now().Format(time.RFC3339),
		GetRequeueInterval: func(Instance *ReconcileIteration) int64 {
			return i.config.StandardInterval
		},
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(
		&typedcorev1.EventSinkImpl{
			Interface: i.Clientset.CoreV1().Events(i.Instance.Namespace),
		})
	i.Recorder = eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "cyndipipeline"})

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

// +kubebuilder:rbac:groups=cyndi.cloud.redhat.com,resources=cyndipipelines;cyndipipelines/status;cyndipipelines/finalizers,verbs=*
// +kubebuilder:rbac:groups=kafka.strimzi.io,resources=kafkaconnectors;kafkaconnectors/finalizers,verbs=*
// +kubebuilder:rbac:groups="",resources=configmaps;secrets,verbs=get;list;watch

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

	// remove any stale dependencies
	// if we're shutting down this removes all dependencies
	err = i.deleteStaleDependencies()

	if err != nil {
		i.errorWithEvent("Error deleting stale dependencies", err)
	}

	// STATE_REMOVED
	if i.Instance.GetState() == cyndi.STATE_REMOVED {
		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error deleting stale dependencies", err)
		}

		if err = i.removeFinalizer(); err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error updating resource after finalizer.", err)
		}

		i.Log.Info("Successfully finalized CyndiPipeline")
		return reconcile.Result{}, nil
	}

	// STATE_NEW
	if i.Instance.GetState() == cyndi.STATE_NEW {
		if err := i.addFinalizer(); err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error adding finalizer", err)
		}

		i.Instance.Status.CyndiConfigVersion = i.config.ConfigMapVersion

		pipelineVersion := fmt.Sprintf("1_%s", strconv.FormatInt(time.Now().UnixNano(), 10))
		i.Log.Info("New pipeline version", "version", pipelineVersion)
		i.Instance.TransitionToInitialSync(pipelineVersion)

		err = i.AppDb.CreateTable(cyndi.TableName(pipelineVersion), i.config.DBTableInitScript)
		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error creating table", err)
		}

		err = i.createConnector(cyndi.ConnectorName(pipelineVersion, i.Instance.Spec.AppName))
		if err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error creating connector", err)
		}

		i.Log.Info("Transitioning to InitialSync")
		return i.updateStatusAndRequeue()
	}

	problem, err := i.checkForDeviation()
	if err != nil {
		return reconcile.Result{}, i.errorWithEvent("Error validating dependencies", err)
	} else if problem != nil {
		i.Log.Info("Refreshing pipeline due to state deviation", "reason", problem)
		i.Instance.TransitionToNew()
		return i.updateStatusAndRequeue()
	}

	// STATE_VALID
	if i.Instance.GetState() == cyndi.STATE_VALID {
		if err = i.recreateViewIfNeeded(); err != nil {
			return reconcile.Result{}, i.errorWithEvent("Error updating hosts view", err)
		}

		return i.updateStatusAndRequeue()
	}

	// invalid pipeline - either STATE_INITIAL_SYNC or STATE_INVALID
	if i.Instance.GetValid() == metav1.ConditionFalse {
		if i.Instance.Status.ValidationFailedCount > i.getValidationConfig().AttemptsThreshold {

			// This pipeline never became valid.
			if i.Instance.GetState() == cyndi.STATE_INITIAL_SYNC {
				if err = i.updateViewIfHealthier(); err != nil {
					// if this continue and do refresh, keeping the old table active
					i.Log.Error(err, "Failed to evaluate which table is healthier")
				}
			}

			i.Log.Info("Pipeline failed to become valid. Refreshing.")
			i.Instance.TransitionToNew()
			probes.PipelineRefreshed(i.Instance)
			return i.updateStatusAndRequeue()
		}
	}

	return i.updateStatusAndRequeue()
}

func (i *ReconcileIteration) deleteStaleDependencies() error {
	var (
		connectorsToKeep []string
		tablesToKeep     []string
	)

	if i.Instance.GetState() != cyndi.STATE_REMOVED && i.Instance.Status.PipelineVersion != "" {
		connectorsToKeep = append(connectorsToKeep, cyndi.ConnectorName(i.Instance.Status.PipelineVersion, i.Instance.Spec.AppName))
		tablesToKeep = append(tablesToKeep, cyndi.TableName(i.Instance.Status.PipelineVersion))
	}

	currentTable, err := i.AppDb.GetCurrentTable()
	if err != nil {
		return err
	}

	if currentTable != nil && i.Instance.GetState() != cyndi.STATE_REMOVED {
		connectorsToKeep = append(connectorsToKeep, cyndi.TableNameToConnectorName(*currentTable, i.Instance.Spec.AppName))
		tablesToKeep = append(tablesToKeep, *currentTable)
	}

	connectors, err := connect.GetConnectorsForApp(i.Client, i.Instance.Namespace, i.Instance.Spec.AppName)
	if err != nil {
		return err
	}

	for _, connector := range connectors.Items {
		if !utils.ContainsString(connectorsToKeep, connector.GetName()) {
			i.Log.Info("Removing stale connector", "connector", connector.GetName())
			if err = connect.DeleteConnector(i.Client, connector.GetName(), i.Instance.Namespace); err != nil {
				return err
			}
		}
	}

	tables, err := i.AppDb.GetCyndiTables()
	if err != nil {
		return err
	}

	for _, table := range tables {
		if !utils.ContainsString(tablesToKeep, table) {
			i.Log.Info("Removing stale table", "table", table)
			if err = i.AppDb.DeleteTable(table); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *CyndiPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cyndi.CyndiPipeline{}).
		Owns(connect.EmptyConnector()).
		// trigger Reconcile if "cyndi" ConfigMap changes
		Watches(&source.Kind{Type: &v1.ConfigMap{}}, &handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(
				func(configMap handler.MapObject) []reconcile.Request {
					requests := []reconcile.Request{}

					if configMap.Meta.GetName() != configMapName {
						return requests
					}

					// cyndi configmap changed - let's Reconcile all CyndiPipelines in the given namespace
					pipelines, err := utils.FetchCyndiPipelines(r.Client, configMap.Meta.GetNamespace())
					if err != nil {
						r.Log.Error(err, "Failed to fetch CyndiPipelines", "namespace", configMap.Meta.GetNamespace())
						return requests
					}

					for _, pipeline := range pipelines.Items {
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: configMap.Meta.GetNamespace(),
								Name:      pipeline.GetName(),
							},
						})
					}

					r.Log.Info("Cyndi ConfigMap changed. Reconciling CyndiPipelines", "namespace", configMap.Meta.GetNamespace(), "pipelines", requests)
					return requests
				}),
		}).
		Complete(r)
}

func (i *ReconcileIteration) addFinalizer() error {
	if !utils.ContainsString(i.Instance.GetFinalizers(), cyndipipelineFinalizer) {
		controllerutil.AddFinalizer(i.Instance, cyndipipelineFinalizer)
		return i.Client.Update(context.TODO(), i.Instance)
	}

	return nil
}

func (i *ReconcileIteration) removeFinalizer() error {
	controllerutil.RemoveFinalizer(i.Instance, cyndipipelineFinalizer)
	return i.Client.Update(context.TODO(), i.Instance)
}

func (i *ReconcileIteration) createConnector(name string) error {
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

	return connect.CreateConnector(i.Client, name, i.Instance.Namespace, config, i.Instance, i.Scheme)
}

func (i *ReconcileIteration) recreateViewIfNeeded() error {
	table, err := i.AppDb.GetCurrentTable()
	if err != nil {
		return err
	}

	if table == nil || *table != i.Instance.Status.TableName {
		i.Log.Info("Updating view", "table", i.Instance.Status.TableName)
		if err = i.AppDb.UpdateView(i.Instance.Status.TableName); err != nil {
			return err
		}
	}

	return nil
}

func (i *ReconcileIteration) checkForDeviation() (problem error, err error) {
	if i.Instance.Status.CyndiConfigVersion != i.config.ConfigMapVersion {
		return fmt.Errorf("ConfigMap changed. New version is %s", i.config.ConfigMapVersion), nil
	}

	dbTableExists, err := i.AppDb.CheckIfTableExists(i.Instance.Status.TableName)
	if err != nil {
		return nil, err
	} else if dbTableExists == false {
		return fmt.Errorf("Database table %s not found", i.Instance.Status.TableName), nil
	}

	connector, err := connect.GetConnector(i.Client, i.Instance.Status.ConnectorName, i.Instance.Namespace)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("Connector %s not found in %s", i.Instance.Status.ConnectorName, i.Instance.Namespace), nil
		}

		return nil, err

	}

	if connector.GetLabels()[connect.LabelAppName] != i.Instance.Spec.AppName {
		return fmt.Errorf("App name disagrees (%s vs %s)", connector.GetLabels()[connect.LabelAppName], i.Instance.Spec.AppName), nil
	}

	if connector.GetLabels()[connect.LabelInsightsOnly] != strconv.FormatBool(i.Instance.Spec.InsightsOnly) {
		return fmt.Errorf("InsightsOnly changed"), nil
	}

	if connector.GetLabels()[connect.LabelStrimziCluster] != i.config.ConnectCluster {
		return fmt.Errorf("ConnectCluster changed from %s to %s", connector.GetLabels()[connect.LabelStrimziCluster], i.config.ConnectCluster), nil
	}

	if connector.GetLabels()[connect.LabelMaxAge] != strconv.FormatInt(i.config.ConnectorMaxAge, 10) {
		return fmt.Errorf("MaxAge changed from %s to %d", connector.GetLabels()[connect.LabelMaxAge], i.config.ConnectorMaxAge), nil
	}

	// TODO: this should be expanded to fully cover the connector

	return nil, nil
}

/*
 * Should be called when a refreshed pipeline failed to become valid.
 * This method will either keep the old invalid table "active" (i.e. used by the view) or update the view to the new (also invalid) table.
 * None of these options a good one - this is about picking lesser evil
 */
func (i *ReconcileIteration) updateViewIfHealthier() error {
	table, err := i.AppDb.GetCurrentTable()

	if err != nil {
		return fmt.Errorf("Failed to determine active table %w", err)
	}

	if table != nil {
		if *table == i.Instance.Status.TableName {
			return nil // table is already active, nothing to do
		}

		// no need to close this as that's done in ReconcileIteration.Close()
		i.InventoryDb = database.NewBaseDatabase(&i.HBIDBParams)

		if err = i.InventoryDb.Connect(); err != nil {
			return fmt.Errorf("Error while connecting to HBI DB %w", err)
		}

		hbiHostCount, err := i.InventoryDb.CountHosts(inventoryTableName, i.Instance.Spec.InsightsOnly)
		if err != nil {
			return fmt.Errorf("Failed to get host count from inventory %w", err)
		}

		activeTable := utils.AppFullTableName(*table)
		activeTableHostCount, err := i.AppDb.CountHosts(activeTable, false)
		if err != nil {
			return fmt.Errorf("Failed to get host count from active table %w", err)
		}

		appTable := utils.AppFullTableName(i.Instance.Status.TableName)
		latestTableHostCount, err := i.AppDb.CountHosts(appTable, false)
		if err != nil {
			return fmt.Errorf("Failed to get host count from application table %w", err)
		}

		if utils.Abs(hbiHostCount-latestTableHostCount) > utils.Abs(hbiHostCount-activeTableHostCount) {
			return nil // the active table is healthier; do not update anything
		}
	}

	if err = i.AppDb.UpdateView(i.Instance.Status.TableName); err != nil {
		return err
	}

	return nil
}
