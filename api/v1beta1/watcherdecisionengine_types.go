/*
Copyright 2024.

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
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WatcherDecisionEngineSpec defines the desired state of WatcherDecisionEngine
type WatcherDecisionEngineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	WatcherCommon `json:",inline"`

	// +kubebuilder:validation:Required
	// Secret containing all passwords / keys needed
	Secret string `json:"secret"`

	WatcherSubCrsCommon `json:",inline"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=1
	// +kubebuilder:validation:Minimum=0
	// Replicas of Watcher service to run
	Replicas *int32 `json:"replicas"`
}

// WatcherDecisionEngineStatus defines the observed state of WatcherDecisionEngine
type WatcherDecisionEngineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ObservedGeneration - the most recent generation observed for this
	// service. If the observed generation is less than the spec generation,
	// then the controller has not processed the latest changes injected by
	// the openstack-operator in the top-level CR (e.g. the ContainerImage)
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// WatcherDecisionEngine is the Schema for the watcherdecisionengines API
type WatcherDecisionEngine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WatcherDecisionEngineSpec   `json:"spec,omitempty"`
	Status WatcherDecisionEngineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WatcherDecisionEngineList contains a list of WatcherDecisionEngine
type WatcherDecisionEngineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WatcherDecisionEngine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WatcherDecisionEngine{}, &WatcherDecisionEngineList{})
}
