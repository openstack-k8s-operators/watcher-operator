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
	common_webhook "github.com/openstack-k8s-operators/lib-common/modules/common/webhook"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var watcherapilog = logf.Log.WithName("watcherapi-resource")

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *WatcherAPI) Default() {
	watcherapilog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *WatcherAPI) ValidateCreate() (admission.Warnings, error) {
	watcherapilog.Info("validate create", "name", r.Name)

	return nil, r.validateOverrides()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *WatcherAPI) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	watcherapilog.Info("validate update", "name", r.Name)

	// Only validate when the override changed, so a CR carrying a legacy
	// value can still be updated (finalizers, status) and deleted.
	if o, ok := old.(*WatcherAPI); ok && equality.Semantic.DeepEqual(o.Spec.Override, r.Spec.Override) {
		return nil, nil
	}
	return nil, r.validateOverrides()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *WatcherAPI) ValidateDelete() (admission.Warnings, error) {
	watcherapilog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

func (r *WatcherAPI) validateOverrides() error {
	errs := r.Spec.Override.validate(field.NewPath("spec"))
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(schema.GroupKind{Group: "watcher.openstack.org", Kind: "WatcherAPI"}, r.Name, errs)
}

// ValidateOverrides rejects service overrides (public or internal) with a port
// name other than the bare endpoint type, or an invalid endpointURL. See
// webhook.ValidatePortNameOverrides.
func (r *WatcherAPITemplate) ValidateOverrides(basePath *field.Path) field.ErrorList {
	return r.Override.validate(basePath)
}

func (o APIOverrideSpec) validate(basePath *field.Path) field.ErrorList {
	return common_webhook.ValidatePortNameOverrides(
		basePath.Child("override").Child("service"),
		o.Service,
		common_webhook.DefaultServicePortNames(),
	)
}

// ValidateTopology validates the referenced TopoRef.Namespace.
func (r *WatcherAPITemplate) ValidateTopology(
	basePath *field.Path,
	namespace string,
) field.ErrorList {
	return topologyv1.ValidateTopologyRef(
		r.TopologyRef,
		*basePath.Child("topologyRef"),
		namespace,
	)
}
