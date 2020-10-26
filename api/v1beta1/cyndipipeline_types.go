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

// CyndiPipelineSpec defines the desired state of CyndiPipeline
type CyndiPipelineSpec struct {

	// +kubebuilder:validation:MinLength:=1
	// +kubebuilder:validation:MaxLength:=64
	// +kubebuilder:validation:Required
	AppName string `json:"appName"`

	// +optional
	InsightsOnly bool `json:"insightsOnly"`
}

// CyndiPipelineStatus defines the observed state of CyndiPipeline
type CyndiPipelineStatus struct {
	ValidationFailedCount int64  `json:"validationFailedCount"`
	ConnectorName         string `json:"cyndiPipelineName"`
	TableName             string `json:"tableName"`
	PipelineVersion       string `json:"pipelineVersion"`
	CyndiConfigVersion    string `json:"cyndiConfigVersion"`
	InitialSyncInProgress bool   `json:"initialSyncInProgress"`

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
