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
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// WatcherDefaults -
type WatcherDefaults struct {
	APIContainerImageURL            string
	DecisionEngineContainerImageURL string
	ApplierContainerImageURL        string
}

var watcherDefaults WatcherDefaults

// log is for logging in this package.
var watcherlog = logf.Log.WithName("watcher-resource")

func SetupWatcherDefaults(defaults WatcherDefaults) {
	watcherDefaults = defaults
	watcherlog.Info("Watcher defaults initialized", "defaults", defaults)
}

//+kubebuilder:webhook:path=/mutate-watcher-openstack-org-v1beta1-watcher,mutating=true,failurePolicy=fail,sideEffects=None,groups=watcher.openstack.org,resources=watchers,verbs=create;update,versions=v1beta1,name=mwatcher.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Watcher{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Watcher) Default() {
	watcherlog.Info("default", "name", r.Name)

	r.Spec.Default()
}

// Default - set defaults for this WatcherCore spec.
func (spec *WatcherSpec) Default() {
	spec.WatcherImages.Default(watcherDefaults)
}

//+kubebuilder:webhook:path=/validate-watcher-openstack-org-v1beta1-watcher,mutating=false,failurePolicy=fail,sideEffects=None,groups=watcher.openstack.org,resources=watchers,verbs=create;update,versions=v1beta1,name=vwatcher.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Watcher{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Watcher) ValidateCreate() (admission.Warnings, error) {
	watcherlog.Info("validate create", "name", r.Name)

	var allErrs field.ErrorList
	basePath := field.NewPath("spec")

	if *r.Spec.DatabaseInstance == "" || r.Spec.DatabaseInstance == nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("databaseInstance"), "", "databaseInstance field should not be empty"),
		)
	}

	if *r.Spec.RabbitMqClusterName == "" || r.Spec.RabbitMqClusterName == nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("rabbitMqClusterName"), "", "rabbitMqClusterName field should not be empty"),
		)
	}

	allErrs = append(allErrs, r.Spec.ValidateWatcherTopology(basePath, r.Namespace)...)

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "watcher.openstack.org", Kind: "Watcher"},
			r.Name, allErrs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Watcher) ValidateUpdate(runtime.Object) (admission.Warnings, error) {
	watcherlog.Info("validate update", "name", r.Name)

	var allErrs field.ErrorList
	basePath := field.NewPath("spec")

	if *r.Spec.DatabaseInstance == "" || r.Spec.DatabaseInstance == nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("databaseInstance"), "", "databaseInstance field should not be empty"),
		)
	}

	if *r.Spec.RabbitMqClusterName == "" || r.Spec.RabbitMqClusterName == nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("rabbitMqClusterName"), "", "rabbitMqClusterName field should not be empty"),
		)
	}

	allErrs = append(allErrs, r.Spec.ValidateWatcherTopology(basePath, r.Namespace)...)

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "watcher.openstack.org", Kind: "Watcher"},
			r.Name, allErrs)
	}

	return nil, nil

}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Watcher) ValidateDelete() (admission.Warnings, error) {
	watcherlog.Info("validate delete", "name", r.Name)

	return nil, nil
}

// ValidateWatcherTopology - Returns an ErrorList if the Topology is referenced
// on a different namespace
func (spec *WatcherSpec) ValidateWatcherTopology(basePath *field.Path, namespace string) field.ErrorList {
	watcherlog.Info("validate topology")
	var allErrs field.ErrorList

	// When a TopologyRef CR is referenced, fail if a different Namespace is
	// referenced because is not supported
	allErrs = append(allErrs, topologyv1.ValidateTopologyRef(
		spec.TopologyRef, *basePath.Child("topologyRef"), namespace)...)

	// When a TopologyRef CR is referenced with an override to any of the SubCRs, fail
	// if a different Namespace is referenced because not supported
	apiPath := basePath.Child("apiServiceTemplate")
	allErrs = append(allErrs,
		spec.APIServiceTemplate.ValidateTopology(apiPath, namespace)...)

	decisionEnginePath := basePath.Child("decisionengineServiceTemplate")
	allErrs = append(allErrs,
		spec.DecisionEngineServiceTemplate.ValidateTopology(decisionEnginePath, namespace)...)

	applierPath := basePath.Child("applierServiceTemplate")
	allErrs = append(allErrs,
		spec.ApplierServiceTemplate.ValidateTopology(applierPath, namespace)...)

	return allErrs
}
