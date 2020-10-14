package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const Topic = "platform.inventory.events"
const appNameLabel = "cyndi/appName"

var connectorGVK = schema.GroupVersionKind{
	Group:   "kafka.strimzi.io",
	Kind:    "KafkaConnector",
	Version: "v1alpha1",
}

var connectorsGVK = schema.GroupVersionKind{
	Group:   "kafka.strimzi.io",
	Kind:    "KafkaConnectorList",
	Version: "v1alpha1",
}

type ConnectorConfiguration struct {
	AppName      string
	InsightsOnly bool
	Cluster      string
	Topic        string
	TableName    string
	DB           DBParams
	TasksMax     int64
	BatchSize    int64
	MaxAge       int64
	Template     string
}

func (i ReconcileIteration) checkIfConnectorExists(connectorName string) (bool, error) {
	if connectorName == "" {
		return false, nil
	}

	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "KafkaConnector",
		Version: "kafka.strimzi.io/v1alpha1",
	})

	err := i.Client.Get(context.TODO(), client.ObjectKey{Name: connectorName, Namespace: i.Instance.Namespace}, found)

	if err != nil && errors.IsNotFound(err) {
		return false, nil
	} else if err == nil {
		return true, nil
	} else {
		return false, err
	}
}

func newConnectorResource(name string, namespace string, config ConnectorConfiguration) (*unstructured.Unstructured, error) {
	m := make(map[string]string)
	m["TableName"] = config.TableName
	m["Topic"] = config.Topic
	m["DBPort"] = config.DB.Port
	m["DBHostname"] = config.DB.Host
	m["DBName"] = config.DB.Name
	m["DBUser"] = config.DB.User
	m["DBPassword"] = config.DB.Password
	m["TasksMax"] = strconv.FormatInt(config.TasksMax, 10) // TODO: why int64?
	m["BatchSize"] = strconv.FormatInt(config.BatchSize, 10)
	m["MaxAge"] = strconv.FormatInt(config.MaxAge, 10)
	m["InsightsOnly"] = strconv.FormatBool(config.InsightsOnly)
	tmpl, err := template.New("configTemplate").Parse(config.Template)
	if err != nil {
		return nil, err
	}

	var configTemplateBuffer bytes.Buffer
	err = tmpl.Execute(&configTemplateBuffer, m)
	if err != nil {
		return nil, err
	}
	configTemplateParsed := configTemplateBuffer.String()

	var configTemplateInterface interface{}

	err = json.Unmarshal([]byte(strings.ReplaceAll(configTemplateParsed, "\n", "")), &configTemplateInterface)
	if err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{}
	u.Object = map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"labels": map[string]interface{}{
				"strimzi.io/cluster": config.Cluster,
				appNameLabel:         config.AppName,
				"cyndi/insightsOnly": strconv.FormatBool(config.InsightsOnly),
			},
		},
		"spec": map[string]interface{}{
			"tasksMax": config.TasksMax,
			"class":    "io.confluent.connect.jdbc.JdbcSinkConnector",
			"config":   configTemplateInterface,
			"pause":    false,
		},
	}

	u.SetGroupVersionKind(connectorGVK)
	return u, nil
}

// TODO refactor
func (i *ReconcileIteration) createConnector() error {
	var config = ConnectorConfiguration{
		AppName:      i.Instance.Spec.AppName,
		InsightsOnly: i.Instance.Spec.InsightsOnly,
		Cluster:      i.ConnectCluster,
		Topic:        "platform.inventory.events",
		TableName:    i.Instance.Status.TableName,
		DB:           i.AppDBParams,
		TasksMax:     i.ConnectorTasksMax,
		BatchSize:    1000,
		MaxAge:       45,
		Template:     i.ConnectorConfig,
	}

	return CreateConnector(i.Client, i.Instance.Status.ConnectorName, i.Instance.Namespace, config, i.Instance, i.Scheme)
}

func CreateConnector(c client.Client, name string, namespace string, config ConnectorConfiguration, owner metav1.Object, ownerScheme *runtime.Scheme) error {
	connector, err := newConnectorResource(name, namespace, config)

	if err != nil {
		return err
	}

	if owner != nil {
		if err := controllerutil.SetControllerReference(owner, connector, ownerScheme); err != nil {
			return err
		}
	}

	return c.Create(context.TODO(), connector)
}

func (i *ReconcileIteration) deleteConnector(connectorName string) error {
	connectorExists, err := i.checkIfConnectorExists(connectorName)
	if err != nil {
		return err
	} else if connectorExists != true {
		return nil
	}

	u := &unstructured.Unstructured{}
	u.SetName(connectorName)
	u.SetNamespace(i.Instance.Namespace)
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "KafkaConnector",
		Version: "kafka.strimzi.io/v1alpha1",
	})
	err = i.Client.Delete(context.Background(), u)
	return err

}

// TODO move to k8s?
func GetConnector(c client.Client, name string, namespace string) (*unstructured.Unstructured, error) {
	connector := &unstructured.Unstructured{}
	connector.SetGroupVersionKind(connectorGVK)

	err := c.Get(context.TODO(), client.ObjectKey{Name: name, Namespace: namespace}, connector)
	return connector, err
}

func GetConnectorsForApp(c client.Client, namespace string, appName string) (*unstructured.UnstructuredList, error) {
	connectors := &unstructured.UnstructuredList{}
	connectors.SetGroupVersionKind(connectorsGVK)

	err := c.List(context.TODO(), connectors, client.InNamespace(namespace), client.MatchingLabels{appNameLabel: appName})
	return connectors, err
}

/*
 * Delete the given connector. This operation is idempotent i.e. it silently ignores if the connector does not exist.
 */
func DeleteConnector(c client.Client, name string, namespace string) error {
	connector := &unstructured.Unstructured{}
	connector.SetName(name)
	connector.SetNamespace(namespace)
	connector.SetGroupVersionKind(connectorGVK)

	if err := c.Delete(context.TODO(), connector); err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}
