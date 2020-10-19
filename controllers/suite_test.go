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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v1"

	cyndiv1beta1 "cyndi-operator/api/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "cyndi-operator/controllers/config"
	"cyndi-operator/test"
	// +kubebuilder:scaffold:imports
)

type DBConfig struct {
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

func getTestConfig() *DBConfig {
	options := viper.New()
	options.SetDefault("DBHostHBI", "localhost")
	options.SetDefault("DBHostApp", "localhost")
	options.SetDefault("DBPort", "5432")
	options.SetDefault("DBUser", "postgres")
	options.SetDefault("DBPass", "postgres")
	options.SetDefault("DBName", "test")
	options.AutomaticEnv()

	return &DBConfig{
		DBHostHBI: options.GetString("DBHostHBI"),
		DBHostAPP: options.GetString("DBHostApp"),
		DBPort:    options.GetString("DBPort"),
		DBUser:    options.GetString("DBUser"),
		DBPass:    options.GetString("DBPass"),
		DBName:    options.GetString("DBName"),
	}
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

	err := test.Client.Create(ctx, &pipeline)

	if err != nil {
		return err
	}

	err = test.Client.Status().Update(ctx, &pipeline)

	if err != nil {
		return err
	}

	return nil
}

func TestControllers(t *testing.T) {
	test.Setup(t, "Controllers")
}

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

		/*
			This is just a basic skeleton. For this to be useful the test needs to be finished.

			TODO:
			 - configmap
			 - mock db access

			 _, err = r.Reconcile(req)


			r := &CyndiPipelineReconciler{Client: test.Client, Scheme: scheme.Scheme, Log: logf.Log.WithName("test")}

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
