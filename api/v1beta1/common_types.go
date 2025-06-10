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
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"

	corev1 "k8s.io/api/core/v1"
)

// Container image fall-back defaults
const (
	WatcherAPIContainerImage            = "quay.io/podified-master-centos9/openstack-watcher-api:current-podified"
	WatcherDecisionEngineContainerImage = "quay.io/podified-master-centos9/openstack-watcher-decision-engine:current-podified"
	WatcherApplierContainerImage        = "quay.io/podified-master-centos9/openstack-watcher-applier:current-podified"
)

// WatcherCommon defines a spec based reusable for all the CRDs
type WatcherCommon struct {

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=watcher
	// ServiceUser - optional username used for this service to register in keystone
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={service: WatcherPassword,}
	// PasswordSelectors - Selectors to identify the ServiceUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=memcached
	// MemcachedInstance is the name of the Memcached CR that all watcher service will use.
	MemcachedInstance string `json:"memcachedInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this component. Setting here overrides
	// any global NodeSelector settings within the Watcher CR.
	NodeSelector *map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as a custom config file.
	CustomServiceConfig string `json:"customServiceConfig,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=metric-storage-prometheus-endpoint
	// Secret containing prometheus connection parameters
	PrometheusSecret string `json:"prometheusSecret"`

	// +kubebuilder:validation:Optional
	// TopologyRef to apply the Topology defined by the associated CR referenced
	// by name
	TopologyRef *topologyv1.TopoRef `json:"topologyRef,omitempty"`
}

// WatcherSpecCore defines the fields used from the OpenStackControlPlane CR (does not include images)
type WatcherSpecCore struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	WatcherCommon `json:",inline"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=rabbitmq
	// RabbitMQ instance name
	// Needed to request a transportURL that is created and used in Watcher
	RabbitMqClusterName *string `json:"rabbitMqClusterName"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=osp-secret
	// Secret containing all passwords / keys needed
	Secret string `json:"secret"`

	// +kubebuilder:validation:Required
	// MariaDB instance name
	// Required to use the mariadb-operator instance to create the DB and user
	DatabaseInstance *string `json:"databaseInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=watcher
	// DatabaseAccount - MariaDBAccount CR name used for watcher DB, defaults to watcher
	DatabaseAccount string `json:"databaseAccount"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default={replicas:1}
	// APIServiceTemplate - define the watcher-api service
	APIServiceTemplate WatcherAPITemplate `json:"apiServiceTemplate"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default={replicas:1}
	// WatcherApplierTemplate - define the watcher-applier service
	ApplierServiceTemplate WatcherApplierTemplate `json:"applierServiceTemplate"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default={replicas:1}
	// DecisionEngineServiceTemplate - define the watcher-decision-engine service
	DecisionEngineServiceTemplate WatcherDecisionEngineTemplate `json:"decisionengineServiceTemplate"`

	// +kubebuilder:validation:Optional
	// DBPurge defines the parameters for the Watcher DB purging cron job
	DBPurge WatcherDBPurge `json:"dbPurge"`
}

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="WatcherPassword"
	// Service - Selector to get the watcher service user password from the Secret
	Service string `json:"service"`
}

// WatcherSubCrsCommon
type WatcherSubCrsCommon struct {
	// +kubebuilder:validation:Optional
	// The service specific Container Image URL (will be set to environmental default if empty)
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Required
	// ServiceAccount - service account name used internally to provide
	// Watcher services the default SA name
	ServiceAccount string `json:"serviceAccount"`
}

// WatcherSubCrsTemplate define de common part of the input parameters specified by the user to
// create a 2nd CR via higher level CRDs.
type WatcherSubCrsTemplate struct {
	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this component. Setting here overrides
	// any global NodeSelector settings within the Watcher CR.
	NodeSelector *map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as a custom config file.
	CustomServiceConfig string `json:"customServiceConfig,omitempty"`

	// +kubebuilder:validation:Optional
	// TopologyRef to apply the Topology defined by the associated CR referenced
	// by name
	TopologyRef *topologyv1.TopoRef `json:"topologyRef,omitempty"`
}

type WatcherImages struct {
	// +kubebuilder:validation:Required
	// APIContainerImageURL
	APIContainerImageURL string `json:"apiContainerImageURL"`

	// +kubebuilder:validation:Required
	// DecisionEngineContainerImageURL
	DecisionEngineContainerImageURL string `json:"decisionengineContainerImageURL"`

	// +kubebuilder:validation:Required
	// ApplierContainerImageURL
	ApplierContainerImageURL string `json:"applierContainerImageURL"`
}

func (r *WatcherImages) Default(defaults WatcherDefaults) {
	if r.APIContainerImageURL == "" {
		r.APIContainerImageURL = defaults.APIContainerImageURL
	}
	if r.DecisionEngineContainerImageURL == "" {
		r.DecisionEngineContainerImageURL = defaults.DecisionEngineContainerImageURL
	}
	if r.ApplierContainerImageURL == "" {
		r.ApplierContainerImageURL = defaults.ApplierContainerImageURL
	}
}

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)

func SetupDefaults() {
	// Acquire environmental defaults and initialize Watcher defaults with them
	watcherDefaults := WatcherDefaults{
		APIContainerImageURL:            util.GetEnvVar("RELATED_IMAGE_WATCHER_API_IMAGE_URL_DEFAULT", WatcherAPIContainerImage),
		ApplierContainerImageURL:        util.GetEnvVar("RELATED_IMAGE_WATCHER_APPLIER_IMAGE_URL_DEFAULT", WatcherApplierContainerImage),
		DecisionEngineContainerImageURL: util.GetEnvVar("RELATED_IMAGE_WATCHER_DECISION_ENGINE_IMAGE_URL_DEFAULT", WatcherDecisionEngineContainerImage),
	}
	SetupWatcherDefaults(watcherDefaults)
}
