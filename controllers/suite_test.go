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
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
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
})

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
