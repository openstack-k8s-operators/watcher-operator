/*
Copyright 2025.

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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
)

// nolint:unused
// log is for logging in this package.
var watcherapplierlog = logf.Log.WithName("watcherapplier-resource")

// SetupWatcherApplierWebhookWithManager registers the webhook for WatcherApplier in the manager.
func SetupWatcherApplierWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&watcherv1beta1.WatcherApplier{}).
		WithValidator(&WatcherApplierCustomValidator{}).
		WithDefaulter(&WatcherApplierCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-watcher-openstack-org-v1beta1-watcherapplier,mutating=true,failurePolicy=fail,sideEffects=None,groups=watcher.openstack.org,resources=watcherappliers,verbs=create;update,versions=v1beta1,name=mwatcherapplier-v1beta1.kb.io,admissionReviewVersions=v1

// WatcherApplierCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind WatcherApplier when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type WatcherApplierCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &WatcherApplierCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind WatcherApplier.
func (d *WatcherApplierCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	watcherapplier, ok := obj.(*watcherv1beta1.WatcherApplier)

	if !ok {
		return fmt.Errorf("%w: expected WatcherApplier but got %T", ErrInvalidWatcherApplierType, obj)
	}
	watcherapplierlog.Info("Defaulting for WatcherApplier", "name", watcherapplier.GetName())

	watcherapplier.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-watcher-openstack-org-v1beta1-watcherapplier,mutating=false,failurePolicy=fail,sideEffects=None,groups=watcher.openstack.org,resources=watcherappliers,verbs=create;update,versions=v1beta1,name=vwatcherapplier-v1beta1.kb.io,admissionReviewVersions=v1

// WatcherApplierCustomValidator struct is responsible for validating the WatcherApplier resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WatcherApplierCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &WatcherApplierCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type WatcherApplier.
func (v *WatcherApplierCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	watcherapplier, ok := obj.(*watcherv1beta1.WatcherApplier)
	if !ok {
		return nil, fmt.Errorf("%w: expected WatcherApplier but got %T", ErrInvalidWatcherApplierType, obj)
	}
	watcherapplierlog.Info("Validation for WatcherApplier upon creation", "name", watcherapplier.GetName())

	return watcherapplier.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type WatcherApplier.
func (v *WatcherApplierCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	watcherapplier, ok := newObj.(*watcherv1beta1.WatcherApplier)
	if !ok {
		return nil, fmt.Errorf("%w: expected WatcherApplier for newObj but got %T", ErrInvalidWatcherApplierType, newObj)
	}
	watcherapplierlog.Info("Validation for WatcherApplier upon update", "name", watcherapplier.GetName())

	return watcherapplier.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type WatcherApplier.
func (v *WatcherApplierCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	watcherapplier, ok := obj.(*watcherv1beta1.WatcherApplier)
	if !ok {
		return nil, fmt.Errorf("%w: expected WatcherApplier but got %T", ErrInvalidWatcherApplierType, obj)
	}
	watcherapplierlog.Info("Validation for WatcherApplier upon deletion", "name", watcherapplier.GetName())

	return watcherapplier.ValidateDelete()
}
