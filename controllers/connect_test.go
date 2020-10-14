package controllers

import (
	"context"
	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
)

var dbParams = DBParams{
	Host:     "db",
	Port:     "5432",
	Name:     "dbname",
	User:     "dbuser",
	Password: "dbpass",
}

func sampleConnectorConfig() ConnectorConfiguration {
	return ConnectorConfiguration{
		AppName:      "advisor",
		InsightsOnly: true,
		Cluster:      "cluster01",
		Topic:        "platform.inventory.events",
		TableName:    "table",
		DB:           dbParams,
		TasksMax:     8,
		BatchSize:    3000,
		MaxAge:       45,
		Template:     "{}",
	}
}

var _ = Describe("Connect", func() {
	var namespace string

	BeforeEach(func() {
		namespace = fmt.Sprintf("test-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		err := k8sClient.Create(context.TODO(), ns)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Create Connector", func() {
		It("Creates a simple connector", func() {
			const connectorName = "advisor-01"
			var config = ConnectorConfiguration{
				AppName:      "advisor",
				InsightsOnly: false,
				Cluster:      "cluster01",
				Topic:        "platform.inventory.events",
				TableName:    "tableName",
				DB:           dbParams,
				TasksMax:     8,
				BatchSize:    3000,
				MaxAge:       45,
				Template:     "{}",
			}

			err := CreateConnector(k8sClient, connectorName, namespace, config, nil, nil)
			Expect(err).ToNot(HaveOccurred())

			connector, err := GetConnector(k8sClient, connectorName, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(connector.GetName()).To(Equal(connectorName))

			labels := connector.GetLabels()
			Expect(labels["strimzi.io/cluster"]).To(Equal(config.Cluster))
			Expect(labels["cyndi/appName"]).To(Equal(config.AppName))
			Expect(labels["cyndi/insightsOnly"]).To(Equal("false"))
		})

		It("Creates a connector with interpolated configuration", func() {
			const connectorName = "advisor-02"
			const configTemplate = `
				{
					"tasks.max": "{{.TasksMax}}",
					"topics": "{{.Topic}}",
					"connection.url": "jdbc:postgresql://{{.DBHostname}}:{{.DBPort}}/{{.DBName}}",
					"connection.user": "{{.DBUser}}",
					"connection.password": "{{.DBPassword}}",
					"batch.size": "{{.BatchSize}}",
					"table.name.format": "{{.TableName}}",
					"max.age": "{{.MaxAge}}",
					{{ if eq .InsightsOnly "true" }}
					"transforms": "true"
					{{ else  }}
					"transforms": "false"
					{{ end }}
				}
			`

			var config = ConnectorConfiguration{
				AppName:      "advisor",
				InsightsOnly: false,
				Cluster:      "cluster01",
				Topic:        "platform.inventory.events",
				TableName:    "inventory.hosts001",
				DB:           dbParams,
				TasksMax:     64,
				BatchSize:    10,
				MaxAge:       45,
				Template:     configTemplate,
			}

			err := CreateConnector(k8sClient, connectorName, namespace, config, nil, nil)
			Expect(err).ToNot(HaveOccurred())

			connector, err := GetConnector(k8sClient, connectorName, namespace)
			Expect(err).ToNot(HaveOccurred())

			spec, ok, err := unstructured.NestedMap(connector.UnstructuredContent(), "spec")
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(spec).To(HaveKey("config"))
			Expect(spec).To(HaveKeyWithValue("class", "io.confluent.connect.jdbc.JdbcSinkConnector"))
			Expect(spec).To(HaveKeyWithValue("tasksMax", int64(64)))
			Expect(spec).To(HaveKeyWithValue("pause", false))

			Expect(spec["config"]).To(HaveKeyWithValue("tasks.max", "64"))
			Expect(spec["config"]).To(HaveKeyWithValue("topics", "platform.inventory.events"))
			Expect(spec["config"]).To(HaveKeyWithValue("connection.url", "jdbc:postgresql://db:5432/dbname"))
			Expect(spec["config"]).To(HaveKeyWithValue("connection.user", "dbuser"))
			Expect(spec["config"]).To(HaveKeyWithValue("connection.password", "dbpass"))
			Expect(spec["config"]).To(HaveKeyWithValue("batch.size", "10"))
			Expect(spec["config"]).To(HaveKeyWithValue("table.name.format", "inventory.hosts001"))
			Expect(spec["config"]).To(HaveKeyWithValue("max.age", "45"))
			Expect(spec["config"]).To(HaveKeyWithValue("transforms", "false"))
		})

		It("Sets the controller reference", func() {
			const connectorName = "advisor-01"
			var config = ConnectorConfiguration{
				AppName:      "advisor",
				InsightsOnly: false,
				Cluster:      "cluster01",
				Topic:        "platform.inventory.events",
				TableName:    "tableName",
				DB:           dbParams,
				TasksMax:     8,
				BatchSize:    3000,
				MaxAge:       45,
				Template:     "{}",
			}

			pipeline := &cyndiv1beta1.CyndiPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipeline-01",
					Namespace: namespace,
					UID:       "969f1d71-4187-432f-99aa-5accf8dc3fef",
				},
				Spec: cyndiv1beta1.CyndiPipelineSpec{
					AppName: config.AppName,
				},
			}

			err := CreateConnector(k8sClient, connectorName, namespace, config, pipeline, scheme.Scheme)
			Expect(err).ToNot(HaveOccurred())

			connector, err := GetConnector(k8sClient, connectorName, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(connector.GetName()).To(Equal(connectorName))

			references := connector.GetOwnerReferences()
			Expect(references).To(HaveLen(1))
			Expect(references[0].Name).To(Equal("pipeline-01"))
		})
	})

	Context("List Connectors", func() {
		It("Lists 0 connectors in an empty namespace", func() {
			connectors, err := GetConnectorsForApp(k8sClient, namespace, "advisor")
			Expect(err).ToNot(HaveOccurred())
			Expect(connectors.Items).To(HaveLen(0))
		})

		It("Lists connectors for the given app", func() {
			var advisorConfig = sampleConnectorConfig()
			advisorConfig.AppName = "advisor"
			var patchConfig = sampleConnectorConfig()
			patchConfig.AppName = "patch"

			err := CreateConnector(k8sClient, "advisor-01", namespace, advisorConfig, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			err = CreateConnector(k8sClient, "advisor-02", namespace, advisorConfig, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			err = CreateConnector(k8sClient, "patch-01", namespace, patchConfig, nil, nil)
			Expect(err).ToNot(HaveOccurred())

			advisorConnectors, err := GetConnectorsForApp(k8sClient, namespace, "advisor")
			Expect(err).ToNot(HaveOccurred())
			Expect(advisorConnectors.Items).To(HaveLen(2))

			patchConnectors, err := GetConnectorsForApp(k8sClient, namespace, "patch")
			Expect(err).ToNot(HaveOccurred())
			Expect(patchConnectors.Items).To(HaveLen(1))

			complianceConnectors, err := GetConnectorsForApp(k8sClient, namespace, "compliance")
			Expect(err).ToNot(HaveOccurred())
			Expect(complianceConnectors.Items).To(HaveLen(0))
		})
	})

	Context("Delete Connector", func() {
		It("Deletes a connector", func() {
			name := "connector-to-be-deleted-01"
			err := CreateConnector(k8sClient, name, namespace, sampleConnectorConfig(), nil, nil)
			Expect(err).ToNot(HaveOccurred())

			_, err = GetConnector(k8sClient, name, namespace)
			Expect(err).ToNot(HaveOccurred())

			err = DeleteConnector(k8sClient, name, namespace)
			Expect(err).ToNot(HaveOccurred())

			_, err = GetConnector(k8sClient, name, namespace)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("Does not fail on non-existing connector", func() {
			name := "connector-does-not-exist"

			err := DeleteConnector(k8sClient, name, namespace)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
