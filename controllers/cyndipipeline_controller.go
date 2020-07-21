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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cyndiv1beta1 "cyndi-operator/api/v1beta1"
)

// CyndiPipelineReconciler reconciles a CyndiPipeline object
type CyndiPipelineReconciler struct {
	client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cyndi.cloud.redhat.com,resources=cyndipipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cyndi.cloud.redhat.com,resources=cyndipipelines/status,verbs=get;update;patch

func (r *CyndiPipelineReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	reqLogger := r.Log.WithValues("cyndipipeline", request.NamespacedName)

	reqLogger.Info("Reconciling CyndiPipeline")

	instance := &cyndiv1beta1.CyndiPipeline{}

	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if instance.Status.PipelineVersion == "" {
		log.Info("Setting pipeline version vars")
		instance.Status.PipelineVersion = fmt.Sprintf(
			"1_%s",
			strconv.FormatInt(time.Now().UnixNano(), 10))

		log.Info(instance.Status.PipelineVersion)

		instance.Status.ConnectorName = fmt.Sprintf(
			"syndication-pipeline-%s-%s",
			instance.Spec.AppName,
			strings.Replace(instance.Status.PipelineVersion, "_", "-", 1))

		log.Info(instance.Status.ConnectorName)

		instance.Status.TableName = fmt.Sprintf(
			"hosts_v%s",
			instance.Status.PipelineVersion)
		log.Info(instance.Status.TableName)
	}

	dbSchema, connectorConfig, err := parseConfig(instance, r)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.client.Status().Update(context.TODO(), instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("Setting up database")
	appDb, err := connectToAppDB(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	exists, err := checkIfTableExists(instance, appDb)
	if err != nil {
		return reconcile.Result{}, err
	}

	if exists != true {
		reqLogger.Info("Creating table")
		err = createTable(instance, appDb, dbSchema)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		reqLogger.Info("Table exists")
	}

	isValid, err := validate(instance, appDb)
	if err != nil {
		return reconcile.Result{}, err
	}
	reqLogger.Info(strconv.FormatBool(isValid))

	err = updateView(instance, appDb)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = createConnector(instance, r, connectorConfig)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = appDb.Close()
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: time.Second * 15}, nil

	return ctrl.Result{}, nil
}

func (r *CyndiPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cyndiv1beta1.CyndiPipeline{}).
		Complete(r)
}
