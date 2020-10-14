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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	cyndiv1beta1 "cyndi-operator/api/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"cyndi-operator/test"
	// +kubebuilder:scaffold:imports
)

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
