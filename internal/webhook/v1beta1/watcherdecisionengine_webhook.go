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
var watcherdecisionenginelog = logf.Log.WithName("watcherdecisionengine-resource")

// SetupWatcherDecisionEngineWebhookWithManager registers the webhook for WatcherDecisionEngine in the manager.
func SetupWatcherDecisionEngineWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&watcherv1beta1.WatcherDecisionEngine{}).
		WithValidator(&WatcherDecisionEngineCustomValidator{}).
		WithDefaulter(&WatcherDecisionEngineCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-watcher-openstack-org-v1beta1-watcherdecisionengine,mutating=true,failurePolicy=fail,sideEffects=None,groups=watcher.openstack.org,resources=watcherdecisionengines,verbs=create;update,versions=v1beta1,name=mwatcherdecisionengine-v1beta1.kb.io,admissionReviewVersions=v1

// WatcherDecisionEngineCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind WatcherDecisionEngine when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type WatcherDecisionEngineCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &WatcherDecisionEngineCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind WatcherDecisionEngine.
func (d *WatcherDecisionEngineCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	watcherdecisionengine, ok := obj.(*watcherv1beta1.WatcherDecisionEngine)

	if !ok {
		return fmt.Errorf("%w: expected WatcherDecisionEngine but got %T", ErrInvalidWatcherDecisionEngineType, obj)
	}
	watcherdecisionenginelog.Info("Defaulting for WatcherDecisionEngine", "name", watcherdecisionengine.GetName())

	watcherdecisionengine.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-watcher-openstack-org-v1beta1-watcherdecisionengine,mutating=false,failurePolicy=fail,sideEffects=None,groups=watcher.openstack.org,resources=watcherdecisionengines,verbs=create;update,versions=v1beta1,name=vwatcherdecisionengine-v1beta1.kb.io,admissionReviewVersions=v1

// WatcherDecisionEngineCustomValidator struct is responsible for validating the WatcherDecisionEngine resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WatcherDecisionEngineCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &WatcherDecisionEngineCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type WatcherDecisionEngine.
func (v *WatcherDecisionEngineCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	watcherdecisionengine, ok := obj.(*watcherv1beta1.WatcherDecisionEngine)
	if !ok {
		return nil, fmt.Errorf("%w: expected WatcherDecisionEngine but got %T", ErrInvalidWatcherDecisionEngineType, obj)
	}
	watcherdecisionenginelog.Info("Validation for WatcherDecisionEngine upon creation", "name", watcherdecisionengine.GetName())

	return watcherdecisionengine.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type WatcherDecisionEngine.
func (v *WatcherDecisionEngineCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	watcherdecisionengine, ok := newObj.(*watcherv1beta1.WatcherDecisionEngine)
	if !ok {
		return nil, fmt.Errorf("%w: expected WatcherDecisionEngine for newObj but got %T", ErrInvalidWatcherDecisionEngineType, newObj)
	}
	watcherdecisionenginelog.Info("Validation for WatcherDecisionEngine upon update", "name", watcherdecisionengine.GetName())

	return watcherdecisionengine.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type WatcherDecisionEngine.
func (v *WatcherDecisionEngineCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	watcherdecisionengine, ok := obj.(*watcherv1beta1.WatcherDecisionEngine)
	if !ok {
		return nil, fmt.Errorf("%w: expected WatcherDecisionEngine but got %T", ErrInvalidWatcherDecisionEngineType, obj)
	}
	watcherdecisionenginelog.Info("Validation for WatcherDecisionEngine upon deletion", "name", watcherdecisionengine.GetName())

	return watcherdecisionengine.ValidateDelete()
}
