package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

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

func (i *ReconcileIteration) newConnectorForCR() (*unstructured.Unstructured, error) {
	m := make(map[string]string)
	m["TableName"] = i.Instance.Status.TableName
	m["Topic"] = "platform.inventory.events"
	m["DBPort"] = i.AppDBParams.Port
	m["DBHostname"] = i.AppDBParams.Host
	m["DBName"] = i.AppDBParams.Name
	m["DBUser"] = i.AppDBParams.User
	m["DBPassword"] = i.AppDBParams.Password
	m["TasksMax"] = strconv.FormatInt(i.ConnectorTasksMax, 10) // TODO: why int64?
	m["BatchSize"] = "3000"                                    // TODO
	m["InsightsOnly"] = strconv.FormatBool(i.Instance.Spec.InsightsOnly)
	tmpl, err := template.New("connectorConfig").Parse(i.ConnectorConfig)
	if err != nil {
		return nil, err
	}

	var connectorConfigBuffer bytes.Buffer
	err = tmpl.Execute(&connectorConfigBuffer, m)
	if err != nil {
		return nil, err
	}
	connectorConfigParsed := connectorConfigBuffer.String()

	var connectorConfigInterface interface{}

	err = json.Unmarshal([]byte(strings.ReplaceAll(connectorConfigParsed, "\n", "")), &connectorConfigInterface)
	if err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{}
	u.Object = map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      i.Instance.Status.ConnectorName,
			"namespace": i.Instance.Namespace,
			"labels": map[string]interface{}{
				"strimzi.io/cluster": i.ConnectCluster,
				"cyndi/appName":      i.Instance.Spec.AppName,
			},
		},
		"spec": map[string]interface{}{
			"tasksMax": i.ConnectorTasksMax,
			"class":    "io.confluent.connect.jdbc.JdbcSinkConnector",
			"config":   connectorConfigInterface,
		},
	}

	u.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "KafkaConnector",
		Version: "kafka.strimzi.io/v1alpha1",
	})
	return u, nil
}

func (i *ReconcileIteration) createConnector() error {
	connector, err := i.newConnectorForCR()
	if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(i.Instance, connector, i.Scheme); err != nil {
		return err
	}

	err = i.Client.Create(context.TODO(), connector)
	return err
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
