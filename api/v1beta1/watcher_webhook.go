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

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	common_webhook "github.com/openstack-k8s-operators/lib-common/modules/common/webhook"
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

// SetupWatcherDefaults initializes the Watcher defaults for the webhook
func SetupWatcherDefaults(defaults WatcherDefaults) {
	watcherDefaults = defaults
	watcherlog.Info("Watcher defaults initialized", "defaults", defaults)
}

var _ webhook.Defaulter = &Watcher{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Watcher) Default() {
	watcherlog.Info("default", "name", r.Name)

	r.Spec.Default()
}

// Default - set defaults for this WatcherCore spec.
func (spec *WatcherSpec) Default() {
	spec.WatcherSpecCore.Default()
	spec.WatcherImages.Default(watcherDefaults)
}

// Default - set defaults for this WatcherSpecCore spec.
func (spec *WatcherSpecCore) Default() {
	// Apply kubebuilder default for RabbitMqClusterName if not set
	if spec.RabbitMqClusterName == nil {
		spec.RabbitMqClusterName = ptr.To("rabbitmq")
	}

	// Default MessagingBus.Cluster from RabbitMqClusterName if not already set
	if spec.MessagingBus.Cluster == "" {
		spec.MessagingBus.Cluster = *spec.RabbitMqClusterName
	}

	// Default NotificationsBus if NotificationsBusInstance is specified
	if spec.NotificationsBusInstance != nil && *spec.NotificationsBusInstance != "" {
		if spec.NotificationsBus == nil {
			// Initialize empty NotificationsBus - credentials will be created dynamically
			// to ensure separation from MessagingBus (RPC and notifications should never share credentials)
			spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{}
		}
		// Default cluster name if not already set
		if spec.NotificationsBus.Cluster == "" {
			spec.NotificationsBus.Cluster = *spec.NotificationsBusInstance
		}
	}
}

// getDeprecatedFields returns the centralized list of deprecated fields for WatcherSpecCore
func (spec *WatcherSpecCore) getDeprecatedFields(old *WatcherSpecCore) []common_webhook.DeprecatedFieldUpdate {
	// Get new field value (handle nil NotificationsBus)
	var newNotifBusCluster *string
	if spec.NotificationsBus != nil {
		newNotifBusCluster = &spec.NotificationsBus.Cluster
	}

	deprecatedFields := []common_webhook.DeprecatedFieldUpdate{
		{
			DeprecatedFieldName: "rabbitMqClusterName",
			NewFieldPath:        []string{"messagingBus", "cluster"},
			NewDeprecatedValue:  spec.RabbitMqClusterName,
			NewValue:            &spec.MessagingBus.Cluster,
		},
		{
			DeprecatedFieldName: "notificationsBusInstance",
			NewFieldPath:        []string{"notificationsBus", "cluster"},
			NewDeprecatedValue:  spec.NotificationsBusInstance,
			NewValue:            newNotifBusCluster,
		},
	}

	// If old spec is provided (UPDATE operation), add old values
	if old != nil {
		deprecatedFields[0].OldDeprecatedValue = old.RabbitMqClusterName
		deprecatedFields[1].OldDeprecatedValue = old.NotificationsBusInstance
	}

	return deprecatedFields
}

var _ webhook.Validator = &Watcher{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Watcher) ValidateCreate() (admission.Warnings, error) {
	watcherlog.Info("validate create", "name", r.Name)

	var allErrs field.ErrorList
	var allWarns admission.Warnings

	basePath := field.NewPath("spec")

	if warns, errs := r.Spec.ValidateCreate(basePath, r.Namespace); errs != nil {
		allErrs = append(allErrs, errs...)
		allWarns = append(allWarns, warns...)
	}

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "watcher.openstack.org", Kind: "Watcher"},
			r.Name, allErrs)
	}

	return allWarns, nil
}

// ValidateCreate validates the WatcherSpec during the webhook invocation.
func (spec *WatcherSpec) ValidateCreate(basePath *field.Path, namespace string) ([]string, field.ErrorList) {
	return spec.WatcherSpecCore.ValidateCreate(basePath, namespace)
}

// validateDeprecatedFieldsCreate validates deprecated fields during CREATE operations
func (spec *WatcherSpecCore) validateDeprecatedFieldsCreate(basePath *field.Path) ([]string, field.ErrorList) {
	// Get deprecated fields list (without old values for CREATE)
	deprecatedFieldsUpdate := spec.getDeprecatedFields(nil)

	// Convert to DeprecatedField list for CREATE validation
	deprecatedFields := make([]common_webhook.DeprecatedField, len(deprecatedFieldsUpdate))
	for i, df := range deprecatedFieldsUpdate {
		deprecatedFields[i] = common_webhook.DeprecatedField{
			DeprecatedFieldName: df.DeprecatedFieldName,
			NewFieldPath:        df.NewFieldPath,
			DeprecatedValue:     df.NewDeprecatedValue,
			NewValue:            df.NewValue,
		}
	}

	return common_webhook.ValidateDeprecatedFieldsCreate(deprecatedFields, basePath), nil
}

// validateDeprecatedFieldsUpdate validates deprecated fields during UPDATE operations
func (spec *WatcherSpecCore) validateDeprecatedFieldsUpdate(old WatcherSpecCore, basePath *field.Path) ([]string, field.ErrorList) {
	// Get deprecated fields list with old values
	deprecatedFields := spec.getDeprecatedFields(&old)
	return common_webhook.ValidateDeprecatedFieldsUpdate(deprecatedFields, basePath)
}

// ValidateCreate validates the WatcherSpecCore during the webhook invocation. It is
// expected to be called by the validation webhook in the higher level meta
// operator
func (spec *WatcherSpecCore) ValidateCreate(basePath *field.Path, namespace string) ([]string, field.ErrorList) {
	var allErrs field.ErrorList
	var allWarns admission.Warnings

	// Validate deprecated fields using shared helper
	warns, errs := spec.validateDeprecatedFieldsCreate(basePath)
	allWarns = append(allWarns, warns...)
	allErrs = append(allErrs, errs...)

	if spec.DatabaseInstance == nil || *spec.DatabaseInstance == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("databaseInstance"), "", "databaseInstance field should not be empty"),
		)
	}

	// Validate messagingBus.cluster instead of deprecated rabbitMqClusterName
	if spec.MessagingBus.Cluster == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("messagingBus").Child("cluster"), "", "messagingBus.cluster field should not be empty"),
		)
	}

	allErrs = append(allErrs, spec.ValidateWatcherTopology(basePath, namespace)...)

	return allWarns, allErrs
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Watcher) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {

	watcherlog.Info("validate update", "name", r.Name)
	oldWatcher, ok := old.(*Watcher)
	if !ok || oldWatcher == nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("unable to convert existing object"))
	}

	var allErrs field.ErrorList
	var allWarns admission.Warnings

	basePath := field.NewPath("spec")

	if warns, errs := r.Spec.ValidateUpdate(oldWatcher.Spec, basePath, r.Namespace); errs != nil {
		allErrs = append(allErrs, errs...)
		allWarns = append(allWarns, warns...)
	}

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "watcher.openstack.org", Kind: "Watcher"},
			r.Name, allErrs)
	}

	return allWarns, nil

}

// ValidateUpdate validates the WatcherSpec during the webhook update invocation.
func (spec *WatcherSpec) ValidateUpdate(old WatcherSpec, basePath *field.Path, namespace string) ([]string, field.ErrorList) {
	return spec.WatcherSpecCore.ValidateUpdate(old.WatcherSpecCore, basePath, namespace)
}

// ValidateUpdate validates the WatcherSpecCore during the webhook invocation. It is
// expected to be called by the validation webhook in the higher level meta
// operator
func (spec *WatcherSpecCore) ValidateUpdate(old WatcherSpecCore, basePath *field.Path, namespace string) ([]string, field.ErrorList) {
	var allErrs field.ErrorList
	var allWarns admission.Warnings

	if spec.DatabaseInstance == nil || *spec.DatabaseInstance == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("databaseInstance"), "", "databaseInstance field should not be empty"),
		)
	}

	// Validate messagingBus.cluster instead of deprecated rabbitMqClusterName
	if spec.MessagingBus.Cluster == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				basePath.Child("messagingBus").Child("cluster"), "", "messagingBus.cluster field should not be empty"),
		)
	}

	// Validate deprecated fields using shared helper
	warns, errs := spec.validateDeprecatedFieldsUpdate(old, basePath)
	allWarns = append(allWarns, warns...)
	allErrs = append(allErrs, errs...)

	allErrs = append(allErrs, spec.ValidateWatcherTopology(basePath, namespace)...)

	return allWarns, allErrs
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
