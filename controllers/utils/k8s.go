package utils

/*

Utility functions for talking to the k8s API.

*/

import (
	"context"

	cyndiv1beta1 "cyndi-operator/api/v1beta1"

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

func FetchCyndiPipeline(c client.Client, namespacedName types.NamespacedName) (*cyndiv1beta1.CyndiPipeline, error) {
	instance := &cyndiv1beta1.CyndiPipeline{}
	err := c.Get(context.TODO(), namespacedName, instance)
	return instance, err
}
