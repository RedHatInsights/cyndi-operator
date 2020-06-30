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
    PipelineVersion int `json:"pipelineVersion"`
    PipelineName string `json:"pipelineName"`
    AppDBHostname string `json:"appDBHostname"`
    AppDBPort int32 `json:"appDBPort"`
    AppDBName string `json:"appDBName"`
    AppDBPassword string `json:"appDBPassword"`
    InventoryDBHostname string `json:"inventoryDBHostname"`
    InventoryDBPort int32 `json:"inventoryDBPort"`
    InventoryDBName string `json:"inventoryDBName"`
    InventoryDBPassword string `json:"inventoryDBPassword"`
    KafkaConnectHostname string `json:"kafkaConnectHostname"`
    KafkaConnectPort int32 `json:"kafkaConnectPort"`
    InsightsOnly bool `json:"insightsOnly"`
}

// CyndiPipelineStatus defines the observed state of CyndiPipeline
type CyndiPipelineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CyndiPipeline is the Schema for the cyndipipelines API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=cyndipipelines,scope=Namespaced
type CyndiPipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CyndiPipelineSpec   `json:"spec,omitempty"`
	Status CyndiPipelineStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CyndiPipelineList contains a list of CyndiPipeline
type CyndiPipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CyndiPipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CyndiPipeline{}, &CyndiPipelineList{})
}
