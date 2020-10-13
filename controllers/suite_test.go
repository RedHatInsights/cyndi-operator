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
	"io/ioutil"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	cyndiv1beta1 "cyndi-operator/api/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

type Config struct {
	DBHostHBI string
	DBHostAPP string
	DBPort    string
	DBPass    string
	DBUser    string
	DBName    string
}

type ConfigMap struct {
	APIVersion string `yaml:"apiVersion"`
	Data       struct {
		ConnConfig                        string `yaml:"connector.config"`
		DBSchema                          string `yaml:"db.schema"`
		ValidationInterval                string `yaml:"validation.interval"`
		ValidationAttemptsThreshold       string `yaml:"validation.attempts.threshold"`
		ValidationPercentageThreshold     string `yaml:"validation.percentage.threshold"`
		InitValidationInterval            string `yaml:"init.validation.interval"`
		InitValidationAttemptsThreshold   string `yaml:"init.validation.attempts.threshold"`
		InitValidationPrecentageThreshold string `yaml:"init.validation.percentage.threshold"`
		ConnectCluster                    string `yaml:"connect.cluster"`
		ConnectorTasksMax                 string `yaml:"connector.tasks.max"`
	} `yaml:"data"`
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
}

func (c *ConfigMap) getConfigMap() (*ConfigMap, error) {

	yamlFile, err := ioutil.ReadFile("../examples/cyndi.configmap.yml")
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

func createPipeline(namespace string, name string) error {
	ctx := context.Background()

	pipeline := cyndiv1beta1.CyndiPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cyndiv1beta1.CyndiPipelineSpec{
			AppName: name,
		},
	}

	err := k8sClient.Create(ctx, &pipeline)

	if err != nil {
		return err
	}

	err = k8sClient.Status().Update(ctx, &pipeline)

	if err != nil {
		return err
	}

	return nil
}

func getDBArgs(host string) DBParams {
	cfg := getTestConfig()
	return DBParams{
		Host:     host,
		Port:     cfg.DBPort,
		Name:     cfg.DBName,
		User:     cfg.DBUser,
		Password: cfg.DBPass,
	}
}

func getTestConfig() *Config {
	options := viper.New()
	options.SetDefault("DBHostHBI", "localhost")
	options.SetDefault("DBHostApp", "localhost")
	options.SetDefault("DBPort", "5432")
	options.SetDefault("DBUser", "postgres")
	options.SetDefault("DBPass", "postgres")
	options.SetDefault("DBName", "test")
	options.AutomaticEnv()

	return &Config{
		DBHostHBI: options.GetString("DBHostHBI"),
		DBHostAPP: options.GetString("DBHostApp"),
		DBPort:    options.GetString("DBPort"),
		DBUser:    options.GetString("DBUser"),
		DBPass:    options.GetString("DBPass"),
		DBName:    options.GetString("DBName"),
	}
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = cyndiv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:Scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Pipeline provisioning", func() {
	Context("Basic", func() {
		It("Should create a Kafka Connector", func() {
			const (
				name      = "test01"
				namespace = "default"
			)

			err := createPipeline(namespace, name)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should connect to the DB", func() {
			cfg := getTestConfig()
			params := getDBArgs(cfg.DBHostHBI)

			_, err := connectToDB(params)
			Expect(err).ToNot(HaveOccurred())
		})

		/*
			This is just a basic skeleton. For this to be useful the test needs to be finished.

			TODO:
			 - configmap
			 - mock db access

			 _, err = r.Reconcile(req)


			r := &CyndiPipelineReconciler{Client: k8sClient, Scheme: scheme.Scheme, Log: logf.Log.WithName("test")}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      name,
					Namespace: namespace,
				},
			}

			Expect(err).ToNot(HaveOccurred())
		*/
	})
})

var _ = Describe("Database operations", func() {

	var (
		RI *ReconcileIteration
		c  ConfigMap
	)

	config := getTestConfig()
	params := getDBArgs(config.DBHostHBI)

	BeforeEach(func() {
		dbconn, err := connectToDB(params)
		Expect(err).ToNot(HaveOccurred())

		c.getConfigMap()

		RI = &ReconcileIteration{
			Client:      k8sClient,
			AppDb:       dbconn,
			AppDBParams: params,
			DBSchema:    c.Data.DBSchema,
		}
	})

	AfterEach(func() {
		RI.closeDB(RI.AppDb)
	})

	Context("with successful connection", func() {
		It("should successfully query", func() {
			query := `CREATE SCHEMA "inventory";`
			_, err := RI.runQuery(RI.AppDb, query)
			Expect(err).To(BeNil())
		})

		It("should be able to create a table", func() {
			err := RI.createTable("test_table")
			Expect(err).To(BeNil())

		})

		It("should check for table existence", func() {
			exists, err := RI.checkIfTableExists("test_table")
			Expect(exists).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be able to delete the table", func() {
			err := RI.deleteTable("test_table")
			Expect(err).ToNot(HaveOccurred())
		})
	})

})
