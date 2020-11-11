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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cyndi "cyndi-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"

	. "cyndi-operator/controllers/config"
	connect "cyndi-operator/controllers/connect"
	"cyndi-operator/controllers/database"
	"cyndi-operator/controllers/utils"
	"cyndi-operator/test"
	// +kubebuilder:scaffold:imports
)

/*
 * Tests for CyndiPipelineReconciler. ValidationController is mocked (see setPipelineValid)
 */

func getDBParams() DBParams {
	options := viper.New()
	options.SetDefault("DBHostHBI", "localhost")
	options.SetDefault("DBPort", "5432")
	options.SetDefault("DBUser", "postgres")
	options.SetDefault("DBPass", "postgres")
	options.SetDefault("DBName", "test")
	options.AutomaticEnv()

	return DBParams{
		Host:     options.GetString("DBHostHBI"),
		Port:     options.GetString("DBPort"),
		Name:     options.GetString("DBName"),
		User:     options.GetString("DBUser"),
		Password: options.GetString("DBPass"),
	}
}

func createPipeline(namespacedName types.NamespacedName, specs ...*cyndi.CyndiPipelineSpec) {
	var (
		ctx  = context.Background()
		spec *cyndi.CyndiPipelineSpec
	)

	Expect(len(specs) <= 1).To(BeTrue())

	if len(specs) == 1 {
		spec = specs[0]
	} else {
		spec = &cyndi.CyndiPipelineSpec{}
	}

	spec.AppName = namespacedName.Name

	pipeline := cyndi.CyndiPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: *spec,
	}

	err := test.Client.Create(ctx, &pipeline)
	Expect(err).ToNot(HaveOccurred())
}

func createDbSecret(namespace string, name string, params DBParams) {
	secret := &corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"db.host":     []byte(params.Host),
			"db.port":     []byte(params.Port),
			"db.name":     []byte(params.Name),
			"db.user":     []byte(params.User),
			"db.password": []byte(params.Password),
		},
	}

	err := test.Client.Create(context.TODO(), secret)
	Expect(err).ToNot(HaveOccurred())
}

func createConfigMap(namespace string, name string, data map[string]string) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}

	err := test.Client.Create(context.TODO(), configMap)
	Expect(err).ToNot(HaveOccurred())
}

func newCyndiReconciler() *CyndiPipelineReconciler {
	return NewCyndiReconciler(test.Client, test.Clientset, scheme.Scheme, logf.Log.WithName("test"), record.NewFakeRecorder(10))
}

func getPipeline(namespacedName types.NamespacedName) (pipeline *cyndi.CyndiPipeline) {
	pipeline, err := utils.FetchCyndiPipeline(test.Client, namespacedName)
	Expect(err).ToNot(HaveOccurred())
	return
}

func getConfigMap(namespace string) *corev1.ConfigMap {
	configMap, err := utils.FetchConfigMap(test.Client, namespace, "cyndi")
	Expect(err).ToNot(HaveOccurred())
	return configMap
}

func setPipelineValid(namespacedName types.NamespacedName, valid bool, fns ...func(i *cyndi.CyndiPipeline)) {
	pipeline, err := utils.FetchCyndiPipeline(test.Client, namespacedName)
	Expect(err).ToNot(HaveOccurred())

	if valid {
		pipeline.Status.InitialSyncInProgress = false
		pipeline.SetValid(metav1.ConditionTrue, "ValidationSucceeded", "Validation succeeded", -1)
	} else {
		pipeline.SetValid(metav1.ConditionFalse, "ValidationFailed", "Validation failed", -1)
	}

	for _, fn := range fns {
		fn(pipeline)
	}

	err = test.Client.Status().Update(context.TODO(), pipeline)
	Expect(err).ToNot(HaveOccurred())
}

func TestControllers(t *testing.T) {
	test.Setup(t, "Controllers")
}

var _ = Describe("Pipeline operations", func() {
	var (
		namespacedName types.NamespacedName
		dbParams       DBParams
		db             *database.AppDatabase
		r              *CyndiPipelineReconciler
	)

	var reconcile = func() (result ctrl.Result) {
		result, err := r.Reconcile(ctrl.Request{NamespacedName: namespacedName})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())
		return
	}

	BeforeEach(func() {
		namespacedName = types.NamespacedName{
			Name:      "test-pipeline-01",
			Namespace: test.UniqueNamespace(),
		}

		r = newCyndiReconciler()

		dbParams = getDBParams()

		createDbSecret(namespacedName.Namespace, "host-inventory-db", dbParams)
		createDbSecret(namespacedName.Namespace, utils.AppDbSecretName(namespacedName.Name), dbParams)

		db = database.NewAppDatabase(&dbParams)
		err := db.Connect()
		Expect(err).ToNot(HaveOccurred())

		_, err = db.Exec(`DROP SCHEMA IF EXISTS "inventory" CASCADE; CREATE SCHEMA "inventory";`)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		db.Close()
	})

	Describe("New -> InitialSync", func() {
		It("Creates a connector and db table for a new pipeline", func() {
			createPipeline(namespacedName)
			reconcile()

			pipeline := getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeTrue())
			Expect(pipeline.GetValid()).To(Equal(metav1.ConditionUnknown))
			Expect(pipeline.Status.ActiveTableName).To(Equal(""))

			connector, err := connect.GetConnector(test.Client, pipeline.Status.ConnectorName, namespacedName.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(connector.GetLabels()["cyndi/appName"]).To(Equal(namespacedName.Name))
			Expect(connector.GetLabels()["cyndi/insightsOnly"]).To(Equal("false"))
			Expect(connector.GetLabels()["strimzi.io/cluster"]).To(Equal("xjoin-kafka-connect-strimzi"))

			exists, err := db.CheckIfTableExists(pipeline.Status.TableName)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("Considers configmap configuration", func() {
			createPipeline(namespacedName)
			createConfigMap(namespacedName.Namespace, "cyndi", map[string]string{"connect.cluster": "test01"})
			reconcile()

			pipeline := getPipeline(namespacedName)
			connector, err := connect.GetConnector(test.Client, pipeline.Status.ConnectorName, namespacedName.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(connector.GetLabels()["cyndi/appName"]).To(Equal(namespacedName.Name))
			Expect(connector.GetLabels()["cyndi/insightsOnly"]).To(Equal("false"))
			Expect(connector.GetLabels()["strimzi.io/cluster"]).To(Equal("test01"))
		})

		It("Removes stale connectors", func() {
			createPipeline(namespacedName)
			reconcile()

			// simulate multiple tables/connectors left behind (e.g. due to operator error)
			pipeline := getPipeline(namespacedName)
			pipeline.Status.PipelineVersion = ""
			err := test.Client.Status().Update(context.TODO(), pipeline)
			Expect(err).ToNot(HaveOccurred())
			reconcile()

			pipeline = getPipeline(namespacedName)
			pipeline.Status.PipelineVersion = ""
			err = test.Client.Status().Update(context.TODO(), pipeline)
			Expect(err).ToNot(HaveOccurred())
			reconcile()

			pipeline = getPipeline(namespacedName)
			connectors, err := connect.GetConnectorsForOwner(test.Client, namespacedName.Namespace, pipeline.GetUIDString())
			Expect(err).ToNot(HaveOccurred())
			Expect(connectors.Items).To(HaveLen(1))
			Expect(connectors.Items[0].GetName()).To(Equal(pipeline.Status.ConnectorName))
		})

		It("Removes stale tables", func() {
			createPipeline(namespacedName)
			reconcile()

			// simulate multiple tables/connectors left behind (e.g. due to operator error)
			pipeline := getPipeline(namespacedName)
			pipeline.Status.PipelineVersion = ""
			err := test.Client.Status().Update(context.TODO(), pipeline)
			Expect(err).ToNot(HaveOccurred())
			reconcile()

			pipeline = getPipeline(namespacedName)
			pipeline.Status.PipelineVersion = ""
			err = test.Client.Status().Update(context.TODO(), pipeline)
			Expect(err).ToNot(HaveOccurred())
			reconcile()

			pipeline = getPipeline(namespacedName)
			tables, err := db.GetCyndiTables()
			Expect(err).ToNot(HaveOccurred())
			Expect(tables).To(HaveLen(1))
			Expect(tables[0]).To(Equal(pipeline.Status.TableName))
		})
	})

	Describe("InitialSync -> Valid", func() {
		It("Creates the hosts view", func() {
			createPipeline(namespacedName)
			reconcile()

			pipeline := getPipeline(namespacedName)
			setPipelineValid(namespacedName, true)
			reconcile()

			pipeline = getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeFalse())
			Expect(pipeline.Status.ActiveTableName).ToNot(Equal(""))

			viewExists, err := db.CheckIfTableExists("hosts")
			Expect(err).ToNot(HaveOccurred())
			Expect(viewExists).To(BeTrue())
		})

		It("Triggers refresh if pipeline fails to become valid for too long", func() {
			createPipeline(namespacedName)
			reconcile()

			setPipelineValid(namespacedName, false, func(pipeline *cyndi.CyndiPipeline) {
				pipeline.Status.ValidationFailedCount = 16
			})

			reconcile()

			pipeline := getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(0)))
			Expect(pipeline.Status.PipelineVersion).To(Equal(""))
		})
	})

	Describe("Invalid -> New", func() {
		It("Triggers refresh if pipeline in invalid for too long", func() {
			createPipeline(namespacedName)
			reconcile()

			pipeline := getPipeline(namespacedName)
			setPipelineValid(namespacedName, true)
			reconcile()

			setPipelineValid(namespacedName, false, func(pipeline *cyndi.CyndiPipeline) {
				pipeline.Status.ValidationFailedCount = 6
			})

			reconcile()

			pipeline = getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(0)))
			Expect(pipeline.Status.PipelineVersion).To(Equal(""))
		})

		Context("In a refresh", func() {
			It("Keeps the old table active until the new one is valid", func() {
				createPipeline(namespacedName)
				reconcile()

				pipeline := getPipeline(namespacedName)
				setPipelineValid(namespacedName, true)
				reconcile()

				pipeline = getPipeline(namespacedName)
				activeTableName := pipeline.Status.ActiveTableName
				setPipelineValid(namespacedName, false, func(pipeline *cyndi.CyndiPipeline) {
					pipeline.Status.ValidationFailedCount = 6
				})

				reconcile()

				pipeline = getPipeline(namespacedName)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
				Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(0)))
				Expect(pipeline.Status.PipelineVersion).To(Equal(""))
				Expect(pipeline.Status.ActiveTableName).To(Equal(activeTableName))
				reconcile()

				pipeline = getPipeline(namespacedName)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))
				// while the new table is being seeded, the old one should still remain active (backing the hosts view)
				Expect(pipeline.Status.ActiveTableName).To(Equal(activeTableName))

				table, err := db.GetCurrentTable()
				Expect(err).ToNot(HaveOccurred())
				Expect(*table).To(Equal(pipeline.Status.ActiveTableName))

				setPipelineValid(namespacedName, true)
				reconcile()

				// at this point the new table should be active
				pipeline = getPipeline(namespacedName)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))
				Expect(pipeline.Status.ActiveTableName).ToNot(Equal(activeTableName))
			})
		})
	})

	Describe("Valid -> New", func() {
		It("Triggers refresh if configmap is created", func() {
			createPipeline(namespacedName)
			reconcile()

			setPipelineValid(namespacedName, true)
			reconcile()

			pipeline := getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))
			pipelineVersion := pipeline.Status.PipelineVersion
			tableName := pipeline.Status.TableName
			Expect(pipeline.Status.CyndiConfigVersion).To(Equal("-1"))

			// now add a new configmap
			createConfigMap(namespacedName.Namespace, "cyndi", map[string]string{"connect.cluster": "override"})
			reconcile()

			pipeline = getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeFalse())
			Expect(pipeline.GetValid()).To(Equal(metav1.ConditionUnknown))
			Expect(pipeline.Status.PipelineVersion).ToNot(Equal(pipelineVersion))

			// ensure the view still points to the old table while the new one is being synchronized
			table, err := db.GetCurrentTable()
			Expect(err).ToNot(HaveOccurred())
			Expect(*table).To(Equal(tableName))
			Expect(*table).To(Equal(pipeline.Status.ActiveTableName))
		})

		It("Triggers refresh if configmap changes", func() {
			createConfigMap(namespacedName.Namespace, "cyndi", map[string]string{"connect.cluster": "value1"})

			createPipeline(namespacedName)
			reconcile()

			setPipelineValid(namespacedName, true)
			reconcile()

			pipeline := getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))
			configMap := getConfigMap(namespacedName.Namespace)
			pipelineVersion := pipeline.Status.PipelineVersion
			tableName := pipeline.Status.TableName
			Expect(pipeline.Status.CyndiConfigVersion).To(Equal("1768911245"))

			// with pipeline in the Valid state, change the configmap
			configMap.Data = map[string]string{"connect.cluster": "value2"}
			err := test.Client.Update(context.TODO(), configMap)
			Expect(err).ToNot(HaveOccurred())

			reconcile()

			// as a result, the pipeline should start re-sync to reflect the change
			configMap = getConfigMap(namespacedName.Namespace)
			pipeline = getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeFalse())
			Expect(pipeline.GetValid()).To(Equal(metav1.ConditionUnknown))
			Expect(pipeline.Status.PipelineVersion).ToNot(Equal(pipelineVersion))

			// ensure the view still points to the old table while the new one is being synchronized
			table, err := db.GetCurrentTable()
			Expect(err).ToNot(HaveOccurred())
			Expect(*table).To(Equal(tableName))
			Expect(*table).To(Equal(pipeline.Status.ActiveTableName))
		})

		It("Triggers refresh if table disappears", func() {
			createPipeline(namespacedName)
			reconcile()

			setPipelineValid(namespacedName, true)
			reconcile()

			pipeline := getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))

			err := db.DeleteTable(pipeline.Status.TableName)
			Expect(err).ToNot(HaveOccurred())
			reconcile()

			pipeline = getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
		})

		It("Triggers refresh if connector disappears", func() {
			createPipeline(namespacedName)
			reconcile()

			setPipelineValid(namespacedName, true)
			reconcile()

			pipeline := getPipeline(namespacedName)
			tableName := pipeline.Status.ActiveTableName
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))

			err := connect.DeleteConnector(test.Client, pipeline.Status.ConnectorName, namespacedName.Namespace)
			Expect(err).ToNot(HaveOccurred())
			reconcile()

			pipeline = getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
			Expect(pipeline.Status.ActiveTableName).To(Equal(tableName))
		})

		It("Triggers refresh if connector configuration disagrees", func() {
			createPipeline(namespacedName)
			reconcile()

			setPipelineValid(namespacedName, true)
			reconcile()

			pipeline := getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))

			pipeline.Spec.InsightsOnly = true
			err := test.Client.Update(context.TODO(), pipeline)
			Expect(err).ToNot(HaveOccurred())
			reconcile()

			pipeline = getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
		})

		It("Triggers refresh if connect cluster changes", func() {
			createConfigMap(namespacedName.Namespace, "cyndi", map[string]string{"connect.cluster": "cluster01"})
			createPipeline(namespacedName)
			reconcile()

			setPipelineValid(namespacedName, true)
			reconcile()

			pipeline := getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))

			cluster := "cluster02"
			pipeline.Spec.ConnectCluster = &cluster
			err := test.Client.Update(context.TODO(), pipeline)
			Expect(err).ToNot(HaveOccurred())
			reconcile()

			pipeline = getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
			reconcile()

			pipeline = getPipeline(namespacedName)
			connector, err := connect.GetConnector(test.Client, pipeline.Status.ConnectorName, namespacedName.Namespace)
			Expect(connector.GetLabels()["strimzi.io/cluster"]).To(Equal(cluster))
		})

		It("Triggers refresh if MaxAge changes", func() {
			createConfigMap(namespacedName.Namespace, "cyndi", map[string]string{"connector.max.age": "10"})
			createPipeline(namespacedName)
			reconcile()

			setPipelineValid(namespacedName, true)
			reconcile()

			pipeline := getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))

			var newValue int64 = 5
			pipeline.Spec.MaxAge = &newValue
			err := test.Client.Update(context.TODO(), pipeline)
			Expect(err).ToNot(HaveOccurred())
			reconcile()

			pipeline = getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
			reconcile()

			pipeline = getPipeline(namespacedName)
			connector, err := connect.GetConnector(test.Client, pipeline.Status.ConnectorName, namespacedName.Namespace)
			Expect(connector.GetLabels()["cyndi/maxAge"]).To(Equal("5"))
		})
	})

	Describe("-> Removed", func() {
		It("Artifacts removed when initializing pipeline is removed", func() {
			createPipeline(namespacedName)
			reconcile()

			pipeline := getPipeline(namespacedName)
			status := pipeline.Status

			err := test.Client.Delete(context.TODO(), pipeline)
			Expect(err).ToNot(HaveOccurred())

			result := reconcile()
			Expect(result).To(BeZero())

			tableExists, err := db.CheckIfTableExists(status.TableName)
			Expect(err).ToNot(HaveOccurred())
			Expect(tableExists).To(BeFalse())

			viewExists, err := db.CheckIfTableExists("hosts")
			Expect(err).ToNot(HaveOccurred())
			Expect(viewExists).To(BeFalse())

			pipeline, err = utils.FetchCyndiPipeline(test.Client, namespacedName)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(BeTrue())

			connectors, err := connect.GetConnectorsForOwner(test.Client, namespacedName.Namespace, pipeline.GetUIDString())
			Expect(err).ToNot(HaveOccurred())
			Expect(connectors.Items).To(BeEmpty())

			Expect(reconcile()).To(BeZero())
		})

		It("Artifacts removed when valid pipeline is removed", func() {
			createPipeline(namespacedName)
			reconcile()

			setPipelineValid(namespacedName, true)
			reconcile()

			pipeline := getPipeline(namespacedName)
			status := pipeline.Status

			err := test.Client.Delete(context.TODO(), pipeline)
			Expect(err).ToNot(HaveOccurred())

			result := reconcile()
			Expect(result).To(BeZero())

			tableExists, err := db.CheckIfTableExists(status.TableName)
			Expect(err).ToNot(HaveOccurred())
			Expect(tableExists).To(BeFalse())

			viewExists, err := db.CheckIfTableExists("hosts")
			Expect(err).ToNot(HaveOccurred())
			Expect(viewExists).To(BeFalse())

			pipeline, err = utils.FetchCyndiPipeline(test.Client, namespacedName)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(BeTrue())

			connectors, err := connect.GetConnectorsForOwner(test.Client, namespacedName.Namespace, pipeline.GetUIDString())
			Expect(err).ToNot(HaveOccurred())
			Expect(connectors.Items).To(BeEmpty())

			Expect(reconcile()).To(BeZero())
		})
	})

	Describe("Failures", func() {
		It("Fails if App DB secret is missing", func() {
			appDbSecret, err := utils.FetchSecret(test.Client, namespacedName.Namespace, utils.AppDbSecretName(namespacedName.Name))
			Expect(err).ToNot(HaveOccurred())
			err = test.Client.Delete(context.TODO(), appDbSecret)
			Expect(err).ToNot(HaveOccurred())

			createPipeline(namespacedName)
			_, err = r.Reconcile(ctrl.Request{NamespacedName: namespacedName})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(`secrets "test-pipeline-01-db" not found`))

			recorder, _ := r.Recorder.(*record.FakeRecorder)
			Expect(recorder.Events).To(HaveLen(1))
		})

		It("Fails if App DB secret is misconfigured", func() {
			appDbSecret, err := utils.FetchSecret(test.Client, namespacedName.Namespace, utils.AppDbSecretName(namespacedName.Name))
			Expect(err).ToNot(HaveOccurred())

			appDbSecret.Data["db.host"] = []byte("localhost")
			appDbSecret.Data["db.port"] = []byte("55432")
			err = test.Client.Update(context.TODO(), appDbSecret)
			Expect(err).ToNot(HaveOccurred())

			createPipeline(namespacedName)
			_, err = r.Reconcile(ctrl.Request{NamespacedName: namespacedName})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix(`Error connecting to localhost:55432/test as postgres`))

			recorder, _ := r.Recorder.(*record.FakeRecorder)
			Expect(recorder.Events).To(HaveLen(1))
		})

		It("Fails if the configmap is misconfigured", func() {
			createConfigMap(namespacedName.Namespace, "cyndi", map[string]string{"standard.interval": "abcd"})
			createPipeline(namespacedName)

			_, err := r.Reconcile(ctrl.Request{NamespacedName: namespacedName})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(fmt.Sprintf(`Error parsing cyndi configmap in %s: "abcd" is not a valid value for "standard.interval"`, namespacedName.Namespace)))

			recorder, _ := r.Recorder.(*record.FakeRecorder)
			Expect(recorder.Events).To(HaveLen(1))
		})

		It("Fails if DB table cannot be created", func() {
			_, err := db.Exec("DROP SCHEMA inventory CASCADE")
			Expect(err).ToNot(HaveOccurred())

			createPipeline(namespacedName)
			_, err = r.Reconcile(ctrl.Request{NamespacedName: namespacedName})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("Error executing query"))

			recorder, _ := r.Recorder.(*record.FakeRecorder)
			Expect(recorder.Events).To(HaveLen(2))
		})

		It("Fails if inventory.hosts view cannot be created", func() {
			createPipeline(namespacedName)
			reconcile()

			setPipelineValid(namespacedName, true)

			_, err := db.Exec("CREATE TABLE inventory.hosts ()")
			Expect(err).ToNot(HaveOccurred())

			_, err = r.Reconcile(ctrl.Request{NamespacedName: namespacedName})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("Error executing query"))

			recorder, _ := r.Recorder.(*record.FakeRecorder)
			Expect(recorder.Events).To(HaveLen(2))
		})
	})
})
