package test

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
)

func UniqueNamespace() (namespace string) {
	namespace = fmt.Sprintf("test-%d", time.Now().UnixNano())
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	err := Client.Create(context.TODO(), ns)
	Expect(err).ToNot(HaveOccurred())

	return
}
