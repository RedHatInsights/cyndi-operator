package connect

import (
	"context"
	"fmt"
	"testing"
	"time"

	cyndi "github.com/RedHatInsights/cyndi-operator/api/v1alpha1"
	"github.com/RedHatInsights/cyndi-operator/test"

	. "github.com/RedHatInsights/cyndi-operator/controllers/config"
	"github.com/RedHatInsights/cyndi-operator/controllers/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestConnect(t *testing.T) {
	test.Setup(t, "Connect")
}

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

	var createPipeline = func(appName string) *cyndi.CyndiPipeline {
		pipeline := &cyndi.CyndiPipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: cyndi.CyndiPipelineSpec{
				AppName: appName,
			},
		}

		err := test.Client.Create(context.TODO(), pipeline)
		Expect(err).ToNot(HaveOccurred())

		pipeline, err = utils.FetchCyndiPipeline(test.Client, types.NamespacedName{
			Name:      appName,
			Namespace: namespace,
		})

		Expect(err).ToNot(HaveOccurred())
		return pipeline
	}

	BeforeEach(func() {
		namespace = fmt.Sprintf("test-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		err := test.Client.Create(context.TODO(), ns)
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

			_, err := CreateConnector(test.Client, connectorName, namespace, config, nil, nil, false)
			Expect(err).ToNot(HaveOccurred())

			connector, err := GetConnector(test.Client, connectorName, namespace)
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

			_, err := CreateConnector(test.Client, connectorName, namespace, config, nil, nil, false)
			Expect(err).ToNot(HaveOccurred())

			connector, err := GetConnector(test.Client, connectorName, namespace)
			Expect(err).ToNot(HaveOccurred())

			spec, ok, err := unstructured.NestedMap(connector.UnstructuredContent(), "spec")
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(spec).To(HaveKey("config"))
			Expect(spec).To(HaveKeyWithValue("class", "io.confluent.connect.jdbc.JdbcSinkConnector"))
			Expect(spec).To(HaveKeyWithValue("tasksMax", int64(64)))
			Expect(spec).To(HaveKeyWithValue("state", "running"))

			Expect(spec["config"]).To(HaveKeyWithValue("tasks.max", "64"))
			Expect(spec["config"]).To(HaveKeyWithValue("topics", "platform.inventory.events"))
			Expect(spec["config"]).To(HaveKeyWithValue("connection.url", "jdbc:postgresql://${env:ADVISOR_DB_HOSTNAME}:${env:ADVISOR_DB_PORT}/${env:ADVISOR_DB_NAME}"))
			Expect(spec["config"]).To(HaveKeyWithValue("connection.user", "${env:ADVISOR_DB_USERNAME}"))
			Expect(spec["config"]).To(HaveKeyWithValue("connection.password", "${env:ADVISOR_DB_PASSWORD}"))
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

			pipeline := &cyndi.CyndiPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipeline-01",
					Namespace: namespace,
					UID:       "969f1d71-4187-432f-99aa-5accf8dc3fef",
				},
				Spec: cyndi.CyndiPipelineSpec{
					AppName: config.AppName,
				},
			}

			_, err := CreateConnector(test.Client, connectorName, namespace, config, pipeline, scheme.Scheme, false)
			Expect(err).ToNot(HaveOccurred())

			connector, err := GetConnector(test.Client, connectorName, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(connector.GetName()).To(Equal(connectorName))
			Expect(connector.GetLabels()[LabelOwner]).To(Equal(pipeline.GetUIDString()))

			references := connector.GetOwnerReferences()
			Expect(references).To(HaveLen(1))
			Expect(references[0].Name).To(Equal("pipeline-01"))
		})
	})

	Context("List Connectors", func() {
		It("Lists 0 connectors in an empty namespace", func() {
			pipeline := createPipeline("test-01")
			connectors, err := GetConnectorsForOwner(test.Client, namespace, pipeline.GetUIDString())
			Expect(err).ToNot(HaveOccurred())
			Expect(connectors.Items).To(HaveLen(0))
		})

		It("Lists connectors for the given app", func() {
			var advisorConfig = sampleConnectorConfig()
			advisorConfig.AppName = "advisor"
			var patchConfig = sampleConnectorConfig()
			patchConfig.AppName = "patch"

			var (
				advisorPipeline    = createPipeline("advisor")
				patchPipeline      = createPipeline("patch")
				compliancePipeline = createPipeline("compliance")
			)

			_, err := CreateConnector(test.Client, "advisor-01", namespace, advisorConfig, advisorPipeline, scheme.Scheme, false)
			Expect(err).ToNot(HaveOccurred())
			_, err = CreateConnector(test.Client, "advisor-02", namespace, advisorConfig, advisorPipeline, scheme.Scheme, false)
			Expect(err).ToNot(HaveOccurred())
			_, err = CreateConnector(test.Client, "patch-01", namespace, patchConfig, patchPipeline, scheme.Scheme, false)
			Expect(err).ToNot(HaveOccurred())

			advisorConnectors, err := GetConnectorsForOwner(test.Client, namespace, advisorPipeline.GetUIDString())
			Expect(err).ToNot(HaveOccurred())
			Expect(advisorConnectors.Items).To(HaveLen(2))

			patchConnectors, err := GetConnectorsForOwner(test.Client, namespace, patchPipeline.GetUIDString())
			Expect(err).ToNot(HaveOccurred())
			Expect(patchConnectors.Items).To(HaveLen(1))

			complianceConnectors, err := GetConnectorsForOwner(test.Client, namespace, compliancePipeline.GetUIDString())
			Expect(err).ToNot(HaveOccurred())
			Expect(complianceConnectors.Items).To(HaveLen(0))
		})
	})

	Context("Delete Connector", func() {
		It("Deletes a connector", func() {
			name := "connector-to-be-deleted-01"
			_, err := CreateConnector(test.Client, name, namespace, sampleConnectorConfig(), nil, nil, false)
			Expect(err).ToNot(HaveOccurred())

			_, err = GetConnector(test.Client, name, namespace)
			Expect(err).ToNot(HaveOccurred())

			err = DeleteConnector(test.Client, name, namespace)
			Expect(err).ToNot(HaveOccurred())

			_, err = GetConnector(test.Client, name, namespace)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("Does not fail on non-existing connector", func() {
			name := "connector-does-not-exist"

			err := DeleteConnector(test.Client, name, namespace)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("IsFailed", func() {
		It("Does not consider an empty connector to be FAILED", func() {
			connector, err := newConnectorResource("test01", namespace, sampleConnectorConfig())
			Expect(err).ToNot(HaveOccurred())
			Expect(IsFailed(connector)).To(BeFalse())
		})

		It("Correctly flags a failed connector", func() {
			connector := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"connectorStatus": map[string]interface{}{
							"connector": map[string]interface{}{
								"state": "FAILED",
							},
						},
					},
				},
			}

			Expect(IsFailed(connector)).To(BeTrue())
		})

		It("Correctly recognizes a running connector", func() {
			connector := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"connectorStatus": map[string]interface{}{
							"connector": map[string]interface{}{
								"state": "RUNNING",
							},
							"tasks": []interface{}{
								map[string]interface{}{
									"state": "RUNNING",
								},
								map[string]interface{}{
									"state": "RUNNING",
								},
							},
						},
					},
				},
			}

			Expect(IsFailed(connector)).To(BeFalse())
		})

		It("Correctly flags a connector with one failed task", func() {
			connector := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"connectorStatus": map[string]interface{}{
							"connector": map[string]interface{}{
								"state": "RUNNING",
							},
							"tasks": []interface{}{
								map[string]interface{}{
									"state": "RUNNING",
								},
								map[string]interface{}{
									"state": "RUNNING",
								},
								map[string]interface{}{
									"state": "FAILED",
								},
							},
						},
					},
				},
			}

			Expect(IsFailed(connector)).To(BeTrue())
		})
	})
})
