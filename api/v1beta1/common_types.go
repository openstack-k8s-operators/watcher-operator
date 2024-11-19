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

// WatcherTemplate defines a spec based reusable for all the CRDs
type WatcherTemplate struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=osp-secret
	// Secret containing all passwords / keys needed
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={service: WatcherPassword,}
	// PasswordSelectors - Selectors to identify the ServiceUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Required
	// MariaDB instance name
	// Required to use the maridb-operator instance to create the DB and user
	DatabaseInstance string `json:"databaseInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=watcher
	// DatabaseAccount - MariaDBAccount CR name used for watcher DB, defaults to watcher
	DatabaseAccount string `json:"databaseAccount"`
}

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="WatcherPassword"
	// Service - Selector to get the watcher service user password from the Secret
	Service string `json:"service"`
}
