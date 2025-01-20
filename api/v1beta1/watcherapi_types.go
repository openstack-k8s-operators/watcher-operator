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

// WatcherAPISpec defines the desired state of WatcherAPI
type WatcherAPISpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	WatcherCommon `json:",inline"`

	// +kubebuilder:validation:Required
	// Secret containing all passwords / keys needed
	Secret string `json:"secret"`

	WatcherSubCrsCommon `json:",inline"`

	// +kubebuilder:validation:Optional
	// ExternalEndpoints, expose a VIP via MetalLB on the pre-created address pool
	ExternalEndpoints []MetalLBConfig `json:"externalEndpoints,omitempty"`
}

// WatcherAPIStatus defines the observed state of WatcherAPI
type WatcherAPIStatus struct {
	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ObservedGeneration - the most recent generation observed for this
	// service. If the observed generation is less than the spec generation,
	// then the controller has not processed the latest changes injected by
	// the openstack-operator in the top-level CR (e.g. the ContainerImage)
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReadyCount of watcher API instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`
}

// WatcherAPITemplate defines the input parameters specified by the user to
// create a WatcherAPI via higher level CRDs.
type WatcherAPITemplate struct {
	WatcherSubCrsTemplate `json:",inline"`

	// +kubebuilder:validation:Optional
	// ExternalEndpoints, expose a VIP via MetalLB on the pre-created address pool
	ExternalEndpoints []MetalLBConfig `json:"externalEndpoints,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// WatcherAPI is the Schema for the watcherapis API
type WatcherAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WatcherAPISpec   `json:"spec,omitempty"`
	Status WatcherAPIStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WatcherAPIList contains a list of WatcherAPI
type WatcherAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WatcherAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WatcherAPI{}, &WatcherAPIList{})
}
