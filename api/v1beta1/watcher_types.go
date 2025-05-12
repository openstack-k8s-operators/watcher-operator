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

const (
	// DbSyncHash hash
	DbSyncHash = "dbsync"
)

// WatcherSpec defines the desired state of Watcher
type WatcherSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	WatcherTemplate `json:",inline"`

	WatcherImages `json:",inline"`
}

// WatcherStatus defines the observed state of Watcher
type WatcherStatus struct {
	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ServiceID - The ID of the watcher service registered in keystone
	ServiceID string `json:"serviceID,omitempty"`

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// ObservedGeneration - the most recent generation observed for this
	// service. If the observed generation is less than the spec generation,
	// then the controller has not processed the latest changes injected by
	// the opentack-operator in the top-level CR (e.g. the ContainerImage)
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// APIServiceReadyCount defines the number or replicas ready from watcher-api
	APIServiceReadyCount int32 `json:"apiServiceReadyCount,omitempty"`

	// ApplierServiceReadyCount defines the number or replicas ready from watcher-applier
	ApplierServiceReadyCount int32 `json:"applierServiceReadyCount,omitempty"`

	// DecisionEngineServiceReadyCount defines the number or replicas ready from watcher-decision-engine
	DecisionEngineServiceReadyCount int32 `json:"decisionengineServiceReadyCount,omitempty"`
}

type WatcherDBPurge struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="0 1 * * *"
	// Schedule defines when to run the Watcher DB Purge job in a cron format.
	// By default it runs everyday at 01:00.
	Schedule *string `json:"schedule"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=90
	// +kubebuilder:validation:Minimum=1
	// PurgeAge defines the minimum age of soft-deleted records in days that can be
	// deleted from the shadow tables
	PurgeAge *int `json:"purgeAge"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

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

// RbacConditionsSet - set the conditions for the rbac object
func (instance Watcher) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance Watcher) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance Watcher) RbacResourceName() string {
	return "watcher-" + instance.Name
}

// IsReady returns true if the ReadyCondition is true
func (r *Watcher) IsReady() bool {
	return r.Status.Conditions.IsTrue(condition.ReadyCondition)
}

func init() {
	SchemeBuilder.Register(&Watcher{}, &WatcherList{})
}
