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
	PipelineVersion     int64  `json:"pipelineVersion"`
	AppName             string `json:"appName"`
	AppDBHostname       string `json:"appDBHostname"`
	AppDBPort           int64  `json:"appDBPort"`
	AppDBName           string `json:"appDBName"`
	AppDBUser           string `json:"appDBUser"`
	AppDBPassword       string `json:"appDBPassword"`
	AppDBSSLMode        string `json:"appDBSSLMode"`
	KafkaConnectCluster string `json:"kafkaConnectCluster"`
	InsightsOnly        bool   `json:"insightsOnly"`
	TasksMax            int64  `json:"tasksMax"`
}

// CyndiPipelineStatus defines the observed state of CyndiPipeline
type CyndiPipelineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	SyndicatedDataIsValid   bool   `json:"syndicatedDataIsValid"`
	ValidationFailedCount   int    `json:"validationFailedCount"`
	ConnectorName           string `json:"cyndiPipelineName"`
	TableName               string `json:"tableName"`
	PipelineVersion         string `json:"pipelineVersion"`
	CyndiConfigVersion      string `json:"cyndiConfigVersion"`
	ValidationDelaySeconds  int    `json:"validationDelaySeconds"`
	PreviousPipelineVersion string `json:"previousPipelineVersion"`
	InitialSyncInProgress   bool   `json:"initialSyncInProgress"`
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
