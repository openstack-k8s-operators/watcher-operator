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

// WatcherSpec defines the desired state of Watcher
type WatcherSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	WatcherTemplate `json:",inline"`
}

// WatcherStatus defines the observed state of Watcher
type WatcherStatus struct {
	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ServiceID - The ID of the watcher service registered in keystone
	ServiceID string `json:"serviceID,omitempty"`

	// ObservedGeneration - the most recent generation observed for this
	// service. If the observed generation is less than the spec generation,
	// then the controller has not processed the latest changes injected by
	// the opentack-operator in the top-level CR (e.g. the ContainerImage)
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Watcher is the Schema for the watchers API
type Watcher struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WatcherSpec   `json:"spec,omitempty"`
	Status WatcherStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WatcherList contains a list of Watcher
type WatcherList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Watcher `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Watcher{}, &WatcherList{})
}

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {
}
