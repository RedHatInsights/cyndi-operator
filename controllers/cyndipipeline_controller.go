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
	"os"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1 "k8s.io/api/core/v1"
	k8errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cyndi "github.com/RedHatInsights/cyndi-operator/api/v1alpha1"
	"github.com/RedHatInsights/cyndi-operator/controllers/config"
	connect "github.com/RedHatInsights/cyndi-operator/controllers/connect"
	"github.com/RedHatInsights/cyndi-operator/controllers/database"
	"github.com/RedHatInsights/cyndi-operator/controllers/metrics"
	"github.com/RedHatInsights/cyndi-operator/controllers/utils"
)

// CyndiPipelineReconciler reconciles a CyndiPipeline object
type CyndiPipelineReconciler struct {
	Client    client.Client
	Clientset *kubernetes.Clientset
	Scheme    *runtime.Scheme
	Log       logr.Logger
	Recorder  record.EventRecorder
}

const cyndipipelineFinalizer = "cyndi.cloud.redhat.com/finalizer"

var log = logf.Log.WithName("controller_cyndipipeline")

func (r *CyndiPipelineReconciler) setup(reqLogger logr.Logger, request ctrl.Request, ctx context.Context) (ReconcileIteration, error) {

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
		AppDb:            &database.AppDatabase{},
		Log:              reqLogger,
		Now:              time.Now().Format(time.RFC3339),
		GetRequeueInterval: func(Instance *ReconcileIteration) int64 {
			return i.config.StandardInterval
		},
		Recorder: r.Recorder,
		ctx:      ctx,
	}

	if err = i.parseConfig(); err != nil {
		return i, err
	}

	if i.HBIDBParams, err = config.LoadDBSecret(i.config, i.Client, i.Instance.Namespace, i.config.InventoryDbSecret); err != nil {
		return i, err
	}

	if i.AppDBParams, err = config.LoadDBSecret(i.config, i.Client, i.Instance.Namespace, utils.AppDbSecretName(i.Instance.Spec)); err != nil {
		return i, err
	}

	i.AppDb = database.NewAppDatabase(&i.AppDBParams)

	if err = i.AppDb.Connect(); err != nil {
		return i, err
	}

	return i, nil
}

// +kubebuilder:rbac:groups=cyndi.cloud.redhat.com,resources=cyndipipelines;cyndipipelines/status;cyndipipelines/finalizers,verbs=*
// +kubebuilder:rbac:groups=kafka.strimzi.io,resources=kafkaconnectors;kafkaconnectors/finalizers,verbs=*
// +kubebuilder:rbac:groups="",resources=configmaps;secrets,verbs=get;list;watch

func (r *CyndiPipelineReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()

	//capture errors until the finalizer completed
	var setupErrors []error

	reqLogger := log.WithValues("Pipeline", request.Name, "Namespace", request.Namespace)
	reqLogger.Info("Reconciling CyndiPipeline")

	i, err := r.setup(reqLogger, request, ctx)
	if err != nil {
		setupErrors = append(setupErrors, i.error(err))
	}
	defer i.Close()

	// Request object not found, could have been deleted after reconcile request.
	if i.Instance == nil {
		return reconcile.Result{}, nil
	}

	// remove any stale dependencies
	// if we're shutting down this removes all dependencies
	setupErrors = append(setupErrors, i.deleteStaleDependencies()...)

	for _, err := range setupErrors {
		_ = i.error(err, "Error deleting stale dependency")
	}

	// STATE_REMOVED
	ephemeral, err := strconv.ParseBool(os.Getenv("EPHEMERAL"))
	if err != nil {
		ephemeral = false
	}

	if i.Instance.GetState() == cyndi.STATE_REMOVED {
		if len(setupErrors) > 0 && !ephemeral {
			return reconcile.Result{}, setupErrors[0]
		}

		if err = i.removeFinalizer(); err != nil {
			return reconcile.Result{}, i.error(err, "Error removing finalizer")
		}

		i.Log.Info("Successfully finalized CyndiPipeline")
		return reconcile.Result{}, nil
	}

	//finalizer is complete so throw any errors that previously occurred
	if len(setupErrors) > 0 {
		return reconcile.Result{}, setupErrors[0]
	}

	metrics.InitLabels(i.Instance)

	// STATE_NEW
	if i.Instance.GetState() == cyndi.STATE_NEW {
		if err := i.addFinalizer(); err != nil {
			return reconcile.Result{}, i.error(err, "Error adding finalizer")
		}

		i.Instance.Status.CyndiConfigVersion = i.config.ConfigMapVersion

		pipelineVersion := fmt.Sprintf("1_%s", strconv.FormatInt(time.Now().UnixNano(), 10))
		if err := i.Instance.TransitionToInitialSync(pipelineVersion); err != nil {
			return reconcile.Result{}, i.error(err, "Error Transitioning to initial state")
		}
		i.probeStartingInitialSync()

		err = i.AppDb.CreateTable(cyndi.TableName(pipelineVersion), i.config.DBTableInitScript)
		if err != nil {
			return reconcile.Result{}, i.error(err, "Error creating table")
		}

		_, err = i.createConnector(cyndi.ConnectorName(pipelineVersion, i.Instance.Spec.AppName), false)
		if err != nil {
			return reconcile.Result{}, i.error(err, "Error creating connector")
		}

		i.Log.Info("Transitioning to InitialSync")
		return i.updateStatusAndRequeue()
	}

	problem, err := i.checkForDeviation()
	if err != nil {
		return reconcile.Result{}, i.error(err, "Error checking for state deviation")
	} else if problem != nil {
		i.probeStateDeviationRefresh(problem.Error())
		i.Instance.TransitionToNew()
		return i.updateStatusAndRequeue()
	}

	// STATE_VALID
	if i.Instance.GetState() == cyndi.STATE_VALID {
		if updated, err := i.recreateViewIfNeeded(); err != nil {
			return reconcile.Result{}, i.error(err, "Error updating hosts view")
		} else if updated {
			i.eventNormal("ValidationSucceeded", "Pipeline became valid. inventory.hosts view now points to %s", i.Instance.Status.TableName)
		}

		return i.updateStatusAndRequeue()
	}

	// invalid pipeline - either STATE_INITIAL_SYNC or STATE_INVALID
	if i.Instance.GetValid() == metav1.ConditionFalse {
		if i.Instance.Status.ValidationFailedCount >= i.getValidationConfig().AttemptsThreshold {

			// This pipeline never became valid.
			if i.Instance.GetState() == cyndi.STATE_INITIAL_SYNC {
				if err = i.updateViewIfHealthier(); err != nil {
					// if this fails continue and do refresh, keeping the old table active
					i.Log.Error(err, "Failed to evaluate which table is healthier")
				}
			}

			i.Instance.TransitionToNew()
			i.probePipelineDidNotBecomeValid()
			return i.updateStatusAndRequeue()
		}
	}

	return i.updateStatusAndRequeue()
}

func (i *ReconcileIteration) deleteStaleDependencies() (errors []error) {
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
		errors = append(errors, err)
	} else if currentTable != nil && i.Instance.GetState() != cyndi.STATE_REMOVED {
		connectorsToKeep = append(connectorsToKeep, cyndi.TableNameToConnectorName(*currentTable, i.Instance.Spec.AppName))
		tablesToKeep = append(tablesToKeep, *currentTable)
	}

	connectors, err := connect.GetConnectorsForOwner(i.Client, i.Instance.Namespace, i.Instance.GetUIDString())
	if err != nil {
		errors = append(errors, err)
	} else {
		for _, connector := range connectors.Items {
			if !utils.ContainsString(connectorsToKeep, connector.GetName()) {
				i.Log.Info("Removing stale connector", "connector", connector.GetName())
				if err = connect.DeleteConnector(i.Client, connector.GetName(), i.Instance.Namespace); err != nil {
					errors = append(errors, err)
				}
			}
		}
	}

	tables, err := i.AppDb.GetCyndiTables()
	if err != nil {
		errors = append(errors, err)
	} else {
		for _, table := range tables {
			if !utils.ContainsString(tablesToKeep, table) {
				i.Log.Info("Removing stale table", "table", table)
				if err = i.AppDb.DeleteTable(table); err != nil {
					errors = append(errors, err)
				}
			}
		}
	}

	return
}

func (r *CyndiPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("cyndi-controller").
		For(&cyndi.CyndiPipeline{}).
		Owns(connect.EmptyConnector()).
		// trigger Reconcile if "cyndi" ConfigMap changes
		Watches(&source.Kind{Type: &v1.ConfigMap{}}, handler.EnqueueRequestsFromMapFunc(func(configMap client.Object) []reconcile.Request {
			var requests []reconcile.Request

			if configMap.GetName() != configMapName {
				return requests
			}

			// cyndi configmap changed - let's Reconcile all CyndiPipelines in the given namespace
			pipelines, err := utils.FetchCyndiPipelines(r.Client, configMap.GetNamespace())
			if err != nil {
				r.Log.Error(err, "Failed to fetch CyndiPipelines", "namespace", configMap.GetNamespace())
				return requests
			}

			for _, pipeline := range pipelines.Items {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: configMap.GetNamespace(),
						Name:      pipeline.GetName(),
					},
				})
			}

			r.Log.Info("Cyndi ConfigMap changed. Reconciling CyndiPipelines", "namespace", configMap.GetNamespace(), "pipelines", requests)
			return requests
		})).
		Complete(r)
}

func (i *ReconcileIteration) addFinalizer() error {
	if !utils.ContainsString(i.Instance.GetFinalizers(), cyndipipelineFinalizer) {
		controllerutil.AddFinalizer(i.Instance, cyndipipelineFinalizer)
		return i.Client.Update(i.ctx, i.Instance)
	}

	return nil
}

func (i *ReconcileIteration) removeFinalizer() error {
	controllerutil.RemoveFinalizer(i.Instance, cyndipipelineFinalizer)
	return i.Client.Update(i.ctx, i.Instance)
}

func (i *ReconcileIteration) createConnector(name string, dryRun bool) (*unstructured.Unstructured, error) {
	var connectorConfig = connect.ConnectorConfiguration{
		AppName:                  i.Instance.Spec.AppName,
		InsightsOnly:             i.Instance.Spec.InsightsOnly,
		Cluster:                  i.config.ConnectCluster,
		Topic:                    i.config.Topic,
		TableName:                i.Instance.Status.TableName,
		DB:                       i.AppDBParams,
		TasksMax:                 i.config.ConnectorTasksMax,
		BatchSize:                i.config.ConnectorBatchSize,
		MaxAge:                   i.config.ConnectorMaxAge,
		Template:                 i.config.ConnectorTemplate,
		AllowlistSystemProfile:   i.config.ConnectorAllowlistSystemProfile,
		TopicReplicationFactor:   i.config.TopicReplicationFactor,
		DeadLetterQueueTopicName: i.config.DeadLetterQueueTopicName,
	}

	return connect.CreateConnector(i.Client, name, i.Instance.Namespace, connectorConfig, i.Instance, i.Scheme, dryRun)
}

func (i *ReconcileIteration) recreateViewIfNeeded() (bool, error) {
	table, err := i.AppDb.GetCurrentTable()
	if err != nil {
		return false, err
	}

	if table == nil || *table != i.Instance.Status.TableName {
		i.Log.Info("Updating view", "table", i.Instance.Status.TableName)
		if err = i.AppDb.UpdateView(i.Instance.Status.TableName); err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

func (i *ReconcileIteration) checkForDeviation() (problem error, err error) {
	if i.Instance.Status.CyndiConfigVersion != i.config.ConfigMapVersion {
		return fmt.Errorf("ConfigMap changed. New version is %s", i.config.ConfigMapVersion), nil
	}

	dbTableExists, err := i.AppDb.CheckIfTableExists(i.Instance.Status.TableName)
	if err != nil {
		return nil, err
	} else if !dbTableExists {
		return fmt.Errorf("Database table %s not found", i.Instance.Status.TableName), nil
	}

	connector, err := connect.GetConnector(i.Client, i.Instance.Status.ConnectorName, i.Instance.Namespace)
	if err != nil {
		if k8errors.IsNotFound(err) {
			return fmt.Errorf("Connector %s not found in %s", i.Instance.Status.ConnectorName, i.Instance.Namespace), nil
		}

		return nil, err

	}

	if connect.IsFailed(connector) {
		return fmt.Errorf("Connector %s is in the FAILED state", i.Instance.Status.ConnectorName), nil
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

	// compares the spec of the existing connector with the spec we would create if we were creating a new connector now
	newConnector, err := i.createConnector(i.Instance.Status.ConnectorName, true)
	if err != nil {
		return nil, err
	}

	currentConnectorConfig, _, err1 := unstructured.NestedMap(connector.UnstructuredContent(), "spec", "config")
	newConnectorConfig, _, err2 := unstructured.NestedMap(newConnector.UnstructuredContent(), "spec", "config")

	if err1 == nil && err2 == nil {
		diff := cmp.Diff(currentConnectorConfig, newConnectorConfig, NumberNormalizer)

		if len(diff) > 0 {
			return fmt.Errorf("Connector configuration has changed: %s", diff), nil
		}
	}

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
			return err
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

func NewCyndiReconciler(client client.Client, clientset *kubernetes.Clientset, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder) *CyndiPipelineReconciler {
	return &CyndiPipelineReconciler{
		Client:    client,
		Clientset: clientset,
		Log:       log,
		Scheme:    scheme,
		Recorder:  recorder,
	}
}
