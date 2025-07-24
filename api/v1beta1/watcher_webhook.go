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
	"fmt"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
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

// Default - set defaults for this WatcherSpecCore spec.
func (spec *WatcherSpecCore) Default() {
	// no validations . Placeholder for defaulting webhook integrated in the OpenStackControlPlane
}

//+kubebuilder:webhook:path=/validate-watcher-openstack-org-v1beta1-watcher,mutating=false,failurePolicy=fail,sideEffects=None,groups=watcher.openstack.org,resources=watchers,verbs=create;update,versions=v1beta1,name=vwatcher.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Watcher{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Watcher) ValidateCreate() (admission.Warnings, error) {
	watcherlog.Info("validate create", "name", r.Name)

	allErrs := r.Spec.ValidateCreate(field.NewPath("spec"), r.Namespace)

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "watcher.openstack.org", Kind: "Watcher"},
			r.Name, allErrs)
	}

	return nil, nil
}

// ValidateCreate validates the WatcherSpec during the webhook invocation.
func (r *WatcherSpec) ValidateCreate(basePath *field.Path, namespace string) field.ErrorList {
	return r.WatcherSpecCore.ValidateCreate(basePath, namespace)
}

// ValidateCreate validates the WatcherSpecCore during the webhook invocation. It is
// expected to be called by the validation webhook in the higher level meta
// operator
func (r *WatcherSpecCore) ValidateCreate(basePath *field.Path, namespace string) field.ErrorList {
	var allErrs field.ErrorList

	if *r.DatabaseInstance == "" || r.DatabaseInstance == nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("databaseInstance"), "", "databaseInstance field should not be empty"),
		)
	}

	if *r.RabbitMqClusterName == "" || r.RabbitMqClusterName == nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("rabbitMqClusterName"), "", "rabbitMqClusterName field should not be empty"),
		)
	}

	allErrs = append(allErrs, r.ValidateWatcherTopology(basePath, namespace)...)

	return allErrs
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Watcher) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {

	watcherlog.Info("validate update", "name", r.Name)
	oldWatcher, ok := old.(*Watcher)
	if !ok || oldWatcher == nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("unable to convert existing object"))
	}

	allErrs := r.Spec.ValidateUpdate(oldWatcher.Spec, field.NewPath("spec"), r.Namespace)

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "watcher.openstack.org", Kind: "Watcher"},
			r.Name, allErrs)
	}

	return nil, nil

}

// ValidateCreate validates the WatcherSpec during the webhook invocation.
func (r *WatcherSpec) ValidateUpdate(old WatcherSpec, basePath *field.Path, namespace string) field.ErrorList {
	return r.WatcherSpecCore.ValidateUpdate(old.WatcherSpecCore, basePath, namespace)
}

// ValidateUpdate validates the WatcherSpecCore during the webhook invocation. It is
// expected to be called by the validation webhook in the higher level meta
// operator
func (r *WatcherSpecCore) ValidateUpdate(old WatcherSpecCore, basePath *field.Path, namespace string) field.ErrorList {
	var allErrs field.ErrorList

	if *r.DatabaseInstance == "" || r.DatabaseInstance == nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("databaseInstance"), "", "databaseInstance field should not be empty"),
		)
	}

	if *r.RabbitMqClusterName == "" || r.RabbitMqClusterName == nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("rabbitMqClusterName"), "", "rabbitMqClusterName field should not be empty"),
		)
	}

	allErrs = append(allErrs, r.ValidateWatcherTopology(basePath, namespace)...)

	return allErrs
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Watcher) ValidateDelete() (admission.Warnings, error) {
	watcherlog.Info("validate delete", "name", r.Name)

	return nil, nil
}

// ValidateWatcherTopology - Returns an ErrorList if the Topology is referenced
// on a different namespace
func (spec *WatcherSpecCore) ValidateWatcherTopology(basePath *field.Path, namespace string) field.ErrorList {
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

// SetDefaultRouteAnnotations sets HAProxy timeout values for Watcher API routes
// This function is called by the OpenStackControlPlane webhook to set the default HAProxy timeout
// for the Watcher API routes.
func (spec *WatcherSpecCore) SetDefaultRouteAnnotations(annotations map[string]string) {
	const haProxyAnno = "haproxy.router.openshift.io/timeout"
	// Use a custom annotation to flag when the operator has set the default HAProxy timeout
	// With the annotation func determines when to overwrite existing HAProxy timeout with the APITimeout
	const watcherAnno = "api.watcher.openstack.org/timeout"
	valWatcherAPI, okWatcherAPI := annotations[watcherAnno]
	valHAProxy, okHAProxy := annotations[haProxyAnno]

	// Human operator set the HAProxy timeout manually
	if !okWatcherAPI && okHAProxy {
		return
	}
	// Human operator modified the HAProxy timeout manually without removing the Watcher flag
	if okWatcherAPI && okHAProxy && valWatcherAPI != valHAProxy {
		delete(annotations, watcherAnno)
		return
	}

	// The Default webhook is called before applying kubebuilder defaults so we need to manage
	// the defaulting of the APITimeout field.
	if spec.APITimeout == nil || *spec.APITimeout == 0 {
		spec.APITimeout = ptr.To(int(APITimeoutDefault))
	}
	timeout := fmt.Sprintf("%ds", *spec.APITimeout)
	annotations[watcherAnno] = timeout
	annotations[haProxyAnno] = timeout
}
