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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CyndiPipelineSpec defines the desired state of CyndiPipeline
type CyndiPipelineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	AppName      string `json:"appName"`
	InsightsOnly bool   `json:"insightsOnly"`
}

// CyndiPipelineStatus defines the observed state of CyndiPipeline
type CyndiPipelineStatus struct {
	ValidationFailedCount  int64  `json:"validationFailedCount"`
	ConnectorName          string `json:"cyndiPipelineName"`
	TableName              string `json:"tableName"`
	PipelineVersion        string `json:"pipelineVersion"`
	CyndiConfigVersion     string `json:"cyndiConfigVersion"`
	ValidationDelaySeconds int64  `json:"validationDelaySeconds"`
	InitialSyncInProgress  bool   `json:"initialSyncInProgress"`

	// Name of the database table that is currently backing the "inventory.hosts" view
	// May differ from TableName e.g. during a refresh
	ActiveTableName string `json:"activeTableName"`

	Conditions []metav1.Condition `json:"conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// CyndiPipeline is the Schema for the cyndipipelines API
type CyndiPipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CyndiPipelineSpec   `json:"spec,omitempty"`
	Status CyndiPipelineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CyndiPipelineList contains a list of CyndiPipeline
type CyndiPipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CyndiPipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CyndiPipeline{}, &CyndiPipelineList{})
}
