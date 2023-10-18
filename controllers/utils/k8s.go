package utils

/*

Utility functions for talking to the k8s API.

*/

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"

	cyndi "github.com/RedHatInsights/cyndi-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FetchSecret(c client.Client, namespace string, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := c.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: name}, secret)
	return secret, err
}

func FetchConfigMap(c client.Client, namespace string, name string) (*corev1.ConfigMap, error) {
	config := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), client.ObjectKey{Name: name, Namespace: namespace}, config)
	return config, err
}

func FetchCyndiPipeline(c client.Client, namespacedName types.NamespacedName) (*cyndi.CyndiPipeline, error) {
	instance := &cyndi.CyndiPipeline{}
	err := c.Get(context.TODO(), namespacedName, instance)
	return instance, err
}

func FetchCyndiPipelines(c client.Client, namespace string) (*cyndi.CyndiPipelineList, error) {
	list := &cyndi.CyndiPipelineList{}
	err := c.List(context.TODO(), list)
	return list, err
}

func ConfigMapHash(cm map[string]string, ignoredKeys ...string) string {
	if len(cm) == 0 {
		return "-1"
	}

	values := Omit(cm, ignoredKeys...)

	json, err := json.Marshal(values)

	if err != nil {
		return "-2"
	}

	algorithm := fnv.New32a()
	algorithm.Write(json)
	return fmt.Sprint(algorithm.Sum32())
}

func SpecHash(spec interface{}) (string, error) {
	jsonVal, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}

	algorithm := fnv.New32a()
	_, err = algorithm.Write(jsonVal)
	return fmt.Sprint(algorithm.Sum32()), err
}
