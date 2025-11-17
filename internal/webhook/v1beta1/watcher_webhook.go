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

// Package v1beta1 contains webhook implementations for Watcher API v1beta1 resources.
package v1beta1

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	watcherv1beta1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
)

var (
	// ErrInvalidWatcherType is returned when an object is not of type Watcher
	ErrInvalidWatcherType = errors.New("invalid Watcher object type")
	// ErrInvalidWatcherAPIType is returned when an object is not of type WatcherAPI
	ErrInvalidWatcherAPIType = errors.New("invalid WatcherAPI object type")
	// ErrInvalidWatcherApplierType is returned when an object is not of type WatcherApplier
	ErrInvalidWatcherApplierType = errors.New("invalid WatcherApplier object type")
	// ErrInvalidWatcherDecisionEngineType is returned when an object is not of type WatcherDecisionEngine
	ErrInvalidWatcherDecisionEngineType = errors.New("invalid WatcherDecisionEngine object type")
)

// nolint:unused
// log is for logging in this package.
var watcherlog = logf.Log.WithName("watcher-resource")

// SetupWatcherWebhookWithManager registers the webhook for Watcher in the manager.
func SetupWatcherWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&watcherv1beta1.Watcher{}).
		WithValidator(&WatcherCustomValidator{}).
		WithDefaulter(&WatcherCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-watcher-openstack-org-v1beta1-watcher,mutating=true,failurePolicy=fail,sideEffects=None,groups=watcher.openstack.org,resources=watchers,verbs=create;update,versions=v1beta1,name=mwatcher-v1beta1.kb.io,admissionReviewVersions=v1

// WatcherCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Watcher when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type WatcherCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &WatcherCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Watcher.
func (d *WatcherCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	watcher, ok := obj.(*watcherv1beta1.Watcher)

	if !ok {
		return fmt.Errorf("%w: expected Watcher but got %T", ErrInvalidWatcherType, obj)
	}
	watcherlog.Info("Defaulting for Watcher", "name", watcher.GetName())

	watcher.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-watcher-openstack-org-v1beta1-watcher,mutating=false,failurePolicy=fail,sideEffects=None,groups=watcher.openstack.org,resources=watchers,verbs=create;update,versions=v1beta1,name=vwatcher-v1beta1.kb.io,admissionReviewVersions=v1

// WatcherCustomValidator struct is responsible for validating the Watcher resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WatcherCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &WatcherCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Watcher.
func (v *WatcherCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	watcher, ok := obj.(*watcherv1beta1.Watcher)
	if !ok {
		return nil, fmt.Errorf("%w: expected Watcher but got %T", ErrInvalidWatcherType, obj)
	}
	watcherlog.Info("Validation for Watcher upon creation", "name", watcher.GetName())

	return watcher.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Watcher.
func (v *WatcherCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	watcher, ok := newObj.(*watcherv1beta1.Watcher)
	if !ok {
		return nil, fmt.Errorf("%w: expected Watcher for newObj but got %T", ErrInvalidWatcherType, newObj)
	}
	watcherlog.Info("Validation for Watcher upon update", "name", watcher.GetName())

	return watcher.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Watcher.
func (v *WatcherCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	watcher, ok := obj.(*watcherv1beta1.Watcher)
	if !ok {
		return nil, fmt.Errorf("%w: expected Watcher but got %T", ErrInvalidWatcherType, obj)
	}
	watcherlog.Info("Validation for Watcher upon deletion", "name", watcher.GetName())

	return watcher.ValidateDelete()
}
