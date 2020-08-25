package controllers

import (
	"bytes"
	"context"
	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"encoding/json"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"
	"strings"
	"text/template"
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

func newConnectorForCR(instance *cyndiv1beta1.CyndiPipeline, connectorConfig string) (*unstructured.Unstructured, error) {
	m := make(map[string]string)
	m["TableName"] = instance.Status.TableName
	m["Topic"] = "platform.inventory.events"
	m["DBPort"] = strconv.FormatInt(instance.Spec.AppDBPort, 10)
	m["DBHostname"] = instance.Spec.AppDBHostname
	m["DBName"] = instance.Spec.AppDBName
	m["DBUser"] = instance.Spec.AppDBUser
	m["DBPassword"] = instance.Spec.AppDBPassword
	m["BatchSize"] = "3000"
	m["InsightsOnly"] = strconv.FormatBool(instance.Spec.InsightsOnly)
	tmpl, err := template.New("connectorConfig").Parse(connectorConfig)
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
			"name":      instance.Status.ConnectorName,
			"namespace": instance.Namespace,
			"labels": map[string]interface{}{
				"strimzi.io/cluster": instance.Spec.KafkaConnectCluster,
			},
		},
		"spec": map[string]interface{}{
			"tasksMax": instance.Spec.TasksMax,
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
	connector, err := newConnectorForCR(i.Instance, i.ConnectorConfig)
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
