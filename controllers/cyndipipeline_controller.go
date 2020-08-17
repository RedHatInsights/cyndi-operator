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
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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

// +kubebuilder:rbac:groups=cyndi.cloud.redhat.com,resources=cyndipipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cyndi.cloud.redhat.com,resources=cyndipipelines/status,verbs=get;update;patch

func (r *CyndiPipelineReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	reqLogger.Info("Reconciling CyndiPipeline")

	instance := &cyndiv1beta1.CyndiPipeline{}

	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
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

	//new pipeline
	if instance.Status.PipelineVersion == "" {
		refreshPipelineVersion(instance)
		instance.Status.InitialSyncInProgress = true
	}

	dbSchema, connectorConfig, err := parseConfig(instance, r)
	if err != nil {
		return reconcile.Result{}, err
	}

	appDb, err := connectToAppDB(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	dbTableExists, err := checkIfTableExists(instance.Status.TableName, appDb)
	if err != nil {
		return reconcile.Result{}, err
	}

	connectorExists, err := checkIfConnectorExists(instance.Status.ConnectorName, instance.Namespace, r)
	if err != nil {
		return reconcile.Result{}, err
	}

	//part, or all, of the pipeline is missing, create a new pipeline
	if dbTableExists != true || connectorExists != true {
		if instance.Status.InitialSyncInProgress != true {
			instance.Status.PreviousPipelineVersion = instance.Status.PipelineVersion
			refreshPipelineVersion(instance)
		}

		err = createTable(instance.Status.TableName, appDb, dbSchema)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = createConnector(instance, r, connectorConfig)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	isValid, err := validate(instance, r, appDb)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.Client.Status().Update(context.TODO(), instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isValid != true {
		validationFailedCountThreshold := 10
		if instance.Status.InitialSyncInProgress == true {
			validationFailedCountThreshold = 60
		}

		if instance.Status.ValidationFailedCount == validationFailedCountThreshold {
			err = deleteTable(instance.Status.TableName, appDb)
			if err != nil {
				return reconcile.Result{}, err
			}

			err = deleteConnector(instance.Status.TableName, instance.Namespace, r)
			if err != nil {
				return reconcile.Result{}, err
			}

			return reconcile.Result{RequeueAfter: time.Second * 1}, nil
		} else {
			return reconcile.Result{RequeueAfter: time.Second * 15}, nil
		}
	}

	err = updateView(instance, appDb)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = appDb.Close()
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: time.Second * 15}, nil
}

func (r *CyndiPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cyndiv1beta1.CyndiPipeline{}).
		Complete(r)
}

func parseConfig(instance *cyndiv1beta1.CyndiPipeline, r *CyndiPipelineReconciler) (string, string, error) {
	cyndiConfig := &corev1.ConfigMap{}
	err := r.Client.Get(context.TODO(), client.ObjectKey{Name: "cyndi", Namespace: instance.Namespace}, cyndiConfig)
	if err != nil {
		return "", "", err
	}
	connectorConfig := cyndiConfig.Data["connector.config"]
	dbSchema := cyndiConfig.Data["db.schema"]
	return dbSchema, connectorConfig, err
}

func refreshPipelineVersion(instance *cyndiv1beta1.CyndiPipeline) {
	instance.Status.PipelineVersion = fmt.Sprintf(
		"1_%s",
		strconv.FormatInt(time.Now().UnixNano(), 10))

	instance.Status.ConnectorName = fmt.Sprintf(
		"syndication-pipeline-%s-%s",
		instance.Spec.AppName,
		strings.Replace(instance.Status.PipelineVersion, "_", "-", 1))

	instance.Status.TableName = fmt.Sprintf(
		"hosts_v%s",
		instance.Status.PipelineVersion)
}
